package keystore

import (
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

type fakeKeyring struct {
	data map[string]string
	down error // when set, every call fails with it (keychain not reachable)
}

func newFake() *fakeKeyring { return &fakeKeyring{data: map[string]string{}} }

func (f *fakeKeyring) Get(s, u string) (string, error) {
	if f.down != nil {
		return "", f.down
	}
	v, ok := f.data[s+"/"+u]
	if !ok {
		return "", ErrKeyNotFound
	}
	return v, nil
}
func (f *fakeKeyring) Set(s, u, v string) error {
	if f.down != nil {
		return f.down
	}
	f.data[s+"/"+u] = v
	return nil
}
func (f *fakeKeyring) Delete(s, u string) error { delete(f.data, s+"/"+u); return nil }

func setup(t *testing.T) (dir, db string) {
	t.Helper()
	dir = t.TempDir()
	return dir, filepath.Join(dir, "socrates.db")
}

func noSleep(s *Store) *Store { s.sleep = func(time.Duration) {}; return s }

func TestFreshInstallWithKeychainUsesItAndRemembersThat(t *testing.T) {
	dir, db := setup(t)
	kr := newFake()
	s := Open(dir, db, kr)
	if st := s.Status(); st.State != StateReady || st.Backend != BackendKeychain {
		t.Fatalf("status = %+v", st)
	}
	k, ok := s.Key()
	if !ok || len(k) != 32 {
		t.Fatal("no key")
	}
	// A second start gets the same key from the keychain.
	k2, _ := Open(dir, db, kr).Key()
	if hex.EncodeToString(k) != hex.EncodeToString(k2) {
		t.Fatal("the key changed between starts")
	}
	if _, err := os.Stat(filepath.Join(dir, metaFile)); err != nil {
		t.Fatal("the choice was not recorded")
	}
}

func TestNoKeychainOnFreshInstallAsksAndNeverPicksSilently(t *testing.T) {
	dir, db := setup(t)
	kr := newFake()
	kr.down = errors.New("no secret service")
	s := Open(dir, db, kr)
	st := s.Status()
	if st.State != StateNeedsSetup || st.KeychainAvailable {
		t.Fatalf("status = %+v", st)
	}
	if strings.Join(st.Options, ",") != "passphrase,file" {
		t.Fatalf("options = %v", st.Options)
	}
	if _, ok := s.Key(); ok {
		t.Fatal("a key was given before the person chose")
	}
	if _, err := os.Stat(filepath.Join(dir, keyFile)); err == nil {
		t.Fatal("a key file was created silently")
	}
}

func TestPassphraseModeSealsTheKeyAndLocksAtNextStart(t *testing.T) {
	dir, db := setup(t)
	kr := newFake()
	kr.down = errors.New("no secret service")
	s := Open(dir, db, kr)
	if err := s.Setup("passphrase", "short", false); err == nil {
		t.Fatal("a short passphrase was accepted")
	}
	if err := s.Setup("passphrase", "a long enough passphrase", false); err != nil {
		t.Fatal(err)
	}
	k1, ok := s.Key()
	if !ok {
		t.Fatal("not ready after setup")
	}
	raw, _ := os.ReadFile(filepath.Join(dir, metaFile))
	if strings.Contains(string(raw), hex.EncodeToString(k1)) {
		t.Fatal("the key is stored in the clear")
	}
	// Next start: locked until the right passphrase.
	s2 := noSleep(Open(dir, db, kr))
	if s2.Status().State != StateLocked {
		t.Fatalf("status = %+v", s2.Status())
	}
	if _, ok := s2.Key(); ok {
		t.Fatal("key given while locked")
	}
	if err := s2.Unlock("the wrong passphrase!!"); !errors.Is(err, ErrWrongPassphrase) {
		t.Fatalf("err = %v", err)
	}
	if s2.Status().State != StateLocked {
		t.Fatal("a wrong guess unlocked")
	}
	if err := s2.Unlock("a long enough passphrase"); err != nil {
		t.Fatal(err)
	}
	k2, _ := s2.Key()
	if hex.EncodeToString(k1) != hex.EncodeToString(k2) {
		t.Fatal("unlocking gave another key")
	}
	select {
	case <-s2.Ready():
	default:
		t.Fatal("Ready not closed")
	}
}

func TestWrongGuessesAreSlowedDown(t *testing.T) {
	dir, db := setup(t)
	kr := newFake()
	kr.down = errors.New("none")
	s := Open(dir, db, kr)
	s.Setup("passphrase", "a long enough passphrase", false)
	s2 := Open(dir, db, kr)
	var waited time.Duration
	s2.sleep = func(d time.Duration) { waited += d }
	for i := 0; i < 3; i++ {
		s2.Unlock("nope nope nope nope")
	}
	if waited < 6*time.Second { // 1 + 2 + 3
		t.Fatalf("waited only %v", waited)
	}
}

func TestFileModeNeedsExplicitConfirmation(t *testing.T) {
	dir, db := setup(t)
	kr := newFake()
	kr.down = errors.New("none")
	s := Open(dir, db, kr)
	if err := s.Setup("file", "", false); !errors.Is(err, ErrNeedsConfirm) {
		t.Fatalf("err = %v", err)
	}
	if err := s.Setup("file", "", true); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, keyFile))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("key file: %v %v", info, err)
	}
	k1, _ := s.Key()
	k2, ok := Open(dir, db, kr).Key()
	if !ok || hex.EncodeToString(k1) != hex.EncodeToString(k2) {
		t.Fatal("file mode did not give the same key on restart")
	}
}

func TestKeychainDisappearingLaterIsReportedNotReplaced(t *testing.T) {
	dir, db := setup(t)
	kr := newFake()
	Open(dir, db, kr)
	os.WriteFile(db, []byte("pretend database"), 0o600)
	kr.down = errors.New("dbus: no session")
	s := Open(dir, db, kr)
	st := s.Status()
	if st.State != StateKeychainUnavailable || !strings.Contains(st.Reason, "keychain") {
		t.Fatalf("status = %+v", st)
	}
	if _, ok := s.Key(); ok {
		t.Fatal("key given")
	}
	// Keychain returns: Recheck recovers without a restart.
	kr.down = nil
	if st := s.Recheck(); st.State != StateReady {
		t.Fatalf("after recheck: %+v", st)
	}
}

func TestEntryLostFromKeychainNeverMakesANewKey(t *testing.T) {
	dir, db := setup(t)
	kr := newFake()
	Open(dir, db, kr)
	os.WriteFile(db, []byte("pretend database"), 0o600)
	kr.data = map[string]string{} // the keychain was reset
	s := Open(dir, db, kr)
	if s.Status().State != StateUnknownKey {
		t.Fatalf("status = %+v", s.Status())
	}
	if len(kr.data) != 0 {
		t.Fatal("a new key was written over the lost one")
	}
}

func TestExistingDatabaseWithoutAnyKeyInfoIsNeverGivenANewKey(t *testing.T) {
	dir, db := setup(t)
	os.WriteFile(db, []byte("pretend database"), 0o600)
	kr := newFake()
	s := Open(dir, db, kr)
	if s.Status().State != StateUnknownKey {
		t.Fatalf("status = %+v", s.Status())
	}
	if len(kr.data) != 0 {
		t.Fatal("a key was created for an existing database")
	}
}

func TestInstallationFromBeforeKeystoreFileIsAdopted(t *testing.T) {
	dir, db := setup(t)
	kr := newFake()
	key := strings.Repeat("ab", 32)
	kr.data[keychainService+"/"+legacyUser(db)] = key
	os.WriteFile(db, []byte("pretend database"), 0o600)
	s := Open(dir, db, kr)
	k, ok := s.Key()
	if !ok || hex.EncodeToString(k) != key {
		t.Fatalf("status = %+v", s.Status())
	}
	if _, err := os.Stat(filepath.Join(dir, metaFile)); err != nil {
		t.Fatal("the adopted choice was not recorded")
	}
}

func TestEnvironmentKeyOverridesAndIsValidated(t *testing.T) {
	dir, db := setup(t)
	s := newStore(dir, db, newFake())
	s.getenv = func(string) string { return strings.Repeat("cd", 32) }
	s.resolve(true)
	if s.Status().State != StateReady || s.Status().Backend != BackendEnv {
		t.Fatalf("status = %+v", s.Status())
	}
	s2 := newStore(dir, db, newFake())
	s2.getenv = func(string) string { return "not hex" }
	s2.resolve(true)
	if s2.Status().State != StateBroken {
		t.Fatalf("status = %+v", s2.Status())
	}
}

func TestDamagedSettingsFileIsReportedAndNothingIsOverwritten(t *testing.T) {
	dir, db := setup(t)
	os.WriteFile(filepath.Join(dir, metaFile), []byte("{broken"), 0o600)
	kr := newFake()
	s := Open(dir, db, kr)
	if s.Status().State != StateBroken {
		t.Fatalf("status = %+v", s.Status())
	}
	if len(kr.data) != 0 {
		t.Fatal("a key was created")
	}
}

func TestStatusNeverContainsTheKey(t *testing.T) {
	dir, db := setup(t)
	s := Open(dir, db, newFake())
	k, _ := s.Key()
	if strings.Contains(strings.ToLower(s.Status().Reason), hex.EncodeToString(k)) {
		t.Fatal("key in status")
	}
}

func TestSetupOnlyWhenNeeded(t *testing.T) {
	dir, db := setup(t)
	s := Open(dir, db, newFake())
	if err := s.Setup("file", "", true); !errors.Is(err, ErrNotNeeded) {
		t.Fatalf("err = %v", err)
	}
}

func TestKeychainAppearingAfterNeedsSetupCanBeChosen(t *testing.T) {
	dir, db := setup(t)
	kr := newFake()
	kr.down = errors.New("none yet")
	s := Open(dir, db, kr)
	kr.down = nil
	if err := s.Setup("keychain", "", false); err != nil {
		t.Fatal(err)
	}
	if st := s.Status(); st.State != StateReady || st.Backend != BackendKeychain {
		t.Fatalf("status = %+v", st)
	}
}

func TestStatusCanBeReadWhileAWrongGuessIsBeingSlowedDown(t *testing.T) {
	dir, db := setup(t)
	s := Open(dir, db, &fakeKeyring{data: map[string]string{}, down: errors.New("no keychain")})
	if err := s.Setup("passphrase", "a long enough passphrase", false); err != nil {
		t.Fatal(err)
	}
	s2 := Open(dir, db, &fakeKeyring{data: map[string]string{}, down: errors.New("no keychain")})
	statusDuringWait := make(chan State, 1)
	s2.sleep = func(time.Duration) {
		// The wait runs outside the lock: Status must not block here (it would deadlock this test if it did).
		statusDuringWait <- s2.Status().State
	}
	if err := s2.Unlock("the wrong passphrase!!"); !errors.Is(err, ErrWrongPassphrase) {
		t.Fatalf("wrong guess: %v", err)
	}
	select {
	case st := <-statusDuringWait:
		if st != StateLocked {
			t.Fatalf("state during the wait = %s", st)
		}
	default:
		t.Fatal("the wait never ran")
	}
}

func TestPrivateFilesAreWrittenWithMode0600AndLeaveNoTemporaryFile(t *testing.T) {
	dir, db := setup(t)
	s := Open(dir, db, &fakeKeyring{data: map[string]string{}, down: errors.New("no keychain")})
	if err := s.Setup("file", "", true); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{keyFile, metaFile} {
		st, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if runtime.GOOS != "windows" && st.Mode().Perm() != 0o600 {
			t.Fatalf("%s has mode %v", name, st.Mode().Perm())
		}
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Fatalf("a temporary file was left behind: %s", e.Name())
		}
	}
}

func TestAShortPassphraseIsRefusedAndNothingIsWritten(t *testing.T) {
	dir, db := setup(t)
	s := Open(dir, db, &fakeKeyring{data: map[string]string{}, down: errors.New("no keychain")})
	if err := s.Setup("passphrase", "only11chars", false); !errors.Is(err, ErrShortPassphrase) {
		t.Fatalf("an 11-character passphrase: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, metaFile)); err == nil {
		t.Fatal("the key settings were written for a refused passphrase")
	}
	if st := s.Status(); st.State != StateNeedsSetup {
		t.Fatalf("state = %s", st.State)
	}
}

// A profile of the desktop app is protected by a passphrase and by nothing else: the keychain is not used even when it works, and neither
// the keychain nor the key file can be chosen.
func TestPassphraseOnlyOffersOnlyAPassphraseAndNeverTouchesTheKeychain(t *testing.T) {
	dir, db := setup(t)
	s := OpenPassphraseOnly(dir, db)
	st := s.Status()
	if st.State != StateNeedsSetup || len(st.Options) != 1 || st.Options[0] != "passphrase" {
		t.Fatalf("status = %+v", st)
	}
	for _, mode := range []string{"file", "keychain"} {
		if err := s.Setup(mode, "", true); err == nil {
			t.Fatalf("mode %q was accepted", mode)
		}
		if s.Status().State != StateNeedsSetup {
			t.Fatalf("mode %q changed the state", mode)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, keyFile)); err == nil {
		t.Fatal("a key file was written")
	}
	if err := s.Setup("passphrase", "a long enough passphrase", false); err != nil {
		t.Fatal(err)
	}
	s2 := noSleep(OpenPassphraseOnly(dir, db))
	if s2.Status().State != StateLocked {
		t.Fatalf("status = %+v", s2.Status())
	}
	if err := s2.Unlock("a long enough passphrase"); err != nil {
		t.Fatal(err)
	}
}

func TestPassphraseOnlyIgnoresAKeyInTheEnvironment(t *testing.T) {
	dir, db := setup(t)
	s := newStore(dir, db, noKeyring{})
	s.passOnly = true
	s.getenv = func(string) string { return strings.Repeat("ab", 32) }
	s.resolve(true)
	if s.Status().State != StateNeedsSetup {
		t.Fatalf("a key from the environment opened a profile: %+v", s.Status())
	}
	if _, ok := s.Key(); ok {
		t.Fatal("a key was given")
	}
}
