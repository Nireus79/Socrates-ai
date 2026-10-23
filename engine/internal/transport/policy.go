// Package transport is the engine's door: it decides who may talk to it and how.
//
// The engine is started by a shell (the desktop window or the phone app) and listens on the loopback address only. A request is
// let in when, in this order: its Host is the loopback address and the engine's own port (this stops DNS rebinding); it has no
// Origin header, or one the shell has registered (a browser page of another site always has one, and is refused); and it carries
// the per-run token. There is no development mode that relaxes any of this.
package transport

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
)

// TokenHeader carries the per-run token.
const TokenHeader = "X-Socrates-Token"

// DefaultMaxBody is the largest request body accepted unless a route asks for more.
const DefaultMaxBody = 1 << 20

// Policy holds the token and the rules of the door.
type Policy struct {
	token   string
	origins map[string]bool
	maxBody int64

	mu   sync.RWMutex
	port string
}

// NewPolicy makes a policy with a fresh random token. allowedOrigins are exact Origin values a shell may send (for example
// "http://localhost" from a phone web view); with none, every request that carries an Origin header is refused.
func NewPolicy(allowedOrigins []string) (*Policy, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return nil, err
	}
	p := &Policy{token: hex.EncodeToString(raw), origins: map[string]bool{}, maxBody: DefaultMaxBody}
	for _, o := range allowedOrigins {
		if o = strings.TrimSpace(o); o != "" && o != "*" && o != "null" {
			p.origins[o] = true
		}
	}
	return p, nil
}

// Token is the per-run token. It is handed to the shell once, in the handshake, and written nowhere.
func (p *Policy) Token() string { return p.token }

// SetPort tells the policy which port the engine listens on (known only after the listener is bound).
func (p *Policy) SetPort(port int) {
	p.mu.Lock()
	p.port = strconv.Itoa(port)
	p.mu.Unlock()
}

// SetMaxBody changes the body limit.
func (p *Policy) SetMaxBody(n int64) { p.maxBody = n }

func (p *Policy) hostAllowed(hostport string) bool {
	host, port, err := net.SplitHostPort(hostport)
	if err != nil {
		return false
	}
	p.mu.RLock()
	want := p.port
	p.mu.RUnlock()
	if want == "" || port != want {
		return false
	}
	switch host {
	case "127.0.0.1", "localhost", "::1":
		return true
	}
	return false
}

func (p *Policy) originAllowed(origin string) bool { return p.origins[origin] }

// needsToken: everything except the liveness check.
func needsToken(r *http.Request) bool {
	return !(r.URL.Path == "/api/status" && (r.Method == http.MethodGet || r.Method == http.MethodHead))
}

// WriteJSON answers with a JSON body.
func WriteJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func deny(w http.ResponseWriter, code int, msg string) { WriteJSON(w, code, map[string]string{"error": msg}) }

// ReadBody decodes a JSON body into v and answers 400 or 413 itself when it cannot. It reports whether the caller may go on.
func ReadBody(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			deny(w, http.StatusRequestEntityTooLarge, "the request is too large")
		} else {
			deny(w, http.StatusBadRequest, "the request body is not valid JSON")
		}
		return false
	}
	return true
}

// Middleware puts the door in front of next.
func (p *Policy) Middleware(next http.Handler) http.Handler {
	inner := recoverPanics(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Cache-Control", "no-store")
		h.Set("X-Content-Type-Options", "nosniff")
		if !p.hostAllowed(r.Host) {
			deny(w, http.StatusForbidden, "host not allowed")
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			if !p.originAllowed(origin) {
				deny(w, http.StatusForbidden, "origin not allowed")
				return
			}
			h.Set("Access-Control-Allow-Origin", origin)
			h.Set("Vary", "Origin")
			h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			h.Set("Access-Control-Allow-Headers", "Content-Type, "+TokenHeader)
			h.Set("Access-Control-Max-Age", "600")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h.Set("Content-Type", "application/json")
		if needsToken(r) && subtle.ConstantTimeCompare([]byte(r.Header.Get(TokenHeader)), []byte(p.token)) != 1 {
			deny(w, http.StatusUnauthorized, "missing or wrong token")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, p.maxBody)
		inner.ServeHTTP(w, r)
	})
}

// recoverPanics turns a panic in a handler into a plain 500. The log gets the place and the kind of the panic, never its value
// (which could hold what the user wrote).
func recoverPanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("[http] panic serving %s %s: %s\n%s", r.Method, r.URL.Path, fmt.Sprintf("%T", rec), debug.Stack())
				deny(w, http.StatusInternalServerError, "internal error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
