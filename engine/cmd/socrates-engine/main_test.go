package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"socrates/internal/transport"
)

var (
	buildOnce sync.Once
	binary    string
	buildErr  error
)

func build(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		dir, err := os.MkdirTemp("", "socrates-engine-test")
		if err != nil {
			buildErr = err
			return
		}
		binary = filepath.Join(dir, "socrates-engine")
		out, err := exec.Command("go", "build", "-tags", "testprovider", "-o", binary, ".").CombinedOutput()
		if err != nil {
			buildErr = fmt.Errorf("build failed: %v\n%s", err, out)
		}
	})
	if buildErr != nil {
		t.Fatal(buildErr)
	}
	return binary
}

// safeBuffer is a buffer that may be written by one goroutine while another reads it.
type safeBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *safeBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}
func (s *safeBuffer) String() string { s.mu.Lock(); defer s.mu.Unlock(); return s.b.String() }
func (s *safeBuffer) Bytes() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]byte(nil), s.b.Bytes()...)
}

// engine is a running copy, started the way a shell starts it.
type engine struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	hs     transport.Handshake
	stdout *safeBuffer
	stderr *safeBuffer
	dir    string
	done   chan error
}

func start(t *testing.T, env ...string) *engine {
	t.Helper()
	bin := build(t)
	dir := filepath.Join(t.TempDir(), "data")
	cmd := exec.Command(bin, "--data-dir", dir)
	// The key comes from the environment so that the test never touches the real keychain.
	cmd.Env = append(append(os.Environ(), "SOCRATES_DB_KEY="+strings.Repeat("ab", 32)), env...)
	stdin, _ := cmd.StdinPipe()
	stdoutPipe, _ := cmd.StdoutPipe()
	e := &engine{cmd: cmd, stdin: stdin, stdout: &safeBuffer{}, stderr: &safeBuffer{}, dir: dir, done: make(chan error, 1)}
	cmd.Stderr = e.stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		stdin.Close()
		select {
		case <-e.done:
		case <-time.After(5 * time.Second):
			cmd.Process.Kill()
		}
	})
	r := bufio.NewReader(io.TeeReader(stdoutPipe, e.stdout))
	hs, err := transport.ReadHandshake(r)
	if err != nil {
		t.Fatalf("no handshake: %v\nstderr: %s", err, e.stderr)
	}
	e.hs = hs
	go func() {
		io.Copy(io.Discard, r)
		e.done <- cmd.Wait()
	}()
	return e
}

func (e *engine) url(path string) string {
	return fmt.Sprintf("http://127.0.0.1:%d%s", e.hs.Port, path)
}

func (e *engine) get(t *testing.T, path string, hdr map[string]string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest("GET", e.url(path), nil)
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
	return resp.StatusCode, string(data)
}

func (e *engine) waitReady(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if code, body := e.get(t, "/api/status", nil); code == 200 && strings.Contains(body, "ready") {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("the engine never became ready; stderr: %s", e.stderr)
}

func (e *engine) waitExit(t *testing.T, within time.Duration) error {
	t.Helper()
	select {
	case err := <-e.done:
		e.done <- err // let the cleanup see it too
		return err
	case <-time.After(within):
		t.Fatalf("the engine did not stop; stderr: %s", e.stderr)
		return nil
	}
}

func TestTheEngineHandsTheShellItsPortAndTokenAndNothingElseOnStandardOutput(t *testing.T) {
	e := start(t)
	e.waitReady(t)
	if e.hs.Port == 0 || len(e.hs.Token) != 64 || e.hs.Version == "" {
		t.Fatalf("%+v", e.hs)
	}
	var line map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(e.stdout.Bytes()), &line); err != nil {
		t.Fatalf("standard output is not exactly one JSON line: %q", e.stdout.String())
	}
	if strings.Contains(e.stderr.String(), e.hs.Token) {
		t.Fatal("the token reached the log")
	}
}

func TestTheDoorIsInPlaceAroundTheRealEngine(t *testing.T) {
	e := start(t)
	e.waitReady(t)
	tok := map[string]string{transport.TokenHeader: e.hs.Token}
	if code, _ := e.get(t, "/api/settings/llm", nil); code != 401 {
		t.Fatalf("no token: %d", code)
	}
	if code, body := e.get(t, "/api/settings/llm", tok); code != 200 || !strings.Contains(body, `"hasKey":false`) {
		t.Fatalf("with the token: %d %s", code, body)
	}
	if code, _ := e.get(t, "/api/settings/llm", map[string]string{transport.TokenHeader: e.hs.Token, "Origin": "http://evil.example"}); code != 403 {
		t.Fatalf("a foreign origin: %d", code)
	}
	if code, _ := e.get(t, "/api/settings/llm", map[string]string{transport.TokenHeader: e.hs.Token, "Host": "evil.example"}); code != 403 {
		t.Fatalf("a foreign host: %d", code)
	}
	if code, _ := e.get(t, "/internal/idle", nil); code != 401 {
		t.Fatalf("the idle report without the token: %d", code)
	}
	if code, body := e.get(t, "/internal/idle", tok); code != 200 || !strings.Contains(body, `"idleSeconds"`) {
		t.Fatalf("the idle report with the token: %d %s", code, body)
	}
	if code, _ := e.get(t, "/api/nothing-here", tok); code != 404 {
		t.Fatalf("an unknown route: %d", code)
	}
}

func TestTheDataFolderAndTheDatabaseArePrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("modes do not apply on Windows")
	}
	e := start(t)
	e.waitReady(t)
	for _, p := range []string{e.dir, filepath.Join(e.dir, "socrates.db")} {
		st, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if st.Mode().Perm()&0o077 != 0 {
			t.Fatalf("%s has mode %v", p, st.Mode().Perm())
		}
	}
}

func TestTheEngineStopsWhenTheShellAsksOrWhenTheShellIsGone(t *testing.T) {
	e := start(t)
	e.waitReady(t)
	req, _ := http.NewRequest("POST", e.url("/internal/shutdown"), nil)
	if resp, err := http.DefaultClient.Do(req); err != nil || resp.StatusCode != 401 {
		t.Fatalf("shutdown without the token must be refused: %v", err)
	}
	req.Header.Set(transport.TokenHeader, e.hs.Token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != 204 {
		t.Fatalf("shutdown: %v", err)
	}
	if err := e.waitExit(t, 10*time.Second); err != nil {
		t.Fatalf("exit after a requested shutdown: %v", err)
	}

	e2 := start(t)
	e2.waitReady(t)
	e2.stdin.Close() // the shell is gone
	if err := e2.waitExit(t, 10*time.Second); err != nil {
		t.Fatalf("exit after the shell closed its input: %v", err)
	}
}

func TestADatabaseMadeWithAnotherKeyIsNotOpenedOrChanged(t *testing.T) {
	e := start(t)
	e.waitReady(t)
	e.stdin.Close()
	e.waitExit(t, 10*time.Second)
	// Start again on the same folder with a different key: the engine must refuse and leave the file alone.
	before, _ := os.ReadFile(filepath.Join(e.dir, "socrates.db"))
	cmd := exec.Command(binary, "--data-dir", e.dir)
	cmd.Env = append(os.Environ(), "SOCRATES_DB_KEY="+strings.Repeat("cd", 32))
	stdin, _ := cmd.StdinPipe()
	defer stdin.Close()
	errBuf := &safeBuffer{}
	cmd.Stderr = errBuf
	if err := cmd.Run(); err == nil {
		t.Fatal("the engine started with a key that does not open the database")
	}
	if !strings.Contains(errBuf.String(), "cannot open the database") {
		t.Fatalf("stderr: %s", errBuf.String())
	}
	after, _ := os.ReadFile(filepath.Join(e.dir, "socrates.db"))
	if !bytes.Equal(before, after) {
		t.Fatal("a refused start changed the database file")
	}
}
