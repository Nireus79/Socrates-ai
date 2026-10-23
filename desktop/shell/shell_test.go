package shell

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// engine builds the real engine once and starts it on a scratch folder with a test database key (never the real keychain).
func startEngine(t *testing.T) *Engine {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "socrates-engine")
	if out, err := exec.Command("go", "build", "-C", "../../engine", "-o", bin, "./cmd/socrates-engine").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	e, err := Start(bin, []string{"--data-dir", filepath.Join(t.TempDir(), "data")}, []string{"SOCRATES_DB_KEY=" + strings.Repeat("ef", 32)}, 20*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(e.Stop)
	return e
}

func get(t *testing.T, h http.Handler, method, path, origin string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	body, _ := io.ReadAll(rec.Body)
	return rec.Code, string(body)
}

func TestTheProxyAddsTheTokenAndOnlyServesTheAPI(t *testing.T) {
	e := startEngine(t)
	h := e.Proxy(nil)
	var code int
	var body string
	for i := 0; i < 50; i++ { // the engine opens its database after the handshake
		if code, body = get(t, h, "GET", "/api/status", ""); code == 200 {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if code != 200 || !strings.Contains(body, "ready") {
		t.Fatalf("status through the proxy: %d %s", code, body)
	}
	if code, _ = get(t, h, "GET", "/api/philosophers", ""); code != 200 {
		t.Fatalf("the philosophers: %d", code)
	}
	for _, p := range []string{"/", "/index.html", "/other", "/apix"} {
		if code, _ = get(t, h, "GET", p, ""); code != 404 {
			t.Errorf("%s must not reach the engine: %d", p, code)
		}
	}
	// the engine itself refuses a request without the token: the proxy is what holds it
	direct, err := http.Get("http://127.0.0.1:" + itoa(e.Hand.Port) + "/api/profile")
	if err != nil {
		t.Fatal(err)
	}
	direct.Body.Close()
	if direct.StatusCode != 401 && direct.StatusCode != 403 {
		t.Fatalf("a request without the token got %d", direct.StatusCode)
	}
	// an Origin set by the page is not passed on (the engine refuses unknown origins)
	if code, _ = get(t, h, "GET", "/api/status", "https://evil.example"); code != 200 {
		t.Fatalf("with a page origin: %d", code)
	}
}

func itoa(n int) string { return strconv.Itoa(n) }

func TestTheEngineStopsWhenTheShellLetsGoOfIt(t *testing.T) {
	e := startEngine(t)
	pid := e.cmd.Process.Pid
	e.Stop()
	select {
	case <-e.Done():
	default:
		t.Fatal("Stop returned while the engine was still running")
	}
	if err := syscall.Kill(pid, 0); err == nil {
		t.Fatal("the engine process is still there after Stop")
	}
}

func TestAnEngineThatCannotStartOrStaysSilentIsAnError(t *testing.T) {
	if _, err := Start("/no/such/engine", nil, nil, time.Second); err == nil {
		t.Fatal("a missing engine")
	}
	if _, err := Start("/bin/true", nil, nil, 2*time.Second); err == nil {
		t.Fatal("an engine that stops at once")
	}
	if _, err := Start("/bin/sleep", []string{"30"}, nil, 500*time.Millisecond); err == nil {
		t.Fatal("an engine that says nothing")
	}
	if _, err := Start("/bin/echo", []string{"not json"}, nil, 2*time.Second); err == nil {
		t.Fatal("a handshake that is not JSON")
	}
}

type recorder struct{ seen []*http.Request }

func (r *recorder) RoundTrip(req *http.Request) (*http.Response, error) {
	r.seen = append(r.seen, req)
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("{}")), Header: http.Header{"Content-Type": {"application/json"}}}, nil
}

func TestTheProxyForwardsOnlyTheAPIWithTheTokenAndNoOrigin(t *testing.T) {
	rec := &recorder{}
	e := &Engine{Hand: Handshake{Port: 1234, Token: "tok"}}
	h := e.Proxy(&http.Client{Transport: rec})
	for _, p := range []string{"/", "/x", "/apix", "/index.html"} {
		if code, _ := get(t, h, "GET", p, ""); code != 404 {
			t.Errorf("%s: %d", p, code)
		}
	}
	if len(rec.seen) != 0 {
		t.Fatalf("%d requests outside /api reached the engine", len(rec.seen))
	}
	get(t, h, "POST", "/api/conversations?x=1", "https://evil.example")
	if len(rec.seen) != 1 {
		t.Fatal("the API request was not forwarded")
	}
	r := rec.seen[0]
	if r.Header.Get(tokenHeader) != "tok" || r.Header.Get("Origin") != "" || r.URL.String() != "http://127.0.0.1:1234/api/conversations?x=1" || r.Method != "POST" {
		t.Fatalf("%v %v", r.URL, r.Header)
	}
}
