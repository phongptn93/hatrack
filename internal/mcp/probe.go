package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// protocolVersion is the MCP revision hatrack asks for; servers answer with one they speak.
const protocolVersion = "2025-06-18"

// Info is what a server reports about itself when it starts.
type Info struct {
	Server   string
	Version  string
	Protocol string
	Tools    []Tool
}

type Tool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// ErrAuth means an HTTP server wants a sign-in hatrack cannot do. Claude signs in through /mcp.
var ErrAuth = errors.New("the server asks to sign in; sign in from Claude with /mcp, or check the token in the headers")

// Probe starts the server the way Claude would, runs the MCP handshake and lists its tools.
// stdio servers run as a child process; http servers are contacted at their URL.
func Probe(ctx context.Context, sp Spec) (*Info, error) {
	sp = sp.Expand(nil)
	if left := sp.Placeholders(); len(left) > 0 {
		return nil, fmt.Errorf("{{%s}} was never filled in", left[0])
	}
	switch sp.Kind() {
	case "stdio":
		return probeStdio(ctx, sp)
	case "http":
		return handshake(ctx, &httpConn{url: sp.URL, headers: sp.Headers})
	case "sse":
		return nil, errors.New("testing the older sse transport is not supported; Claude's /mcp shows its status")
	}
	return nil, fmt.Errorf("unknown type %q", sp.Type)
}

type conn interface {
	call(ctx context.Context, method string, params any) (json.RawMessage, error)
	notify(ctx context.Context, method string) error
}

func handshake(ctx context.Context, c conn) (*Info, error) {
	res, err := c.call(ctx, "initialize", map[string]any{
		"protocolVersion": protocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]string{"name": "hatrack", "version": "1"},
	})
	if err != nil {
		return nil, fmt.Errorf("initialize: %w", err)
	}
	var init struct {
		ProtocolVersion string `json:"protocolVersion"`
		ServerInfo      struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"serverInfo"`
		Capabilities map[string]json.RawMessage `json:"capabilities"`
	}
	if err := json.Unmarshal(res, &init); err != nil {
		return nil, fmt.Errorf("initialize: %w", err)
	}
	info := &Info{Server: init.ServerInfo.Name, Version: init.ServerInfo.Version, Protocol: init.ProtocolVersion}
	if hc, ok := c.(*httpConn); ok {
		hc.protocol = init.ProtocolVersion
	}
	if err := c.notify(ctx, "notifications/initialized"); err != nil {
		return nil, err
	}
	if _, ok := init.Capabilities["tools"]; !ok {
		return info, nil
	}
	cursor := ""
	for range 50 { // pages; a server that never stops paging is cut off
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		res, err := c.call(ctx, "tools/list", params)
		if err != nil {
			return nil, fmt.Errorf("tools/list: %w", err)
		}
		var page struct {
			Tools      []Tool `json:"tools"`
			NextCursor string `json:"nextCursor"`
		}
		if err := json.Unmarshal(res, &page); err != nil {
			return nil, fmt.Errorf("tools/list: %w", err)
		}
		info.Tools = append(info.Tools, page.Tools...)
		if cursor = page.NextCursor; cursor == "" {
			break
		}
	}
	return info, nil
}

type message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  any             `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (m *message) outcome() (json.RawMessage, error) {
	if m.Error != nil {
		return nil, fmt.Errorf("server error %d: %s", m.Error.Code, m.Error.Message)
	}
	return m.Result, nil
}

// stdio

func probeStdio(ctx context.Context, sp Spec) (*Info, error) {
	cmd := exec.CommandContext(ctx, sp.Command, sp.Args...)
	cmd.Env = os.Environ()
	for k, v := range sp.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	cmd.WaitDelay = 2 * time.Second
	stderr := &tail{max: 4 << 10}
	cmd.Stderr = stderr
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	lines := make(chan []byte)
	go func() { // a separate reader, so a stuck server cannot outlast ctx
		defer close(lines)
		r := bufio.NewReaderSize(out, 64<<10)
		for {
			b, err := r.ReadBytes('\n')
			if len(bytes.TrimSpace(b)) > 0 {
				select {
				case lines <- b:
				case <-ctx.Done():
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()
	c := &stdioConn{in: in, lines: lines}
	info, err := handshake(ctx, c)
	in.Close() // servers exit when their input closes
	done := make(chan struct{})
	go func() { cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		cmd.Process.Kill()
		<-done
	}
	if err != nil {
		if ctx.Err() != nil {
			err = fmt.Errorf("no answer in time (%w)", ctx.Err())
		}
		if t := strings.TrimSpace(stderr.String()); t != "" {
			err = fmt.Errorf("%w\nserver output:\n%s", err, t)
		}
		return nil, err
	}
	return info, nil
}

type stdioConn struct {
	in    io.Writer
	lines <-chan []byte
	next  int
}

func (c *stdioConn) send(m message) error {
	m.JSONRPC = "2.0"
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	_, err = c.in.Write(append(b, '\n'))
	return err
}

func (c *stdioConn) notify(_ context.Context, method string) error {
	return c.send(message{Method: method})
}

func (c *stdioConn) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	c.next++
	id := json.RawMessage(fmt.Sprint(c.next))
	if err := c.send(message{ID: id, Method: method, Params: params}); err != nil {
		return nil, err
	}
	for {
		var b []byte
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case l, ok := <-c.lines:
			if !ok {
				return nil, errors.New("the server exited")
			}
			b = l
		}
		var m message
		if err := json.Unmarshal(b, &m); err != nil {
			continue // a log line on stdout; Claude would choke on it, but the answer may follow
		}
		if m.Method != "" {
			if len(m.ID) > 0 { // a request to the client (roots, sampling): decline it
				c.send(message{ID: m.ID, Result: json.RawMessage(`{}`)})
			}
			continue
		}
		if string(m.ID) == string(id) {
			return m.outcome()
		}
	}
}

// http (streamable HTTP transport)

type httpConn struct {
	url      string
	headers  map[string]string
	session  string
	protocol string
	next     int
}

func (c *httpConn) post(ctx context.Context, m message) (*http.Response, error) {
	m.JSONRPC = "2.0"
	b, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if c.session != "" {
		req.Header.Set("Mcp-Session-Id", c.session)
	}
	if c.protocol != "" {
		req.Header.Set("MCP-Protocol-Version", c.protocol)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	if sid := resp.Header.Get("Mcp-Session-Id"); sid != "" {
		c.session = sid
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		resp.Body.Close()
		return nil, fmt.Errorf("HTTP %d: %w", resp.StatusCode, ErrAuth)
	}
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		resp.Body.Close()
		return nil, fmt.Errorf("HTTP %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return resp, nil
}

func (c *httpConn) notify(ctx context.Context, method string) error {
	resp, err := c.post(ctx, message{Method: method})
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

func (c *httpConn) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	c.next++
	id := json.RawMessage(fmt.Sprint(c.next))
	resp, err := c.post(ctx, message{ID: id, Method: method, Params: params})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		var m message
		if err := json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(&m); err != nil {
			return nil, fmt.Errorf("bad answer: %w", err)
		}
		return m.outcome()
	}
	// an event stream: the answer is the data of the event carrying our id
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 64<<10), 16<<20)
	var data strings.Builder
	for sc.Scan() {
		line := sc.Text()
		if d, ok := strings.CutPrefix(line, "data:"); ok {
			data.WriteString(strings.TrimPrefix(d, " "))
			continue
		}
		if line != "" || data.Len() == 0 {
			continue
		}
		var m message
		err := json.Unmarshal([]byte(data.String()), &m)
		data.Reset()
		if err == nil && m.Method == "" && string(m.ID) == string(id) {
			return m.outcome()
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return nil, errors.New("the server closed the stream without answering")
}

// tail keeps the last max bytes written to it.
type tail struct {
	mu  sync.Mutex
	max int
	buf []byte
}

func (t *tail) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if len(t.buf) > t.max {
		t.buf = t.buf[len(t.buf)-t.max:]
	}
	return len(p), nil
}

func (t *tail) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.buf)
}
