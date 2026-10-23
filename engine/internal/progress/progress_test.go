package progress

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"
	"time"

	"socrates/internal/entitlements"
	"socrates/internal/philosophers"
	"socrates/internal/store"
)

func e(kind string, examined int, mods ...func(*store.Entry)) store.Entry {
	x := store.Entry{Kind: kind, Status: store.StatusActive, Examined: examined}
	for _, m := range mods {
		m(&x)
	}
	return x
}

func suggested(x *store.Entry) { x.Suggested = true }
func retracted(x *store.Entry) { x.Status = store.StatusRetracted }

func TestNothingExaminedScoresZeroAndAnEmptyInquiryNeverCompletes(t *testing.T) {
	if r := Score(nil, Default); r.Maturity != 0 || r.Complete(Default) {
		t.Fatalf("%+v", r)
	}
	r := Score([]store.Entry{e("claim", 0), e("term", 0), e("tension", 0)}, Default)
	if r.Maturity != 0 || r.Settled != 0 {
		t.Fatalf("%+v", r)
	}
}

func TestMaturityIsSettledOverSettledPlusOpenAndFallsWhenAnOpenItemAppears(t *testing.T) {
	a := []store.Entry{e("term", 1), e("claim", 1)}
	if r := Score(a, Default); r.Maturity != 1 || r.Settled != 2 {
		t.Fatalf("%+v", r)
	}
	b := append(a, e("claim", 0), e("claim", 0))
	if r := Score(b, Default); r.Maturity != 0.5 {
		t.Fatalf("a new open claim should lower it: %+v", r)
	}
}

func TestSuggestedWordsWeighLessAndAWithdrawnClaimIsSettled(t *testing.T) {
	r := Score([]store.Entry{e("claim", 1, suggested), e("claim", 2), e("claim", 1)}, Default)
	want := (0.4 + 0.4 + 1) / 3
	if d := r.Maturity - want; d > 1e-9 || d < -1e-9 || r.Own != 1 || r.Settled != 3 {
		t.Fatalf("%+v want %.3f", r, want)
	}
	if r := Score([]store.Entry{e("claim", 0, retracted)}, Default); r.Settled != 1 || r.Maturity != 1 {
		t.Fatalf("giving a claim up is an answer to it: %+v", r)
	}
}

func TestPilingUpOneKindOfEntryDoesNotRaiseTheScore(t *testing.T) {
	var many []store.Entry
	for i := 0; i < 20; i++ {
		many = append(many, e("term", 1))
	}
	many = append(many, e("claim", 0), e("claim", 0))
	r := Score(many, Default)
	if r.Settled != Default.CapTerms {
		t.Fatalf("only %d terms should count: %+v", Default.CapTerms, r)
	}
}

func TestAChallengeNeedsTheScoreEnoughSettledItemsTheirOwnWordsAndAnExaminedView(t *testing.T) {
	p := Default
	own := func(kinds ...string) Result {
		var es []store.Entry
		for _, k := range kinds {
			es = append(es, e(k, 1))
		}
		return Score(es, p)
	}
	if !own("claim", "term", "term").Complete(p) {
		t.Fatal("a tested claim and two defined words, all in the person's words, is a challenge")
	}
	if own("term", "term", "term").Complete(p) {
		t.Fatal("defining words alone is not examining a view")
	}
	if own("claim", "term").Complete(p) {
		t.Fatal("two items are too few")
	}
	if Score([]store.Entry{e("claim", 2), e("term", 2), e("term", 2)}, p).Complete(p) {
		t.Fatal("suggested wording alone cannot finish a challenge")
	}
	if r := Score([]store.Entry{e("claim", 1), e("term", 2), e("term", 2)}, p); r.Maturity < p.Threshold || r.Settled < p.MinSettled || r.Complete(p) {
		t.Fatalf("one item in the person's own words is too few, however the rest scores: %+v", r)
	}
	low := Score([]store.Entry{e("claim", 1), e("term", 1), e("term", 1), e("claim", 0), e("claim", 0), e("claim", 0), e("tension", 0)}, p)
	if low.Complete(p) {
		t.Fatalf("under the threshold: %+v", low)
	}
}

func TestEachPhilosopherNeedsFiveChallengesWithEveryoneBeforeHim(t *testing.T) {
	r := philosophers.New().All()
	st := StatusOf(r, map[string]int{})
	if !st[0].Unlocked || st[1].Unlocked || st[2].Unlocked || st[3].Unlocked {
		t.Fatalf("%+v", st)
	}
	st = StatusOf(r, map[string]int{"socrates": 5, "diogenes": 4})
	if !st[1].Unlocked || st[2].Unlocked || len(st[2].Missing) != 1 || st[2].Missing[0] != (Need{Philosopher: "diogenes", Done: 4}) {
		t.Fatalf("%+v", st[2])
	}
	st = StatusOf(r, map[string]int{"socrates": 9, "diogenes": 5, "plato": 5})
	if !st[3].Unlocked {
		t.Fatalf("%+v", st[3])
	}
	// a surplus with one does not stand in for another
	st = StatusOf(r, map[string]int{"socrates": 50})
	if st[2].Unlocked {
		t.Fatal("fifty with Socrates do not open Plato")
	}
}

func TestTheProductionSourceReadsTheStoreAndDefaultsToNo(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "d", "s.db"), bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	src := Source{St: st, Roster: philosophers.New()}
	ctx := context.Background()
	ok := func(f entitlements.Feature) bool {
		v, err := src.Allowed(ctx, f)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	if !ok(entitlements.Philosopher("socrates")) || ok(entitlements.Philosopher("diogenes")) || !ok(entitlements.Suggestions) || ok(entitlements.Feature("unknown")) || ok(entitlements.Philosopher("nobody")) {
		t.Fatal("the starting rights are wrong")
	}
	c, _ := st.CreateConversation(ctx, "socrates")
	for i := 0; i < Required; i++ {
		in, _ := st.CreateInquiry(ctx, store.Inquiry{PhilosopherID: "socrates", ConversationID: c.ID, Question: "q", Quote: "quote"})
		st.CompleteInquiry(ctx, in.ID)
	}
	if !ok(entitlements.Philosopher("diogenes")) {
		t.Fatal("five challenges should open Diogenes")
	}
}

func TestTheDayStartsAtLocalMidnight(t *testing.T) {
	now := time.Date(2026, 10, 23, 18, 30, 0, 0, time.Local)
	if d := DayStart(now); d.Hour() != 0 || d.Day() != 23 {
		t.Fatal(d)
	}
}
