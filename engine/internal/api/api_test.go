package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"socrates/internal/llm"
	"socrates/internal/store"
	"socrates/internal/transport"
)

const (
	claudeKey = "sk-ant-api03-USERS-OWN-KEY-0123456789"
	fixedFmt  = `{"data":[{"id":"claude-sonnet-5-5","line":"sonnet"},{"id":"claude-haiku-5-5","line":"haiku"}],"has_more":false}`
)

// provider is a stand-in Claude: it answers the model list and the messages call, and records every request.
type provider struct {
	mu       sync.Mutex
	srv      *httptest.Server
	listCode int
	callCode int
	Seen     []string
}

func newProvider(t *testing.T) *provider {
	p := &provider{listCode: 200, callCode: 200}
	p.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		p.mu.Lock()
		p.Seen = append(p.Seen, r.Method+" "+r.URL.Path)
		code := p.callCode
		if strings.HasPrefix(r.URL.Path, "/v1/models") {
			code = p.listCode
		}
		p.mu.Unlock()
		w.WriteHeader(code)
		switch {
		case code == 401:
			io.WriteString(w, `{"error":{"type":"authentication_error","message":"invalid x-api-key `+claudeKey+`"}}`)
		case strings.HasPrefix(r.URL.Path, "/v1/models"):
			io.WriteString(w, fixedFmt)
		case code == 200:
			io.WriteString(w, `{"content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":1}}`)
		default:
			io.WriteString(w, `{"error":{"message":"failed"}}`)
		}
	}))
	t.Cleanup(p.srv.Close)
	return p
}

type env struct {
	srv    *httptest.Server
	policy *transport.Policy
	store  *store.Store
	prov   *provider
	path   string
	logs   *bytes.Buffer
}

func setup(t *testing.T) *env {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "data")
	path := filepath.Join(dir, "socrates.db")
	st, err := store.Open(path, bytes.Repeat([]byte{3}, 32))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	prov := newProvider(t)
	mux := New(Deps{Store: st, Version: "test", LLMOptions: []llm.Option{llm.WithBaseURL(prov.srv.URL), llm.WithSleep(func(context.Context, time.Duration) error { return nil })}})
	pol, _ := transport.NewPolicy(nil)
	srv := httptest.NewServer(pol.Middleware(mux))
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	port, _ := strconv.Atoi(u.Port())
	pol.SetPort(port)
	logs := &bytes.Buffer{}
	log.SetOutput(logs)
	t.Cleanup(func() { log.SetOutput(io.Discard) })
	return &env{srv: srv, policy: pol, store: st, prov: prov, path: path, logs: logs}
}

func (e *env) do(t *testing.T, method, path, body string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest(method, e.srv.URL+path, strings.NewReader(body))
	req.Header.Set(transport.TokenHeader, e.policy.Token())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(data)
}

func TestStatusIsOpenAndTheRestNeedsTheToken(t *testing.T) {
	e := setup(t)
	resp, _ := http.Get(e.srv.URL + "/api/status")
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	resp, _ = http.Get(e.srv.URL + "/api/settings/llm")
	if resp.StatusCode != 401 {
		t.Fatalf("settings without a token: %d", resp.StatusCode)
	}
}

func TestChoosingAProviderChecksTheKeyFirstAndNeverReturnsIt(t *testing.T) {
	e := setup(t)
	code, body := e.do(t, "PUT", "/api/settings/llm", `{"provider":"claude","apiKey":"`+claudeKey+`"}`)
	if code != 200 || strings.Contains(body, claudeKey) {
		t.Fatalf("%d %s", code, body)
	}
	var v LLMSettingsView
	json.Unmarshal([]byte(body), &v)
	if v.Provider != "claude" || !v.HasKey || v.FastModel != "claude-haiku-5-5" || v.ReplyModel != "claude-sonnet-5-5" || v.Tested {
		t.Fatalf("%+v", v)
	}
	code, body = e.do(t, "GET", "/api/settings/llm", "")
	if code != 200 || strings.Contains(body, claudeKey) || !strings.Contains(body, `"hasKey":true`) {
		t.Fatalf("%d %s", code, body)
	}
	if got, ok, _ := e.store.Setting(context.Background(), "llm.key"); !ok || got != claudeKey {
		t.Fatal("the key was not stored")
	}
}

func TestAWrongKeyIsNeverStored(t *testing.T) {
	e := setup(t)
	e.prov.listCode = 401
	code, body := e.do(t, "PUT", "/api/settings/llm", `{"provider":"claude","apiKey":"`+claudeKey+`"}`)
	if code != 422 || strings.Contains(body, claudeKey) || !strings.Contains(body, `"kind":"auth"`) {
		t.Fatalf("%d %s", code, body)
	}
	if _, ok, _ := e.store.Setting(context.Background(), "llm.key"); ok {
		t.Fatal("a key the provider refused was stored")
	}
	if _, ok, _ := e.store.Setting(context.Background(), "llm.provider"); ok {
		t.Fatal("the provider was saved with a refused key")
	}
}

func TestTheKeyIsNeverInTheLogsOrInTheFileOnDisk(t *testing.T) {
	e := setup(t)
	e.do(t, "PUT", "/api/settings/llm", `{"provider":"claude","apiKey":"`+claudeKey+`"}`)
	e.prov.listCode = 401
	e.do(t, "PUT", "/api/settings/llm", `{"provider":"claude","apiKey":"`+claudeKey+`-other"}`)
	e.prov.callCode = 401
	e.do(t, "POST", "/api/settings/llm/test", "")
	if strings.Contains(e.logs.String(), claudeKey) {
		t.Fatalf("the key reached the log: %s", e.logs.String())
	}
	e.store.Close()
	for _, f := range []string{e.path, e.path + "-wal", e.path + "-shm"} {
		data, err := os.ReadFile(f)
		if err == nil && bytes.Contains(data, []byte(claudeKey)) {
			t.Fatalf("the key is readable in %s", filepath.Base(f))
		}
	}
}

func TestOmittingTheKeyKeepsTheStoredOneAndChangingTheProviderDropsIt(t *testing.T) {
	e := setup(t)
	e.do(t, "PUT", "/api/settings/llm", `{"provider":"claude","apiKey":"`+claudeKey+`"}`)
	if code, _ := e.do(t, "PUT", "/api/settings/llm", `{"provider":"claude"}`); code != 200 {
		t.Fatalf("keeping the key: %d", code)
	}
	if got, _, _ := e.store.Setting(context.Background(), "llm.key"); got != claudeKey {
		t.Fatal("the stored key was lost")
	}
	code, body := e.do(t, "PUT", "/api/settings/llm", `{"provider":"openai"}`)
	if code != 400 || !strings.Contains(body, "API key") {
		t.Fatalf("a new provider cannot reuse another provider's key: %d %s", code, body)
	}
	if got, _, _ := e.store.Setting(context.Background(), "llm.key"); got != claudeKey {
		t.Fatal("a refused change altered the stored key")
	}
}

func TestBadInputIsRefused(t *testing.T) {
	e := setup(t)
	for name, body := range map[string]string{
		"unknown provider":  `{"provider":"gemini","apiKey":"k"}`,
		"no key":            `{"provider":"claude"}`,
		"bad ollama":        `{"provider":"ollama","ollamaEndpoint":"ftp://x"}`,
		"not json":          `nonsense`,
		"credentials in it": `{"provider":"ollama","ollamaEndpoint":"http://u:p@127.0.0.1:11434"}`,
	} {
		if code, _ := e.do(t, "PUT", "/api/settings/llm", body); code != 400 {
			t.Errorf("%s: %d", name, code)
		}
	}
	if code, _ := e.do(t, "PUT", "/api/settings/llm", `{"provider":"claude","apiKey":"`+strings.Repeat("a", 2<<20)+`"}`); code != 413 {
		t.Errorf("a huge body: %d", code)
	}
	if _, ok, _ := e.store.Setting(context.Background(), "llm.provider"); ok {
		t.Error("something was saved from refused input")
	}
}

func TestTheConnectionTestAndRemovingTheKey(t *testing.T) {
	e := setup(t)
	if code, _ := e.do(t, "POST", "/api/settings/llm/test", ""); code != 409 {
		t.Fatalf("a test before any provider: %d", code)
	}
	e.do(t, "PUT", "/api/settings/llm", `{"provider":"claude","apiKey":"`+claudeKey+`"}`)
	if code, body := e.do(t, "POST", "/api/settings/llm/test", ""); code != 200 || !strings.Contains(body, `"ok":true`) {
		t.Fatalf("%d %s", code, body)
	}
	e.prov.callCode = 401
	if code, body := e.do(t, "POST", "/api/settings/llm/test", ""); code != 422 || strings.Contains(body, claudeKey) {
		t.Fatalf("a refused key at test time: %d %s", code, body)
	}
	code, body := e.do(t, "DELETE", "/api/settings/llm/key", "")
	if code != 200 || !strings.Contains(body, `"hasKey":false`) {
		t.Fatalf("%d %s", code, body)
	}
	if _, ok, _ := e.store.Setting(context.Background(), "llm.key"); ok {
		t.Fatal("the key is still stored")
	}
}

func TestProviderFailuresAreMappedToPlainAnswers(t *testing.T) {
	for kind, want := range map[llm.Kind]int{llm.KindAuth: 422, llm.KindRateLimit: 429, llm.KindOverloaded: 503, llm.KindModelGone: 409, llm.KindBadRequest: 400, llm.KindNetwork: 502, llm.KindServer: 502} {
		rec := httptest.NewRecorder()
		failLLM(rec, &llm.Error{Kind: kind, Reason: "secret-reason-" + claudeKey})
		if rec.Code != want || strings.Contains(rec.Body.String(), "secret-reason") || strings.Contains(rec.Body.String(), claudeKey) {
			t.Errorf("%s: %d %s", kind, rec.Code, rec.Body)
		}
	}
	rec := httptest.NewRecorder()
	failLLM(rec, io.ErrUnexpectedEOF)
	if rec.Code != 500 {
		t.Errorf("an unexpected error: %d", rec.Code)
	}
}
