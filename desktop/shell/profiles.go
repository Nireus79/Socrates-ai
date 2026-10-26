package shell

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

// Profiles are the people who use one computer. Each has a name and a folder of its own with its own database, provider key and settings,
// protected by its own passphrase (the engine is started with --passphrase-only), so that one person cannot read another's conversations
// without the passphrase. The manager starts the engine of the open profile and stops it when the person logs out. There is no server:
// the list lives in a small file next to the profiles.
//
// Data made before profiles existed (a database directly in the base folder) is offered as one profile, "Existing data", opened as it
// always was, with the key where it was kept then.

const (
	legacyID   = "existing"
	maxName    = 40
	registry   = "profiles.json"
	profileDir = "profiles"
)

var idPattern = regexp.MustCompile(`^[0-9a-f]{16}$`)

// Profile is one entry of the list shown to the person.
type Profile struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Legacy bool   `json:"legacy,omitempty"`
}

// Manager keeps the list and the running engine of the open profile.
type Manager struct {
	base       string
	enginePath string
	wait       time.Duration

	mu     sync.Mutex
	eng    *Engine
	active string
	proxy  http.Handler
}

// NewManager makes a manager for the profiles under base.
func NewManager(base, enginePath string) *Manager {
	return &Manager{base: base, enginePath: enginePath, wait: 30 * time.Second}
}

type file struct {
	Profiles []Profile `json:"profiles"`
}

func (m *Manager) load() ([]Profile, error) {
	b, err := os.ReadFile(filepath.Join(m.base, registry))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var f file
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, errors.New("The list of profiles is damaged.")
	}
	out := f.Profiles[:0]
	for _, p := range f.Profiles {
		if idPattern.MatchString(p.ID) && validName(p.Name) == nil {
			out = append(out, Profile{ID: p.ID, Name: p.Name})
		}
	}
	return out, nil
}

func (m *Manager) save(list []Profile) error {
	if err := os.MkdirAll(m.base, 0o700); err != nil {
		return err
	}
	b, _ := json.Marshal(file{Profiles: list})
	tmp, err := os.CreateTemp(m.base, registry+".tmp*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(m.base, registry))
}

func validName(name string) error {
	if strings.TrimSpace(name) != name || name == "" {
		return errors.New("Give the profile a name.")
	}
	if utf8.RuneCountInString(name) > maxName {
		return errors.New("The name is too long.")
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return errors.New("The name has characters that cannot be used.")
		}
	}
	return nil
}

// List is the profiles in the order they were made, the old data first.
func (m *Manager) List() ([]Profile, error) {
	list, err := m.load()
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(filepath.Join(m.base, "socrates.db")); err == nil {
		list = append([]Profile{{ID: legacyID, Name: "Existing data", Legacy: true}}, list...)
	}
	return list, nil
}

// dir is the folder of a profile, or an error for an id that is not on the list. An id is never used in a path before it has been matched
// against the list.
func (m *Manager) dir(id string) (string, bool, error) {
	list, err := m.List()
	if err != nil {
		return "", false, err
	}
	for _, p := range list {
		if p.ID == id {
			if p.Legacy {
				return m.base, true, nil
			}
			return filepath.Join(m.base, profileDir, p.ID), false, nil
		}
	}
	return "", false, errors.New("There is no such profile.")
}

// Create adds a profile with a name nobody else here has (ignoring case). Nothing is protected yet: the passphrase is chosen when the
// profile is first opened.
func (m *Manager) Create(name string) (Profile, error) {
	name = strings.TrimSpace(name)
	if err := validName(name); err != nil {
		return Profile{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	list, err := m.List()
	if err != nil {
		return Profile{}, err
	}
	for _, p := range list {
		if strings.EqualFold(p.Name, name) {
			return Profile{}, errors.New("A profile with that name already exists.")
		}
	}
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return Profile{}, err
	}
	p := Profile{ID: hex.EncodeToString(raw[:]), Name: name}
	stored, err := m.load()
	if err != nil {
		return Profile{}, err
	}
	if err := m.save(append(stored, p)); err != nil {
		return Profile{}, err
	}
	return p, nil
}

// Open starts the engine of a profile, after stopping the one that was open.
func (m *Manager) Open(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	dir, legacy, err := m.dir(id)
	if err != nil {
		return err
	}
	m.stopLocked()
	args := []string{"--data-dir", dir}
	if !legacy {
		args = append(args, "--passphrase-only")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return errors.New("The profile's folder could not be made.")
	}
	eng, err := Start(m.enginePath, args, nil, m.wait)
	if err != nil {
		return err
	}
	m.eng, m.active, m.proxy = eng, id, eng.Proxy(nil)
	return nil
}

// Close stops the engine of the open profile (logging out).
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopLocked()
}

func (m *Manager) stopLocked() {
	if m.eng != nil {
		m.eng.Stop()
	}
	m.eng, m.active, m.proxy = nil, "", nil
}

// Remove deletes a profile and everything in it. The old data cannot be removed here (Settings deletes it), and neither can the profile
// that is open.
func (m *Manager) Remove(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if id == m.active && id != "" {
		return errors.New("Log out of the profile first.")
	}
	dir, legacy, err := m.dir(id)
	if err != nil {
		return err
	}
	if legacy {
		return errors.New("The existing data is removed from Settings, inside it.")
	}
	stored, err := m.load()
	if err != nil {
		return err
	}
	kept := stored[:0]
	for _, p := range stored {
		if p.ID != id {
			kept = append(kept, p)
		}
	}
	if err := m.save(kept); err != nil {
		return err
	}
	return os.RemoveAll(dir)
}

// Stop ends the engine when the window closes.
func (m *Manager) Stop() { m.Close() }

// Active is the id of the open profile, or "".
func (m *Manager) Active() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.active
}

// Handler serves what the page may ask of the window: /shell/ for the profiles, and /api/ for the engine of the open profile. Anything else
// is refused. With no profile open the engine's paths answer that none is.
func (m *Manager) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /shell/profiles", m.handleList)
	mux.HandleFunc("POST /shell/profiles", m.handleCreate)
	mux.HandleFunc("POST /shell/open", m.handleOpen)
	mux.HandleFunc("POST /shell/close", func(w http.ResponseWriter, r *http.Request) { m.Close(); writeJSON(w, 200, map[string]string{}) })
	mux.HandleFunc("POST /shell/remove", m.handleRemove)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		h := m.proxy
		m.mu.Unlock()
		if h == nil {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "No profile is open.", "kind": "no_profile"})
			return
		}
		h.ServeHTTP(w, r)
	})
	return mux
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(v); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "That did not work."})
		return false
	}
	return true
}

func (m *Manager) handleList(w http.ResponseWriter, r *http.Request) {
	list, err := m.List()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "The list of profiles could not be read."})
		return
	}
	if list == nil {
		list = []Profile{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"profiles": list, "active": m.Active()})
}

func (m *Manager) handleCreate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	p, err := m.Create(body.Name)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (m *Manager) handleOpen(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID string `json:"id"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if err := m.Open(body.ID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{})
}

func (m *Manager) handleRemove(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID string `json:"id"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if err := m.Remove(body.ID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{})
}
