// Package shell is what the desktop window needs besides the window: it starts the engine as a child process, reads its handshake, and
// serves the engine's API to the page with the token added. The page never holds the token and never talks to the engine directly.
package shell

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

const tokenHeader = "X-Socrates-Token"

// Handshake is the one JSON line the engine prints when it is listening.
type Handshake struct {
	Port    int    `json:"port"`
	Token   string `json:"token"`
	Version string `json:"version"`
}

// Engine is a running engine process.
type Engine struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser
	Hand  Handshake
	done  chan struct{}
}

// Start runs the engine at path with extra arguments and environment, and waits for its handshake. The engine stops when Stop closes its
// standard input, and also if this process dies.
func Start(path string, args, env []string, wait time.Duration) (*Engine, error) {
	cmd := exec.Command(path, args...)
	cmd.Env = append(os.Environ(), env...)
	cmd.Stderr = os.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("the engine could not be started: %w", err)
	}
	e := &Engine{cmd: cmd, stdin: stdin, done: make(chan struct{})}
	go func() { cmd.Wait(); close(e.done) }()

	lines := make(chan string, 1)
	go func() {
		r := bufio.NewReader(stdout)
		line, _ := r.ReadString('\n')
		lines <- line
		io.Copy(io.Discard, r) // nothing else is expected; never let the pipe fill
	}()
	select {
	case line := <-lines:
		if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &e.Hand); err != nil || e.Hand.Port == 0 || e.Hand.Token == "" {
			e.Stop()
			return nil, errors.New("the engine did not say where it listens")
		}
	case <-e.done:
		return nil, errors.New("the engine stopped at once")
	case <-time.After(wait):
		e.Stop()
		return nil, errors.New("the engine did not answer in time")
	}
	return e, nil
}

// Stop asks the engine to stop by closing its standard input, and kills it if it does not leave.
func (e *Engine) Stop() {
	e.stdin.Close()
	select {
	case <-e.done:
	case <-time.After(12 * time.Second):
		e.cmd.Process.Kill()
		<-e.done
	}
}

// Done is closed when the engine process has ended.
func (e *Engine) Done() <-chan struct{} { return e.done }

// Proxy answers the page's requests for /api/ by forwarding them to the engine with the token added and no Origin header. Anything else
// is not the engine's business and is refused, so the page cannot reach the engine's other paths.
func (e *Engine) Proxy(client *http.Client) http.Handler {
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Minute}
	}
	base := fmt.Sprintf("http://127.0.0.1:%d", e.Hand.Port)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		req, err := http.NewRequestWithContext(r.Context(), r.Method, base+r.URL.RequestURI(), r.Body)
		if err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if v := r.Header.Get("Content-Type"); v != "" {
			req.Header.Set("Content-Type", v)
		}
		req.Header.Set(tokenHeader, e.Hand.Token)
		resp, err := client.Do(req)
		if err != nil {
			http.Error(w, `{"error":"The engine does not answer."}`, http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		if v := resp.Header.Get("Content-Type"); v != "" {
			w.Header().Set("Content-Type", v)
		}
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	})
}
