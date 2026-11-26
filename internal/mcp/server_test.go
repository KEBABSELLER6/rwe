package mcp

import (
	"bufio"
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"io"
	"testing"
	"time"
)

type session struct {
	t   *testing.T
	in  *io.PipeWriter
	out *bufio.Scanner
}

func start(t *testing.T, tools ...Tool) *session {
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	go NewServer("test", "0", tools...).Serve(t.Context(), inR, outW)
	t.Cleanup(func() { inW.Close() })
	return &session{t: t, in: inW, out: bufio.NewScanner(outR)}
}

func (s *session) send(msg string) {
	s.t.Helper()
	if _, err := io.WriteString(s.in, msg+"\n"); err != nil {
		s.t.Fatal(err)
	}
}

func (s *session) recv() map[string]any {
	s.t.Helper()
	if !s.out.Scan() {
		s.t.Fatal("no response")
	}
	var v map[string]any
	if err := json.Unmarshal(s.out.Bytes(), &v); err != nil {
		s.t.Fatal(err)
	}
	return v
}

func echoTool() Tool {
	return Tool{
		Name:        "echo",
		InputSchema: map[string]any{"type": "object"},
		Call: func(ctx context.Context, args jsontext.Value) (string, error) {
			var a struct {
				Text string `json:"text"`
			}
			json.Unmarshal(args, &a)
			if a.Text == "" {
				return "", errors.New("empty")
			}
			return a.Text, nil
		},
	}
}

func TestHandshakeAndCall(t *testing.T) {
	s := start(t, echoTool())

	s.send(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"0"}}}`)
	init := s.recv()
	if v := init["result"].(map[string]any)["protocolVersion"]; v != "2025-06-18" {
		t.Fatalf("protocolVersion = %v", v)
	}

	s.send(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	s.send(`{"jsonrpc":"2.0","id":"list","method":"tools/list"}`)
	list := s.recv()
	if list["id"] != "list" {
		t.Fatalf("id = %v", list["id"])
	}
	tools := list["result"].(map[string]any)["tools"].([]any)
	if len(tools) != 1 || tools[0].(map[string]any)["name"] != "echo" {
		t.Fatalf("tools = %v", tools)
	}

	s.send(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"echo","arguments":{"text":"hi"}}}`)
	res := s.recv()["result"].(map[string]any)
	text := res["content"].([]any)[0].(map[string]any)["text"]
	if text != "hi" || res["isError"] != false {
		t.Fatalf("result = %v", res)
	}

	s.send(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"echo","arguments":{}}}`)
	if res := s.recv()["result"].(map[string]any); res["isError"] != true {
		t.Fatalf("result = %v, want isError", res)
	}
}

func TestErrors(t *testing.T) {
	s := start(t, echoTool())

	s.send(`not json`)
	if code := s.recv()["error"].(map[string]any)["code"]; code != float64(codeParseError) {
		t.Fatalf("code = %v", code)
	}

	s.send(`{"jsonrpc":"2.0","id":1,"method":"resources/list"}`)
	if code := s.recv()["error"].(map[string]any)["code"]; code != float64(codeMethodNotFound) {
		t.Fatalf("code = %v", code)
	}

	s.send(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"nope"}}`)
	if code := s.recv()["error"].(map[string]any)["code"]; code != float64(codeInvalidParams) {
		t.Fatalf("code = %v", code)
	}
}

func TestCancelledCallGetsNoReply(t *testing.T) {
	started, cancelled := make(chan struct{}), make(chan struct{})
	block := Tool{
		Name: "block",
		Call: func(ctx context.Context, _ jsontext.Value) (string, error) {
			close(started)
			<-ctx.Done()
			close(cancelled)
			return "late", nil
		},
	}
	s := start(t, block)

	s.send(`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"block"}}`)
	<-started
	s.send(`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":7}}`)
	select {
	case <-cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("tool context was not cancelled")
	}
	s.send(`{"jsonrpc":"2.0","id":8,"method":"ping"}`)
	if id := s.recv()["id"]; id != float64(8) {
		t.Fatalf("first reply id = %v, want the ping", id)
	}
}
