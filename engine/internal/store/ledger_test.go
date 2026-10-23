package store

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"
)

func claim(c Conversation, text string) Entry {
	return Entry{PhilosopherID: c.PhilosopherID, ConversationID: c.ID, Kind: KindClaim, Text: text, Quote: "I said: " + text}
}

func TestALedgerEntryRoundTripsWithItsEvidenceAndIsKeptPerPhilosopher(t *testing.T) {
	s, _, _ := open(t)
	soc, pla := mustConv(t, &s.Ops, "socrates"), mustConv(t, &s.Ops, "plato")
	e, err := s.AddEntry(ctx, Entry{PhilosopherID: "socrates", ConversationID: soc.ID, Kind: KindTerm, Text: "justice", Detail: "giving each their due", Quote: "justice is giving each their due", Suggested: true})
	if err != nil || e.ID == "" || e.Status != StatusActive {
		t.Fatalf("%+v %v", e, err)
	}
	s.AddEntry(ctx, claim(pla, "lying is always wrong"))
	got, err := s.Entries(ctx, "socrates", false)
	if err != nil || len(got) != 1 || got[0].Text != "justice" || got[0].Detail != "giving each their due" || got[0].Quote == "" || !got[0].Suggested || got[0].Kind != KindTerm {
		t.Fatalf("%+v %v", got, err)
	}
	if other, _ := s.Entries(ctx, "plato", false); len(other) != 1 || other[0].Text != "lying is always wrong" {
		t.Fatalf("plato's own entry: %+v", other)
	}
	if none, _ := s.Entries(ctx, "diogenes", false); len(none) != 0 {
		t.Fatal("an entry leaked to another philosopher")
	}
}

func TestTheLedgerRefusesEntriesThatBreakTheShapeRules(t *testing.T) {
	s, _, _ := open(t)
	c := mustConv(t, &s.Ops, "socrates")
	a, _ := s.AddEntry(ctx, claim(c, "one"))
	b, _ := s.AddEntry(ctx, claim(c, "two"))
	long := strings.Repeat("x", MaxEntryText+1)
	bad := map[string]Entry{
		"no text":             {PhilosopherID: "socrates", ConversationID: c.ID, Kind: KindClaim, Quote: "q"},
		"no quote":            {PhilosopherID: "socrates", ConversationID: c.ID, Kind: KindClaim, Text: "t"},
		"no philosopher":      {ConversationID: c.ID, Kind: KindClaim, Text: "t", Quote: "q"},
		"unknown kind":        {PhilosopherID: "socrates", ConversationID: c.ID, Kind: "belief", Text: "t", Quote: "q"},
		"too long":            {PhilosopherID: "socrates", ConversationID: c.ID, Kind: KindClaim, Text: long, Quote: "q"},
		"a tension needs two": {PhilosopherID: "socrates", ConversationID: c.ID, Kind: KindTension, Text: "t", Quote: "q", A: a.ID},
		"a tension of one":    {PhilosopherID: "socrates", ConversationID: c.ID, Kind: KindTension, Text: "t", Quote: "q", A: a.ID, B: a.ID},
		"a claim with a link": {PhilosopherID: "socrates", ConversationID: c.ID, Kind: KindClaim, Text: "t", Quote: "q", A: a.ID, B: b.ID},
	}
	for name, e := range bad {
		if _, err := s.AddEntry(ctx, e); !errors.Is(err, ErrBadEntry) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := s.AddEntry(ctx, Entry{PhilosopherID: "socrates", ConversationID: "no-such-conversation", Kind: KindClaim, Text: "t", Quote: "q"}); err == nil {
		t.Error("an entry for a conversation that does not exist")
	}
	if _, err := s.AddEntry(ctx, Entry{PhilosopherID: "socrates", ConversationID: c.ID, Kind: KindTension, Text: "they conflict", Quote: "q", A: a.ID, B: b.ID}); err != nil {
		t.Errorf("a good tension: %v", err)
	}
}

func TestTheLedgerHasAHardSizeLimitPerPhilosopher(t *testing.T) {
	s, _, _ := open(t)
	c := mustConv(t, &s.Ops, "socrates")
	err := s.Update(ctx, func(o *Ops) error {
		for i := 0; i < MaxLedgerSize; i++ {
			if _, err := o.AddEntry(ctx, claim(c, "claim "+string(rune('a'+i%26))+strings.Repeat("z", i%7))); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddEntry(ctx, claim(c, "one too many")); !errors.Is(err, ErrBadEntry) {
		t.Fatalf("%v", err)
	}
	other := mustConv(t, &s.Ops, "plato")
	if _, err := s.AddEntry(ctx, claim(other, "another philosopher is not full")); err != nil {
		t.Fatal(err)
	}
}

func TestReviseRetractAndDeleteEntriesAndTheirTensions(t *testing.T) {
	s, _, _ := open(t)
	c := mustConv(t, &s.Ops, "socrates")
	a, _ := s.AddEntry(ctx, claim(c, "one"))
	b, _ := s.AddEntry(ctx, claim(c, "two"))
	tn, _ := s.AddEntry(ctx, Entry{PhilosopherID: "socrates", ConversationID: c.ID, Kind: KindTension, Text: "they conflict", Quote: "q", A: a.ID, B: b.ID})
	if err := s.ReviseEntry(ctx, a.ID, "one, with a limit", "", StatusActive); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Entry(ctx, a.ID); got.Text != "one, with a limit" || got.Quote != a.Quote {
		t.Fatalf("the text changes and the evidence does not: %+v", got)
	}
	if err := s.ReviseEntry(ctx, b.ID, "two", "", StatusRetracted); err != nil {
		t.Fatal(err)
	}
	active, _ := s.Entries(ctx, "socrates", false)
	all, _ := s.Entries(ctx, "socrates", true)
	if len(active) != 2 || len(all) != 3 {
		t.Fatalf("active %d, all %d", len(active), len(all))
	}
	for _, bad := range []struct{ text, status string }{{"", StatusActive}, {"x", "weird"}} {
		if err := s.ReviseEntry(ctx, a.ID, bad.text, "", bad.status); !errors.Is(err, ErrBadEntry) {
			t.Errorf("%+v: %v", bad, err)
		}
	}
	if err := s.ReviseEntry(ctx, "none", "x", "", StatusActive); !errors.Is(err, ErrNotFound) {
		t.Errorf("%v", err)
	}
	if err := s.DeleteEntry(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Entry(ctx, tn.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a tension must go with an entry it rests on: %v", err)
	}
	if err := s.DeleteEntry(ctx, a.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("%v", err)
	}
}

func TestDeletingAConversationOrWipingOrClearingTheLedgerLeavesNothing(t *testing.T) {
	s, _, _ := open(t)
	c1, c2 := mustConv(t, &s.Ops, "socrates"), mustConv(t, &s.Ops, "socrates")
	s.AddEntry(ctx, claim(c1, "learnt in the first"))
	s.AddEntry(ctx, claim(c2, "learnt in the second"))
	if err := s.DeleteConversation(ctx, c1.ID); err != nil {
		t.Fatal(err)
	}
	left, _ := s.Entries(ctx, "socrates", true)
	if len(left) != 1 || left[0].Text != "learnt in the second" {
		t.Fatalf("what was learnt in a deleted conversation must go: %+v", left)
	}
	if err := s.ClearLedger(ctx, "socrates"); err != nil {
		t.Fatal(err)
	}
	if left, _ := s.Entries(ctx, "socrates", true); len(left) != 0 {
		t.Fatal("clear left entries")
	}
	c3 := mustConv(t, &s.Ops, "plato")
	s.AddEntry(ctx, claim(c3, "in plato's ledger"))
	if err := s.Wipe(ctx); err != nil {
		t.Fatal(err)
	}
	if left, _ := s.Entries(ctx, "plato", true); len(left) != 0 {
		t.Fatal("wipe left entries")
	}
}

func TestLedgerTextIsEncryptedOnDiskAndMovingItBetweenRowsIsDetected(t *testing.T) {
	s, path, _ := open(t)
	c := mustConv(t, &s.Ops, "socrates")
	a, _ := s.AddEntry(ctx, Entry{PhilosopherID: "socrates", ConversationID: c.ID, Kind: KindClaim, Text: "OKAPI-CLAIM-TEXT", Quote: "OKAPI-QUOTE-TEXT"})
	b, _ := s.AddEntry(ctx, claim(c, "second"))
	s.db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)")
	for _, f := range []string{path, path + "-wal"} {
		if data, err := os.ReadFile(f); err == nil && (bytes.Contains(data, []byte("OKAPI-CLAIM")) || bytes.Contains(data, []byte("OKAPI-QUOTE"))) {
			t.Fatalf("ledger text in plain on disk (%s)", f)
		}
	}
	// copy one row's sealed text over another's: the binding to the row makes it unreadable
	if _, err := s.db.ExecContext(ctx, "UPDATE ledger SET text = (SELECT text FROM ledger WHERE id = ?) WHERE id = ?", a.ID, b.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Entry(ctx, b.ID); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("a moved ciphertext must be detected: %v", err)
	}
}
