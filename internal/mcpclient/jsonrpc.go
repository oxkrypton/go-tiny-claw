package mcpclient

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// rpcRequest 是 JSON-RPC 2.0 请求。
type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// rpcResponse 是 JSON-RPC 2.0 响应。
type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// rpcClient 是一个最小 MCP stdio JSON-RPC 客户端。
// 它负责启动子进程、写入请求、读取响应。
type rpcClient struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser

	nextID  atomic.Int64
	writeMu sync.Mutex

	pendingMu sync.Mutex
	pending   map[int64]chan rpcResponse

	closeOnce sync.Once
}

// newRPCClient 启动一个 stdio MCP server。
func newRPCClient(ctx context.Context, command string, args []string, env map[string]string) (*rpcClient, error) {
	cmd := exec.CommandContext(ctx, command, args...)

	if len(env) > 0 {
		cmd.Env = append(cmd.Environ(), flattenEnv(env)...)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("创建 MCP stdin 失败: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("创建 MCP stdout 失败: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("创建 MCP stderr 失败: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("启动 MCP server 失败: %w", err)
	}

	c := &rpcClient{
		cmd:     cmd,
		stdin:   stdin,
		stdout:  stdout,
		stderr:  stderr,
		pending: make(map[int64]chan rpcResponse),
	}

	// 后台读循环
	go c.readLoop()
	// 丢弃 stderr
	go io.Copy(io.Discard, stderr)

	return c, nil
}

func flattenEnv(env map[string]string) []string {
	out := make([]string, 0, len(env))
	for k, v := range env {
		out = append(out, k+"="+v)
	}
	return out
}

// call 发送一个 JSON-RPC request，并等待同 ID 的 response。
func (c *rpcClient) call(ctx context.Context, method string, params any, out any) error {
	// 原子自增请求 ID
	id := c.nextID.Add(1)

	// 创建响应 channel
	respCh := make(chan rpcResponse, 1)

	c.pendingMu.Lock()
	// 注册到 pending map
	c.pending[id] = respCh
	c.pendingMu.Unlock()

	req := rpcRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	data, err := json.Marshal(req)
	if err != nil {
		c.deletePending(id)
		return fmt.Errorf("编码 MCP 请求失败: %w", err)
	}

	// 写入子进程 stdin
	writeErr := c.writeMessage(data)
	if writeErr != nil {
		c.deletePending(id)
		return fmt.Errorf("发送 MCP 请求失败: %w", writeErr)
	}

	// 等待响应或超时
	select {
	// 超时处理 
	case <-ctx.Done():
		c.deletePending(id)
		return fmt.Errorf("MCP 请求超时或被取消: %w", ctx.Err())

	// 收到响应 
	case resp := <-respCh:
		if resp.Error != nil {
			return fmt.Errorf("MCP 返回错误: %s", resp.Error.Message)
		}
		if out == nil {
			return nil
		}
		// 反序列化到 out
		if err := json.Unmarshal(resp.Result, out); err != nil {
			return fmt.Errorf("解析 MCP 响应失败: %w", err)
		}
		return nil
	}
}

// notify 发送 JSON-RPC notification，不等待响应。
func (c *rpcClient) notify(method string, params any) error {
	req := rpcRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("编码 MCP 通知失败: %w", err)
	}

	writeErr := c.writeMessage(data)
	if writeErr != nil {
		return fmt.Errorf("发送 MCP 通知失败: %w", writeErr)
	}

	return nil
}

func (c *rpcClient) writeMessage(data []byte) error {
	// MCP 分帧头
	frame := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))

	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	// 写 头
	if _, err := c.stdin.Write([]byte(frame)); err != nil {
		return err
	}
	// 写 JSON body
	_, err := c.stdin.Write(data)
	return err
}

func (c *rpcClient) readLoop() {
	reader := bufio.NewReader(c.stdout)

	for {
		contentLength := -1

		for {
			// 逐行读 header
			line, err := reader.ReadString('\n')
			if err != nil {
				c.failAllPending()
				return
			}

			line = strings.TrimRight(line, "\r\n")
			if line == "" {
				break
			}

			parts := strings.SplitN(line, ":", 2)
			if len(parts) != 2 {
				continue
			}

			if strings.EqualFold(strings.TrimSpace(parts[0]), "Content-Length") {
				// 解析 body 长度
				n, err := strconv.Atoi(strings.TrimSpace(parts[1]))
				if err != nil {
					c.failAllPending()
					return
				}
				contentLength = n
			}
		}

		if contentLength < 0 {
			c.failAllPending()
			return
		}

		body := make([]byte, contentLength)
		// 读指定字节数
		if _, err := io.ReadFull(reader, body); err != nil {
			c.failAllPending()
			return
		}

		var resp rpcResponse
		// 反序列化响应
		if err := json.Unmarshal(body, &resp); err != nil {
			continue
		}

		if resp.ID == 0 {
			continue
		}

		c.pendingMu.Lock()
		// 投递到对应 channel
		ch := c.pending[resp.ID]
		delete(c.pending, resp.ID)
		c.pendingMu.Unlock()

		if ch != nil {
			ch <- resp
		}
	}
}

func (c *rpcClient) deletePending(id int64) {
	c.pendingMu.Lock()
	delete(c.pending, id)
	c.pendingMu.Unlock()
}

func (c *rpcClient) failAllPending() {
	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()

	for id, ch := range c.pending {
		delete(c.pending, id)
		close(ch)
	}
}

func (c *rpcClient) close() error {
	var err error

	c.closeOnce.Do(func() {
		_ = c.stdin.Close()
		_ = c.stdout.Close()
		_ = c.stderr.Close()

		if c.cmd.Process != nil {
			_ = c.cmd.Process.Kill()
		}

		err = c.cmd.Wait()
	})

	return err
}
