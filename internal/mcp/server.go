package mcp

import (
	"bufio"
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"io"
	"slices"
	"sync"
)

var supportedVersions = []string{"2025-11-25", "2025-06-18", "2025-03-26", "2024-11-05"}

type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`

	Call func(ctx context.Context, args jsontext.Value) (string, error) `json:"-"`
}

type Server struct {
	name, version string
	tools         []Tool

	writeMu sync.Mutex
	out     io.Writer

	callsMu sync.Mutex
	calls   map[string]context.CancelFunc
}

func NewServer(name, version string, tools ...Tool) *Server {
	return &Server{name: name, version: version, tools: tools, calls: map[string]context.CancelFunc{}}
}

type message struct {
	ID     jsontext.Value `json:"id,omitzero"`
	Method string         `json:"method"`
	Params jsontext.Value `json:"params,omitzero"`
}

type response struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      jsontext.Value `json:"id"`
	Result  any            `json:"result,omitzero"`
	Error   *rpcError      `json:"error,omitzero"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

const (
	codeParseError     = -32700
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
)

func (s *Server) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	s.out = out

	r := bufio.NewReader(in)
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 {
			s.handle(ctx, line)
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func (s *Server) handle(ctx context.Context, line []byte) {
	var msg message
	if err := json.Unmarshal(line, &msg); err != nil {
		s.reply(jsontext.Value("null"), nil, &rpcError{codeParseError, err.Error()})
		return
	}
	if msg.Method == "" {
		return
	}
	if len(msg.ID) == 0 {
		s.notification(msg)
		return
	}

	switch msg.Method {
	case "initialize":
		s.reply(msg.ID, s.initialize(msg.Params), nil)
	case "ping":
		s.reply(msg.ID, map[string]any{}, nil)
	case "tools/list":
		s.reply(msg.ID, map[string]any{"tools": s.tools}, nil)
	case "tools/call":
		s.startCall(ctx, msg)
	default:
		s.reply(msg.ID, nil, &rpcError{codeMethodNotFound, "unknown method " + msg.Method})
	}
}

func (s *Server) notification(msg message) {
	if msg.Method != "notifications/cancelled" {
		return
	}
	var p struct {
		RequestID jsontext.Value `json:"requestId"`
	}
	if json.Unmarshal(msg.Params, &p) != nil {
		return
	}
	s.callsMu.Lock()
	cancel := s.calls[string(p.RequestID)]
	s.callsMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (s *Server) initialize(params jsontext.Value) map[string]any {
	var p struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	json.Unmarshal(params, &p)

	version := supportedVersions[0]
	if slices.Contains(supportedVersions, p.ProtocolVersion) {
		version = p.ProtocolVersion
	}
	return map[string]any{
		"protocolVersion": version,
		"capabilities":    map[string]any{"tools": map[string]any{}},
		"serverInfo":      map[string]any{"name": s.name, "version": s.version},
	}
}

func (s *Server) startCall(ctx context.Context, msg message) {
	var p struct {
		Name      string         `json:"name"`
		Arguments jsontext.Value `json:"arguments"`
	}
	if err := json.Unmarshal(msg.Params, &p); err != nil {
		s.reply(msg.ID, nil, &rpcError{codeInvalidParams, err.Error()})
		return
	}
	i := slices.IndexFunc(s.tools, func(t Tool) bool { return t.Name == p.Name })
	if i < 0 {
		s.reply(msg.ID, nil, &rpcError{codeInvalidParams, "unknown tool " + p.Name})
		return
	}
	if len(p.Arguments) == 0 {
		p.Arguments = jsontext.Value("{}")
	}

	ctx, cancel := context.WithCancel(ctx)
	key := string(msg.ID)
	s.callsMu.Lock()
	s.calls[key] = cancel
	s.callsMu.Unlock()

	go func() {
		defer func() {
			s.callsMu.Lock()
			delete(s.calls, key)
			s.callsMu.Unlock()
			cancel()
		}()

		text, err := s.tools[i].Call(ctx, p.Arguments)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			text = err.Error()
		}
		s.reply(msg.ID, map[string]any{
			"content": []map[string]any{{"type": "text", "text": text}},
			"isError": err != nil,
		}, nil)
	}()
}

func (s *Server) reply(id jsontext.Value, result any, rpcErr *rpcError) {
	raw, err := json.Marshal(response{JSONRPC: "2.0", ID: id, Result: result, Error: rpcErr})
	if err != nil {
		raw, _ = json.Marshal(response{JSONRPC: "2.0", ID: id, Error: &rpcError{-32603, err.Error()}})
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	s.out.Write(append(raw, '\n'))
}
