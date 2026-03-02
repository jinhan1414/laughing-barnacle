package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
)

type stdioSession struct {
	signature string
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	enc       *json.Encoder
	dec       *json.Decoder
	stderr    *bytes.Buffer
	mu        sync.Mutex
	closeOnce sync.Once
}

func buildStdioSessionSignature(service Service) string {
	envBytes, _ := json.Marshal(service.Env)
	argsBytes, _ := json.Marshal(service.Args)
	return strings.Join([]string{
		strings.TrimSpace(service.Command),
		string(argsBytes),
		string(envBytes),
	}, "\n")
}

func (c *HTTPClient) ensureStdioSession(ctx context.Context, service Service) (*stdioSession, error) {
	signature := buildStdioSessionSignature(service)
	if session := c.getStdioSession(service.ID); session != nil && session.signature == signature {
		return session, nil
	}

	old := c.deleteStdioSession(service.ID)
	if old != nil {
		old.close()
	}

	session, err := c.startStdioSession(ctx, service, signature)
	if err != nil {
		return nil, err
	}

	current := c.getStdioSession(service.ID)
	if current == nil {
		c.setStdioSession(service.ID, session)
		return session, nil
	}
	if current.signature == signature {
		session.close()
		return current, nil
	}

	current.close()
	c.setStdioSession(service.ID, session)
	return session, nil
}

func (c *HTTPClient) startStdioSession(ctx context.Context, service Service, signature string) (*stdioSession, error) {
	command := strings.TrimSpace(service.Command)
	if command == "" {
		return nil, fmt.Errorf("stdio command is required")
	}

	cmd := exec.Command(command, service.Args...)
	cmd.Env = mergeConfiguredEnv(os.Environ(), service.Env)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("open stdio stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("open stdio stdout: %w", err)
	}
	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start stdio command: %w", err)
	}

	session := &stdioSession{
		signature: signature,
		cmd:       cmd,
		stdin:     stdin,
		enc:       json.NewEncoder(stdin),
		dec:       json.NewDecoder(bufio.NewReader(stdout)),
		stderr:    stderr,
	}
	if err := session.initialize(ctx, c.nextReqID(), c.protocolVersion); err != nil {
		session.close()
		return nil, err
	}
	return session, nil
}

func (s *stdioSession) initialize(ctx context.Context, reqID int64, protocolVersion string) error {
	initResp, err := s.roundTrip(ctx, rpcRequest{
		JSONRPC: "2.0",
		ID:      reqID,
		Method:  "initialize",
		Params: map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities": map[string]any{
				"tools": map[string]any{},
			},
			"clientInfo": map[string]any{
				"name":    "laughing-barnacle-agent",
				"version": "1.0.0",
			},
		},
	})
	if err != nil {
		return s.wrapReadError("read initialize response", err)
	}
	if initResp.Error != nil {
		return fmt.Errorf("rpc error %d: %s", initResp.Error.Code, initResp.Error.Message)
	}
	return s.writeOnly(rpcRequest{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
		Params:  map[string]any{},
	}, "write initialized notification")
}

func (s *stdioSession) call(ctx context.Context, reqID int64, method string, params map[string]any) (json.RawMessage, error) {
	resp, err := s.roundTrip(ctx, rpcRequest{
		JSONRPC: "2.0",
		ID:      reqID,
		Method:  method,
		Params:  params,
	})
	if err != nil {
		return nil, s.wrapReadError("read rpc response", err)
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("rpc error %d: %s", resp.Error.Code, resp.Error.Message)
	}
	return resp.Result, nil
}

func (s *stdioSession) roundTrip(ctx context.Context, req rpcRequest) (rpcResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.enc.Encode(req); err != nil {
		return rpcResponse{}, fmt.Errorf("write rpc request: %w", err)
	}
	return s.readResponse(ctx, req.ID)
}

func (s *stdioSession) writeOnly(req rpcRequest, label string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.enc.Encode(req); err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	return nil
}

func (s *stdioSession) readResponse(ctx context.Context, expectID any) (rpcResponse, error) {
	type rpcReadResult struct {
		resp rpcResponse
		err  error
	}

	ch := make(chan rpcReadResult, 1)
	go func() {
		resp, err := waitRPCResponseFromSTDIO(s.dec, expectID)
		ch <- rpcReadResult{resp: resp, err: err}
	}()

	select {
	case res := <-ch:
		return res.resp, res.err
	case <-ctx.Done():
		s.close()
		<-ch
		return rpcResponse{}, ctx.Err()
	}
}

func (s *stdioSession) wrapReadError(label string, err error) error {
	if err == nil {
		return nil
	}
	if tail := s.stderrTail(); tail != "" {
		return fmt.Errorf("%s: %w; stderr: %s", label, err, tail)
	}
	return fmt.Errorf("%s: %w", label, err)
}

func (s *stdioSession) stderrTail() string {
	return strings.TrimSpace(s.stderr.String())
}

func (s *stdioSession) close() {
	s.closeOnce.Do(func() {
		if s.stdin != nil {
			_ = s.stdin.Close()
		}
		if s.cmd != nil && s.cmd.Process != nil {
			_ = s.cmd.Process.Kill()
		}
		if s.cmd != nil {
			_ = s.cmd.Wait()
		}
	})
}
