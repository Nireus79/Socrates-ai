package store

import (
	"path/filepath"
	"testing"
	"time"
)

func newInquiry(t *testing.T, s *Store, c Conversation, q string) Inquiry {
	t.Helper()
	in, err := s.CreateInquiry(ctx, Inquiry{PhilosopherID: c.PhilosopherID, ConversationID: c.ID, Question: q, Quote: "I asked " + q})
	if err != nil {
		t.Fatal(err)
	}
	return in
}

func TestAnInquiryRoundTripsSealedAndTheNewestIsCurrent(t *testing.T) {
	s, _, _ := open(t)
	c := mustConv(t, &s.Ops, "socrates")
	if _, ok, _ := s.CurrentInquiry(ctx, c.ID); ok {
		t.Fatal("no inquiry yet")
	}
	a := newInquiry(t, s, c, "what is equality")
	b, err := s.CreateInquiry(ctx, Inquiry{PhilosopherID: "socrates", ConversationID: c.ID, ParentID: a.ID, Question: "what is freedom", Quote: "and freedom?"})
	if err != nil {
		t.Fatal(err)
	}
	cur, ok, err := s.CurrentInquiry(ctx, c.ID)
	if err != nil || !ok || cur.ID != b.ID || cur.ParentID != a.ID || cur.Question != "what is freedom" || cur.Quote != "and freedom?" {
		t.Fatalf("%+v %v", cur, err)
	}
	if _, err := s.CreateInquiry(ctx, Inquiry{PhilosopherID: "socrates", ConversationID: c.ID, Question: "", Quote: "x"}); err == nil {
		t.Fatal("an inquiry with no question was accepted")
	}
}

func TestEntriesLinkToAnInquiryAndOrphansJoinTheFirstOne(t *testing.T) {
	s, _, _ := open(t)
	c := mustConv(t, &s.Ops, "socrates")
	e1, _ := s.AddEntry(ctx, claim(c, "one"))
	e2, _ := s.AddEntry(ctx, claim(c, "two"))
	in := newInquiry(t, s, c, "q")
	if err := s.LinkOrphans(ctx, in.ID, c.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := s.InquiryEntries(ctx, in.ID)
	if len(got) != 2 {
		t.Fatalf("orphans were not linked: %+v", got)
	}
	// an entry already in an inquiry is not taken by a later one
	in2 := newInquiry(t, s, c, "q2")
	e3, _ := s.AddEntry(ctx, claim(c, "three"))
	if err := s.LinkOrphans(ctx, in2.ID, c.ID); err != nil {
		t.Fatal(err)
	}
	got2, _ := s.InquiryEntries(ctx, in2.ID)
	if len(got2) != 1 || got2[0].ID != e3.ID {
		t.Fatalf("%+v", got2)
	}
	if err := s.LinkEntries(ctx, in2.ID, e1.ID, e2.ID, e1.ID, "no-such-entry"); err != nil {
		t.Fatal(err)
	}
	if ids, _ := s.InquiryEntryIDs(ctx, in2.ID); len(ids) != 3 {
		t.Fatalf("%v", ids)
	}
}

func TestBankingNeverLowersAndACompletedInquiryMakesOneChallengeOnce(t *testing.T) {
	s, _, _ := open(t)
	c := mustConv(t, &s.Ops, "plato")
	in := newInquiry(t, s, c, "q")
	s.Bank(ctx, in.ID, 700)
	s.Bank(ctx, in.ID, 300)
	if cur, _, _ := s.CurrentInquiry(ctx, c.ID); cur.Banked != 700 {
		t.Fatalf("%d", cur.Banked)
	}
	for i := 0; i < 3; i++ {
		if err := s.CompleteInquiry(ctx, in.ID); err != nil {
			t.Fatal(err)
		}
	}
	if n, _ := s.ChallengeCounts(ctx); n["plato"] != 1 {
		t.Fatalf("%v", n)
	}
	if n, _ := s.ChallengesSince(ctx, time.Now().Add(-time.Hour)); n != 1 {
		t.Fatal(n)
	}
	if n, _ := s.ChallengesSince(ctx, time.Now().Add(time.Hour)); n != 0 {
		t.Fatal(n)
	}
}

func TestAnnouncementsAreTakenOnce(t *testing.T) {
	s, _, _ := open(t)
	c := mustConv(t, &s.Ops, "socrates")
	in := newInquiry(t, s, c, "q")
	if n, _ := s.TakeAnnouncements(ctx, c.ID); n != 0 {
		t.Fatal("nothing finished yet")
	}
	s.CompleteInquiry(ctx, in.ID)
	if n, _ := s.TakeAnnouncements(ctx, c.ID); n != 1 {
		t.Fatal(n)
	}
	if n, _ := s.TakeAnnouncements(ctx, c.ID); n != 0 {
		t.Fatal("told twice")
	}
}

func TestAChallengeOutlivesItsConversationButNotTheProfileAndHoldsNoText(t *testing.T) {
	s, _, _ := open(t)
	c := mustConv(t, &s.Ops, "socrates")
	e, _ := s.AddEntry(ctx, claim(c, "one"))
	in := newInquiry(t, s, c, "a private question")
	s.LinkEntries(ctx, in.ID, e.ID)
	s.CompleteInquiry(ctx, in.ID)
	if err := s.DeleteConversation(ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	if n, _ := s.ChallengeCounts(ctx); n["socrates"] != 1 {
		t.Fatal("the count went with the conversation")
	}
	var inq, links, entries int
	s.db.QueryRow("SELECT COUNT(*) FROM inquiry").Scan(&inq)
	s.db.QueryRow("SELECT COUNT(*) FROM inquiry_entry").Scan(&links)
	s.db.QueryRow("SELECT COUNT(*) FROM ledger").Scan(&entries)
	if inq != 0 || links != 0 || entries != 0 {
		t.Fatalf("what was learnt stayed: %d %d %d", inq, links, entries)
	}
	if err := s.Wipe(ctx); err != nil {
		t.Fatal(err)
	}
	if n, _ := s.ChallengeCounts(ctx); len(n) != 0 {
		t.Fatal("a wiped profile kept its progress")
	}
}

func TestClearingTheLedgerKeepsTheProgress(t *testing.T) {
	s, _, _ := open(t)
	c := mustConv(t, &s.Ops, "socrates")
	s.AddEntry(ctx, claim(c, "one"))
	in := newInquiry(t, s, c, "q")
	s.CompleteInquiry(ctx, in.ID)
	if err := s.ClearLedger(ctx, "socrates"); err != nil {
		t.Fatal(err)
	}
	if n, _ := s.ChallengeCounts(ctx); n["socrates"] != 1 {
		t.Fatal("forgetting what was said must not undo a finished challenge")
	}
}

func TestExaminedMarksNeverGoDownAndImportedChallengesAreCounted(t *testing.T) {
	s, _, _ := open(t)
	c := mustConv(t, &s.Ops, "socrates")
	e, _ := s.AddEntry(ctx, claim(c, "one"))
	s.SetExamined(ctx, e.ID, 2)
	if got, _ := s.Entry(ctx, e.ID); got.Examined != 2 {
		t.Fatal(got.Examined)
	}
	s.SetExamined(ctx, e.ID, 1)
	s.SetExamined(ctx, e.ID, 2)
	if got, _ := s.Entry(ctx, e.ID); got.Examined != 1 {
		t.Fatalf("an own-words mark was lowered: %d", got.Examined)
	}
	if err := s.SetExamined(ctx, e.ID, 3); err == nil {
		t.Fatal("a bad level was accepted")
	}
	if err := s.AddChallenge(ctx, Challenge{PhilosopherID: "diogenes", CreatedAt: time.Unix(1000, 0)}); err != nil {
		t.Fatal(err)
	}
	if cs, _ := s.Challenges(ctx); len(cs) != 1 || cs[0].PhilosopherID != "diogenes" || cs[0].CreatedAt.Unix() != 1000 {
		t.Fatalf("%+v", cs)
	}
}

func TestADatabaseFromBeforeInquiriesIsUpgradedWithItsLedgerIntactAndDefinedTermsCountAsExamined(t *testing.T) {
	full := migrations
	migrations = full[:3]
	path := filepath.Join(t.TempDir(), "data", "socrates.db")
	key := testKey(8)
	s, err := Open(path, key)
	if err != nil {
		migrations = full
		t.Fatal(err)
	}
	c := mustConv(t, &s.Ops, "socrates")
	seal := func(col, id, v string) []byte {
		b, e := s.c.seal("ledger", col, id, []byte(v))
		if e != nil {
			t.Fatal(e)
		}
		return b
	}
	// entries as the third version wrote them: no "examined" column
	for _, row := range []struct{ id, kind, text, detail string }{{"c1", "claim", "lying is wrong", ""}, {"t1", "term", "lie", "saying what is false"}, {"t2", "term", "wrong", ""}} {
		var detail []byte
		if row.detail != "" {
			detail = seal("detail", row.id, row.detail)
		}
		if _, err := s.db.ExecContext(ctx, `INSERT INTO ledger(id, philosopher_id, conversation_id, kind, text, detail, quote, status, created_at, updated_at)
			VALUES (?, 'socrates', ?, ?, ?, ?, ?, 'active', 1, 1)`, row.id, c.ID, row.kind, seal("text", row.id, row.text), detail, seal("quote", row.id, "I said so myself")); err != nil {
			migrations = full
			t.Fatal(err)
		}
	}
	s.Close()
	migrations = full
	s2, err := Open(path, key)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	got := map[string]int{}
	es, err := s2.Entries(ctx, "socrates", true)
	if err != nil || len(es) != 3 {
		t.Fatalf("%+v %v", es, err)
	}
	for _, e := range es {
		got[e.ID] = e.Examined
	}
	if got["c1"] != 0 || got["t1"] != 1 || got["t2"] != 0 {
		t.Fatalf("%v", got)
	}
	if _, err := s2.CreateInquiry(ctx, Inquiry{PhilosopherID: "socrates", ConversationID: c.ID, Question: "q", Quote: "quote"}); err != nil {
		t.Fatal(err)
	}
}
