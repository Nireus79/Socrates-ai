package transport

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"socrates/internal/keystore"
)

// noKeychain is a keychain that cannot be reached, so a fresh install has to ask the person.
type noKeychain struct{}

func (noKeychain) Get(string, string) (string, error) { return "", errors.New("no keychain") }
func (noKeychain) Set(string, string, string) error   { return errors.New("no keychain") }
func (noKeychain) Delete(string, string) error        { return nil }

func gate(t *testing.T) (*keystore.Store, *Policy, *httptest.Server, string) {
	t.Helper()
	dir := t.TempDir()
	store := keystore.Open(dir, filepath.Join(dir, "socrates.db"), noKeychain{})
	p, _ := NewPolicy(nil)
	srv := httptest.NewServer(p.Middleware(KeyGate(store, "test")))
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	port, _ := strconv.Atoi(u.Port())
	p.SetPort(port)
	return store, p, srv, dir
}

func call(t *testing.T, p *Policy, srv *httptest.Server, method, path, body string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
	req.Header.Set(TokenHeader, p.Token())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var sb strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		sb.Write(buf[:n])
		if err != nil {
			break
		}
	}
	return resp.StatusCode, sb.String()
}

func TestTheGateIsLockedUntilTheKeyIsChosenAndAnswersNothingElse(t *testing.T) {
	_, p, srv, _ := gate(t)
	if code, body := call(t, p, srv, "GET", "/api/status", ""); code != 423 || !strings.Contains(body, "locked") {
		t.Fatalf("status: %d %s", code, body)
	}
	code, body := call(t, p, srv, "GET", "/api/keystore/status", "")
	var st keystore.Status
	json.Unmarshal([]byte(body), &st)
	if code != 200 || st.State != keystore.StateNeedsSetup || strings.Join(st.Options, ",") != "passphrase,file" {
		t.Fatalf("keystore status: %d %s", code, body)
	}
	for _, path := range []string{"/api/conversations", "/api/philosophers", "/anything"} {
		if code, _ := call(t, p, srv, "GET", path, ""); code != 423 {
			t.Fatalf("%s: %d (nothing may be served before the key is there)", path, code)
		}
	}
}

func TestASetupThatIsRefusedNeverEchoesThePassphraseOrWritesAnything(t *testing.T) {
	store, p, srv, _ := gate(t)
	code, body := call(t, p, srv, "POST", "/api/keystore/setup", `{"mode":"passphrase","passphrase":"tooshort-pw"}`)
	if code != 400 || strings.Contains(body, "tooshort-pw") {
		t.Fatalf("short passphrase: %d %s", code, body)
	}
	if code, _ := call(t, p, srv, "POST", "/api/keystore/setup", `{"mode":"file"}`); code != 400 {
		t.Fatalf("a key file without confirmation: %d", code)
	}
	if code, _ := call(t, p, srv, "POST", "/api/keystore/setup", `not json`); code != 400 {
		t.Fatalf("a bad body: %d", code)
	}
	if st := store.Status(); st.State != keystore.StateNeedsSetup {
		t.Fatalf("state after refused setups: %s", st.State)
	}
}

func TestSetupWithAPassphraseThenALockedRestartAndUnlock(t *testing.T) {
	store, p, srv, dir := gate(t)
	if code, body := call(t, p, srv, "POST", "/api/keystore/setup", `{"mode":"passphrase","passphrase":"a long enough passphrase"}`); code != 200 || strings.Contains(body, "a long enough passphrase") {
		t.Fatalf("setup: %d %s", code, body)
	}
	if _, ok := store.Key(); !ok {
		t.Fatal("no key after setup")
	}
	// A new start finds the key sealed.
	store2 := keystore.Open(dir, filepath.Join(dir, "socrates.db"), noKeychain{})
	p2, _ := NewPolicy(nil)
	srv2 := httptest.NewServer(p2.Middleware(KeyGate(store2, "test")))
	defer srv2.Close()
	u, _ := url.Parse(srv2.URL)
	port, _ := strconv.Atoi(u.Port())
	p2.SetPort(port)
	if code, body := call(t, p2, srv2, "POST", "/api/keystore/unlock", `{"passphrase":"the wrong passphrase!!"}`); code != 403 || strings.Contains(body, "wrong passphrase!!") {
		t.Fatalf("wrong passphrase: %d %s", code, body)
	}
	if code, _ := call(t, p2, srv2, "POST", "/api/keystore/unlock", `{"passphrase":"a long enough passphrase"}`); code != 200 {
		t.Fatalf("right passphrase: %d", code)
	}
	if _, ok := store2.Key(); !ok {
		t.Fatal("unlocked, but no key")
	}
	if code, _ := call(t, p2, srv2, "POST", "/api/keystore/unlock", `{"passphrase":"a long enough passphrase"}`); code != 409 {
		t.Fatalf("unlocking twice: %d", code)
	}
}

func TestTheGateNeedsTheTokenForTheKeyRoutes(t *testing.T) {
	_, _, srv, _ := gate(t)
	for _, path := range []string{"/api/keystore/status", "/api/keystore/setup", "/api/keystore/unlock"} {
		resp, err := http.Post(srv.URL+path, "application/json", strings.NewReader("{}"))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 401 {
			t.Fatalf("%s without the token: %d", path, resp.StatusCode)
		}
	}
}
