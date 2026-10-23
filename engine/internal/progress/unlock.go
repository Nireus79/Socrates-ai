package progress

import (
	"context"
	"time"

	"socrates/internal/entitlements"
	"socrates/internal/philosophers"
	"socrates/internal/store"
)

const (
	// Required is the number of challenges to finish with each earlier philosopher to unlock the next one (founder, 2026-10-23).
	Required = 5
	// MaxPerDay is the most challenges that count in one day, so that unlocking cannot be farmed. An inquiry that qualifies beyond it
	// is not lost: it is counted on the first turn of a later day.
	MaxPerDay = 3
)

// Need says what unlocking a philosopher asks for: this many challenges with each of these earlier ones.
type Need struct {
	Philosopher string
	Done        int
}

// Status is where the person stands with one philosopher.
type Status struct {
	ID         string `json:"id"`
	Challenges int    `json:"challenges"` // finished with him
	Unlocked   bool   `json:"unlocked"`   // by progress alone, whether or not the rules are enforced
	Missing    []Need `json:"-"`
}

// StatusOf computes, for every philosopher in order, whether the person has earned him. The first is always earned; each later one needs
// Required challenges with every philosopher before him.
func StatusOf(roster []philosophers.Philosopher, counts map[string]int) []Status {
	out := make([]Status, len(roster))
	for i, p := range roster {
		s := Status{ID: p.ID, Challenges: counts[p.ID], Unlocked: true}
		for _, before := range roster[:i] {
			if counts[before.ID] < Required {
				s.Unlocked = false
				s.Missing = append(s.Missing, Need{Philosopher: before.ID, Done: counts[before.ID]})
			}
		}
		out[i] = s
	}
	return out
}

// DayStart is the start of the local day, the point from which the daily limit counts.
func DayStart(now time.Time) time.Time {
	y, m, d := now.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, now.Location())
}

// Source is the production rule of entitlements: a philosopher is available when the person has earned him. It is a real
// entitlements.Source; the beta bypasses it by using entitlements.Beta in its place (see cmd/socrates-engine).
type Source struct {
	St     *store.Store
	Roster *philosophers.Registry
}

func (s Source) Allowed(ctx context.Context, f entitlements.Feature) (bool, error) {
	if f == entitlements.Suggestions {
		return true, nil
	}
	counts, err := s.St.ChallengeCounts(ctx)
	if err != nil {
		return false, err
	}
	for _, st := range StatusOf(s.Roster.All(), counts) {
		if entitlements.Philosopher(st.ID) == f {
			return st.Unlocked, nil
		}
	}
	return false, nil
}
