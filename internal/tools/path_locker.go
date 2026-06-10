package tools

import (
	"sync"
	"sync/atomic"
)

// modeName 仅用于日志可读性
func modeName(m LockMode) string {
	if m == LockWrite {
		return "W"
	}
	return "R"
}

// 单调递增的请求编号，方便在日志里把同一笔 acquire/release 串起来
var lockSeq atomic.Uint64

// LockMode 表示工具对某个路径需要的锁类型
type LockMode int

const (
	LockRead  LockMode = iota // 共享锁，允许多个并发读
	LockWrite                 // 独占锁，写入或修改
)

// LockRequest 是工具向 Registry 声明的"我要锁这条路径"的意图。
// Path 必须是已经归一化的绝对路径（见各工具的 LockHints 实现）。
type LockRequest struct {
	Path string
	Mode LockMode
}

// pathEntry 是 pathLockManager 内部按路径维度持有的锁条目。
// refCount 用于在持锁/等锁结束后把 entry 从 map 里移除，避免长期跑下来 map 无限膨胀。
type pathEntry struct {
	rw       sync.RWMutex
	refCount int
}

// pathLockManager 管理两层锁：
//  1. global：全局 RWMutex，bash 这类无法静态分析路径的工具拿写锁，
//     文件类工具拿读锁，做 bash 与文件工具之间的互斥兜底。
//  2. m：按归一化路径的 *RWMutex 池，read 拿 RLock、write/edit 拿 Lock，
//     做"同路径串行、跨路径并行"。
type pathLockManager struct {
	global sync.RWMutex

	mu sync.Mutex
	m  map[string]*pathEntry
}

func newPathLockManager() *pathLockManager {
	return &pathLockManager{m: make(map[string]*pathEntry)}
}

// acquire 在指定 path 上按 mode 获取锁。
// refCount++ 必须在拿 RW 锁之前完成，否则可能与并发的 release 出现 entry 被提前回收的竞态。
func (p *pathLockManager) acquire(path string, mode LockMode) {
	p.mu.Lock()
	e, ok := p.m[path]
	if !ok {
		e = &pathEntry{}
		p.m[path] = e
	}
	e.refCount++
	p.mu.Unlock()

	if mode == LockWrite {
		e.rw.Lock()
	} else {
		e.rw.RLock()
	}
}

// release 与 acquire 配对。refCount 归零时把 entry 从 map 中删除，回收容量。
func (p *pathLockManager) release(path string, mode LockMode) {
	p.mu.Lock()
	e := p.m[path]
	p.mu.Unlock()

	if mode == LockWrite {
		e.rw.Unlock()
	} else {
		e.rw.RUnlock()
	}

	p.mu.Lock()
	e.refCount--
	if e.refCount == 0 {
		delete(p.m, path)
	}
	p.mu.Unlock()
}
