package conversation

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"socrates/internal/entitlements"
	"socrates/internal/llm"
	"socrates/internal/philosophers"
	"socrates/internal/progress"
	"socrates/internal/store"
)

// turnSteps is the scripted model for n ordinary turns (judge, reply, gate), with the title written after the first.
func turnSteps(n int) []llm.MockStep {
	var s []llm.MockStep
	for i := 0; i < n; i++ {
		s = append(s, llm.Say(judgeOK), llm.Say("A reply."), llm.Say(gateOK))
		if i == 0 {
			s = append(s, llm.Say("t"))
		}
		if i%4 == 3 { // the running summary is written every eight messages
			s = append(s, llm.Say("A summary."))
		}
	}
	return s
}

func reading(inquiry string, ops ...string) string {
	out := `{"ops":[` + strings.Join(ops, ",") + `]`
	if inquiry != "" {
		out += `,"inquiry":` + inquiry
	}
	return out + `}`
}

func inq(question, quote, relation string, inherit ...string) string {
	q := fmt.Sprintf(`{"question":%q,"quote":%q,"relation":%q`, question, quote, relation)
	if len(inherit) > 0 {
		q += `,"inherit":["` + strings.Join(inherit, `","`) + `"]`
	}
	return q + `}`
}

func opAdd(kind, text, quote string) string {
	return fmt.Sprintf(`{"op":"add_%s","text":%q,"quote":%q}`, kind, text, quote)
}

func opTerm(text, def, quote string) string {
	return fmt.Sprintf(`{"op":"add_term","text":%q,"definition":%q,"quote":%q}`, text, def, quote)
}

func opOn(op, id, quote string) string {
	return fmt.Sprintf(`{"op":%q,"id":%q,"quote":%q}`, op, id, quote)
}

func (e *env) current(t *testing.T) store.Inquiry {
	t.Helper()
	in, ok, err := e.st.CurrentInquiry(bg, e.conv.ID)
	if err != nil || !ok {
		t.Fatalf("no inquiry: %v", err)
	}
	return in
}

func counts(t *testing.T, e *env) map[string]int {
	t.Helper()
	c, err := e.st.ChallengeCounts(bg)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

const (
	q1 = "I am sure that lying is always wrong, whatever the reason."
	q2 = "A lie is saying what you know is false, to deceive."
	q3 = "Wrong means that it harms someone who trusted you."
	q4 = "I would still say it is wrong even to lie to a murderer."
)

// finish scripts the turns of a whole challenge in one conversation and returns the results of the last two.
func finish(t *testing.T, e *env) []*Result {
	t.Helper()
	e.tap.answer = []string{
		reading(inq("whether lying is ever right", "lying is always wrong", "new"), opAdd("claim", "lying is always wrong", "lying is always wrong"), opTerm("lie", "saying what you know is false to deceive", "saying what you know is false")),
		reading("", opTerm("wrong", "harming someone who trusted you", "harms someone who trusted you")),
		reading("", opOn("test_claim", "E1", "still say it is wrong even to lie to a murderer")),
		reading(""),
	}
	var out []*Result
	for _, text := range []string{q1 + " " + q2, q3, q4, "Nothing more to add here, really."} {
		out = append(out, e.send(t, text))
	}
	return out
}

func TestAnInquiryStartsFromThePersonsWordsAndTheEntriesBelongToIt(t *testing.T) {
	e := newEnv(t, turnSteps(1)...)
	e.tap.answer = []string{reading(inq("whether lying is ever right", "lying is always wrong", "new"), opAdd("claim", "lying is always wrong", "lying is always wrong"))}
	e.send(t, q1)
	in := e.current(t)
	if in.Question != "whether lying is ever right" || in.Quote != "lying is always wrong" || in.ParentID != "" || in.Completed {
		t.Fatalf("%+v", in)
	}
	es, _ := e.st.InquiryEntries(bg, in.ID)
	if len(es) != 1 || es[0].Text != "lying is always wrong" {
		t.Fatalf("%+v", es)
	}
}

func TestAnInquiryQuotingWordsThePersonNeverWroteIsNotStarted(t *testing.T) {
	e := newEnv(t, turnSteps(1)...)
	e.tap.answer = []string{reading(inq("whether the earth is flat", "I believe the earth is flat", "new"))}
	e.send(t, q1)
	if _, ok, _ := e.st.CurrentInquiry(bg, e.conv.ID); ok {
		t.Fatal("an inquiry rested on words nobody wrote")
	}
}

func TestEntriesNotedBeforeTheFirstInquiryJoinIt(t *testing.T) {
	e := newEnv(t, turnSteps(2)...)
	e.tap.answer = []string{
		reading("", opAdd("claim", "lying is always wrong", "lying is always wrong")),
		reading(inq("whether lying is ever right", "even to lie to a murderer", "new")),
	}
	e.send(t, q1)
	e.send(t, q4)
	es, _ := e.st.InquiryEntries(bg, e.current(t).ID)
	if len(es) != 1 {
		t.Fatalf("%+v", es)
	}
}

func TestADeepeningStartsANewInquiryLinkedToItsParentAndKeepsTheParentsScore(t *testing.T) {
	e := newEnv(t, turnSteps(2)...)
	e.tap.answer = []string{
		reading(inq("whether lying is ever right", "lying is always wrong", "new"), opAdd("claim", "lying is always wrong", "lying is always wrong"), opTerm("lie", "saying what you know is false to deceive", "saying what you know is false")),
		reading(inq("what harm is", "harms someone who trusted you", "deepening", "E2"), opTerm("harm", "making someone worse off", "harms someone who trusted you")),
	}
	e.send(t, q1+" "+q2)
	first := e.current(t)
	if first.Banked != 500 {
		t.Fatalf("a defined term and an untested claim should bank one half: %d", first.Banked)
	}
	e.send(t, q3)
	in := e.current(t)
	if in.ID == first.ID || in.ParentID == "" || in.Question != "what harm is" {
		t.Fatalf("%+v", in)
	}
	es, _ := e.st.InquiryEntries(bg, in.ID)
	var names []string
	for _, x := range es {
		names = append(names, x.Text)
	}
	if old, _, _ := e.st.CurrentInquiry(bg, e.conv.ID); old.ID != in.ID {
		t.Fatal("the current inquiry changed")
	}
	got := strings.Join(names, "|")
	if !strings.Contains(got, "lie") || !strings.Contains(got, "harm") || strings.Contains(got, "always wrong") {
		t.Fatalf("the new inquiry should hold the inherited term and its own, not the untouched claim: %s", got)
	}
}

func TestADifferentQuestionStartsFromNothing(t *testing.T) {
	e := newEnv(t, turnSteps(2)...)
	e.tap.answer = []string{
		reading(inq("whether lying is ever right", "lying is always wrong", "new"), opAdd("claim", "lying is always wrong", "lying is always wrong")),
		reading(inq("what a good life is", "a lie is saying what you know is false", "different", "E1")),
	}
	e.send(t, q1)
	e.send(t, q2)
	in := e.current(t)
	es, _ := e.st.InquiryEntries(bg, in.ID)
	if in.ParentID != "" || len(es) != 0 {
		t.Fatalf("%+v %+v", in, es)
	}
}

func TestAProposalThatOnlyRewordsTheCurrentInquiryChangesNothing(t *testing.T) {
	e := newEnv(t, turnSteps(2)...)
	e.tap.answer = []string{
		reading(inq("whether lying is ever right", "lying is always wrong", "new")),
		reading(inq("Whether lying is ever right", "a lie is saying what you know is false", "deepening")),
	}
	e.send(t, q1)
	first := e.current(t)
	e.send(t, q2)
	if e.current(t).ID != first.ID {
		t.Fatal("a reworded inquiry became a new one")
	}
}

func TestAnInquiryWhoseClaimsAreTestedAndTermsDefinedBecomesAChallengeOnceAndIsToldOnce(t *testing.T) {
	e := newEnv(t, turnSteps(5)...)
	results := finish(t, e)
	in := e.current(t)
	if !in.Completed || in.Banked < 600 {
		t.Fatalf("%+v", in)
	}
	if counts(t, e)["socrates"] != 1 {
		t.Fatalf("%v", counts(t, e))
	}
	told := 0
	for _, r := range results {
		if strings.Contains(r.Milestone, "challenge with Socrates") {
			told++
		}
	}
	e.tap.answer = []string{reading("")}
	last := e.send(t, "Nothing more to add here, really.")
	if last.Milestone != "" || counts(t, e)["socrates"] != 1 {
		t.Fatalf("told again or counted twice: %q %v", last.Milestone, counts(t, e))
	}
	if told > 1 {
		t.Fatalf("told %d times", told)
	}
	// the milestone appears on the turn after the work is done at the latest
	e2 := newEnv(t, turnSteps(5)...)
	rs := finish(t, e2)
	e2.tap.answer = []string{reading("")}
	rs = append(rs, e2.send(t, "One more thing to say to you."))
	n := 0
	for _, r := range rs {
		if r.Milestone != "" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("the person was told %d times", n)
	}
}

func TestDefiningWordsAloneIsNotAChallengeAndNeitherIsWhatCameFromSuggestions(t *testing.T) {
	e := newEnv(t, turnSteps(3)...)
	e.tap.answer = []string{
		reading(inq("what lying is", "saying what you know is false", "new"), opTerm("lie", "saying what you know is false", "saying what you know is false"), opTerm("deceive", "making someone believe what is untrue", "to deceive")),
		reading("", opTerm("wrong", "harming someone who trusted you", "harms someone who trusted you")),
		reading(""),
	}
	e.send(t, q2)
	e.send(t, q3)
	e.send(t, q4)
	if in := e.current(t); in.Completed || counts(t, e)["socrates"] != 0 {
		t.Fatalf("three definitions are not a challenge: %+v", in)
	}
}

func TestWhatCameFromSuggestionsWeighsLessAndCannotFinishAChallengeAlone(t *testing.T) {
	e := newEnv(t, turnSteps(4)...)
	e.tap.answer = []string{
		reading(inq("whether lying is ever right", "lying is always wrong", "new"), opAdd("claim", "lying is always wrong", "lying is always wrong")),
		reading("", opTerm("lie", "saying what you know is false", "saying what you know is false"), opTerm("wrong", "harming someone who trusted you", "harms someone who trusted you")),
		reading("", opOn("test_claim", "E1", "even to lie to a murderer")),
	}
	e.send(t, q1)
	for _, text := range []string{q2 + " " + q3, q4} {
		if _, err := e.svc.SendSuggested(bg, e.conv.ID, text); err != nil {
			t.Fatal(err)
		}
		e.svc.Wait()
	}
	if in := e.current(t); in.Completed || counts(t, e)["socrates"] != 0 {
		t.Fatalf("a challenge was won with the suggestions' words: %+v", in)
	}
}

func TestOnlyThreeChallengesCountInADayAndTheFourthIsCountedOnALaterDay(t *testing.T) {
	e := newEnv(t, turnSteps(12)...)
	day := time.Date(2026, 10, 23, 12, 0, 0, 0, time.Local)
	e.svc.now = func() time.Time { return day }
	for i := 0; i < progress.MaxPerDay; i++ { // three earlier challenges today, from other conversations
		c, _ := e.st.CreateConversation(bg, "socrates")
		x, err := e.st.CreateInquiry(bg, store.Inquiry{PhilosopherID: "socrates", ConversationID: c.ID, Question: "q", Quote: "quote"})
		if err != nil {
			t.Fatal(err)
		}
		if err := e.st.CompleteInquiry(bg, x.ID); err != nil {
			t.Fatal(err)
		}
	}
	// those were stamped with the real clock; the test's day must contain them
	e.svc.now = time.Now
	finish(t, e)
	in := e.current(t)
	if in.Completed || counts(t, e)["socrates"] != progress.MaxPerDay {
		t.Fatalf("the fourth challenge of the day counted: %+v %v", in, counts(t, e))
	}
	if in.Banked < 600 {
		t.Fatalf("its score should still be banked: %+v", in)
	}
	e.svc.now = func() time.Time { return time.Now().Add(26 * time.Hour) }
	e.tap.answer = []string{reading("")}
	if r := e.send(t, "Back again the next day, with nothing new."); r.Outcome != OutcomeReplied {
		t.Fatalf("the turn did not run: %+v", r)
	}
	if in := e.current(t); !in.Completed || counts(t, e)["socrates"] != progress.MaxPerDay+1 {
		t.Fatalf("not counted on the later day: %+v %v", in, counts(t, e))
	}
}

func TestAFinishedChallengeSurvivesTheConversationItWasWonIn(t *testing.T) {
	e := newEnv(t, turnSteps(5)...)
	finish(t, e)
	if err := e.svc.Delete(bg, e.conv.ID, false); err != nil {
		t.Fatal(err)
	}
	if counts(t, e)["socrates"] != 1 {
		t.Fatalf("progress was lost with the conversation: %v", counts(t, e))
	}
	if n, _ := e.st.ChallengesSince(bg, time.Unix(0, 0)); n != 1 {
		t.Fatal(n)
	}
	// and it holds no words
	if cs, _ := e.st.Challenges(bg); len(cs) != 1 || cs[0].PhilosopherID != "socrates" {
		t.Fatalf("%+v", cs)
	}
}

func TestWithMemoryOffNoChallengeCanBeWon(t *testing.T) {
	e := newEnv(t, turnSteps(5)...)
	if err := e.svc.SetMemoryEnabled(bg, false); err != nil {
		t.Fatal(err)
	}
	finish(t, e)
	if counts(t, e)["socrates"] != 0 {
		t.Fatal("a challenge was counted with memory off")
	}
}

func TestTheRuleOfUnlockingFiveChallengesWithEachEarlierPhilosopher(t *testing.T) {
	e := newEnv(t, turnSteps(1)...)
	e.svc.SetEntitlements(progress.Source{St: e.st, Roster: philosophers.New()})
	allowed := func(id string) bool { return e.svc.Allowed(bg, entitlements.Philosopher(id)) }
	give := func(id string, n int) {
		for i := 0; i < n; i++ {
			c, _ := e.st.CreateConversation(bg, id)
			x, _ := e.st.CreateInquiry(bg, store.Inquiry{PhilosopherID: id, ConversationID: c.ID, Question: "q", Quote: "quote"})
			if err := e.st.CompleteInquiry(bg, x.ID); err != nil {
				t.Fatal(err)
			}
		}
	}
	if !allowed("socrates") || allowed("diogenes") || allowed("plato") || allowed("aristotle") {
		t.Fatal("only Socrates is open at the start")
	}
	if _, err := e.svc.Start(bg, "diogenes"); err != ErrLocked {
		t.Fatalf("%v", err)
	}
	give("socrates", progress.Required-1)
	if allowed("diogenes") {
		t.Fatal("four are not five")
	}
	give("socrates", 1)
	if !allowed("diogenes") || allowed("plato") {
		t.Fatal("five with Socrates opens Diogenes only")
	}
	give("diogenes", progress.Required)
	if !allowed("plato") || allowed("aristotle") {
		t.Fatal("Plato needs five with each of the two before him")
	}
	give("plato", progress.Required-1)
	if allowed("aristotle") {
		t.Fatal("Aristotle needs five with Plato too")
	}
	give("plato", 1)
	if !allowed("aristotle") {
		t.Fatal("all earned")
	}
	if _, err := e.svc.Start(bg, "aristotle"); err != nil {
		t.Fatal(err)
	}
}
