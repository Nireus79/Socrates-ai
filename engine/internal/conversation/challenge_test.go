package conversation

import (
	"errors"
	"strings"
	"testing"

	"socrates/internal/entitlements"
	"socrates/internal/llm"
	"socrates/internal/store"
)

const topicJSON = `{"topic":"What do you owe to someone who has done you wrong?"}`

// twoTurns brings a conversation to the point where a challenge can be posed.
func chatted(t *testing.T, extra ...llm.MockStep) *env {
	t.Helper()
	steps := append(turnSteps(1), extra...)
	e := newEnv(t, steps...)
	e.send(t, "I keep wondering whether I was right to forgive him.")
	return e
}

func (e *env) offer(t *testing.T) *Result {
	t.Helper()
	res, err := e.svc.Offer(bg, e.conv.ID)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestAChallengeIsPosedInHisOwnWordsAndWaitsForAnAnswerWithoutPosingAnother(t *testing.T) {
	e := chatted(t, llm.Say(topicJSON), llm.Say("Then let me put a question to you, and you may leave it: what do you owe someone who wronged you?"), llm.Say(gateOK))
	res := e.offer(t)
	if res.Outcome != OutcomeReplied || res.Offer == nil || res.Offer.State != "pending" || len(res.Added) != 1 || res.Added[0].Role != "philosopher" {
		t.Fatalf("%+v", res)
	}
	f, ok, _ := e.st.PendingOffer(bg, e.conv.ID)
	if !ok || f.Topic != "What do you owe to someone who has done you wrong?" || f.Seq != res.Added[0].Seq {
		t.Fatalf("%+v", f)
	}
	topicCall := e.mock.Requests[len(e.mock.Requests)-3]
	if topicCall.Role != llm.RoleReply || !strings.Contains(topicCall.System, "fundamental topic") || !strings.Contains(topicCall.System, "artificial Socrates") {
		t.Fatalf("the topic is chosen by the philosopher: %s", topicCall.System)
	}
	if again := e.offer(t); again.Offer == nil || again.Offer.State != "pending" || len(again.Added) != 0 || e.mock.Remaining() != 0 {
		t.Fatal("a second challenge was made while one waits")
	}
	if got := e.messages(t); len(got) != 3 || got[2].Role != "philosopher" {
		t.Fatalf("%s", roles(got))
	}
}

func TestTheTopicIsChosenForThePersonFromWhatHeKnowsAndNeverRepeatsOne(t *testing.T) {
	e := newEnv(t, append(turnSteps(2),
		llm.Say(topicJSON), llm.Say("First challenge."), llm.Say(gateOK),
		llm.Say(`{"topic":"Is a promise still binding when the other person has broken theirs?"}`), llm.Say("Second challenge."), llm.Say(gateOK))...)
	e.tap.answer = []string{reading("", `{"op":"add_fact","text":"is deciding whether to forgive a brother","quote":"whether I was right to forgive him"}`)}
	e.send(t, "I keep wondering whether I was right to forgive him.")
	e.send(t, "He is my brother, you see.")
	e.offer(t)
	first := e.mock.Requests[len(e.mock.Requests)-3]
	if !strings.Contains(first.System, `They told you about themselves: "is deciding whether to forgive a brother"`) {
		t.Fatalf("the topic was not chosen from what he knows of the person: %s", first.System)
	}
	if strings.Contains(first.System, "pose none of these") {
		t.Fatal("nothing had been posed yet")
	}
	if err := e.svc.Decline(bg, e.conv.ID); err != nil {
		t.Fatal(err)
	}
	e.offer(t)
	second := e.mock.Requests[len(e.mock.Requests)-3]
	if !strings.Contains(second.System, "pose none of these") || !strings.Contains(second.System, "What do you owe to someone who has done you wrong?") {
		t.Fatalf("the topic already posed was not named as taken: %s", second.System)
	}
	if f, _, _ := e.st.PendingOffer(bg, e.conv.ID); f.Topic != "Is a promise still binding when the other person has broken theirs?" {
		t.Fatalf("%+v", f)
	}
}

func TestDecliningLeavesTheOfferAndIsRefusedWhenNothingWaits(t *testing.T) {
	e := chatted(t, llm.Say(topicJSON), llm.Say("First challenge."), llm.Say(gateOK))
	e.offer(t)
	if err := e.svc.Decline(bg, e.conv.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := e.st.PendingOffer(bg, e.conv.ID); ok {
		t.Fatal("still pending")
	}
	if s := e.svc.OfferState(bg, e.conv.ID); s != nil {
		t.Fatalf("%+v", s)
	}
	if err := e.svc.Decline(bg, e.conv.ID); !errors.Is(err, ErrNoOffer) {
		t.Fatalf("%v", err)
	}
}

func TestTakingUpAChallengeMakesItTheInquiryAndTheAcceptanceIsAnOrdinaryTurn(t *testing.T) {
	e := chatted(t, llm.Say(topicJSON), llm.Say("The challenge."), llm.Say(gateOK), llm.Say(judgeOK), llm.Say("What do you think is owed?"), llm.Say(gateOK))
	e.offer(t)
	res, err := e.svc.Accept(bg, e.conv.ID)
	e.svc.Wait()
	if err != nil || res.Outcome != OutcomeReplied || res.Added[0].Role != "user" || res.Added[0].Text != "I'll take the challenge." || res.Added[1].Text != "What do you think is owed?" {
		t.Fatalf("%+v %v", res, err)
	}
	in := e.current(t)
	if !in.Challenge || in.Question != "What do you owe to someone who has done you wrong?" || in.Completed {
		t.Fatalf("%+v", in)
	}
	if _, ok, _ := e.st.PendingOffer(bg, e.conv.ID); ok {
		t.Fatal("still pending")
	}
	reply := e.mock.Requests[len(e.mock.Requests)-2]
	if !strings.Contains(reply.System, "has just taken up the challenge") || !strings.Contains(reply.System, "What do you owe to someone") {
		t.Fatalf("the philosopher was not told: %s", reply.System)
	}
	asked := e.tap.asked[len(e.tap.asked)-1].Messages[0].Content
	if !strings.Contains(asked, `The current inquiry: "What do you owe to someone who has done you wrong?"`) {
		t.Fatalf("the ledger did not see the challenge as the inquiry: %s", asked)
	}
	if _, err := e.svc.Accept(bg, e.conv.ID); !errors.Is(err, ErrNoOffer) {
		t.Fatalf("a second acceptance: %v", err)
	}
}

func TestAnAcceptanceThatCannotBeAnsweredChangesNothingAndTheChallengeStillWaits(t *testing.T) {
	e := chatted(t, llm.Say(topicJSON), llm.Say("The challenge."), llm.Say(gateOK), llm.Fail(&llm.Error{Kind: llm.KindOverloaded, Reason: "busy"}))
	e.offer(t)
	res, err := e.svc.Accept(bg, e.conv.ID)
	if err != nil || res.Outcome != OutcomeUnavailable {
		t.Fatalf("%+v %v", res, err)
	}
	if _, ok, _ := e.st.PendingOffer(bg, e.conv.ID); !ok {
		t.Fatal("the challenge was lost")
	}
	if _, ok, _ := e.st.CurrentInquiry(bg, e.conv.ID); ok {
		t.Fatal("an inquiry was left behind")
	}
	if got := e.messages(t); len(got) != 3 {
		t.Fatalf("a failed acceptance kept a message: %s", roles(got))
	}
}

func TestAPitchTheGateHoldsBackTwiceIsNeverShownAndNoChallengeIsMade(t *testing.T) {
	e := chatted(t, llm.Say(topicJSON), llm.Say("Bad pitch."), llm.Say(gateFlag), llm.Say("Still bad."), llm.Say(gateFlag))
	res := e.offer(t)
	if res.Outcome != OutcomeUnavailable {
		t.Fatalf("%+v", res)
	}
	if _, ok, _ := e.st.PendingOffer(bg, e.conv.ID); ok || len(e.messages(t)) != 2 {
		t.Fatal("something was kept")
	}
}

func TestAModelThatNamesNoUsableTopicMakesNoChallenge(t *testing.T) {
	for name, bad := range map[string]string{"prose": "Sure, how about justice?", "empty": `{"topic":"  "}`, "too long": `{"topic":"` + strings.Repeat("x", 400) + `"}`} {
		e := chatted(t, llm.Say(bad))
		if res := e.offer(t); res.Outcome != OutcomeUnavailable {
			t.Errorf("%s: %+v", name, res)
		}
		if _, ok, _ := e.st.PendingOffer(bg, e.conv.ID); ok {
			t.Errorf("%s: an offer was made", name)
		}
	}
}

func TestNoChallengeIsPosedToSomeoneInCrisisOrAfterARefusalOrForALockedPhilosopher(t *testing.T) {
	e := newEnv(t, llm.Say(verdict("self_harm_clear", "0.97", "kill myself tonight")))
	e.send(t, "I am going to kill myself tonight")
	if _, err := e.svc.Offer(bg, e.conv.ID); !errors.Is(err, ErrNotNow) {
		t.Fatalf("after a crisis: %v", err)
	}
	e2 := chatted(t)
	e2.svc.SetEntitlements(entitlements.Static{})
	if _, err := e2.svc.Offer(bg, e2.conv.ID); !errors.Is(err, ErrLocked) {
		t.Fatalf("locked: %v", err)
	}
	e3 := newEnv(t, llm.Say(judgeOK), llm.Say("Hello."), llm.Say(gateOK), llm.Say("t"))
	e3.svc.ConfirmAge(bg)
	e3.send(t, "Hello there, philosopher.")
	e3.st.SetChallengeHint(bg, e3.conv.ID, true)
	e3.st.AppendMessage(bg, e3.conv.ID, "user", "Something I said.")
	if s := e3.svc.OfferState(bg, e3.conv.ID); s != nil {
		t.Fatal("a challenge was suggested while the person has the floor")
	}
}

func TestANaturalPauseNotedByTheLedgerMakesAChallengeReadyOnlyOnceTheTalkIsFarEnoughAndNotRightAfterAnOffer(t *testing.T) {
	steps := append(turnSteps(4), llm.Say(topicJSON), llm.Say("A challenge."), llm.Say(gateOK), llm.Say(judgeOK), llm.Say("A reply."), llm.Say(gateOK))
	e := newEnv(t, steps...)
	pause := `{"ops":[],"offer":true}`
	e.tap.answer = []string{pause, pause, pause, pause, pause}
	var offers []bool
	for _, text := range []string{"I keep wondering about forgiveness.", "He never apologised, you see.", "It was years ago now.", "And still I think of it."} {
		e.send(t, text)
		offers = append(offers, e.svc.OfferState(bg, e.conv.ID) != nil)
	}
	if offers[0] || offers[1] || offers[2] || !offers[3] {
		t.Fatalf("a challenge should be ready only once the talk is six messages in: %v", offers)
	}
	res := e.offer(t)
	if res.Offer == nil || res.Offer.State != "pending" {
		t.Fatalf("%+v", res)
	}
	if hint, _ := e.st.ChallengeHint(bg, e.conv.ID); hint {
		t.Fatal("the pause was not used up by posing the challenge")
	}
	if err := e.svc.Decline(bg, e.conv.ID); err != nil {
		t.Fatal(err)
	}
	e.send(t, "Perhaps I should let it go.")
	if s := e.svc.OfferState(bg, e.conv.ID); s != nil {
		t.Fatalf("another challenge was suggested right after one was left: %+v", s)
	}
}

func TestWhatThePersonTellsAboutThemselvesIsKeptAsAFactAboutThemAndNotAboutAnyInquiry(t *testing.T) {
	e := newEnv(t, turnSteps(2)...)
	e.tap.answer = []string{
		reading(inq("whether lying is ever right", "lying is always wrong", "new"), opAdd("claim", "lying is always wrong", "lying is always wrong"), `{"op":"add_fact","text":"works as a nurse","quote":"I work as a nurse on nights"}`),
		reading("", `{"op":"retract_fact","id":"E1","quote":"I have left nursing now"}`),
	}
	e.send(t, q1+" I work as a nurse on nights.")
	es, _ := e.st.Entries(bg, "socrates", false)
	var fact store.Entry
	for _, x := range es {
		if x.Kind == store.KindFact {
			fact = x
		}
	}
	if fact.Text != "works as a nurse" || fact.Quote != "I work as a nurse on nights" {
		t.Fatalf("%+v", es)
	}
	linked, _ := e.st.InquiryEntries(bg, e.current(t).ID)
	if len(linked) != 1 || linked[0].Kind != store.KindClaim {
		t.Fatalf("a fact was counted as part of an inquiry: %+v", linked)
	}
	e.send(t, "I have left nursing now, as it happens.")
	if r := e.mock.Requests[len(e.mock.Requests)-2]; !strings.Contains(r.System, `They told you about themselves: "works as a nurse" (their words: "I work as a nurse on nights")`) {
		t.Fatalf("what he knew was not told to him: %s", r.System)
	}
	all, _ := e.st.Entries(bg, "socrates", true)
	for _, x := range all {
		if x.Kind == store.KindFact && x.Status != store.StatusRetracted {
			t.Fatalf("the fact the person took back is still active: %+v", x)
		}
	}
}

func TestTheCharacterIsToldToGetToKnowThePersonOnlyWhileMemoryIsOn(t *testing.T) {
	e := newEnv(t, turnSteps(2)...)
	e.send(t, "Hello there, philosopher.")
	if r := e.mock.Requests[1]; !strings.Contains(r.System, "getting to know this person") {
		t.Fatalf("%s", r.System)
	}
	if err := e.svc.SetMemoryEnabled(bg, false); err != nil {
		t.Fatal(err)
	}
	e.send(t, "And what do you think of me?")
	var found bool
	for _, r := range e.mock.Requests[4:] {
		if r.Role == llm.RoleReply && strings.Contains(r.System, "getting to know") {
			found = true
		}
	}
	if found {
		t.Fatal("told to get to know the person while memory is off")
	}
}
