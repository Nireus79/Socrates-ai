//go:build !windows

package shell

import (
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// memSecrets stands in for the system's secure storage: no test writes a real keychain.
type memSecrets struct {
	mu     sync.Mutex
	m      map[string]string
	broken bool
}

func newMem() *memSecrets { return &memSecrets{m: map[string]string{}} }
func (s *memSecrets) Get(id string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.broken {
		return "", errors.New("no secure storage")
	}
	v, ok := s.m[id]
	if !ok {
		return "", ErrNoSecret
	}
	return v, nil
}
func (s *memSecrets) Set(id, v string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.broken {
		return errors.New("no secure storage")
	}
	s.m[id] = v
	return nil
}
func (s *memSecrets) Delete(id string) error { s.mu.Lock(); defer s.mu.Unlock(); delete(s.m, id); return nil }
func (s *memSecrets) has(id string) bool     { s.mu.Lock(); defer s.mu.Unlock(); _, ok := s.m[id]; return ok }

const annaPass = "anna has a long passphrase"

// registerAnna makes a profile with a passphrase through the shell's own routes, as the screens do, and leaves it open and unlocked.
func registerAnna(t *testing.T, h http.Handler) string {
	t.Helper()
	_, out := call(t, h, "POST", "/shell/profiles", `{"name":"Anna"}`)
	id := out["id"].(string)
	if code, o := call(t, h, "POST", "/shell/open", `{"id":"`+id+`"}`); code != 200 {
		t.Fatalf("open: %d %v", code, o)
	}
	apiWhenReady(t, h, "GET", "/api/keystore/status", "", 200)
	if code, _ := call(t, h, "POST", "/api/keystore/setup", `{"mode":"passphrase","passphrase":"`+annaPass+`"}`); code != 200 {
		t.Fatalf("setup: %d", code)
	}
	return id
}

// nextStart is the window opened again: a new manager over the same folder and the same secure storage, with nothing open.
func nextStart(t *testing.T, base string, sec Secrets) (*Manager, http.Handler) {
	t.Helper()
	m := NewManager(base, engineBinary(t))
	m.wait = 20 * time.Second
	m.SetSecrets(sec)
	t.Cleanup(m.Stop)
	return m, m.Handler()
}

func TestStayingSignedInOpensTheLastProfileAndUnlocksItWithoutThePagePassphrase(t *testing.T) {
	m, h, base := newManager(t)
	sec := newMem()
	m.SetSecrets(sec)
	id := registerAnna(t, h)

	// Nothing is remembered until the person asks.
	if _, out := call(t, h, "GET", "/shell/profiles", ""); out["stay"] == true || out["last"] != id || out["canRemember"] != true {
		t.Fatalf("before asking: %v", out)
	}
	if code, out := call(t, h, "POST", "/shell/stay", `{"id":"`+id+`","stay":true,"passphrase":"`+annaPass+`"}`); code != 200 {
		t.Fatalf("stay: %d %v", code, out)
	}
	if !sec.has(id) {
		t.Fatal("the passphrase was not kept in the secure storage")
	}
	// the passphrase is in no file of ours
	assertNoFileHolds(t, base, annaPass)
	m.Stop() // closing the window is not logging out

	m2, h2 := nextStart(t, base, sec)
	_, view := call(t, h2, "GET", "/shell/profiles", "")
	if view["stay"] != true || view["last"] != id || view["active"] != "" {
		t.Fatalf("next start: %v", view)
	}
	code, out := call(t, h2, "POST", "/shell/autologin", `{"id":"`+id+`"}`)
	if code != 200 {
		t.Fatalf("autologin: %d %v", code, out)
	}
	if strings.Contains(strings.ToLower(toJSON(out)), strings.ToLower(annaPass)) {
		t.Fatal("the passphrase reached the page")
	}
	if m2.Active() != id {
		t.Fatalf("active = %q", m2.Active())
	}
	// the engine is unlocked: it answers as a ready engine, not with the key gate's 423
	apiWhenReady(t, h2, "GET", "/api/status", "", 200)
}

func TestLoggingOutForgetsTheStayAndTheStoredPassphrase(t *testing.T) {
	m, h, base := newManager(t)
	sec := newMem()
	m.SetSecrets(sec)
	id := registerAnna(t, h)
	call(t, h, "POST", "/shell/stay", `{"id":"`+id+`","stay":true,"passphrase":"`+annaPass+`"}`)
	call(t, h, "POST", "/shell/close", "")
	if sec.has(id) {
		t.Fatal("the passphrase survived logging out")
	}
	_, h2 := nextStart(t, base, sec)
	if _, view := call(t, h2, "GET", "/shell/profiles", ""); view["stay"] == true {
		t.Fatalf("still staying after logging out: %v", view)
	}
	if code, _ := call(t, h2, "POST", "/shell/autologin", `{"id":"`+id+`"}`); code != 401 {
		t.Fatalf("autologin after logout: %d", code)
	}
}

func TestAPassphraseTheEngineRefusesIsForgottenAndNotTriedAgain(t *testing.T) {
	m, h, base := newManager(t)
	sec := newMem()
	m.SetSecrets(sec)
	id := registerAnna(t, h)
	call(t, h, "POST", "/shell/stay", `{"id":"`+id+`","stay":true,"passphrase":"`+annaPass+`"}`)
	m.Stop()
	sec.Set(id, "a different long passphrase") // the passphrase was changed behind our back, or the entry is stale

	_, h2 := nextStart(t, base, sec)
	if code, _ := call(t, h2, "POST", "/shell/autologin", `{"id":"`+id+`"}`); code != 401 {
		t.Fatalf("a wrong stored passphrase: %d", code)
	}
	if sec.has(id) {
		t.Fatal("the refused passphrase was kept")
	}
	if _, view := call(t, h2, "GET", "/shell/profiles", ""); view["stay"] == true {
		t.Fatalf("still staying: %v", view)
	}
}

func TestOpeningAnotherProfileEndsTheStayOfTheFirst(t *testing.T) {
	m, h, _ := newManager(t)
	sec := newMem()
	m.SetSecrets(sec)
	id := registerAnna(t, h)
	call(t, h, "POST", "/shell/stay", `{"id":"`+id+`","stay":true,"passphrase":"`+annaPass+`"}`)
	_, ben := call(t, h, "POST", "/shell/profiles", `{"name":"Ben"}`)
	call(t, h, "POST", "/shell/open", `{"id":"`+ben["id"].(string)+`"}`)
	if sec.has(id) {
		t.Fatal("Anna's passphrase stayed in the storage while Ben is at the screen")
	}
	_, view := call(t, h, "GET", "/shell/profiles", "")
	if view["last"] != ben["id"] || view["stay"] == true {
		t.Fatalf("%v", view)
	}
	// and the window will not open Anna by itself
	if code, _ := call(t, h, "POST", "/shell/autologin", `{"id":"`+id+`"}`); code != 401 {
		t.Fatalf("autologin of a profile that is no longer the one staying: %d", code)
	}
}

func TestAProfileWithoutAPassphraseStaysWithoutAnySecret(t *testing.T) {
	m, h, base := newManager(t)
	sec := newMem()
	m.SetSecrets(sec)
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "unix:path=/nonexistent/bus")
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	_, out := call(t, h, "POST", "/shell/profiles", `{"name":"Cy","open":true}`)
	id := out["id"].(string)
	call(t, h, "POST", "/shell/open", `{"id":"`+id+`"}`)
	apiWhenReady(t, h, "GET", "/api/keystore/status", "", 200)
	call(t, h, "POST", "/api/keystore/setup", `{"mode":"file","confirmInsecure":true}`)
	if code, o := call(t, h, "POST", "/shell/stay", `{"id":"`+id+`","stay":true}`); code != 200 {
		t.Fatalf("stay: %d %v", code, o)
	}
	if sec.has(id) {
		t.Fatal("a secret was kept for a profile that has no passphrase")
	}
	m.Stop()
	m2, h2 := nextStart(t, base, sec)
	if code, o := call(t, h2, "POST", "/shell/autologin", `{"id":"`+id+`"}`); code != 200 || m2.Active() != id {
		t.Fatalf("autologin: %d %v active=%q", code, o, m2.Active())
	}
	// logging out ends the staying for this kind of profile too
	call(t, h2, "POST", "/shell/close", "")
	if code, _ := call(t, h2, "POST", "/shell/autologin", `{"id":"`+id+`"}`); code != 401 {
		t.Fatalf("autologin after logging out: %d", code)
	}
}

func TestStayingNeedsAPassphraseAndAWorkingSecureStorage(t *testing.T) {
	m, h, _ := newManager(t)
	sec := newMem()
	m.SetSecrets(sec)
	id := registerAnna(t, h)
	if code, _ := call(t, h, "POST", "/shell/stay", `{"id":"`+id+`","stay":true}`); code != 400 {
		t.Fatalf("a protected profile without its passphrase: %d", code)
	}
	if code, _ := call(t, h, "POST", "/shell/stay", `{"id":"0123456789abcdef","stay":true,"passphrase":"x"}`); code != 400 {
		t.Fatalf("an unknown profile: %d", code)
	}
	sec.broken = true
	if _, view := call(t, h, "GET", "/shell/profiles", ""); view["canRemember"] != false {
		t.Fatalf("a missing secure storage must be reported: %v", view)
	}
	if code, _ := call(t, h, "POST", "/shell/stay", `{"id":"`+id+`","stay":true,"passphrase":"`+annaPass+`"}`); code != 400 {
		t.Fatalf("no secure storage: %d", code)
	}
	if _, view := call(t, h, "GET", "/shell/profiles", ""); view["stay"] == true {
		t.Fatalf("staying was recorded without a secret: %v", view)
	}
}

func TestRemovingAProfileForgetsItsStoredPassphrase(t *testing.T) {
	m, h, _ := newManager(t)
	sec := newMem()
	m.SetSecrets(sec)
	id := registerAnna(t, h)
	call(t, h, "POST", "/shell/stay", `{"id":"`+id+`","stay":true,"passphrase":"`+annaPass+`"}`)
	m.Close() // the window lets go without logging out; the profile is then removed from the manage list
	if err := m.Remove(id); err != nil {
		t.Fatal(err)
	}
	if sec.has(id) {
		t.Fatal("the passphrase of a removed profile was kept")
	}
}

func toJSON(v any) string { b, _ := json.Marshal(v); return string(b) }

// assertNoFileHolds fails if any file under dir holds the text (a secret must not be written to a file of ours).
func assertNoFileHolds(t *testing.T, dir, secret string) {
	t.Helper()
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if b, err := os.ReadFile(p); err == nil && strings.Contains(string(b), secret) {
			t.Fatalf("%s holds the passphrase", p)
		}
		return nil
	})
}
