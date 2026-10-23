package transport

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"
)

// Listen binds the loopback address. port 0 asks the system for a free one, so two copies never fight for a port and nobody can
// guess it; a fixed port is for development. It binds before the slow start-up work, so a second copy stops at once.
func Listen(port int) (net.Listener, error) {
	lis, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return nil, fmt.Errorf("cannot listen on port %d (is another copy running?): %w", port, err)
	}
	return lis, nil
}

// PortOf is the port a listener is bound to.
func PortOf(lis net.Listener) int { return lis.Addr().(*net.TCPAddr).Port }

// Handshake is the one line the engine prints on standard output for the shell that started it.
type Handshake struct {
	Port    int    `json:"port"`
	Token   string `json:"token"`
	Version string `json:"version"`
}

// WriteHandshake prints the handshake as one JSON line.
func WriteHandshake(w io.Writer, h Handshake) error {
	line, err := json.Marshal(h)
	if err != nil {
		return err
	}
	_, err = w.Write(append(line, '\n'))
	return err
}

// ReadHandshake reads the handshake line (the shell's side).
func ReadHandshake(r io.Reader) (Handshake, error) {
	var h Handshake
	var line []byte
	buf := make([]byte, 1)
	for len(line) < 4096 {
		n, err := r.Read(buf)
		if n == 1 {
			if buf[0] == '\n' {
				break
			}
			line = append(line, buf[0])
		}
		if err != nil {
			if len(line) == 0 {
				return h, err
			}
			break
		}
	}
	if err := json.Unmarshal(line, &h); err != nil || h.Port == 0 || h.Token == "" {
		return Handshake{}, errors.New("the engine did not give a valid handshake")
	}
	return h, nil
}

// WatchParent calls onClose when r (the engine's standard input, held open by the shell) reaches its end: the shell is gone, so the
// engine goes too. Nothing is ever read from it for any other purpose.
func WatchParent(r io.Reader, onClose func()) {
	go func() {
		io.Copy(io.Discard, r)
		onClose()
	}()
}

// NewHTTPServer makes the server with limits that fit a local door: slow or oversized headers are cut off; the write side has no
// timeout because a reply waits for a model call that may take minutes (the handler's own context bounds it).
func NewHTTPServer(h http.Handler) *http.Server {
	return &http.Server{
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    32 << 10,
	}
}

// StartupSwitch answers 503 "starting" until the real handler is set, so the port can be taken before the database is open.
type StartupSwitch struct {
	mu sync.RWMutex
	h  http.Handler
}

func (s *StartupSwitch) Set(h http.Handler) {
	s.mu.Lock()
	s.h = h
	s.mu.Unlock()
}

func (s *StartupSwitch) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	h := s.h
	s.mu.RUnlock()
	if h == nil {
		WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "starting"})
		return
	}
	h.ServeHTTP(w, r)
}

// RegisterShutdown adds POST /internal/shutdown (the token is needed): the shell can ask the engine to stop cleanly.
func RegisterShutdown(mux *http.ServeMux, quit chan<- struct{}) {
	mux.HandleFunc("POST /internal/shutdown", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
		select {
		case quit <- struct{}{}:
		default:
		}
	})
}
