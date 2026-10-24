package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"socrates/internal/transport"
)

// A stand-in Claude: it lists two models and answers /v1/messages according to who is asking (judge, gate, ledger, title, summary or the
// philosopher).
type fakeClaude struct {
	mu       sync.Mutex
	verdicts []string // the judge's answers, in order
	replies  []string // the philosopher's answers, in order
	prompts  []string // every system prompt seen (for the ordering check)
}

func (f *fakeClaude) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"data":[{"id":"claude-sonnet-5-5","line":"sonnet"},{"id":"claude-haiku-5-5","line":"haiku"}],"has_more":false}`)
	})
	mux.HandleFunc("/v1/messages", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			System string `json:"system"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		sys := req.System
		f.mu.Lock()
		defer f.mu.Unlock()
		var out string
		switch {
		case strings.Contains(sys, "safety judge"):
			out, f.verdicts = f.verdicts[0], f.verdicts[1:]
			f.prompts = append(f.prompts, "judge")
		case strings.Contains(sys, "You check a draft reply"):
			out = `{"flagged":false,"category":"none","reason":"fine"}`
			f.prompts = append(f.prompts, "gate")
		case strings.HasPrefix(sys, "You keep the ledger"):
			out = `{"ops":[{"op":"add_claim","text":"courage matters, says ZEBRA-CLAIM","quote":"ZEBRA-USER-WORDS about courage"}]}`
			f.prompts = append(f.prompts, "ledger")
		case strings.Contains(sys, "title"):
			out = "A short title"
			f.prompts = append(f.prompts, "title")
		case strings.Contains(sys, "running summary"):
			out = "A summary."
			f.prompts = append(f.prompts, "summary")
		default:
			out, f.replies = f.replies[0], f.replies[1:]
			f.prompts = append(f.prompts, "reply")
		}
		json.NewEncoder(w).Encode(map[string]any{"model": "claude-sonnet-5-5", "content": []map[string]string{{"type": "text", "text": out}}, "stop_reason": "end_turn"})
	})
	return mux
}

func (e *engine) call(t *testing.T, method, path, body string) (int, map[string]any, string) {
	t.Helper()
	req, _ := http.NewRequest(method, e.url(path), strings.NewReader(body))
	req.Header.Set(transport.TokenHeader, e.hs.Token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	json.Unmarshal(raw, &out)
	return resp.StatusCode, out, string(raw)
}

func TestAWholeJourneyThroughTheRealEngine(t *testing.T) {
	const (
		secretUser   = "ZEBRA-USER-WORDS about courage"
		secretReply  = "QUOKKA-REPLY-WORDS what do you mean"
		secretCrisis = "WOMBAT-CRISIS-WORDS I want to end my life"
	)
	fake := &fakeClaude{
		verdicts: []string{
			`{"label":"ok","confidence":0.95,"evidence":"","reason":"x"}`,
			`{"label":"self_harm_clear","confidence":0.97,"evidence":"end my life","reason":"x"}`,
		},
		replies: []string{secretReply},
	}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()

	e := start(t, "SOCRATES_TEST_PROVIDER_URL="+srv.URL)
	e.waitReady(t)
	// before anything is set up, a message is refused for the right reason
	_, conv, _ := e.call(t, "POST", "/api/conversations", `{"philosopherId":"socrates"}`)
	id, _ := conv["id"].(string)
	if id == "" {
		t.Fatalf("no conversation: %v", conv)
	}
	if code, body, _ := e.call(t, "POST", "/api/conversations/"+id+"/messages", `{"text":"hello"}`); code != 403 || body["kind"] != "age" {
		t.Fatalf("before the age confirmation: %d %v", code, body)
	}
	e.call(t, "POST", "/api/profile/age", "")
	if code, body, _ := e.call(t, "POST", "/api/conversations/"+id+"/messages", `{"text":"hello"}`); code != 409 || body["kind"] != "no_provider" {
		t.Fatalf("before the provider: %d %v", code, body)
	}
	if code, _, raw := e.call(t, "PUT", "/api/settings/llm", `{"provider":"claude","apiKey":"sk-ant-journey-test"}`); code != 200 {
		t.Fatalf("provider: %d %s", code, raw)
	}

	// an ordinary turn
	code, res, raw := e.call(t, "POST", "/api/conversations/"+id+"/messages", `{"text":"`+secretUser+`"}`)
	if code != 200 || res["outcome"] != "replied" || !strings.Contains(raw, secretReply) {
		t.Fatalf("turn: %d %s", code, raw)
	}
	// a crisis turn: the fixed message, and the words are not kept
	code, res, raw = e.call(t, "POST", "/api/conversations/"+id+"/messages", `{"text":"`+secretCrisis+`"}`)
	if code != 200 || res["outcome"] != "fixed" || !strings.Contains(raw, "emergency number") {
		t.Fatalf("crisis: %d %s", code, raw)
	}
	// the ledger was written after the first turn (in the background) from the person's own words
	var remembered []any
	for i := 0; i < 50 && len(remembered) == 0; i++ {
		_, led, _ := e.call(t, "GET", "/api/ledger?philosopherId=socrates", "")
		remembered, _ = led["entries"].([]any)
		time.Sleep(100 * time.Millisecond)
	}
	if len(remembered) != 1 || remembered[0].(map[string]any)["quote"] != secretUser {
		t.Fatalf("ledger: %v", remembered)
	}
	// the order of the calls was judge, reply, gate (then title and summary in the background), judge for the crisis and nothing else
	want := []string{"judge", "reply", "gate"}
	fake.mu.Lock()
	got := append([]string(nil), fake.prompts[:3]...)
	fake.mu.Unlock()
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order of calls: %v", got)
		}
	}
	_, hist, histRaw := e.call(t, "GET", "/api/conversations/"+id, "")
	msgs := hist["messages"].([]any)
	if len(msgs) != 3 || strings.Contains(histRaw, "WOMBAT") {
		t.Fatalf("history (%d messages): %s", len(msgs), histRaw)
	}
	// the export has the conversation and no key or setting
	_, _, exp := e.call(t, "GET", "/api/profile/export", "")
	if !strings.Contains(exp, "ZEBRA") || strings.Contains(exp, "WOMBAT") || strings.Contains(exp, "llm.") {
		t.Fatalf("export: %s", exp)
	}

	// nothing anyone wrote is in the log or on the disk in plain text
	stop := func() { e.stdin.Close(); e.waitExit(t, 15*1e9) }
	stop()
	logs := e.stderr.String()
	for _, s := range []string{"ZEBRA", "QUOKKA", "WOMBAT", "end my life"} { // ZEBRA covers the message, its quotation and the ledger claim
		if strings.Contains(logs, s) {
			t.Fatalf("%q reached the log:\n%s", s, logs)
		}
	}
	entries, _ := os.ReadDir(e.dir)
	for _, ent := range entries {
		data, err := os.ReadFile(filepath.Join(e.dir, ent.Name()))
		if err != nil {
			continue
		}
		for _, s := range []string{"ZEBRA", "QUOKKA", "WOMBAT", "A short title"} {
			if bytes.Contains(data, []byte(s)) {
				t.Fatalf("%q is in %s in plain text", s, ent.Name())
			}
		}
	}
}
