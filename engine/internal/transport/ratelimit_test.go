package transport

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func limiterAt(now *time.Time) *RateLimiter {
	rl := NewRateLimiter(map[string]LimitSpec{ClassModel: {PerSecond: 1.0 / 3, Burst: 5}, ClassGeneral: {PerSecond: 5, Burst: 100}})
	rl.now = func() time.Time { return *now }
	return rl
}

func TestARateLimiterAllowsABurstThenRefuses(t *testing.T) {
	now := time.Now()
	rl := limiterAt(&now)
	for i := 0; i < 5; i++ {
		if ok, _ := rl.Allow(ClassModel, "a"); !ok {
			t.Fatalf("request %d refused inside the burst", i+1)
		}
	}
	ok, wait := rl.Allow(ClassModel, "a")
	if ok || wait < 2*time.Second || wait > 3*time.Second+time.Millisecond {
		t.Fatalf("sixth request: ok=%v wait=%v", ok, wait)
	}
}

func TestARateLimiterRefillsAndKeepsClientsAndClassesApart(t *testing.T) {
	now := time.Now()
	rl := limiterAt(&now)
	for i := 0; i < 5; i++ {
		rl.Allow(ClassModel, "a")
	}
	if ok, _ := rl.Allow(ClassModel, "b"); !ok {
		t.Fatal("another client shares the bucket")
	}
	if ok, _ := rl.Allow(ClassGeneral, "a"); !ok {
		t.Fatal("another class shares the bucket")
	}
	now = now.Add(3100 * time.Millisecond)
	if ok, _ := rl.Allow(ClassModel, "a"); !ok {
		t.Fatal("the bucket did not refill")
	}
	if ok, _ := rl.Allow(ClassModel, "a"); ok {
		t.Fatal("one refilled token let two requests through")
	}
}

func TestABucketThatHasRefilledIsDroppedAndTheNumberOfBucketsIsCapped(t *testing.T) {
	now := time.Now()
	rl := limiterAt(&now)
	rl.Allow(ClassModel, "a")
	now = now.Add(sweepInterval + 20*time.Second)
	rl.Allow(ClassModel, "b")
	if len(rl.buckets) != 1 {
		t.Fatalf("buckets after the sweep: %d", len(rl.buckets))
	}
	rl = limiterAt(&now)
	for i := 0; i < maxBuckets; i++ {
		rl.Allow(ClassGeneral, "client"+strconv.Itoa(i))
	}
	if ok, _ := rl.Allow(ClassGeneral, "one-more"); ok {
		t.Fatal("the number of buckets must be capped")
	}
	if ok, _ := rl.Allow(ClassGeneral, "client0"); !ok {
		t.Fatal("a known client must still be served when the cap is reached")
	}
}

func TestTheLimitMiddlewareAnswers429WithRetryAfter(t *testing.T) {
	now := time.Now()
	rl := limiterAt(&now)
	h := Limit(rl, nil, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }))
	send := func(method, path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		r := httptest.NewRequest(method, path, nil)
		r.RemoteAddr = "127.0.0.1:5555"
		h.ServeHTTP(rec, r)
		return rec
	}
	for i := 0; i < 5; i++ {
		if rec := send("POST", "/api/conversations/1/messages"); rec.Code != 200 {
			t.Fatalf("request %d: %d", i+1, rec.Code)
		}
	}
	rec := send("POST", "/api/conversations/1/messages")
	if rec.Code != 429 || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("sixth request: %d %v", rec.Code, rec.Header())
	}
	if rec := send("GET", "/api/status"); rec.Code != 200 {
		t.Fatal("the liveness check must never be limited")
	}
	if rec := send("GET", "/api/conversations"); rec.Code != 200 {
		t.Fatal("a general request was limited by the model bucket")
	}
}

func TestDefaultClassifier(t *testing.T) {
	cases := []struct {
		method, path string
		class        string
		limited      bool
	}{
		{"GET", "/api/status", "", false},
		{"POST", "/api/conversations/7/messages", ClassModel, true},
		{"POST", "/api/conversations/7/suggestions", ClassModel, true},
		{"GET", "/api/conversations/7/messages", ClassGeneral, true},
		{"GET", "/api/conversations", ClassGeneral, true},
		{"DELETE", "/api/user", ClassGeneral, true},
	}
	for _, c := range cases {
		class, limited := DefaultClassifier(httptest.NewRequest(c.method, c.path, nil))
		if class != c.class || limited != c.limited {
			t.Errorf("%s %s = %q %v, want %q %v", c.method, c.path, class, limited, c.class, c.limited)
		}
	}
}

func TestANilLimiterLetsEverythingThrough(t *testing.T) {
	h := Limit(nil, nil, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/api/conversations/1/messages", nil))
	if rec.Code != 200 {
		t.Fatal(rec.Code)
	}
}
