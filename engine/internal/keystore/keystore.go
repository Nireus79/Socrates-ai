// Package keystore finds, creates and protects the key of Socrates's encrypted database.
//
// Order of preference, as in other desktop software: the operating system's keychain (Secret Service on Linux, Keychain on macOS,
// Credential Manager on Windows). Where there is none (a minimal Linux desktop, a server), the person chooses, once and
// visibly: a passphrase that seals the key (asked at every start), or, explicitly less secure, a key file. Nothing is ever
// downgraded silently, and a key is never replaced by a new one while a database exists (that would lock the data away).
// What was chosen is written to keystore.json, so a later change of desktop or keychain gives a clear message instead of a
// database that cannot be opened.
package keystore

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/zalando/go-keyring"

	"socrates/internal/secrets"
)

const (
	keychainService = "socrates"
	metaFile        = "keystore.json"
	keyFile         = "db.key"
	envKey          = "SOCRATES_DB_KEY"
	keyBytes        = 32
)

// Backend is where the key lives.
type Backend string

const (
	BackendKeychain   Backend = "keychain"
	BackendPassphrase Backend = "passphrase"
	BackendFile       Backend = "file"
	BackendEnv        Backend = "env"
)

// State is what the person (or the program) has to do next.
type State string

const (
	StateReady               State = "ready"
	StateLocked              State = "locked"               // the key is sealed with a passphrase: ask for it
	StateNeedsSetup          State = "needs_setup"          // a fresh install without a keychain: the person must choose
	StateKeychainUnavailable State = "keychain_unavailable" // the key is in the keychain, which cannot be reached now
	StateUnknownKey          State = "unknown_key"          // a database exists and its key cannot be found
	StateBroken              State = "broken"               // the keystore file is damaged
)

var (
	ErrWrongPassphrase = secrets.ErrWrongPassphrase
	ErrShortPassphrase = secrets.ErrShortPass
	ErrNotNeeded       = errors.New("no choice is needed in the current state")
	ErrNeedsConfirm    = errors.New("the key file mode must be confirmed: it is less secure than a passphrase or the keychain")
	ErrKeyNotFound     = errors.New("not found in the keychain")
)

// Keyring is the operating system's keychain.
type Keyring interface {
	Get(service, user string) (string, error) // ErrKeyNotFound when there is no such entry; any other error: unavailable
	Set(service, user, value string) error
	Delete(service, user string) error
}

// SystemKeyring is the real keychain.
type SystemKeyring struct{}

func (SystemKeyring) Get(service, user string) (string, error) {
	v, err := keyring.Get(service, user)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrKeyNotFound
	}
	return v, err
}
func (SystemKeyring) Set(service, user, value string) error { return keyring.Set(service, user, value) }
func (SystemKeyring) Delete(service, user string) error     { return keyring.Delete(service, user) }

type meta struct {
	Version   int     `json:"version"`
	Backend   Backend `json:"backend"`
	Service   string  `json:"service,omitempty"`
	User      string  `json:"user,omitempty"`
	SealedKey string  `json:"sealedKey,omitempty"`
	CreatedAt int64   `json:"createdAt"`
}

// Status is safe to show: it never contains the key.
type Status struct {
	State             State    `json:"state"`
	Backend           Backend  `json:"backend,omitempty"`
	Reason            string   `json:"reason,omitempty"`
	KeychainAvailable bool     `json:"keychainAvailable"`
	Options           []string `json:"options,omitempty"` // for needs_setup: what the person may choose
}

type Store struct {
	mu       sync.Mutex
	dir      string
	dbPath   string
	kr       Keyring
	getenv   func(string) string
	sleep    func(time.Duration)
	state    State
	backend  Backend
	reason   string
	key      []byte
	kcAvail  bool
	failed   int
	passOnly bool // profiles of the desktop app: only a passphrase may protect the key
	ready    chan struct{}
	once     sync.Once
}

func newStore(dir, dbPath string, kr Keyring) *Store {
	return &Store{dir: dir, dbPath: dbPath, kr: kr, getenv: os.Getenv, sleep: time.Sleep, ready: make(chan struct{})}
}

// Open resolves the key for the database at dbPath, creating a key in the keychain for a fresh install when one is there.
func Open(dir, dbPath string, kr Keyring) *Store {
	s := newStore(dir, dbPath, kr)
	s.resolve(true)
	return s
}

// noKeyring is a keychain that is never there: the key of a profile is never stored outside its own folder.
type noKeyring struct{}

func (noKeyring) Get(string, string) (string, error) { return "", ErrKeyNotFound }
func (noKeyring) Set(string, string, string) error   { return errors.New("not used by a profile") }
func (noKeyring) Delete(string, string) error        { return nil }

// OpenPassphraseOnly is Open for a profile of the desktop app: the system keychain and the key file are not offered, so the only way to
// protect the data is a passphrase, asked at every start, and a profile does not depend on who is logged in to the computer.
func OpenPassphraseOnly(dir, dbPath string) *Store {
	s := newStore(dir, dbPath, noKeyring{})
	s.passOnly = true
	s.resolve(true)
	return s
}

// Inspect is Open without any change: nothing is created, written or stored (for diagnostics).
func Inspect(dir, dbPath string, kr Keyring) *Store {
	s := newStore(dir, dbPath, kr)
	s.resolve(false)
	return s
}

func legacyUser(dbPath string) string { return "database-key:" + dbPath }

func decodeKey(s string) ([]byte, error) {
	key, err := hex.DecodeString(strings.TrimSpace(s))
	if err != nil || len(key) != keyBytes {
		return nil, fmt.Errorf("the database key must be %d hex characters", keyBytes*2)
	}
	return key, nil
}

func newKey() ([]byte, error) {
	k := make([]byte, keyBytes)
	_, err := rand.Read(k)
	return k, err
}

func (s *Store) probeKeychain() error {
	const user = "probe"
	if err := s.kr.Set(keychainService, user, "x"); err != nil {
		return err
	}
	defer s.kr.Delete(keychainService, user)
	v, err := s.kr.Get(keychainService, user)
	if err != nil || v != "x" {
		return errors.New("the keychain accepted a value but did not give it back")
	}
	return nil
}

func (s *Store) set(state State, backend Backend, reason string) {
	s.state, s.backend, s.reason = state, backend, reason
	if state == StateReady {
		s.once.Do(func() { close(s.ready) })
	}
}

// resolve decides the state. Must be called with s.mu held, or before the store is shared.
func (s *Store) resolve(create bool) {
	s.key = nil
	// The key comes from the environment: for a shell that holds the key itself (the phone app) and for development. Anything that can set
	// the environment of the engine can set this, so it is not offered to a person.
	// A profile of the desktop app ignores it: only the passphrase opens a profile.
	if v := s.getenv(envKey); strings.TrimSpace(v) != "" && !s.passOnly {
		k, err := decodeKey(v)
		if err != nil {
			s.set(StateBroken, BackendEnv, envKey+": "+err.Error())
			return
		}
		s.key = k
		s.set(StateReady, BackendEnv, "")
		return
	}
	m, err := readMeta(s.dir)
	if err != nil {
		s.set(StateBroken, "", "The key settings file is damaged ("+err.Error()+"). Restore it from a backup, or move it aside only if you accept to lose the data.")
		return
	}
	if m != nil {
		s.resolveFromMeta(m)
		return
	}
	s.resolveWithoutMeta(create)
}

func (s *Store) resolveFromMeta(m *meta) {
	switch m.Backend {
	case BackendKeychain:
		v, err := s.kr.Get(m.Service, m.User)
		switch {
		case err == nil:
			k, derr := decodeKey(v)
			if derr != nil {
				s.set(StateBroken, BackendKeychain, "The key in the keychain is damaged: "+derr.Error())
				return
			}
			s.key = k
			s.set(StateReady, BackendKeychain, "")
		case errors.Is(err, ErrKeyNotFound):
			s.set(StateUnknownKey, BackendKeychain, "The database key was kept in the system keychain, but it is no longer there (the keychain was reset, or this is another user or computer). The data cannot be opened without it. Restore the keychain, or restore a backup.")
		default:
			s.set(StateKeychainUnavailable, BackendKeychain, "The database key is kept in the system keychain, which cannot be reached now ("+err.Error()+"). Log in to your desktop session or unlock the keychain, then try again.")
		}
	case BackendPassphrase:
		if !secrets.IsSealed(m.SealedKey) {
			s.set(StateBroken, BackendPassphrase, "The sealed key is damaged.")
			return
		}
		s.set(StateLocked, BackendPassphrase, "")
	case BackendFile:
		data, err := os.ReadFile(filepath.Join(s.dir, keyFile))
		if err != nil {
			s.set(StateUnknownKey, BackendFile, "The key file is missing ("+err.Error()+"). The data cannot be opened without it.")
			return
		}
		k, derr := decodeKey(string(data))
		if derr != nil {
			s.set(StateBroken, BackendFile, "The key file is damaged: "+derr.Error())
			return
		}
		s.key = k
		s.set(StateReady, BackendFile, "")
	default:
		s.set(StateBroken, "", "The key settings name an unknown storage ("+string(m.Backend)+"): this data was made by a newer Socrates.")
	}
}

func (s *Store) resolveWithoutMeta(create bool) {
	dbExists := fileExists(s.dbPath)
	// The key settings file is gone but the key is still in the keychain under this database's name: recover it.
	if v, err := s.kr.Get(keychainService, legacyUser(s.dbPath)); err == nil {
		if k, derr := decodeKey(v); derr == nil {
			s.key = k
			s.set(StateReady, BackendKeychain, "")
			if create {
				writeMeta(s.dir, &meta{Version: 1, Backend: BackendKeychain, Service: keychainService, User: legacyUser(s.dbPath), CreatedAt: time.Now().Unix()})
			}
			return
		}
	} else if !errors.Is(err, ErrKeyNotFound) && dbExists {
		s.set(StateKeychainUnavailable, BackendKeychain, "The database key may be in the system keychain, which cannot be reached now ("+err.Error()+"). Log in to your desktop session or unlock the keychain, then try again.")
		return
	}
	if dbExists {
		s.set(StateUnknownKey, "", "A database exists but its key cannot be found. A new key would lock the existing data away, so none is made. Restore the keychain or the key file, or restore a backup.")
		return
	}
	// A fresh install.
	if err := s.probeKeychain(); err == nil {
		s.kcAvail = true
		if !create {
			s.set(StateNeedsSetup, "", "") // Inspect: would use the keychain
			return
		}
		if s.useKeychain() {
			return
		}
	} else {
		s.kcAvail = false
		s.set(StateNeedsSetup, "", "No system keychain is available ("+err.Error()+").")
		return
	}
	s.set(StateNeedsSetup, "", "No system keychain is available.")
}

func (s *Store) useKeychain() bool {
	k, err := newKey()
	if err != nil {
		return false
	}
	user := legacyUser(s.dbPath)
	if err := s.kr.Set(keychainService, user, hex.EncodeToString(k)); err != nil {
		return false
	}
	if err := writeMeta(s.dir, &meta{Version: 1, Backend: BackendKeychain, Service: keychainService, User: user, CreatedAt: time.Now().Unix()}); err != nil {
		s.kr.Delete(keychainService, user)
		return false
	}
	s.key = k
	s.set(StateReady, BackendKeychain, "")
	return true
}

// Status says what is needed. It never contains the key.
func (s *Store) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := Status{State: s.state, Backend: s.backend, Reason: s.reason, KeychainAvailable: s.kcAvail}
	if s.state == StateNeedsSetup {
		st.Options = []string{"passphrase", "file"}
		if s.passOnly {
			st.Options = []string{"passphrase"}
		} else if s.kcAvail {
			st.Options = append([]string{"keychain"}, st.Options...)
		}
	}
	return st
}

// Key returns the database key once the state is ready.
func (s *Store) Key() ([]byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state != StateReady {
		return nil, false
	}
	return append([]byte(nil), s.key...), true
}

// Ready is closed when the key is available.
func (s *Store) Ready() <-chan struct{} { return s.ready }

// Setup records the person's choice on a fresh install without a keychain. mode: "passphrase", "file" (needs confirmInsecure) or
// "keychain" (when one has appeared since).
func (s *Store) Setup(mode, passphrase string, confirmInsecure bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state != StateNeedsSetup {
		return ErrNotNeeded
	}
	if s.passOnly && mode != "passphrase" {
		return errors.New("this profile can only be protected by a passphrase")
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}
	switch mode {
	case "passphrase":
		k, err := newKey()
		if err != nil {
			return err
		}
		sealed, err := secrets.Seal(passphrase, hex.EncodeToString(k))
		if err != nil {
			return err
		}
		if err := writeMeta(s.dir, &meta{Version: 1, Backend: BackendPassphrase, SealedKey: sealed, CreatedAt: time.Now().Unix()}); err != nil {
			return err
		}
		s.key = k
		s.set(StateReady, BackendPassphrase, "")
		return nil
	case "file":
		if !confirmInsecure {
			return ErrNeedsConfirm
		}
		k, err := newKey()
		if err != nil {
			return err
		}
		if err := writeFileAtomic(filepath.Join(s.dir, keyFile), []byte(hex.EncodeToString(k)+"\n")); err != nil {
			return err
		}
		if err := writeMeta(s.dir, &meta{Version: 1, Backend: BackendFile, CreatedAt: time.Now().Unix()}); err != nil {
			os.Remove(filepath.Join(s.dir, keyFile))
			return err
		}
		s.key = k
		s.set(StateReady, BackendFile, "")
		return nil
	case "keychain":
		if err := s.probeKeychain(); err != nil {
			return fmt.Errorf("the system keychain is still not available: %w", err)
		}
		s.kcAvail = true
		if !s.useKeychain() {
			return errors.New("the key could not be stored in the keychain")
		}
		return nil
	}
	return fmt.Errorf("unknown mode %q", mode)
}

// Unlock opens a passphrase-sealed key. Wrong guesses are slowed down; the wait happens outside the lock, so the status of the
// store can still be read meanwhile.
func (s *Store) Unlock(passphrase string) error {
	s.mu.Lock()
	if s.state != StateLocked {
		s.mu.Unlock()
		return ErrNotNeeded
	}
	m, err := readMeta(s.dir)
	if err != nil || m == nil || m.Backend != BackendPassphrase {
		s.mu.Unlock()
		return errors.New("the key settings changed; restart Socrates")
	}
	plain, err := secrets.Open(passphrase, m.SealedKey)
	if err != nil {
		s.failed++
		n := s.failed
		if n > 5 {
			n = 5
		}
		s.mu.Unlock()
		s.sleep(time.Duration(n) * time.Second)
		return ErrWrongPassphrase
	}
	defer s.mu.Unlock()
	k, err := decodeKey(plain)
	if err != nil {
		return err
	}
	s.failed = 0
	s.key = k
	s.set(StateReady, BackendPassphrase, "")
	return nil
}

// Recheck looks again, after the person fixed something (started the keychain, restored a file).
func (s *Store) Recheck() Status {
	s.mu.Lock()
	if s.state != StateReady && s.state != StateLocked {
		s.resolve(true)
	}
	s.mu.Unlock()
	return s.Status()
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

func readMeta(dir string) (*meta, error) {
	data, err := os.ReadFile(filepath.Join(dir, metaFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var m meta
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func writeMeta(dir string, m *meta) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(m, "", "  ")
	return writeFileAtomic(filepath.Join(dir, metaFile), append(data, '\n'))
}

// writeFileAtomic writes a private file (mode 0600) in one step and makes it durable: the data is flushed before the rename and the
// directory after it, so a crash or a power cut leaves either the old file or the new one, never an empty key.
func writeFileAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	if d, err := os.Open(filepath.Dir(path)); err == nil { // a directory cannot be synced on every platform; the error is not fatal
		d.Sync()
		d.Close()
	}
	return nil
}
