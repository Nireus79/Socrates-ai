package transport

import (
	"net/http"
	"sync"
	"time"
)

// Activity tracks whether the engine is serving anything: the number of requests in flight and when the last one ended. The phone app uses
// it to stop the engine only when the person has left and nothing is being written (a model reply can take minutes).
type Activity struct {
	mu       sync.Mutex
	inflight int
	last     time.Time
	now      func() time.Time
}

func NewActivity() *Activity { return &Activity{last: time.Now(), now: time.Now} }

// Wrap counts the requests of next, and answers GET /internal/idle itself (without counting it).
func (a *Activity) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/internal/idle" && r.Method == http.MethodGet {
			WriteJSON(w, http.StatusOK, map[string]int64{"idleSeconds": a.IdleSeconds()})
			return
		}
		a.mu.Lock()
		a.inflight++
		a.mu.Unlock()
		defer func() {
			a.mu.Lock()
			a.inflight--
			a.last = a.now()
			a.mu.Unlock()
		}()
		next.ServeHTTP(w, r)
	})
}

// IdleSeconds is 0 while a request is running, else the seconds since the last one ended.
func (a *Activity) IdleSeconds() int64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.inflight > 0 {
		return 0
	}
	return int64(a.now().Sub(a.last) / time.Second)
}
