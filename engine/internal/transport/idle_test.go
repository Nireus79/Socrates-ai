package transport

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestIdleSecondsIsZeroWhileARequestRunsThenCountsFromItsEnd(t *testing.T) {
	a := NewActivity()
	clock := time.Unix(1000, 0)
	a.now = func() time.Time { return clock }
	a.last = clock
	release := make(chan struct{})
	entered := make(chan struct{})
	h := a.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-release
	}))
	done := make(chan struct{})
	go func() { h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/x", nil)); close(done) }()
	<-entered
	clock = clock.Add(500 * time.Second)
	if a.IdleSeconds() != 0 {
		t.Fatalf("a request is running but idle is %d", a.IdleSeconds())
	}
	close(release)
	<-done
	clock = clock.Add(90 * time.Second)
	if got := a.IdleSeconds(); got != 90 {
		t.Fatalf("90 seconds after the request ended, idle is %d", got)
	}
}

func TestTheIdleRouteReportsButIsNotItselfActivity(t *testing.T) {
	a := NewActivity()
	clock := time.Unix(1000, 0)
	a.now = func() time.Time { return clock }
	a.last = clock
	h := a.Wrap(http.NotFoundHandler())
	clock = clock.Add(30 * time.Second)
	for i := 0; i < 3; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", "/internal/idle", nil))
		var out map[string]int64
		json.Unmarshal(rec.Body.Bytes(), &out)
		if rec.Code != 200 || out["idleSeconds"] != 30 {
			t.Fatalf("%d %v", rec.Code, out)
		}
		clock = clock.Add(0)
	}
	// only GET is the report; anything else under that path is an ordinary (counted) request
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/internal/idle", nil))
	if rec.Code != 404 {
		t.Fatalf("%d", rec.Code)
	}
	if a.IdleSeconds() != 0 {
		t.Fatal("an ordinary request just ended, so idle must restart from zero")
	}
}
