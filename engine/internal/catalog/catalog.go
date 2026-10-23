// Package catalog says what each product the person can obtain unlocks. Products come from outside the engine (a store purchase, a licence key,
// a rewarded ad, a donation tier); the prices and the checkout belong to the platform that sells them, so no price is kept here. The shell of the
// app tells the engine which products are in force (PUT /api/entitlements/grants) or grants one for a while (POST /api/entitlements/pass).
package catalog

import (
	"context"
	"strings"
	"time"

	"socrates/internal/entitlements"
	"socrates/internal/philosophers"
	"socrates/internal/store"
)

const (
	// SupporterID is the one-time supporter product.
	SupporterID = "supporter"
	// AllID meets every philosopher at once.
	AllID = "philosophers.all"
	// PhilosopherPrefix + an id is the product that meets that philosopher at once ("philosopher.diogenes").
	PhilosopherPrefix = "philosopher."
	// PassPrefix + an id is the same for a limited time, from a rewarded ad ("pass.diogenes"); it must carry an end.
	PassPrefix = "pass."
	// MaxPassHours is the longest a pass may be given for.
	MaxPassHours = 72
)

// Sources a grant may come from.
var Sources = map[string]bool{"play": true, "steam": true, "itch": true, "license": true, "ads": true, "sponsor": true, "gift": true}

// Features is what a product unlocks; nothing for a product this version does not know.
func Features(product string, roster []philosophers.Philosopher) []entitlements.Feature {
	switch {
	case product == SupporterID:
		return []entitlements.Feature{entitlements.Supporter}
	case product == AllID:
		var out []entitlements.Feature
		for _, p := range roster {
			out = append(out, entitlements.Philosopher(p.ID))
		}
		return out
	case strings.HasPrefix(product, PhilosopherPrefix):
		return one(roster, strings.TrimPrefix(product, PhilosopherPrefix))
	case strings.HasPrefix(product, PassPrefix):
		return one(roster, strings.TrimPrefix(product, PassPrefix))
	}
	return nil
}

func one(roster []philosophers.Philosopher, id string) []entitlements.Feature {
	for _, p := range roster {
		if p.ID == id {
			return []entitlements.Feature{entitlements.Philosopher(id)}
		}
	}
	return nil
}

// Known says whether this version knows what the product unlocks.
func Known(product string, roster []philosophers.Philosopher) bool {
	return len(Features(product, roster)) > 0
}

// IsPass says whether a product is the time-limited kind.
func IsPass(product string) bool { return strings.HasPrefix(product, PassPrefix) }

// Granted is the entitlement source that reads the grants the shell has recorded.
type Granted struct {
	St     *store.Store
	Roster *philosophers.Registry
	Now    func() time.Time // nil: the real clock
}

func (g Granted) Allowed(ctx context.Context, f entitlements.Feature) (bool, error) {
	now := time.Now
	if g.Now != nil {
		now = g.Now
	}
	products, err := g.St.ActiveGrants(ctx, now())
	if err != nil {
		return false, err
	}
	roster := g.Roster.All()
	for _, p := range products {
		for _, got := range Features(p, roster) {
			if got == f {
				return true, nil
			}
		}
	}
	return false, nil
}
