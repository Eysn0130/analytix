package stdio

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	childenv "analytix.local/runtime-go/internal/adapters/outbound/childenv"
	mcpidentity "analytix.local/runtime-go/internal/adapters/outbound/mcp/identity"
	mcpprotocol "analytix.local/runtime-go/internal/adapters/outbound/mcp/protocol"
	mcpredaction "analytix.local/runtime-go/internal/adapters/outbound/mcp/redaction"
	processsandbox "analytix.local/runtime-go/internal/adapters/outbound/processsandbox"
	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
	proc "analytix.local/runtime-go/internal/proc"
)

type CommandSpec struct {
	Command               string
	Args                  []string
	CWD                   string
	Env                   map[string]string
	ProtectedReadDirs     []string
	AllowReadExecuteRoots []string
	AllowLoopbackTCPPort  uint16
}

type StartedProcess struct {
	Handle *ProcessHandle
	Stdin  io.WriteCloser
	Stdout io.Reader
	Stderr io.Reader
}

type ProcessHandle struct {
	cmd      *exec.Cmd
	job      uintptr
	waitOnce sync.Once
}

type TransportClient struct {
	spec           domainmcp.ServerSpec
	capabilities   mcpprotocol.Capabilities
	identity       domainmcp.ServerIdentity
	cmd            *ProcessHandle
	stdin          io.WriteCloser
	stdout         *bufio.Reader
	mu             sync.Mutex
	nextID         int
	stderrMu       sync.Mutex
	stderrTail     []string
	requestTimeout time.Duration
}

const maxMCPStdioFrameBytes = 4 * 1024 * 1024

type TransportError struct {
	Cause               error
	Retryable           bool
	InvalidatesIdentity bool
}

func (err *TransportError) Error() string {
	return "mcp stdio transport failure"
}

func (err *TransportError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Cause
}

func (err *TransportError) MCPTransportRetryable() bool {
	return err != nil && err.Retryable
}

func (err *TransportError) MCPTransportInvalidatesIdentity() bool {
	return err != nil && err.InvalidatesIdentity
}

func (err *TransportError) MCPTransportUnavailable() bool {
	return err != nil && err.Retryable
}

func NewTransportClient(spec domainmcp.ServerSpec) (*TransportClient, error) {
	return NewTransportClientWithProtectedReadDirs(spec, nil)
}

func NewTransportClientWithProtectedReadDirs(
	spec domainmcp.ServerSpec,
	protectedReadDirs []string,
) (*TransportClient, error) {
	return NewTransportClientWithFilesystemPolicy(
		spec,
		ProcessFilesystemPolicy{ProtectedReadDirs: protectedReadDirs},
	)
}

// ProcessFilesystemPolicy is supplied by host composition, never inferred
// from provider- or user-configurable MCP metadata. Read/execute and loopback
// exceptions are valid only for a separately verified host binding.
type ProcessFilesystemPolicy struct {
	ProtectedReadDirs     []string
	AllowReadExecuteRoots []string
	AllowLoopbackTCPPort  uint16
}

func NewTransportClientWithFilesystemPolicy(
	spec domainmcp.ServerSpec,
	policy ProcessFilesystemPolicy,
) (*TransportClient, error) {
	timeout, err := domainmcp.ResolveServerTimeoutV1(spec.TimeoutMS)
	if err != nil {
		return nil, err
	}
	started, err := Start(CommandSpec{
		Command:               spec.Command,
		Args:                  spec.Args,
		CWD:                   spec.CWD,
		Env:                   spec.Env,
		ProtectedReadDirs:     append([]string(nil), policy.ProtectedReadDirs...),
		AllowReadExecuteRoots: append([]string(nil), policy.AllowReadExecuteRoots...),
		AllowLoopbackTCPPort:  policy.AllowLoopbackTCPPort,
	})
	if err != nil {
		return nil, &TransportError{Cause: err, Retryable: true}
	}
	client := &TransportClient{
		spec:           spec,
		cmd:            started.Handle,
		stdin:          started.Stdin,
		stdout:         bufio.NewReaderSize(started.Stdout, 64*1024),
		requestTimeout: timeout,
	}
	go client.collectStderr(started.Stderr)
	if err := client.initialize(); err != nil {
		client.Close()
		return nil, err
	}
	return client, nil
}

func Start(spec CommandSpec) (*StartedProcess, error) {
	invocation, err := processsandbox.Prepare(
		spec.Command,
		spec.Args,
		processsandbox.FilesystemPolicy{
			DenyRoots: append([]string(nil), spec.ProtectedReadDirs...),
			AllowReadExecuteRoots: append(
				[]string(nil),
				spec.AllowReadExecuteRoots...,
			),
			AllowLoopbackTCPPort: spec.AllowLoopbackTCPPort,
		},
	)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(invocation.Executable, invocation.Args...)
	if strings.TrimSpace(spec.CWD) != "" {
		cmd.Dir = spec.CWD
	}
	environment, err := CommandEnvironment(os.Environ(), spec.Env)
	if err != nil {
		return nil, err
	}
	cmd.Env = environment
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, err
	}
	job, err := proc.StartTracked(cmd)
	if err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		_ = stderr.Close()
		return nil, err
	}
	return &StartedProcess{
		Handle: &ProcessHandle{cmd: cmd, job: job},
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
	}, nil
}

func (h *ProcessHandle) KillAndWait() {
	if h == nil || h.cmd == nil || h.cmd.Process == nil {
		return
	}
	h.waitOnce.Do(func() {
		proc.KillTracked(h.cmd, h.job)
		_ = h.cmd.Wait()
		proc.ReapTracked(h.cmd, h.job)
	})
}

func CommandEnvironment(base []string, env map[string]string) ([]string, error) {
	return childenv.Merge(base, env)
}

func (c *TransportClient) initialize() error {
	result, err := c.request("initialize", map[string]any{
		"protocolVersion": mcpprotocol.ProtocolVersion,
		"clientInfo": map[string]any{
			"name":    "analytix",
			"version": "0.1.0",
		},
		"capabilities": map[string]any{},
	})
	if err != nil {
		return err
	}
	identity, capabilities, err := mcpprotocol.ParseInitializeResult(result, mcpprotocol.ProtocolVersion)
	if err != nil {
		return err
	}
	if err := mcpidentity.VerifyObserved(c.spec, identity); err != nil {
		return err
	}
	c.identity = identity
	c.capabilities = capabilities
	return c.notify("notifications/initialized", map[string]any{})
}

func (c *TransportClient) ListTools() ([]domainmcp.ToolSpec, error) {
	return c.ListToolsContext(context.Background())
}

func (c *TransportClient) ListToolsContext(ctx context.Context) ([]domainmcp.ToolSpec, error) {
	catalog, err := c.ListToolCatalogContext(ctx)
	if err != nil {
		return nil, err
	}
	return catalog.Tools, nil
}

func (c *TransportClient) ListToolCatalogContext(ctx context.Context) (mcpprotocol.ToolCatalog, error) {
	if !c.capabilities.Tools {
		return mcpprotocol.ToolCatalog{}, errors.New("MCP server did not advertise tools capability")
	}
	return mcpprotocol.CollectToolCatalogPages(ctx, func(callCtx context.Context, params map[string]any) (json.RawMessage, error) {
		return c.requestContext(callCtx, "tools/list", params)
	})
}

func (c *TransportClient) ListPrompts() ([]domainmcp.PromptSpec, error) {
	if !c.capabilities.Prompts {
		return nil, nil
	}
	result, err := c.request("prompts/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	return mcpprotocol.ParsePrompts(result), nil
}

func (c *TransportClient) ListResources() ([]domainmcp.ResourceSpec, error) {
	if !c.capabilities.Resources {
		return nil, nil
	}
	result, err := c.request("resources/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	return mcpprotocol.ParseResources(result), nil
}

func (c *TransportClient) CallTool(name string, arguments map[string]any) (any, error) {
	return c.CallToolContext(context.Background(), name, arguments)
}

func (c *TransportClient) CallToolContext(ctx context.Context, name string, arguments map[string]any) (any, error) {
	result, err := c.requestContext(ctx, "tools/call", mcpprotocol.ToolCallParams(name, arguments))
	if err != nil {
		return nil, err
	}
	return mcpprotocol.ExtractLosslessToolResult(result), nil
}

func (c *TransportClient) CallToolWithHostContext(ctx context.Context, name string, arguments map[string]any, envelope domainmcp.HostContextEnvelope) (any, error) {
	_, grant, authorityErr := envelope.Authority()
	if authorityErr != nil || grant.ToolName != "mcp__"+strings.TrimSpace(c.spec.ID)+"__"+strings.TrimSpace(name) {
		return nil, errors.New("MCP host context tool authority is invalid")
	}
	params, err := mcpprotocol.ToolCallParamsWithHostContext(name, arguments, envelope)
	if err != nil {
		return nil, err
	}
	result, err := c.requestContext(ctx, "tools/call", params)
	if err != nil {
		return nil, err
	}
	return mcpprotocol.ExtractLosslessToolResult(result), nil
}

func (c *TransportClient) CallNativeContext(ctx context.Context, method string, params map[string]any) (any, error) {
	result, err := c.CallNativeLosslessContext(ctx, method, params)
	return result.Value, err
}

func (c *TransportClient) CallNativeLosslessContext(ctx context.Context, method string, params map[string]any) (domainmcp.LosslessToolResult, error) {
	result, err := c.requestContext(ctx, strings.TrimSpace(method), params)
	if err != nil {
		return domainmcp.LosslessToolResult{}, err
	}
	return mcpprotocol.DecodeLosslessJSONResult(result), nil
}

func (c *TransportClient) Close() {
	if c.stdin != nil {
		_ = c.stdin.Close()
	}
	if c.cmd != nil {
		c.cmd.KillAndWait()
	}
}

func (c *TransportClient) ObservedServerIdentity() domainmcp.ServerIdentity {
	return c.identity
}

func (c *TransportClient) notify(method string, params map[string]any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	payload := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	}
	data, err := mcpprotocol.MarshalBoundedJSONRPC(payload)
	if err != nil {
		return err
	}
	if len(data)+1 > maxMCPStdioFrameBytes {
		return errors.New("MCP stdio JSON-RPC payload exceeds frame limit")
	}
	if _, err := c.stdin.Write(append(data, '\n')); err != nil {
		c.Close()
		return c.withStdioStderrTail(method, &TransportError{Cause: err, Retryable: true, InvalidatesIdentity: true})
	}
	return nil
}

func (c *TransportClient) request(method string, params map[string]any) (json.RawMessage, error) {
	return c.requestContext(context.Background(), method, params)
}

func (c *TransportClient) requestContext(ctx context.Context, method string, params map[string]any) (json.RawMessage, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c.nextID++
	payload := map[string]any{
		"jsonrpc": "2.0",
		"id":      c.nextID,
		"method":  method,
		"params":  params,
	}
	data, err := mcpprotocol.MarshalBoundedJSONRPC(payload)
	if err != nil {
		return nil, err
	}
	if len(data)+1 > maxMCPStdioFrameBytes {
		return nil, errors.New("MCP stdio JSON-RPC payload exceeds frame limit")
	}
	if _, err := c.stdin.Write(append(data, '\n')); err != nil {
		c.Close()
		return nil, c.withStdioStderrTail(method, &TransportError{Cause: err, Retryable: true, InvalidatesIdentity: true})
	}
	requestTimeout := c.requestTimeout
	if requestTimeout <= 0 {
		requestTimeout, _ = domainmcp.ResolveServerTimeoutV1(0)
	}
	deadline := time.NewTimer(requestTimeout)
	defer deadline.Stop()
	type response struct {
		result json.RawMessage
		err    error
		fatal  bool
	}
	ch := make(chan response, 1)
	go func(expectedID int) {
		for {
			line, readErr := readBoundedFrameLine(c.stdout, maxMCPStdioFrameBytes)
			if readErr != nil {
				if errors.Is(readErr, io.EOF) {
					readErr = &TransportError{Cause: readErr, Retryable: true}
				}
				ch <- response{err: readErr, fatal: true}
				return
			}
			if len(line) == 0 {
				continue
			}
			frame, frameErr := mcpprotocol.ClassifyJSONRPCStreamFrame(line, expectedID)
			if frameErr != nil {
				ch <- response{err: frameErr, fatal: true}
				return
			}
			switch frame.Kind {
			case mcpprotocol.JSONRPCStreamIgnorableNotification:
				continue
			case mcpprotocol.JSONRPCStreamMatchingError:
				ch <- response{err: frame.Err, fatal: frame.InvalidatesIdentity}
				return
			case mcpprotocol.JSONRPCStreamMatchingResult:
				ch <- response{result: frame.Result}
				return
			default:
				ch <- response{err: errors.New("MCP stdio stream frame classification is invalid"), fatal: true}
				return
			}
		}
	}(c.nextID)
	select {
	case item := <-ch:
		if item.err != nil {
			if item.fatal {
				c.Close()
				var transportError *TransportError
				if errors.As(item.err, &transportError) {
					item.err = &TransportError{Cause: item.err, Retryable: transportError.Retryable, InvalidatesIdentity: true}
				} else {
					item.err = &TransportError{Cause: item.err, Retryable: false, InvalidatesIdentity: true}
				}
				return nil, c.withStdioStderrTail(method, item.err)
			}
			return nil, item.err
		}
		return item.result, nil
	case <-ctx.Done():
		c.Close()
		return nil, &TransportError{Cause: ctx.Err(), Retryable: false, InvalidatesIdentity: true}
	case <-deadline.C:
		c.Close()
		return nil, c.withStdioStderrTail(method, &TransportError{Cause: os.ErrDeadlineExceeded, Retryable: true, InvalidatesIdentity: true})
	}
}

func readBoundedFrameLine(reader *bufio.Reader, maxBytes int) ([]byte, error) {
	if reader == nil || maxBytes <= 0 {
		return nil, errors.New("mcp stdio frame limit is invalid")
	}
	line := make([]byte, 0, min(maxBytes, 64*1024))
	for {
		fragment, err := reader.ReadSlice('\n')
		if len(line)+len(fragment) > maxBytes+2 {
			return nil, errors.New("mcp stdio frame exceeds limit")
		}
		line = append(line, fragment...)
		switch {
		case err == nil:
			if len(line) == 0 || line[len(line)-1] != '\n' {
				return nil, errors.New("mcp stdio frame is not newline terminated")
			}
			line = line[:len(line)-1]
			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			if len(line) > maxBytes {
				return nil, errors.New("mcp stdio frame exceeds limit")
			}
			return line, nil
		case errors.Is(err, bufio.ErrBufferFull):
			if len(line) > maxBytes {
				return nil, errors.New("mcp stdio frame exceeds limit")
			}
			continue
		case errors.Is(err, io.EOF):
			if len(line) == 0 {
				return nil, io.EOF
			}
			return nil, errors.New("mcp stdio frame is not newline terminated")
		default:
			return nil, errors.New("mcp stdio frame read failed")
		}
	}
}

func (c *TransportClient) collectStderr(reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 16*1024), 256*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		c.stderrMu.Lock()
		c.stderrTail = append(c.stderrTail, mcpredaction.DiagnosticText(line, c.spec.Headers, c.spec.Env, c.spec.URL))
		if len(c.stderrTail) > 20 {
			c.stderrTail = append([]string(nil), c.stderrTail[len(c.stderrTail)-20:]...)
		}
		c.stderrMu.Unlock()
	}
}

func (c *TransportClient) withStdioStderrTail(method string, err error) error {
	if err == nil {
		return nil
	}
	tail := c.stderrTailText()
	if tail == "" {
		time.Sleep(10 * time.Millisecond)
		tail = c.stderrTailText()
	}
	if tail == "" {
		return err
	}
	return fmt.Errorf("mcp stdio %s failed: %w; stderr tail: %s", method, err, tail)
}

func (c *TransportClient) stderrTailText() string {
	c.stderrMu.Lock()
	defer c.stderrMu.Unlock()
	return strings.Join(append([]string(nil), c.stderrTail...), "\n")
}
