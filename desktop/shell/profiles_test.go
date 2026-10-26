//go:build !windows

package shell

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// engineBinary builds the real engine once per test.
func engineBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "socrates-engine")
	if out, err := exec.Command("go", "build", "-C", "../../engine", "-o", bin, "./cmd/socrates-engine").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	return bin
}

func newManager(t *testing.T) (*Manager, http.Handler, string) {
	t.Helper()
	t.Setenv("SOCRATES_DB_KEY", "") // a key in the environment must not matter for a profile
	base := filepath.Join(t.TempDir(), "base")
	m := NewManager(base, engineBinary(t))
	m.wait = 20 * time.Second
	t.Cleanup(m.Stop)
	return m, m.Handler(), base
}

func call(t *testing.T, h http.Handler, method, path, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out map[string]any
	json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

// apiOK waits for the engine to answer the path (it opens its database after the handshake) and returns the last answer.
func apiWhenReady(t *testing.T, h http.Handler, method, path, body string, want int) map[string]any {
	t.Helper()
	var code int
	var out map[string]any
	for i := 0; i < 60; i++ {
		if code, out = call(t, h, method, path, body); code == want {
			return out
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("%s %s: code %d, want %d (%v)", method, path, code, want, out)
	return nil
}

func TestProfileNamesAreCheckedAndKeptOnTheList(t *testing.T) {
	m, _, base := newManager(t)
	for _, bad := range []string{"", "   ", strings.Repeat("a", 41), "line\nbreak"} {
		if _, err := m.Create(bad); err == nil {
			t.Fatalf("%q was accepted", bad)
		}
	}
	a, err := m.Create("  Anna ") // spaces around a name are dropped
	if err != nil || a.Name != "Anna" {
		t.Fatalf("%+v %v", a, err)
	}
	if _, err := m.Create("ANNA"); err == nil {
		t.Fatal("a name that differs only in case was accepted")
	}
	b, err := m.Create("Βασίλης")
	if err != nil {
		t.Fatal(err)
	}
	// A new manager over the same folder sees both, in order.
	list, err := NewManager(base, "").List()
	if err != nil || len(list) != 2 || list[0].ID != a.ID || list[1].ID != b.ID || list[1].Name != "Βασίλης" {
		t.Fatalf("list = %+v, %v", list, err)
	}
	if st, err := os.Stat(filepath.Join(base, registry)); err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("registry: %v %v", st, err)
	}
}

func TestADamagedListIsReportedNotReplaced(t *testing.T) {
	m, h, base := newManager(t)
	os.MkdirAll(base, 0o700)
	os.WriteFile(filepath.Join(base, registry), []byte("{not json"), 0o600)
	if _, err := m.Create("Anna"); err == nil {
		t.Fatal("a profile was added over a damaged list")
	}
	if code, _ := call(t, h, "GET", "/shell/profiles", ""); code != http.StatusInternalServerError {
		t.Fatalf("code = %d", code)
	}
	if b, _ := os.ReadFile(filepath.Join(base, registry)); string(b) != "{not json" {
		t.Fatal("the damaged list was overwritten")
	}
}

func TestWithNoProfileOpenTheEngineIsNotReachable(t *testing.T) {
	_, h, _ := newManager(t)
	code, out := call(t, h, "GET", "/api/status", "")
	if code != http.StatusConflict || out["kind"] != "no_profile" {
		t.Fatalf("%d %v", code, out)
	}
	if code, _ := call(t, h, "GET", "/etc/passwd", ""); code != http.StatusNotFound {
		t.Fatalf("code = %d", code)
	}
}

func TestEachProfileHasItsOwnDataAndItsOwnPassphrase(t *testing.T) {
	_, h, base := newManager(t)
	_, anna := call(t, h, "POST", "/shell/profiles", `{"name":"Anna"}`)
	_, ben := call(t, h, "POST", "/shell/profiles", `{"name":"Ben"}`)
	annaID, benID := anna["id"].(string), ben["id"].(string)

	// Anna registers: only a passphrase is offered, then her choice is saved.
	if code, out := call(t, h, "POST", "/shell/open", `{"id":"`+annaID+`"}`); code != 200 {
		t.Fatalf("open: %d %v", code, out)
	}
	st := apiWhenReady(t, h, "GET", "/api/keystore/status", "", 200)
	if st["state"] != "needs_setup" || len(st["options"].([]any)) != 1 || st["options"].([]any)[0] != "passphrase" {
		t.Fatalf("status = %v", st)
	}
	if code, _ := call(t, h, "POST", "/api/keystore/setup", `{"mode":"file","confirmInsecure":true}`); code == 200 {
		t.Fatal("a key file was accepted for a profile")
	}
	if code, _ := call(t, h, "POST", "/api/keystore/setup", `{"mode":"passphrase","passphrase":"anna has a long passphrase"}`); code != 200 {
		t.Fatalf("setup: %d", code)
	}
	apiWhenReady(t, h, "GET", "/api/settings/theme", "", 200)
	if code, _ := call(t, h, "PUT", "/api/settings/theme", `{"theme":"dark"}`); code != 200 {
		t.Fatalf("theme: %d", code)
	}

	// Logging out stops her engine; no profile is open.
	call(t, h, "POST", "/shell/close", "")
	if code, _ := call(t, h, "GET", "/api/status", ""); code != http.StatusConflict {
		t.Fatalf("after logout: %d", code)
	}

	// Ben has his own folder and his own, still empty, settings.
	call(t, h, "POST", "/shell/open", `{"id":"`+benID+`"}`)
	if st := apiWhenReady(t, h, "GET", "/api/keystore/status", "", 200); st["state"] != "needs_setup" {
		t.Fatalf("Ben's status = %v", st)
	}
	call(t, h, "POST", "/api/keystore/setup", `{"mode":"passphrase","passphrase":"ben has another long one"}`)
	apiWhenReady(t, h, "GET", "/api/settings/theme", "", 200)
	if _, out := call(t, h, "GET", "/api/settings/theme", ""); out["theme"] != "auto" {
		t.Fatalf("Ben sees %v", out)
	}

	// Anna comes back: locked, Ben's passphrase does not open her, hers does, and her theme is still there.
	call(t, h, "POST", "/shell/open", `{"id":"`+annaID+`"}`)
	if st := apiWhenReady(t, h, "GET", "/api/keystore/status", "", 200); st["state"] != "locked" {
		t.Fatalf("Anna's status = %v", st)
	}
	if code, _ := call(t, h, "POST", "/api/keystore/unlock", `{"passphrase":"ben has another long one"}`); code == 200 {
		t.Fatal("another profile's passphrase opened this one")
	}
	if code, _ := call(t, h, "POST", "/api/keystore/unlock", `{"passphrase":"anna has a long passphrase"}`); code != 200 {
		t.Fatalf("unlock: %d", code)
	}
	apiWhenReady(t, h, "GET", "/api/settings/theme", "", 200)
	if _, out := call(t, h, "GET", "/api/settings/theme", ""); out["theme"] != "dark" {
		t.Fatalf("Anna sees %v", out)
	}
	// Nothing about either person is readable in the other's folder.
	if _, err := os.Stat(filepath.Join(base, profileDir, annaID, "socrates.db")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(base, "socrates.db")); err == nil {
		t.Fatal("a database was made in the base folder")
	}
}

func TestOpeningAnotherProfileStopsTheFirstEngine(t *testing.T) {
	m, _, _ := newManager(t)
	a, _ := m.Create("Anna")
	b, _ := m.Create("Ben")
	if err := m.Open(a.ID); err != nil {
		t.Fatal(err)
	}
	first := m.eng
	if err := m.Open(b.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case <-first.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("the first engine is still running")
	}
	if m.Active() != b.ID {
		t.Fatalf("active = %q", m.Active())
	}
}

func TestOnlyProfilesOnTheListCanBeOpenedOrRemoved(t *testing.T) {
	m, h, base := newManager(t)
	victim := filepath.Join(filepath.Dir(base), "victim")
	os.MkdirAll(victim, 0o700)
	os.WriteFile(filepath.Join(victim, "keep"), []byte("x"), 0o600)
	for _, id := range []string{"../victim", "..", "", "0000000000000000", legacyID} {
		if code, _ := call(t, h, "POST", "/shell/remove", `{"id":"`+id+`"}`); code != http.StatusBadRequest {
			t.Fatalf("remove %q: %d", id, code)
		}
		if code, _ := call(t, h, "POST", "/shell/open", `{"id":"`+id+`"}`); code != http.StatusBadRequest {
			t.Fatalf("open %q: %d", id, code)
		}
	}
	if _, err := os.Stat(filepath.Join(victim, "keep")); err != nil {
		t.Fatal("something outside the profiles was touched")
	}
	if m.Active() != "" {
		t.Fatal("an engine was started")
	}
}

func TestRemovingAProfileDeletesItsFolderButNotWhileItIsOpen(t *testing.T) {
	m, h, base := newManager(t)
	a, _ := m.Create("Anna")
	b, _ := m.Create("Ben")
	if err := m.Open(a.ID); err != nil {
		t.Fatal(err)
	}
	apiWhenReady(t, h, "GET", "/api/keystore/status", "", 200)
	call(t, h, "POST", "/api/keystore/setup", `{"mode":"passphrase","passphrase":"anna has a long passphrase"}`)
	if err := m.Remove(a.ID); err == nil {
		t.Fatal("the open profile was removed")
	}
	m.Close()
	if err := m.Remove(a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(base, profileDir, a.ID)); err == nil {
		t.Fatal("the folder is still there")
	}
	list, _ := m.List()
	if len(list) != 1 || list[0].ID != b.ID {
		t.Fatalf("list = %+v", list)
	}
}

func TestOldDataIsOfferedAsAProfileThatCannotBeRemovedHere(t *testing.T) {
	m, _, base := newManager(t)
	os.MkdirAll(base, 0o700)
	os.WriteFile(filepath.Join(base, "socrates.db"), []byte("x"), 0o600)
	m.Create("Anna")
	list, _ := m.List()
	if len(list) != 2 || !list[0].Legacy || list[0].ID != legacyID || list[0].Name != "Existing data" {
		t.Fatalf("list = %+v", list)
	}
	if err := m.Remove(legacyID); err == nil {
		t.Fatal("the old data was removable from the list")
	}
}
