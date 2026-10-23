package llm

import (
	"context"
	"errors"
	"testing"
)

func TestOllamaEndpointsAreChecked(t *testing.T) {
	good := map[string]string{
		"":                            "http://127.0.0.1:11434",
		"http://127.0.0.1:11434":      "http://127.0.0.1:11434",
		"http://127.0.0.1:11434/":     "http://127.0.0.1:11434",
		"http://localhost:11434":      "http://localhost:11434",
		"https://ollama.example.org":  "https://ollama.example.org",
		"  http://192.168.1.20:11434": "http://192.168.1.20:11434",
	}
	for in, want := range good {
		got, err := NormalizeOllamaEndpoint(in)
		if err != nil || got != want {
			t.Errorf("%q -> %q, %v (want %q)", in, got, err, want)
		}
	}
	for _, bad := range []string{"ftp://x", "file:///etc/passwd", "http://user:pass@127.0.0.1:11434", "http://127.0.0.1:11434?x=1", "http://", "127.0.0.1:11434", "/api", "http://127.0.0.1:11434#a", "javascript:alert(1)"} {
		if got, err := NormalizeOllamaEndpoint(bad); err == nil {
			t.Errorf("%q was accepted as %q", bad, got)
		}
	}
	if _, err := NewOllama("ftp://x"); err == nil {
		t.Error("NewOllama accepted a bad endpoint")
	}
}

func TestOllamaCallHasTheRightShape(t *testing.T) {
	f := newFake(t, ok(`{"model":"mistral:latest","message":{"role":"assistant","content":"Well."},"done_reason":"stop","prompt_eval_count":7,"eval_count":2}`))
	o, err := NewOllama(f.srv.URL, f.opts()...)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := o.Complete(context.Background(), "mistral:latest", Request{System: "persona", Messages: userMsg("hi"), JSON: true})
	if err != nil || resp.Text != "Well." || resp.InputTokens != 7 || resp.OutputTokens != 2 || resp.StopReason != "stop" {
		t.Fatalf("%v %+v", err, resp)
	}
	got := f.last()
	if got.Path != "/api/chat" || got.Body["stream"] != false || got.Body["format"] != "json" {
		t.Fatalf("%s %v", got.Path, got.Body)
	}
	msgs, _ := got.Body["messages"].([]any)
	if len(msgs) != 2 || msgs[0].(map[string]any)["role"] != "system" {
		t.Fatalf("%v", msgs)
	}
	if np, _ := got.Body["options"].(map[string]any)["num_predict"].(float64); np < 2048 {
		t.Fatalf("num_predict %v", np)
	}
	if got.Headers.Get("Authorization") != "" || got.Headers.Get("x-api-key") != "" {
		t.Fatal("a credential was sent to a local server")
	}
}

func TestOllamaFailuresAreClassified(t *testing.T) {
	f := newFake(t, status(404, `{"error":"model 'nope' not found, try pulling it first"}`))
	o, _ := NewOllama(f.srv.URL, f.opts()...)
	if _, err := o.Complete(context.Background(), "nope", Request{Messages: userMsg("x")}); !errors.Is(err, ErrModelGone) {
		t.Fatalf("%v", err)
	}
	f = newFake(t, ok(`{"message":{"content":""}}`))
	o, _ = NewOllama(f.srv.URL, f.opts()...)
	if _, err := o.Complete(context.Background(), "m", Request{Messages: userMsg("x")}); !errors.Is(err, ErrEmpty) {
		t.Fatalf("%v", err)
	}
	f = newFake(t, ok(`not json`))
	o, _ = NewOllama(f.srv.URL, f.opts()...)
	if _, err := o.Complete(context.Background(), "m", Request{Messages: userMsg("x")}); !errors.Is(err, ErrServer) {
		t.Fatalf("%v", err)
	}
}
