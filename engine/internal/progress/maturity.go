// Package progress measures how far an inquiry has been examined (its maturity) and turns finished inquiries into challenges that
// unlock the next philosopher (PLAN 16).
//
// Maturity follows Moly's gap maturity: settled over settled plus open, counted over the ledger entries an inquiry rests on. A term is
// settled when the person has defined it, a claim when they have tested it (kept it, qualified it or given it up), a tension when they
// have seen it themselves. The score can fall when a new open item appears, and nothing is asked of the person but their own thinking.
package progress

import "socrates/internal/store"

// Profile holds the numbers that decide when an inquiry counts as a challenge. They are data, so a philosopher can later have his own.
type Profile struct {
	Threshold  float64 // maturity at which an inquiry is finished
	MinSettled int     // settled items needed, whatever the score
	MinOwn     int     // of those, settled in the person's own words
	MinTurns   int     // turns of the person's the inquiry must have been examined over
	// The most items of each kind that count, so that piling up one kind of entry cannot raise the score.
	CapTerms, CapClaims, CapTensions int
	// Suggested is the weight of a settled item whose words came from a chosen suggestion.
	Suggested float64
}

// Default is the profile of every philosopher until one is given his own.
var Default = Profile{Threshold: 0.6, MinSettled: 2, MinOwn: 2, MinTurns: 5, CapTerms: 5, CapClaims: 6, CapTensions: 3, Suggested: 0.4}

// Result is the reading of one inquiry.
type Result struct {
	Maturity float64 // 0 to 1
	Settled  int     // items settled
	Own      int     // of those, in the person's own words
	Examined int     // settled claims and tensions (a challenge needs at least one: defining words alone is not examining a view)
}

// settled says whether the person has dealt with an entry.
func settled(e store.Entry) bool {
	return e.Examined > 0 || e.Status == store.StatusRetracted || e.Status == store.StatusResolved
}

// own says whether the entry rests wholly on the person's own words.
func own(e store.Entry) bool { return !e.Suggested && e.Examined != 2 }

// Score reads an inquiry from the entries it rests on (in the order they were made; the caps keep the earliest of each kind).
func Score(entries []store.Entry, p Profile) Result {
	kept := map[string]int{}
	limit := map[string]int{store.KindTerm: p.CapTerms, store.KindClaim: p.CapClaims, store.KindTension: p.CapTensions}
	var r Result
	var weight float64
	considered := 0
	for _, e := range entries {
		if kept[e.Kind] >= limit[e.Kind] {
			continue
		}
		kept[e.Kind]++
		considered++
		if !settled(e) {
			continue
		}
		r.Settled++
		if e.Kind != store.KindTerm {
			r.Examined++
		}
		if own(e) {
			r.Own++
			weight++
		} else {
			weight += p.Suggested
		}
	}
	if considered > 0 {
		r.Maturity = weight / float64(considered)
	}
	return r
}

// Thousandths is the maturity as stored.
func (r Result) Thousandths() int { return int(r.Maturity*1000 + 0.5) }

// Complete says whether the inquiry is finished: examined well enough, by enough of the person's own thinking.
func (r Result) Complete(p Profile) bool {
	return r.Maturity >= p.Threshold && r.Settled >= p.MinSettled && r.Own >= p.MinOwn && r.Examined >= 1
}
