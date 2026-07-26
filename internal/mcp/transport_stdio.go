package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"
)

// StdioConfig configures a stdio-based MCP transport.
type StdioConfig struct {
	Command string
	Args    []string
	Env     []string // extra KEY=VALUE pairs
	Dir     string
	Logger  *slog.Logger
}

// StdioTransport talks to an MCP server over stdin/stdout JSON-RPC.
type StdioTransport struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser

	nextID  atomic.Int64
	pending map[int64]chan jsonRPCResponse
	mu      sync.Mutex

	closed  atomic.Bool
	done    chan struct{}
	logger  *slog.Logger
	waitErr error
	waitMu  sync.Mutex
}

// NewStdioTransport starts the MCP server process and begins reading responses.
func NewStdioTransport(cfg StdioConfig) (*StdioTransport, error) {
	if cfg.Command == "" {
		return nil, fmt.Errorf("command is required")
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	cmd := exec.Command(cfg.Command, cfg.Args...)
	if cfg.Dir != "" {
		cmd.Dir = cfg.Dir
	}
	cmd.Env = append(os.Environ(), cfg.Env...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		_ = stderr.Close()
		return nil, fmt.Errorf("start %s: %w", cfg.Command, err)
	}

	t := &StdioTransport{
		cmd:     cmd,
		stdin:   stdin,
		stdout:  stdout,
		stderr:  stderr,
		pending: make(map[int64]chan jsonRPCResponse),
		done:    make(chan struct{}),
		logger:  logger,
	}

	go t.readLoop()
	go t.drainStderr()
	go t.waitProcess()

	return t, nil
}

// Send sends a JSON-RPC request and waits for the matching response.
func (t *StdioTransport) Send(ctx context.Context, method string, params any) (json.RawMessage, error) {
	if t.closed.Load() {
		return nil, fmt.Errorf("transport closed")
	}

	id := t.nextID.Add(1)
	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	ch := make(chan jsonRPCResponse, 1)
	t.mu.Lock()
	t.pending[id] = ch
	t.mu.Unlock()

	defer func() {
		t.mu.Lock()
		delete(t.pending, id)
		t.mu.Unlock()
	}()

	line, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	line = append(line, '\n')

	t.mu.Lock()
	_, err = t.stdin.Write(line)
	t.mu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("write request: %w", err)
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-t.done:
		return nil, fmt.Errorf("transport closed while waiting for %s", method)
	case resp := <-ch:
		if resp.Error != nil {
			return nil, resp.Error
		}
		return resp.Result, nil
	}
}

// Notify sends a JSON-RPC notification (no id, no response expected).
func (t *StdioTransport) Notify(ctx context.Context, method string, params any) error {
	if t.closed.Load() {
		return fmt.Errorf("transport closed")
	}
	_ = ctx

	msg := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
	}
	if params != nil {
		msg["params"] = params
	}
	line, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal notification: %w", err)
	}
	line = append(line, '\n')

	t.mu.Lock()
	defer t.mu.Unlock()
	_, err = t.stdin.Write(line)
	if err != nil {
		return fmt.Errorf("write notification: %w", err)
	}
	return nil
}

// Close terminates the MCP server process gracefully.
func (t *StdioTransport) Close() error {
	if !t.closed.CompareAndSwap(false, true) {
		return nil
	}

	_ = t.stdin.Close()

	select {
	case <-t.done:
	case <-time.After(3 * time.Second):
		if t.cmd.Process != nil {
			_ = t.cmd.Process.Kill()
		}
		<-t.done
	}

	t.waitMu.Lock()
	err := t.waitErr
	t.waitMu.Unlock()
	if err != nil && !isExitOK(err) {
		return err
	}
	return nil
}

func (t *StdioTransport) readLoop() {
	defer close(t.done)

	scanner := bufio.NewScanner(t.stdout)
	// MCP payloads can be large (file contents).
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var resp jsonRPCResponse
		if err := json.Unmarshal(line, &resp); err != nil {
			t.logger.Warn("invalid json-rpc line", "error", err, "line", string(line))
			continue
		}

		// Server-initiated notifications / requests — ignore for MVP.
		if resp.ID == nil {
			t.logger.Debug("ignoring server message without id", "method", resp.Method)
			continue
		}

		t.mu.Lock()
		ch, ok := t.pending[*resp.ID]
		t.mu.Unlock()
		if !ok {
			t.logger.Debug("no pending request for response", "id", *resp.ID)
			continue
		}
		select {
		case ch <- resp:
		default:
		}
	}
	if err := scanner.Err(); err != nil && !t.closed.Load() {
		t.logger.Warn("stdout read error", "error", err)
	}
}

func (t *StdioTransport) drainStderr() {
	scanner := bufio.NewScanner(t.stderr)
	for scanner.Scan() {
		t.logger.Debug("mcp stderr", "line", scanner.Text())
	}
}

func (t *StdioTransport) waitProcess() {
	err := t.cmd.Wait()
	t.waitMu.Lock()
	t.waitErr = err
	t.waitMu.Unlock()
	t.closed.Store(true)
}

func isExitOK(err error) bool {
	if err == nil {
		return true
	}
	if _, ok := err.(*exec.ExitError); ok {
		return true
	}
	return false
}
