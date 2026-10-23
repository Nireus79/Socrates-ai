package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"socrates/internal/conversation"
	"socrates/internal/llm"
	"socrates/internal/store"
	"socrates/internal/transport"
)

// script is a stand-in Claude that answers the messages calls from a queue and the model list at once.
type script struct {
	mu    sync.Mutex
	queue []string
	calls int
	srv   *httptest.Server
}

func reply(text string) string {
	b, _ := json.Marshal(map[string]any{"content": []map[string]string{{"type": "text", "text": text}}, "stop_reason": "end_turn", "usage": map[string]int{"input_tokens": 5, "output_tokens": 5}})
	return string(b)
}

func newScript(t *testing.T, texts ...string) *script {
	s := &script{}
	for _, x := range texts {
		s.queue = append(s.queue, reply(x))
	}
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		if strings.HasPrefix(r.URL.Path, "/v1/models") {
			io.WriteString(w, fixedFmt)
			return
		}
		s.mu.Lock()
		s.calls++
		var body string
		if len(s.queue) > 0 {
			body, s.queue = s.queue[0], s.queue[1:]
		}
		s.mu.Unlock()
		if body == "" {
			w.WriteHeader(529)
			io.WriteString(w, `{"error":{"message":"the script ran out"}}`)
			return
		}
		io.WriteString(w, body)
	}))
	t.Cleanup(s.srv.Close)
	return s
}

type app struct {
	srv *httptest.Server
	pol *transport.Policy
	svc *conversation.Service
	st  *store.Store
}

func newApp(t *testing.T, sc *script) *app {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "d", "socrates.db"), bytes.Repeat([]byte{4}, 32))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	mux, svc := New(Deps{Store: st, Version: "test", LLMOptions: []llm.Option{llm.WithBaseURL(sc.srv.URL), llm.WithSleep(func(context.Context, time.Duration) error { return nil })}})
	pol, _ := transport.NewPolicy(nil)
	srv := httptest.NewServer(pol.Middleware(mux))
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	port, _ := strconv.Atoi(u.Port())
	pol.SetPort(port)
	return &app{srv: srv, pol: pol, svc: svc, st: st}
}

func (a *app) do(t *testing.T, method, path, body string) (int, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest(method, a.srv.URL+path, strings.NewReader(body))
	req.Header.Set(transport.TokenHeader, a.pol.Token())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	var out map[string]any
	json.Unmarshal(data, &out)
	return resp.StatusCode, out
}

func (a *app) setUp(t *testing.T) {
	t.Helper()
	if code, body := a.do(t, "PUT", "/api/settings/llm", `{"provider":"claude","apiKey":"`+claudeKey+`"}`); code != 200 {
		t.Fatalf("the provider: %d %v", code, body)
	}
	if code, _ := a.do(t, "POST", "/api/profile/age", ""); code != 200 {
		t.Fatalf("the age: %d", code)
	}
}

const (
	okJudge = `{"label":"ok","confidence":0.95,"evidence":"","reason":"x"}`
	okGate  = `{"flagged":false,"category":"none","reason":"x"}`
)

func TestTheProfileTheRosterAndTheAgeConfirmation(t *testing.T) {
	a := newApp(t, newScript(t))
	_, p := a.do(t, "GET", "/api/profile", "")
	if p["ageConfirmed"] != false || p["hasProvider"] != false {
		t.Fatalf("%v", p)
	}
	a.setUp(t)
	_, p = a.do(t, "GET", "/api/profile", "")
	if p["ageConfirmed"] != true || p["hasProvider"] != true {
		t.Fatalf("%v", p)
	}
	_, r := a.do(t, "GET", "/api/philosophers", "")
	list, _ := r["philosophers"].([]any)
	if len(list) != 4 || list[0].(map[string]any)["id"] != "socrates" {
		t.Fatalf("%v", r)
	}
}

func TestATurnOverHTTPAndItsHistory(t *testing.T) {
	a := newApp(t, newScript(t, okJudge, "What do you mean by it?", okGate, "On courage"))
	a.setUp(t)
	code, conv := a.do(t, "POST", "/api/conversations", `{"philosopherId":"socrates"}`)
	if code != 201 {
		t.Fatalf("%d %v", code, conv)
	}
	id := conv["id"].(string)
	code, res := a.do(t, "POST", "/api/conversations/"+id+"/messages", `{"text":"What is courage?"}`)
	if code != 200 || res["outcome"] != "replied" {
		t.Fatalf("%d %v", code, res)
	}
	added := res["added"].([]any)
	if len(added) != 2 || added[1].(map[string]any)["text"] != "What do you mean by it?" || added[1].(map[string]any)["role"] != "philosopher" {
		t.Fatalf("%v", added)
	}
	a.svc.Wait()
	code, h := a.do(t, "GET", "/api/conversations/"+id, "")
	msgs := h["messages"].([]any)
	if code != 200 || len(msgs) != 2 || h["conversation"].(map[string]any)["title"] != "On courage" {
		t.Fatalf("%d %v", code, h)
	}
	_, list := a.do(t, "GET", "/api/conversations", "")
	if cs := list["conversations"].([]any); len(cs) != 1 || cs[0].(map[string]any)["id"] != id {
		t.Fatalf("%v", list)
	}
}

func TestAFixedAnswerIsReturnedWithItsTextAndKeepsOnlyAMarker(t *testing.T) {
	a := newApp(t, newScript(t, `{"label":"self_harm_clear","confidence":0.97,"evidence":"end my life","reason":"x"}`))
	a.setUp(t)
	_, conv := a.do(t, "POST", "/api/conversations", `{"philosopherId":"socrates"}`)
	id := conv["id"].(string)
	code, res := a.do(t, "POST", "/api/conversations/"+id+"/messages", `{"text":"I want to end my life"}`)
	if code != 200 || res["outcome"] != "fixed" || !strings.Contains(res["text"].(string), "emergency number") {
		t.Fatalf("%d %v", code, res)
	}
	_, h := a.do(t, "GET", "/api/conversations/"+id, "")
	msgs := h["messages"].([]any)
	m := msgs[0].(map[string]any)
	if len(msgs) != 1 || m["role"] != "marker" || m["kind"] != "crisis" || !strings.Contains(m["text"].(string), "emergency number") {
		t.Fatalf("%v", msgs)
	}
	if strings.Contains(mustJSON(h), "end my life") {
		t.Fatal("the user's words are in the history")
	}
}

func mustJSON(v any) string { b, _ := json.Marshal(v); return string(b) }

func TestAProviderThatFailsGivesA503WithTheFixedTextAndKeepsNothing(t *testing.T) {
	a := newApp(t, newScript(t)) // an empty script: every call is refused as overloaded
	a.setUp(t)
	_, conv := a.do(t, "POST", "/api/conversations", `{"philosopherId":"socrates"}`)
	id := conv["id"].(string)
	code, res := a.do(t, "POST", "/api/conversations/"+id+"/messages", `{"text":"hello there"}`)
	if code != 503 || res["outcome"] != "unavailable" || res["providerKind"] != "overloaded" || !strings.Contains(res["text"].(string), "cannot answer right now") {
		t.Fatalf("%d %v", code, res)
	}
	_, h := a.do(t, "GET", "/api/conversations/"+id, "")
	if len(h["messages"].([]any)) != 0 {
		t.Fatal("a failed turn kept something")
	}
}

func TestTheRulesOfAMessageAreEnforcedOverHTTP(t *testing.T) {
	sc := newScript(t)
	a := newApp(t, sc)
	_, conv := a.do(t, "POST", "/api/conversations", `{"philosopherId":"socrates"}`)
	id := conv["id"].(string)
	path := "/api/conversations/" + id + "/messages"
	if code, r := a.do(t, "POST", path, `{"text":"hello"}`); code != 403 || r["kind"] != "age" {
		t.Fatalf("no age confirmation: %d %v", code, r)
	}
	a.do(t, "POST", "/api/profile/age", "")
	if code, r := a.do(t, "POST", path, `{"text":"hello"}`); code != 409 || r["kind"] != "no_provider" {
		t.Fatalf("no provider: %d %v", code, r)
	}
	a.do(t, "PUT", "/api/settings/llm", `{"provider":"claude","apiKey":"`+claudeKey+`"}`)
	if code, _ := a.do(t, "POST", path, `{"text":"   "}`); code != 400 {
		t.Fatalf("an empty message: %d", code)
	}
	if code, _ := a.do(t, "POST", path, `{"text":"`+strings.Repeat("a", conversation.MaxMessageChars+1)+`"}`); code != 413 {
		t.Fatalf("a long message: %d", code)
	}
	if code, _ := a.do(t, "POST", "/api/conversations/none/messages", `{"text":"hello"}`); code != 404 {
		t.Fatalf("an unknown conversation: %d", code)
	}
	if sc.calls != 0 {
		t.Fatalf("%d model calls were made for messages that were refused before any model", sc.calls)
	}
	if code, _ := a.do(t, "POST", "/api/conversations", `{"philosopherId":"nobody"}`); code != 400 {
		t.Fatalf("an unknown philosopher: %d", code)
	}
	if code, _ := a.do(t, "GET", "/api/conversations/none", ""); code != 404 {
		t.Fatalf("history of an unknown conversation: %d", code)
	}
}

func TestDeletingAConversation(t *testing.T) {
	a := newApp(t, newScript(t))
	_, conv := a.do(t, "POST", "/api/conversations", `{"philosopherId":"plato"}`)
	id := conv["id"].(string)
	if code, _ := a.do(t, "DELETE", "/api/conversations/"+id, ""); code != 204 {
		t.Fatalf("%d", code)
	}
	if code, _ := a.do(t, "DELETE", "/api/conversations/"+id, ""); code != 404 {
		t.Fatalf("twice: %d", code)
	}
}

func TestSuggestionsOverHTTPAndAChosenOneIsMarked(t *testing.T) {
	sug := `{"suggestions":["A lie is saying what you know is false.","I might lie to spare a friend.","It always costs trust.","I really don't know."]}`
	a := newApp(t, newScript(t, okJudge, "What do you mean by a lie?", okGate, "On lying", sug, okJudge, "And if the friend asked?", okGate))
	a.setUp(t)
	_, conv := a.do(t, "POST", "/api/conversations", `{"philosopherId":"socrates"}`)
	id := conv["id"].(string)
	if code, r := a.do(t, "POST", "/api/conversations/"+id+"/suggestions", ""); code != 409 {
		t.Fatalf("before any question: %d %v", code, r)
	}
	a.do(t, "POST", "/api/conversations/"+id+"/messages", `{"text":"Is it ever okay to lie?"}`)
	a.svc.Wait()
	code, r := a.do(t, "POST", "/api/conversations/"+id+"/suggestions", `{"exclude":["x"]}`)
	list, _ := r["suggestions"].([]any)
	if code != 200 || len(list) != 4 || r["fallback"] != false {
		t.Fatalf("%d %v", code, r)
	}
	code, res := a.do(t, "POST", "/api/conversations/"+id+"/messages", `{"text":"I might lie to spare a friend.","suggested":true}`)
	added := res["added"].([]any)
	if code != 200 || added[0].(map[string]any)["suggested"] != true {
		t.Fatalf("%d %v", code, res)
	}
	_, h := a.do(t, "GET", "/api/conversations/"+id, "")
	if !strings.Contains(mustJSON(h), `"suggested":true`) {
		t.Fatal("the mark is not in the history")
	}
	if code, _ := a.do(t, "POST", "/api/conversations/none/suggestions", ""); code != 404 {
		t.Fatalf("unknown conversation: %d", code)
	}
}

func TestSuggestionsWhenTheProviderIsDownAre503WithAPlainSentence(t *testing.T) {
	a := newApp(t, newScript(t, okJudge, "What do you mean?", okGate, "t")) // then the script is empty: overloaded
	a.setUp(t)
	_, conv := a.do(t, "POST", "/api/conversations", `{"philosopherId":"socrates"}`)
	id := conv["id"].(string)
	a.do(t, "POST", "/api/conversations/"+id+"/messages", `{"text":"hello there"}`)
	a.svc.Wait()
	code, r := a.do(t, "POST", "/api/conversations/"+id+"/suggestions", "")
	if code != 503 || !strings.Contains(r["error"].(string), "cannot offer suggestions") {
		t.Fatalf("%d %v", code, r)
	}
}
