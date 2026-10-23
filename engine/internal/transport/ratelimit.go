package transport

import (
	"encoding/json"
	"log"
	"math"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// Rate limiting: one token bucket per (class of request, client address). A request takes one token; an empty bucket answers 429
// with Retry-After. The engine is local and the shell is its only client, so the limit is not about strangers: it stops a runaway
// loop in the shell (or a bug) from running up the bill of the user's own key.

type LimitSpec struct {
	PerSecond float64 // refill rate
	Burst     float64 // bucket size
}

const (
	ClassModel   = "model"   // requests that start model calls
	ClassGeneral = "general" // everything else
)

// DefaultLimits: five model-backed requests at once, then one every three seconds; general requests are generous.
var DefaultLimits = map[string]LimitSpec{
	ClassModel:   {PerSecond: 1.0 / 3, Burst: 5},
	ClassGeneral: {PerSecond: 5, Burst: 100},
}

// Classifier says which class a request belongs to and whether it is limited at all.
type Classifier func(r *http.Request) (class string, limited bool)

// DefaultClassifier: the liveness check is not limited; a POST that ends in /messages or /suggestions starts model calls.
func DefaultClassifier(r *http.Request) (string, bool) {
	if r.URL.Path == "/api/status" {
		return "", false
	}
	if r.Method == http.MethodPost {
		p := r.URL.Path
		for _, suffix := range []string{"/messages", "/suggestions"} {
			if len(p) >= len(suffix) && p[len(p)-len(suffix):] == suffix {
				return ClassModel, true
			}
		}
	}
	return ClassGeneral, true
}

type tokenBucket struct {
	tokens float64
	last   time.Time
}

type RateLimiter struct {
	mu        sync.Mutex
	buckets   map[string]*tokenBucket
	now       func() time.Time
	lastSweep time.Time
	specs     map[string]LimitSpec
}

const (
	maxBuckets    = 10000
	sweepInterval = 5 * time.Minute
)

func NewRateLimiter(specs map[string]LimitSpec) *RateLimiter {
	return &RateLimiter{buckets: map[string]*tokenBucket{}, now: time.Now, specs: specs}
}

// Allow takes a token from the bucket of (class, client). When the bucket is empty it returns false and how long until a token is
// available.
func (rl *RateLimiter) Allow(class, client string) (bool, time.Duration) {
	spec, ok := rl.specs[class]
	if !ok {
		return true, 0
	}
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := rl.now()
	rl.sweep(now)
	key := class + "|" + client
	b := rl.buckets[key]
	if b == nil {
		if len(rl.buckets) >= maxBuckets {
			return false, time.Duration(float64(time.Second) / spec.PerSecond) // refuse new clients rather than grow without bound
		}
		b = &tokenBucket{tokens: spec.Burst, last: now}
		rl.buckets[key] = b
	}
	b.tokens = math.Min(spec.Burst, b.tokens+now.Sub(b.last).Seconds()*spec.PerSecond)
	b.last = now
	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	return false, time.Duration((1 - b.tokens) / spec.PerSecond * float64(time.Second))
}

// sweep drops buckets that have refilled completely. Called with the lock held, at most every sweepInterval.
func (rl *RateLimiter) sweep(now time.Time) {
	if now.Sub(rl.lastSweep) < sweepInterval {
		return
	}
	rl.lastSweep = now
	for key, b := range rl.buckets {
		for class, spec := range rl.specs {
			if len(key) > len(class) && key[:len(class)+1] == class+"|" && b.tokens+now.Sub(b.last).Seconds()*spec.PerSecond >= spec.Burst {
				delete(rl.buckets, key)
			}
		}
	}
}

func clientAddress(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// Limit answers 429 to a client whose bucket is empty. A nil limiter lets everything through (tests only).
func Limit(rl *RateLimiter, classify Classifier, next http.Handler) http.Handler {
	if rl == nil {
		return next
	}
	if classify == nil {
		classify = DefaultClassifier
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		class, limited := classify(r)
		if !limited {
			next.ServeHTTP(w, r)
			return
		}
		if ok, wait := rl.Allow(class, clientAddress(r)); !ok {
			secs := int(math.Ceil(wait.Seconds()))
			log.Printf("[ratelimit] 429 %s %s (class %s), retry in %ds", r.Method, r.URL.Path, class, secs)
			w.Header().Set("Retry-After", strconv.Itoa(secs))
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(map[string]any{"error": "too many requests", "retryAfterSeconds": secs})
			return
		}
		next.ServeHTTP(w, r)
	})
}
