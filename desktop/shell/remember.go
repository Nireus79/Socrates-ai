package shell

import (
	"os"
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/zalando/go-keyring"
)

// Remembering the login. Two things are kept, both on this computer only:
//   - which profile was opened last (profiles.json: Last), so that the login screen offers it first; no secret in that;
//   - when the person asks to stay signed in (Stay), the window opens that profile by itself the next time. A profile with a passphrase
//     has its passphrase kept in the system's secure storage (Windows Credential Manager, the macOS keychain, the Secret Service on Linux),
//     never in a file of ours, and the shell, not the page, uses it to unlock the engine. Logging out, removing the profile, or a passphrase
//     the engine refuses, forgets it. Closing the window does not log out.
// This is a convenience that trades the passphrase's protection against someone who can use this computer account for not typing it; the
// screen says so, and it is off until the person turns it on. A profile made without a passphrase needs no secret: only the flag is kept.

// ErrNoSecret says that no passphrase is kept for the profile.
var ErrNoSecret = errors.New("no stored passphrase")

var errNotRemembered = errors.New("Sign in again.")

// Secrets is where passphrases are kept.
type Secrets interface {
	Get(id string) (string, error) // ErrNoSecret when there is none
	Set(id, passphrase string) error
	Delete(id string) error
}

const secretService = "socrates-desktop"

type systemSecrets struct{}

func secretUser(id string) string { return "passphrase:" + id }

func (systemSecrets) Get(id string) (string, error) {
	v, err := keyring.Get(secretService, secretUser(id))
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrNoSecret
	}
	return v, err
}
func (systemSecrets) Set(id, passphrase string) error {
	return keyring.Set(secretService, secretUser(id), passphrase)
}
func (systemSecrets) Delete(id string) error {
	err := keyring.Delete(secretService, secretUser(id))
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}

// SetSecrets replaces where passphrases are kept (tests).
func (m *Manager) SetSecrets(s Secrets) { m.secrets = s }

// canKeep says whether the system's secure storage can be used on this computer.
func (m *Manager) canKeep() bool {
	_, err := m.secrets.Get("probe")
	return err == nil || errors.Is(err, ErrNoSecret)
}

// stayable says whether staying signed in can work for the profile now.
func (m *Manager) stayable(p Profile) bool {
	if p.Legacy || p.Open {
		return true
	}
	_, err := m.secrets.Get(p.ID)
	return err == nil
}

// listView is what the login screen is told: the profiles, which one is open, the one used last, whether to open it by itself, and
// whether the system can keep a passphrase at all.
func (m *Manager) listView(list []Profile) map[string]any {
	f, _ := m.readFile()
	view := map[string]any{"profiles": list, "active": m.Active(), "canRemember": m.canKeep()}
	for _, p := range list {
		if p.ID == f.Last {
			view["last"] = f.Last
			view["stay"] = f.Stay && m.stayable(p)
		}
	}
	return view
}

// setLast notes the profile that was opened. A different profile than the one that stayed signed in ends the staying.
func (m *Manager) setLast(id string) {
	f, err := m.readFile()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return
	}
	if f.Last != id {
		if f.Stay {
			_ = m.secrets.Delete(f.Last) // the one who stayed signed in is no longer the one in front of the screen
		}
		f.Last, f.Stay = id, false
	}
	_ = m.write(f)
}

// forgetLocked drops everything that keeps a profile signed in (the caller holds m.mu or does not need it).
func (m *Manager) forgetLocked(id string) {
	_ = m.secrets.Delete(id)
	f, err := m.readFile()
	if err != nil {
		return
	}
	if f.Last == id && f.Stay {
		f.Stay = false
		_ = m.write(f)
	}
}

func (m *Manager) forget(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.forgetLocked(id)
}

func (m *Manager) handleClose(w http.ResponseWriter, r *http.Request) {
	if id := m.Active(); id != "" {
		m.forget(id) // logging out is the way to stop being signed in
	}
	m.Close()
	writeJSON(w, http.StatusOK, map[string]string{})
}

func (m *Manager) handleStay(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID         string `json:"id"`
		Stay       bool   `json:"stay"`
		Passphrase string `json:"passphrase"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	list, err := m.List()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "The list of profiles could not be read."})
		return
	}
	var prof *Profile
	for i := range list {
		if list[i].ID == body.ID {
			prof = &list[i]
		}
	}
	if prof == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "That profile does not exist."})
		return
	}
	if !body.Stay {
		m.forget(prof.ID)
		writeJSON(w, http.StatusOK, map[string]string{})
		return
	}
	if !prof.Legacy && !prof.Open {
		if body.Passphrase == "" || !m.canKeep() {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Staying signed in is not available on this computer."})
			return
		}
		if err := m.secrets.Set(prof.ID, body.Passphrase); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "The passphrase could not be kept on this computer."})
			return
		}
	}
	m.mu.Lock()
	f, _ := m.readFile()
	f.Last, f.Stay = prof.ID, true
	err = m.write(f)
	m.mu.Unlock()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "That could not be saved."})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{})
}

func (m *Manager) handleAutoLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID string `json:"id"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if err := m.AutoLogin(body.ID); err != nil {
		code := http.StatusBadGateway
		if errors.Is(err, errNotRemembered) {
			code = http.StatusUnauthorized
		}
		writeJSON(w, code, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{})
}

// AutoLogin opens the profile that stayed signed in and, if it has a passphrase, unlocks it with the one kept in the secure storage. The
// passphrase never reaches the page. A passphrase the engine refuses is forgotten, so it is not tried again.
func (m *Manager) AutoLogin(id string) error {
	list, err := m.List()
	if err != nil {
		return err
	}
	var prof *Profile
	for i := range list {
		if list[i].ID == id {
			prof = &list[i]
		}
	}
	f, _ := m.readFile()
	if prof == nil || f.Last != id || !f.Stay {
		return errNotRemembered
	}
	pass := ""
	if !prof.Legacy && !prof.Open {
		if pass, err = m.secrets.Get(id); err != nil {
			m.forget(id)
			return errNotRemembered
		}
	}
	if m.Active() != id {
		if err := m.Open(id); err != nil {
			return err
		}
	}
	if pass == "" {
		return nil
	}
	state, err := m.waitForKeyState()
	if err != nil {
		return err
	}
	if state != "locked" {
		return nil
	}
	switch code := m.engineCall(http.MethodPost, "/api/keystore/unlock", map[string]string{"passphrase": pass}); code {
	case http.StatusOK:
		return nil
	case http.StatusForbidden:
		m.forget(id)
		return errNotRemembered
	default:
		return errors.New("The key could not be opened.")
	}
}

// waitForKeyState waits until the engine of the open profile answers about its key (it needs a moment after it starts).
func (m *Manager) waitForKeyState() (string, error) {
	deadline := time.Now().Add(m.wait)
	for {
		rec := m.engineRequest(http.MethodGet, "/api/keystore/status", nil)
		if rec != nil && rec.Code == http.StatusNotFound {
			return "ready", nil // the key gate is gone once the key is open
		}
		if rec != nil && rec.Code == http.StatusOK {
			var st struct {
				State string `json:"state"`
			}
			_ = json.Unmarshal(rec.Body.Bytes(), &st)
			return st.State, nil
		}
		if time.Now().After(deadline) {
			return "", errors.New("The engine did not start.")
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (m *Manager) engineCall(method, path string, body any) int {
	if rec := m.engineRequest(method, path, body); rec != nil {
		return rec.Code
	}
	return 0
}

// engineRequest sends a request to the open profile's engine through the same proxy the page uses.
func (m *Manager) engineRequest(method, path string, body any) *httptest.ResponseRecorder {
	m.mu.Lock()
	h := m.proxy
	m.mu.Unlock()
	if h == nil {
		return nil
	}
	var rd *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, rd)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}
