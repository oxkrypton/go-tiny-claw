// internal/tools/registry.go
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"sync"

	"github.com/oxkrypton/go-tiny-claw/internal/schema"
)

// BaseTool 是所有具体工具必须实现的通用接口
type BaseTool interface {
	//Name 返回工具的全局唯一名称 (大模型通过这个 name 调用)
	Name() string

	//Definition 返回用于提交给大模型的工具元信息和参数
	Definition() schema.ToolDefinition

	//Execute 接受大模型吐出的 JSON 参数, 执行具体业务逻辑
	// 注意：参数是 json.RawMessage，反序列化由各个具体工具内部自行处理
	Execute(ctx context.Context, args json.RawMessage) (string, error)
}

// LockHinter 是工具的可选接口：实现它来声明本次调用要锁哪些路径。
// 没有实现的工具（典型的就是 bash）会被 Registry 视为"通配写"，进而抢全局写锁。
type LockHinter interface {
	LockHints(args json.RawMessage) ([]LockRequest, error)
}

// Registry 定义了工具的注册和分发执行接口。
// 对外只暴露 ExecuteParallel 一个执行入口，所有工具调用都必须经过路径锁调度，
// 避免外部调用方绕过锁直接 Execute 导致并发安全问题。
type Registry interface {
	//Register 挂载一个新的工具到系统中
	Register(tool BaseTool)

	//GetAvailavleTools 返回当前系统挂载的所有可用工具的 Schema
	GetAvailableTools() []schema.ToolDefinition

	// ExecuteParallel 并行执行同一轮内的所有工具调用，按路径锁调度。
	// 返回切片的下标与入参 calls 一一对应，保证模型看到的 Observation 顺序与原 ToolCalls 一致。
	ExecuteParallel(ctx context.Context, calls []schema.ToolCall) []schema.ToolResult
}

// registryImpl 是 Registry 接口的默认实现
type registryImpl struct {
	//使用 map 以工具的 name 作为 key 进行快速 O(1) 路由查找
	tools   map[string]BaseTool
	lockMgr *PathLockManager
}

func NewRegistry() Registry {
	return &registryImpl{
		tools:   make(map[string]BaseTool),
		lockMgr: NewPathLockManager(),
	}
}

func (r *registryImpl) Register(tool BaseTool) {
	name := tool.Name()
	if _, exists := r.tools[name]; exists {
		log.Printf("[Warning] 工具 '%s' 已经被注册，将被覆盖。\n", name)
	}
	r.tools[name] = tool
	log.Printf("[Registry] 成功挂载工具: %s\n", name)
}

func (r *registryImpl) GetAvailableTools() []schema.ToolDefinition {
	var defs []schema.ToolDefinition
	for _, tool := range r.tools {
		defs = append(defs, tool.Definition())
	}
	return defs
}

// execute 是内部执行入口：tool 必须由调用方提前查好（runWithLocks 已经查过一次）。
// 这样避免与并行路径上的查表重复。tool == nil 表示工具不存在，统一返回模型可读的错误。
func (r *registryImpl) execute(ctx context.Context, call schema.ToolCall, tool BaseTool) schema.ToolResult {
	if tool == nil {
		return schema.ToolResult{
			ToolCallID: call.ID,
			Output:     fmt.Sprintf("Error: 系统中不存在名为 '%s' 的工具", call.Name),
			IsError:    true,
		}
	}

	output, err := tool.Execute(ctx, call.Arguments)
	if err != nil {
		return schema.ToolResult{
			ToolCallID: call.ID,
			Output:     fmt.Sprintf("Error executing %s: %v", call.Name, err),
			IsError:    true,
		}
	}
	return schema.ToolResult{
		ToolCallID: call.ID,
		Output:     output,
		IsError:    false,
	}
}

// ExecuteParallel 接管原本写在 engine 里的并行调度。
// 锁策略：
//   - 实现了 LockHinter 的工具：global RLock + 按 path 字典序逐个 acquire(path, mode)。
//   - 未实现 LockHinter 的工具（如 bash）：global Lock，期间挡住所有文件类工具。
//
// path 排序保证了多路径锁的获取顺序一致，从根上避免交叉死锁。
func (r *registryImpl) ExecuteParallel(ctx context.Context, calls []schema.ToolCall) []schema.ToolResult {
	results := make([]schema.ToolResult, len(calls))
	sem := make(chan struct{}, 5)
	var wg sync.WaitGroup

	for i, call := range calls {
		wg.Add(1)
		go func(idx int, call schema.ToolCall) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			r.runWithLocks(ctx, call, &results[idx])
		}(i, call)
	}
	wg.Wait()
	return results
}

// runWithLocks 处理单个工具调用的锁获取 → 执行 → 释放。
func (r *registryImpl) runWithLocks(ctx context.Context, call schema.ToolCall, out *schema.ToolResult) {
	tool := r.tools[call.Name] // 可能为 nil，由 execute 统一处理"工具不存在"

	hinter, ok := tool.(LockHinter)
	if !ok {
		// 当其他协程 RLock() 时, 阻塞 bash 操作, 
		// bash 协程 RLock() 时, 同样让其他协程阻塞. 实现了全局锁
		r.lockMgr.global.Lock()
		defer r.lockMgr.global.Unlock()
		*out = r.execute(ctx, call, tool)
		return
	}

	hints, err := hinter.LockHints(call.Arguments)
	if err != nil {
		*out = schema.ToolResult{
			ToolCallID: call.ID,
			Output:     fmt.Sprintf("Error: 解析工具参数失败: %v", err),
			IsError:    true,
		}
		return
	}

	// 路径排序 + 同路径合并（R+W -> W），保证锁顺序一致并消除重入歧义。
	hints = normalizeHints(hints)

	// 每个协程走一遍是为了 bash 全局锁 
	r.lockMgr.global.RLock()
	defer r.lockMgr.global.RUnlock()

	// 一个工具调用内可能会有多条路径操作, 先全部 acquire 再全部 release
	for _, h := range hints {
		r.lockMgr.acquire(h.Path, h.Mode)
	}
	defer func() {
		for i := len(hints) - 1; i >= 0; i-- {
			r.lockMgr.release(hints[i].Path, hints[i].Mode)
		}
	}()

	*out = r.execute(ctx, call, tool)
}

// normalizeHints 把同一调用内的 LockRequest 按 path 升序排序，
// 同 path 出现多次时保留最强的 mode（Write 覆盖 Read）。
func normalizeHints(hints []LockRequest) []LockRequest {
	if len(hints) <= 1 {
		return hints
	}

	//读写锁同路径合并, 避免重入 RWMutex 导致死锁
	merged := make(map[string]LockMode, len(hints))
	for _, h := range hints {
		if cur, ok := merged[h.Path]; !ok || h.Mode > cur {
			merged[h.Path] = h.Mode
		}
	}

	//空切片, 预分配容量, 避免 append 反复扩容
	out := make([]LockRequest, 0, len(merged))
	//把 map 转回切片
	for p, m := range merged {
		out = append(out, LockRequest{Path: p, Mode: m})
	}
	//正序排列 path, 保证相同 path 不会因为顺序不同导致死锁
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}
