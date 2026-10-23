package transport

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestPortZeroGivesADifferentFreePortEachTime(t *testing.T) {
	a, err := Listen(0)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := Listen(0)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if PortOf(a) == 0 || PortOf(a) == PortOf(b) {
		t.Fatalf("ports %d %d", PortOf(a), PortOf(b))
	}
	if !strings.HasPrefix(a.Addr().String(), "127.0.0.1:") {
		t.Fatalf("not the loopback address: %s", a.Addr())
	}
}

func TestASecondCopyCannotTakeAFixedPort(t *testing.T) {
	a, err := Listen(0)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if _, err := Listen(PortOf(a)); err == nil || !strings.Contains(err.Error(), "another copy") {
		t.Fatalf("second listener: %v", err)
	}
}

func TestTheHandshakeRoundTrips(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteHandshake(&buf, Handshake{Port: 4321, Token: "abc", Version: "1"}); err != nil {
		t.Fatal(err)
	}
	if strings.Count(buf.String(), "\n") != 1 {
		t.Fatalf("the handshake must be one line: %q", buf.String())
	}
	got, err := ReadHandshake(&buf)
	if err != nil || got.Port != 4321 || got.Token != "abc" || got.Version != "1" {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestABadHandshakeIsRefused(t *testing.T) {
	for _, in := range []string{"", "\n", "garbage\n", `{"port":0,"token":"x"}` + "\n", `{"port":5}` + "\n", strings.Repeat("a", 10000)} {
		if _, err := ReadHandshake(strings.NewReader(in)); err == nil {
			t.Fatalf("accepted %q", in[:min(len(in), 30)])
		}
	}
}

func TestTheEngineStopsWhenItsParentGoes(t *testing.T) {
	r, w := io.Pipe()
	closed := make(chan struct{})
	WatchParent(r, func() { close(closed) })
	w.Write([]byte("anything the parent writes is ignored"))
	select {
	case <-closed:
		t.Fatal("the engine stopped while its parent was still there")
	case <-time.After(50 * time.Millisecond):
	}
	w.Close()
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("the engine did not notice that its parent was gone")
	}
}

func TestStartupSwitchAnswers503ThenDelegates(t *testing.T) {
	var s StartupSwitch
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest("GET", "/api/status", nil))
	if rec.Code != 503 || !strings.Contains(rec.Body.String(), "starting") {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	s.Set(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
	rec = httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest("GET", "/api/status", nil))
	if rec.Code != 204 {
		t.Fatalf("after Set: %d", rec.Code)
	}
}

func TestShutdownNeedsTheTokenAndSignalsOnce(t *testing.T) {
	p, _ := NewPolicy(nil)
	mux := http.NewServeMux()
	quit := make(chan struct{}, 1)
	RegisterShutdown(mux, quit)
	srv := httptest.NewServer(p.Middleware(mux))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	port, _ := strconv.Atoi(u.Port())
	p.SetPort(port)
	post := func(token string) int {
		req, _ := http.NewRequest("POST", srv.URL+"/internal/shutdown", nil)
		if token != "" {
			req.Header.Set(TokenHeader, token)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if code := post(""); code != 401 {
		t.Fatalf("without the token: %d", code)
	}
	select {
	case <-quit:
		t.Fatal("a request without the token stopped the engine")
	default:
	}
	if code := post(p.Token()); code != 204 {
		t.Fatalf("with the token: %d", code)
	}
	if code := post(p.Token()); code != 204 { // a second request must not block the handler
		t.Fatalf("second request: %d", code)
	}
	select {
	case <-quit:
	default:
		t.Fatal("the engine was not told to stop")
	}
}

func TestTheServerHasLimitsAgainstSlowAndHugeHeaders(t *testing.T) {
	s := NewHTTPServer(http.NotFoundHandler())
	if s.ReadHeaderTimeout <= 0 || s.ReadTimeout <= 0 || s.IdleTimeout <= 0 || s.MaxHeaderBytes <= 0 || s.MaxHeaderBytes > 64<<10 {
		t.Fatalf("limits: %+v", s)
	}
}
