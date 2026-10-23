package transport

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

// door starts a server behind the policy, with a few routes to poke at.
func door(t *testing.T, origins ...string) (*Policy, *httptest.Server) {
	t.Helper()
	p, err := NewPolicy(origins)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/status", func(w http.ResponseWriter, _ *http.Request) { WriteJSON(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("/api/echo", func(w http.ResponseWriter, r *http.Request) {
		var v map[string]any
		if !ReadBody(w, r, &v) {
			return
		}
		WriteJSON(w, 200, v)
	})
	mux.HandleFunc("/internal/x", func(w http.ResponseWriter, _ *http.Request) { WriteJSON(w, 200, map[string]string{"ok": "yes"}) })
	mux.HandleFunc("/api/boom", func(http.ResponseWriter, *http.Request) { panic("the user wrote: SECRET-TEXT") })
	srv := httptest.NewServer(p.Middleware(mux))
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	port, _ := strconv.Atoi(u.Port())
	p.SetPort(port)
	return p, srv
}

func do(t *testing.T, srv *httptest.Server, method, path string, hdr map[string]string, body string) (*http.Response, string) {
	t.Helper()
	req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
	for k, v := range hdr {
		if k == "Host" {
			req.Host = v
			continue
		}
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp, string(data)
}

func TestTheTokenIsRandomAndLong(t *testing.T) {
	a, _ := NewPolicy(nil)
	b, _ := NewPolicy(nil)
	if len(a.Token()) != 64 || a.Token() == b.Token() {
		t.Fatalf("tokens: %q %q", a.Token(), b.Token())
	}
}

func TestAForeignHostIsRefused(t *testing.T) {
	p, srv := door(t)
	good := map[string]string{TokenHeader: p.Token()}
	for _, host := range []string{"evil.example:" + strings.Split(srv.URL, ":")[2], "127.0.0.1:1", "127.0.0.1", "10.0.0.5:" + strings.Split(srv.URL, ":")[2]} {
		h := map[string]string{TokenHeader: p.Token(), "Host": host}
		if resp, _ := do(t, srv, "GET", "/api/echo", h, "{}"); resp.StatusCode != 403 {
			t.Fatalf("Host %q: status %d", host, resp.StatusCode)
		}
	}
	if resp, _ := do(t, srv, "POST", "/api/echo", good, "{}"); resp.StatusCode != 200 {
		t.Fatalf("the real host was refused: %d", resp.StatusCode)
	}
}

func TestTheNamesOfTheLoopbackAreAccepted(t *testing.T) {
	p, srv := door(t)
	port := strings.Split(srv.URL, ":")[2]
	for _, host := range []string{"localhost:" + port, "127.0.0.1:" + port, "[::1]:" + port} {
		h := map[string]string{TokenHeader: p.Token(), "Host": host}
		if resp, _ := do(t, srv, "POST", "/api/echo", h, "{}"); resp.StatusCode != 200 {
			t.Fatalf("Host %q: status %d", host, resp.StatusCode)
		}
	}
}

func TestEveryOriginIsRefusedUnlessTheShellRegisteredIt(t *testing.T) {
	p, srv := door(t)
	for _, origin := range []string{"http://evil.example", "http://localhost", "null", "*", "wails://wails"} {
		h := map[string]string{TokenHeader: p.Token(), "Origin": origin}
		if resp, _ := do(t, srv, "POST", "/api/echo", h, "{}"); resp.StatusCode != 403 {
			t.Fatalf("Origin %q: status %d", origin, resp.StatusCode)
		}
	}
	if resp, _ := do(t, srv, "POST", "/api/echo", map[string]string{TokenHeader: p.Token()}, "{}"); resp.StatusCode != 200 {
		t.Fatalf("a request without Origin (the shell) was refused: %d", resp.StatusCode)
	}
}

func TestARegisteredOriginIsAnsweredExactlyAndNoOtherIs(t *testing.T) {
	p, srv := door(t, "http://localhost", "*", "null", " ")
	h := map[string]string{TokenHeader: p.Token(), "Origin": "http://localhost"}
	resp, _ := do(t, srv, "POST", "/api/echo", h, "{}")
	if resp.StatusCode != 200 || resp.Header.Get("Access-Control-Allow-Origin") != "http://localhost" {
		t.Fatalf("registered origin: %d %v", resp.StatusCode, resp.Header)
	}
	for _, origin := range []string{"http://localhost:1234", "https://localhost", "http://evil.example", "null", "*"} {
		h := map[string]string{TokenHeader: p.Token(), "Origin": origin}
		if resp, _ := do(t, srv, "POST", "/api/echo", h, "{}"); resp.StatusCode != 403 {
			t.Fatalf("Origin %q: status %d (a wildcard or null must never be accepted, even when configured)", origin, resp.StatusCode)
		}
	}
}

func TestPreflightIsAnsweredOnlyForARegisteredOrigin(t *testing.T) {
	_, srv := door(t, "http://localhost")
	resp, _ := do(t, srv, "OPTIONS", "/api/echo", map[string]string{"Origin": "http://localhost", "Access-Control-Request-Method": "POST"}, "")
	if resp.StatusCode != 204 || !strings.Contains(resp.Header.Get("Access-Control-Allow-Headers"), TokenHeader) {
		t.Fatalf("preflight: %d %v", resp.StatusCode, resp.Header)
	}
	resp, _ = do(t, srv, "OPTIONS", "/api/echo", map[string]string{"Origin": "http://evil.example"}, "")
	if resp.StatusCode != 403 || resp.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("foreign preflight: %d %v", resp.StatusCode, resp.Header)
	}
}

func TestTheTokenIsNeededEverywhereButTheLivenessCheck(t *testing.T) {
	p, srv := door(t)
	for _, path := range []string{"/api/echo", "/internal/x", "/api/other", "/"} {
		if resp, _ := do(t, srv, "POST", path, nil, "{}"); resp.StatusCode != 401 {
			t.Fatalf("%s without a token: %d", path, resp.StatusCode)
		}
		if resp, _ := do(t, srv, "POST", path, map[string]string{TokenHeader: "wrong"}, "{}"); resp.StatusCode != 401 {
			t.Fatalf("%s with a wrong token: %d", path, resp.StatusCode)
		}
	}
	if resp, _ := do(t, srv, "GET", "/api/status", nil, ""); resp.StatusCode != 200 {
		t.Fatalf("status needs no token: %d", resp.StatusCode)
	}
	if resp, _ := do(t, srv, "POST", "/api/status", nil, ""); resp.StatusCode != 401 {
		t.Fatalf("only GET and HEAD of the status are free: %d", resp.StatusCode)
	}
	if resp, _ := do(t, srv, "GET", "/internal/x?token="+p.Token(), nil, ""); resp.StatusCode != 401 {
		t.Fatalf("a token in the query string must not be accepted: %d", resp.StatusCode)
	}
	if resp, _ := do(t, srv, "GET", "/internal/x", map[string]string{TokenHeader: p.Token()}, ""); resp.StatusCode != 200 {
		t.Fatalf("the right token: %d", resp.StatusCode)
	}
}

func TestAPanicBecomesAPlainErrorAndNeverReachesTheLogOrTheReply(t *testing.T) {
	p, srv := door(t)
	var logs bytes.Buffer
	log.SetOutput(&logs)
	defer log.SetOutput(io.Discard)
	resp, body := do(t, srv, "GET", "/api/boom", map[string]string{TokenHeader: p.Token()}, "")
	if resp.StatusCode != 500 || strings.Contains(body, "SECRET-TEXT") {
		t.Fatalf("reply: %d %q", resp.StatusCode, body)
	}
	if strings.Contains(logs.String(), "SECRET-TEXT") {
		t.Fatalf("the value of a panic reached the log: %s", logs.String())
	}
	if !strings.Contains(logs.String(), "/api/boom") {
		t.Fatalf("the place of the panic is missing from the log: %s", logs.String())
	}
	// The engine is still up.
	if resp, _ := do(t, srv, "GET", "/api/status", nil, ""); resp.StatusCode != 200 {
		t.Fatal("the server did not survive the panic")
	}
}

func TestAnOversizedBodyIsRefusedWith413(t *testing.T) {
	p, srv := door(t)
	p.SetMaxBody(1024)
	big := `{"x":"` + strings.Repeat("a", 4096) + `"}`
	if resp, _ := do(t, srv, "POST", "/api/echo", map[string]string{TokenHeader: p.Token()}, big); resp.StatusCode != 413 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if resp, _ := do(t, srv, "POST", "/api/echo", map[string]string{TokenHeader: p.Token()}, "not json"); resp.StatusCode != 400 {
		t.Fatalf("a bad body: %d", resp.StatusCode)
	}
}

func TestEveryAnswerIsNotCachedAndNotSniffed(t *testing.T) {
	p, srv := door(t)
	for _, tc := range []struct {
		hdr map[string]string
		why string
	}{{nil, "a refused request"}, {map[string]string{TokenHeader: p.Token()}, "an accepted request"}, {map[string]string{"Host": "evil.example:1"}, "a foreign host"}} {
		resp, _ := do(t, srv, "POST", "/api/echo", tc.hdr, "{}")
		if resp.Header.Get("Cache-Control") != "no-store" || resp.Header.Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("%s: %v", tc.why, resp.Header)
		}
	}
}
