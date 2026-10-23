// Package entitlements answers one question: may this person use this feature? It exists from the start so that nothing else in the
// engine ever asks "is this the beta?" or "did they pay?". The free beta grants everything (decided 2026-10-23); a later source, backed by
// the store's own unlock records or a purchase, replaces Beta without any other change.
package entitlements

import "context"

// Feature is something a person may or may not use.
type Feature string

const (
	// Suggestions is "Help me answer".
	Suggestions Feature = "suggestions"
)

// Philosopher is the feature of talking with one philosopher.
func Philosopher(id string) Feature { return Feature("philosopher:" + id) }

// Source decides. An error means "could not tell", which callers treat as not allowed.
type Source interface {
	Allowed(ctx context.Context, f Feature) (bool, error)
}

// Beta grants every feature: the free beta.
type Beta struct{}

func (Beta) Allowed(context.Context, Feature) (bool, error) { return true, nil }

// Static allows exactly the features it lists. It is for tests and for a simple fixed set.
type Static map[Feature]bool

func (s Static) Allowed(_ context.Context, f Feature) (bool, error) { return s[f], nil }
