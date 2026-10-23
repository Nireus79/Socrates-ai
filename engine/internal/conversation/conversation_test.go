package conversation

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"socrates/internal/entitlements"
	"socrates/internal/llm"
	"socrates/internal/philosophers"
	"socrates/internal/safety"
	"socrates/internal/store"
	"socrates/internal/suggest"
)

var bg = context.Background()

const (
	judgeOK   = `{"label":"ok","confidence":0.95,"evidence":"","reason":"nothing"}`
	gateOK    = `{"flagged":false,"category":"none","reason":"fine"}`
	gateFlag  = `{"flagged":true,"category":"hatred","reason":"it insulted the person"}`
	userWords = "What is courage, do you think?"
)

func verdict(label string, conf string, evidence string) string {
	return `{"label":"` + label + `","confidence":` + conf + `,"evidence":"` + evidence + `","reason":"r"}`
}

type env struct {
	svc  *Service
	st   *store.Store
	mock *llm.Mock
	conv store.Conversation
	logs *bytes.Buffer
}

// newEnv makes a service on a real temporary database with a scripted model. The age limit is confirmed.
func newEnv(t *testing.T, steps ...llm.MockStep) *env {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "d", "socrates.db"), bytes.Repeat([]byte{5}, 32))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	mock := llm.NewMock(steps...)
	client := llm.NewClientWith(mock, llm.Settings{FastModel: "fast-model", ReplyModel: "reply-model"})
	svc := New(st, func(context.Context) (Completer, error) { return client, nil }, philosophers.New())
	if err := svc.ConfirmAge(bg); err != nil {
		t.Fatal(err)
	}
	conv, err := svc.Start(bg, "socrates")
	if err != nil {
		t.Fatal(err)
	}
	logs := &bytes.Buffer{}
	log.SetOutput(logs)
	t.Cleanup(func() { log.SetOutput(io.Discard) })
	return &env{svc: svc, st: st, mock: mock, conv: conv, logs: logs}
}

func (e *env) send(t *testing.T, text string) *Result {
	t.Helper()
	res, err := e.svc.Send(bg, e.conv.ID, text)
	if err != nil {
		t.Fatal(err)
	}
	e.svc.Wait()
	return res
}

func (e *env) messages(t *testing.T) []store.Message {
	t.Helper()
	m, err := e.st.Messages(bg, e.conv.ID)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func roles(msgs []store.Message) string {
	var r []string
	for _, m := range msgs {
		r = append(r, m.Role)
	}
	return strings.Join(r, ",")
}

func TestANormalTurnKeepsTheUsersWordsAndTheReplyAndShowsTheModelsWhatTheyNeed(t *testing.T) {
	e := newEnv(t, llm.Say(judgeOK), llm.Say("Tell me first what you mean by it."), llm.Say(gateOK), llm.Say("On courage"))
	res := e.send(t, userWords)
	if res.Outcome != OutcomeReplied || len(res.Added) != 2 || res.Added[0].Text != userWords || res.Added[1].Text != "Tell me first what you mean by it." {
		t.Fatalf("%+v", res)
	}
	if roles(e.messages(t)) != "user,philosopher" {
		t.Fatalf("roles: %s", roles(e.messages(t)))
	}
	judge, reply, gate := e.mock.Requests[0], e.mock.Requests[1], e.mock.Requests[2]
	if judge.Role != llm.RoleFast || reply.Role != llm.RoleReply || gate.Role != llm.RoleFast {
		t.Fatalf("roles of the calls: %s %s %s", judge.Role, reply.Role, gate.Role)
	}
	if !strings.Contains(reply.System, "artificial Socrates") || len(reply.Messages) != 1 || reply.Messages[0].Content != userWords {
		t.Fatalf("the reply request: %+v", reply)
	}
	if !strings.Contains(gate.Messages[0].Content, "Tell me first what you mean by it.") {
		t.Fatal("the gate did not see the draft")
	}
	if got, _ := e.st.Conversation(bg, e.conv.ID); got.Title != "On courage" {
		t.Fatalf("title %q", got.Title)
	}
	if e.mock.Remaining() != 0 {
		t.Fatal("a scripted call was not made")
	}
}

func TestNothingHappensWithoutTheAgeConfirmationAProviderOrAGoodMessage(t *testing.T) {
	e := newEnv(t)
	long := strings.Repeat("a", MaxMessageChars+1)
	for text, want := range map[string]error{"": ErrEmpty, "   \n": ErrEmpty, long: ErrTooLong} {
		if _, err := e.svc.Send(bg, e.conv.ID, text); !errors.Is(err, want) {
			t.Errorf("%.10q: %v", text, err)
		}
	}
	e.st.DeleteSetting(bg, AgeKey)
	if _, err := e.svc.Send(bg, e.conv.ID, "hello"); !errors.Is(err, ErrAgeNotConfirmed) {
		t.Errorf("without the age confirmation: %v", err)
	}
	e.svc.ConfirmAge(bg)
	e2 := New(e.st, func(context.Context) (Completer, error) { return nil, ErrNoProvider }, philosophers.New())
	if _, err := e2.Send(bg, e.conv.ID, "hello"); !errors.Is(err, ErrNoProvider) {
		t.Errorf("without a provider: %v", err)
	}
	if _, err := e.svc.Send(bg, "no-such-conversation", "hello"); !errors.Is(err, ErrNotFound) {
		t.Errorf("an unknown conversation: %v", err)
	}
	if len(e.mock.Requests) != 0 {
		t.Fatalf("%d model calls were made for turns that could not start", len(e.mock.Requests))
	}
}

func TestWhenNoVerdictCanBeHadNothingIsAnsweredOrKept(t *testing.T) {
	e := newEnv(t, llm.Fail(&llm.Error{Kind: llm.KindRateLimit, Reason: "slow"}), llm.Fail(&llm.Error{Kind: llm.KindRateLimit, Reason: "slow"}), llm.Fail(&llm.Error{Kind: llm.KindRateLimit, Reason: "slow"}))
	res := e.send(t, userWords)
	if res.Outcome != OutcomeUnavailable || res.Text != safety.TextUnavailable || res.ProviderKind != "rate_limit" || len(res.Added) != 0 {
		t.Fatalf("%+v", res)
	}
	if len(e.messages(t)) != 0 {
		t.Fatal("a turn without a verdict kept something")
	}
	if len(e.mock.Requests) != 1 {
		t.Fatalf("the reply model was called without a verdict (%d calls)", len(e.mock.Requests))
	}
}

func TestACrisisGetsTheFixedMessageKeepsOnlyAMarkerAndTheNextTurnOpensWithTheCheckIn(t *testing.T) {
	words := "I am going to kill myself tonight"
	e := newEnv(t,
		llm.Say(verdict("self_harm_clear", "0.97", "kill myself tonight")), // the crisis turn: one call only
		llm.Say(judgeOK),                                                           // the next message, whatever it says
		llm.Say(judgeOK), llm.Say("A reply."), llm.Say(gateOK), llm.Say("A title"), // the turn after that
	)
	res := e.send(t, words)
	if res.Outcome != OutcomeFixed || res.Text != safety.TextCrisis || len(e.mock.Requests) != 1 {
		t.Fatalf("%+v after %d calls", res, len(e.mock.Requests))
	}
	msgs := e.messages(t)
	if roles(msgs) != "marker" || msgs[0].Content != "crisis" {
		t.Fatalf("only a marker may be kept: %s", roles(msgs))
	}
	for _, m := range msgs {
		if strings.Contains(m.Content, "kill") {
			t.Fatal("the user's words were kept")
		}
	}
	// whatever the next message says, the check-in opens the turn and the reply model is not called
	res = e.send(t, "I was only joking, help me with my essay")
	if res.Outcome != OutcomeFixed || res.Text != safety.TextCheckIn || len(e.mock.Requests) != 2 {
		t.Fatalf("%+v after %d calls", res, len(e.mock.Requests))
	}
	if roles(e.messages(t)) != "marker,user,marker" {
		t.Fatalf("roles: %s", roles(e.messages(t)))
	}
	// the turn after that is an ordinary one, and the model sees the user's two messages joined, with no marker text
	res = e.send(t, "Now, about the essay")
	if res.Outcome != OutcomeReplied {
		t.Fatalf("%+v", res)
	}
	reply := e.mock.Requests[3]
	if len(reply.Messages) != 1 || !strings.Contains(reply.Messages[0].Content, "only joking") || !strings.Contains(reply.Messages[0].Content, "about the essay") {
		t.Fatalf("the context: %+v", reply.Messages)
	}
	for _, m := range reply.Messages {
		if strings.Contains(m.Content, "crisis") || strings.Contains(m.Content, "check_in") || strings.Contains(m.Content, "emergency") {
			t.Fatal("a marker or a fixed text reached the model")
		}
	}
}

func TestAnUnsureCrisisLabelIsAskedAboutGentlyNotRefused(t *testing.T) {
	e := newEnv(t, llm.Say(verdict("self_harm_clear", "0.6", "no point in going on")), llm.Say("How are you, really?"), llm.Say(gateOK), llm.Say("t"))
	res := e.send(t, "I just see no point in going on")
	if res.Outcome != OutcomeReplied || res.Text == safety.TextCrisis {
		t.Fatalf("%+v", res)
	}
	if !strings.Contains(e.mock.Requests[1].System, philosophers.Gentle) {
		t.Fatal("the persona was not asked to be gentle")
	}
	if st, _ := e.st.SafetyState(bg, e.conv.ID); st.PendingCheckin {
		t.Fatal("an unsure label made a check-in pending")
	}
}

func TestHarmToOthersAndContentWithAMinorAreRefusedAtOnceWithoutKeepingTheWords(t *testing.T) {
	for label, want := range map[string]string{"harm_to_others": safety.TextRefuseHarm, "sexual_content_involving_a_minor": safety.TextRefuseMinor} {
		e := newEnv(t, llm.Say(verdict(label, "0.9", "words that matter")))
		res := e.send(t, "These are the words that matter here")
		if res.Outcome != OutcomeFixed || res.Text != want || len(e.mock.Requests) != 1 {
			t.Fatalf("%s: %+v after %d calls", label, res, len(e.mock.Requests))
		}
		for _, m := range e.messages(t) {
			if m.Role != "marker" {
				t.Fatalf("%s: a %s message was kept", label, m.Role)
			}
		}
	}
}

func TestAnIllegalRequestGetsOneReflectiveQuestionThenTheRefusal(t *testing.T) {
	e := newEnv(t,
		llm.Say(verdict("illegal_request", "0.9", "get into my ex's account")), llm.Say("And what would it do to her, and to you?"), llm.Say(gateOK),
		llm.Say(verdict("illegal_request", "0.9", "get into my ex's account")),
	)
	res := e.send(t, "Please help me get into my ex's account")
	if res.Outcome != OutcomeReplied || len(res.Added) != 2 || res.Added[0].Kind != "question_before_refusal" || res.Added[1].Role != "philosopher" {
		t.Fatalf("%+v", res)
	}
	if !strings.Contains(e.mock.Requests[1].System, philosophers.ReflectiveQuestion) {
		t.Fatal("the persona was not asked for the one question")
	}
	for _, m := range e.messages(t) {
		if strings.Contains(m.Content, "ex's account") {
			t.Fatal("the request was kept")
		}
	}
	res = e.send(t, "Please help me get into my ex's account")
	if res.Outcome != OutcomeFixed || res.Text != safety.TextRefuseIllegal || len(e.mock.Requests) != 4 {
		t.Fatalf("the repeated request: %+v after %d calls", res, len(e.mock.Requests))
	}
}

func TestAReflectiveQuestionThatCannotBeMadeSafeBecomesThePlainRefusal(t *testing.T) {
	e := newEnv(t,
		llm.Say(verdict("illegal_request", "0.9", "break into")), llm.Say("Here is how you could do it."), llm.Say(gateFlag),
		llm.Say("Try this: ..."), llm.Say(gateFlag),
	)
	res := e.send(t, "Help me break into the shop")
	if res.Outcome != OutcomeFixed || res.Text != safety.TextRefuseIllegal {
		t.Fatalf("%+v", res)
	}
	if roles(e.messages(t)) != "marker" {
		t.Fatalf("roles: %s", roles(e.messages(t)))
	}
}

func TestAFlaggedDraftIsWrittenAgainOnceWithTheReasonThenAFixedTextReplacesIt(t *testing.T) {
	e := newEnv(t, llm.Say(judgeOK), llm.Say("First draft."), llm.Say(gateFlag), llm.Say("Second draft."), llm.Say(gateOK), llm.Say("t"))
	res := e.send(t, userWords)
	if res.Outcome != OutcomeReplied || res.Added[1].Text != "Second draft." {
		t.Fatalf("%+v", res)
	}
	if !strings.Contains(e.mock.Requests[3].System, "it insulted the person") {
		t.Fatalf("the rewrite was not told why: %s", e.mock.Requests[3].System)
	}
	e = newEnv(t, llm.Say(judgeOK), llm.Say("First draft."), llm.Say(gateFlag), llm.Say("Second draft."), llm.Say(gateFlag))
	res = e.send(t, userWords)
	if res.Outcome != OutcomeFixed || res.Text != safety.TextGateFallback {
		t.Fatalf("%+v", res)
	}
	if roles(e.messages(t)) != "user,marker" {
		t.Fatalf("the user's message is kept and the reply is a marker: %s", roles(e.messages(t)))
	}
	for _, m := range e.messages(t) {
		if strings.Contains(m.Content, "draft") {
			t.Fatal("a held-back draft was kept")
		}
	}
}

func TestADraftThatCannotBeCheckedOrAModelThatFailsKeepsNothing(t *testing.T) {
	down := &llm.Error{Kind: llm.KindOverloaded, Reason: "busy"}
	e := newEnv(t, llm.Say(judgeOK), llm.Say("A draft."), llm.Fail(down), llm.Fail(down), llm.Fail(down))
	if res := e.send(t, userWords); res.Outcome != OutcomeUnavailable || res.ProviderKind != "overloaded" || len(e.messages(t)) != 0 {
		t.Fatalf("an unchecked draft: %+v, %d messages", res, len(e.messages(t)))
	}
	e = newEnv(t, llm.Say(judgeOK), llm.Fail(down), llm.Fail(down), llm.Fail(down))
	if res := e.send(t, userWords); res.Outcome != OutcomeUnavailable || len(e.messages(t)) != 0 {
		t.Fatalf("a failed reply model: %+v", res)
	}
}

func TestAMinorEndsTheConversationAndNothingIsKept(t *testing.T) {
	e := newEnv(t, llm.Say(verdict("minor_signal", "0.95", "I'm 13")))
	res := e.send(t, "I'm 13 and I have a question")
	if res.Outcome != OutcomeEnded || res.Text != safety.TextEndForAge {
		t.Fatalf("%+v", res)
	}
	if _, err := e.st.Conversation(bg, e.conv.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("the conversation is still there: %v", err)
	}
}

func TestTheTitleAndTheSummaryAreWrittenInTheBackgroundAndTheSummaryIsUsed(t *testing.T) {
	steps := []llm.MockStep{}
	for i := 0; i < 4; i++ { // four turns of eight messages
		steps = append(steps, llm.Say(judgeOK), llm.Say("Reply number "+string(rune('A'+i))), llm.Say(gateOK))
		if i == 0 {
			steps = append(steps, llm.Say("\"A Title.\""))
		}
		if i == 3 {
			steps = append(steps, llm.Say("The person asks about courage and has said they fear failure."))
		}
	}
	steps = append(steps, llm.Say(judgeOK), llm.Say("A later reply"), llm.Say(gateOK))
	e := newEnv(t, steps...)
	for i := 0; i < 4; i++ {
		e.send(t, "Message "+string(rune('1'+i)))
	}
	conv, _ := e.st.Conversation(bg, e.conv.ID)
	if conv.Title != "A Title" {
		t.Fatalf("title %q", conv.Title)
	}
	text, through, ok, _ := e.st.Summary(bg, e.conv.ID)
	if !ok || through != 8 || !strings.Contains(text, "fear failure") {
		t.Fatalf("summary: %q %d %v", text, through, ok)
	}
	e.send(t, "Message 5")
	reply := e.mock.Requests[len(e.mock.Requests)-2]
	if !strings.Contains(reply.System, "fear failure") {
		t.Fatalf("the summary was not given to the philosopher: %s", reply.System)
	}
}

// slow makes every model call take a while, so that turns that were not kept apart would really interleave.
type slow struct {
	c Completer
	d time.Duration
}

func (s slow) Complete(ctx context.Context, req llm.Request) (*llm.Response, error) {
	time.Sleep(s.d)
	return s.c.Complete(ctx, req)
}

func TestTwoMessagesSentAtOnceAreTakenInTurn(t *testing.T) {
	e := newEnv(t, llm.Say(judgeOK), llm.Say("Reply one."), llm.Say(gateOK), llm.Say("t"), llm.Say(judgeOK), llm.Say("Reply two."), llm.Say(gateOK))
	client := slow{llm.NewClientWith(e.mock, llm.Settings{FastModel: "fast-model", ReplyModel: "reply-model"}), 15 * time.Millisecond}
	svc := New(e.st, func(context.Context) (Completer, error) { return client, nil }, philosophers.New())
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := svc.Send(bg, e.conv.ID, userWords)
			if err != nil || res.Outcome != OutcomeReplied {
				t.Errorf("%v %+v", err, res)
			}
		}()
	}
	wg.Wait()
	svc.Wait()
	if roles(e.messages(t)) != "user,philosopher,user,philosopher" {
		t.Fatalf("roles: %s", roles(e.messages(t)))
	}
	msgs := e.messages(t)
	if msgs[1].Content != "Reply one." || msgs[3].Content != "Reply two." {
		t.Fatalf("the turns were interleaved: %q %q", msgs[1].Content, msgs[3].Content)
	}
}

func TestNothingAnyoneWroteReachesTheLog(t *testing.T) {
	secretUser, secretReply, secretSummary := "SECRET-USER-WORDS", "SECRET-REPLY-WORDS", "SECRET-SUMMARY-WORDS"
	steps := []llm.MockStep{}
	for i := 0; i < 4; i++ {
		steps = append(steps, llm.Say(judgeOK), llm.Say(secretReply), llm.Say(gateFlag), llm.Say(secretReply), llm.Say(gateOK))
		if i == 0 {
			steps = append(steps, llm.Say("SECRET-TITLE-WORDS"))
		}
		if i == 3 {
			steps = append(steps, llm.Say(secretSummary))
		}
	}
	steps = append(steps, llm.Say(verdict("self_harm_clear", "0.9", secretUser)), llm.Fail(&llm.Error{Kind: llm.KindNetwork, Reason: secretUser}))
	e := newEnv(t, steps...)
	for i := 0; i < 4; i++ {
		e.send(t, secretUser+" number "+string(rune('1'+i)))
	}
	e.send(t, secretUser+" in crisis")
	e.send(t, secretUser+" again")
	for _, secret := range []string{secretUser, secretReply, secretSummary, "SECRET-TITLE-WORDS"} {
		if strings.Contains(e.logs.String(), secret) {
			t.Fatalf("%q reached the log:\n%s", secret, e.logs.String())
		}
	}
}

func TestStartingAndReadingConversations(t *testing.T) {
	e := newEnv(t)
	if _, err := e.svc.Start(bg, "nobody"); !errors.Is(err, philosophers.ErrUnknown) {
		t.Fatalf("%v", err)
	}
	locked := philosophers.New()
	svc := New(e.st, func(context.Context) (Completer, error) { return nil, ErrNoProvider }, locked)
	// the standard roster has everyone unlocked; a locked philosopher is refused
	if _, err := svc.Start(bg, "plato"); err != nil {
		t.Fatalf("an unlocked philosopher: %v", err)
	}
	c, msgs, err := e.svc.History(bg, e.conv.ID)
	if err != nil || c.PhilosopherID != "socrates" || len(msgs) != 0 {
		t.Fatalf("%v %v %v", c, msgs, err)
	}
	if _, _, err := e.svc.History(bg, "none"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("%v", err)
	}
	if err := e.svc.Delete(bg, e.conv.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Delete(bg, e.conv.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("%v", err)
	}
}

const suggestionsJSON = `{"suggestions":["A lie is saying what you know is false.","I might lie to spare a friend.","It always costs trust.","I really don't know."]}`

func TestSuggestionsAreOfferedAfterThePhilosophersQuestionAndNeverAfterACrisisOrBeforeAnyQuestion(t *testing.T) {
	e := newEnv(t, llm.Say(judgeOK), llm.Say("What do you mean by a lie?"), llm.Say(gateOK), llm.Say("t"), llm.Say(suggestionsJSON),
		llm.Say(verdict("self_harm_clear", "0.97", "end my life")))
	if _, err := e.svc.Suggest(bg, e.conv.ID, nil); err == nil || !errors.Is(err, suggest.ErrNothingToAnswer) {
		t.Fatalf("an empty conversation: %v", err)
	}
	e.send(t, "Is it ever okay to lie?")
	r, err := e.svc.Suggest(bg, e.conv.ID, []string{"old one"})
	if err != nil || len(r.Suggestions) != 4 || r.Fallback {
		t.Fatalf("%+v %v", r, err)
	}
	asked := e.mock.Requests[len(e.mock.Requests)-1].Messages[0].Content
	if !strings.Contains(asked, "What do you mean by a lie?") || !strings.Contains(asked, "old one") {
		t.Fatalf("what the model was given: %s", asked)
	}
	e.send(t, "I want to end my life")
	n := len(e.mock.Requests)
	if _, err := e.svc.Suggest(bg, e.conv.ID, nil); !errors.Is(err, ErrNotNow) {
		t.Fatalf("after a crisis step: %v", err)
	}
	if len(e.mock.Requests) != n {
		t.Fatal("a model call was made when suggestions were not offered")
	}
}

func TestAMessageChosenFromSuggestionsIsKeptMarkedAndThePhilosopherIsToldItIsTentative(t *testing.T) {
	e := newEnv(t, llm.Say(judgeOK), llm.Say("What do you mean by a lie?"), llm.Say(gateOK), llm.Say("t"),
		llm.Say(judgeOK), llm.Say("And if the friend asked?"), llm.Say(gateOK))
	e.send(t, "Is it ever okay to lie?")
	res, err := e.svc.SendSuggested(bg, e.conv.ID, "I might lie to spare a friend.")
	if err != nil || res.Outcome != OutcomeReplied || !res.Added[0].Suggested || res.Added[1].Suggested {
		t.Fatalf("%+v %v", res, err)
	}
	msgs := e.messages(t)
	if !msgs[2].Suggested || msgs[0].Suggested {
		t.Fatalf("%+v", msgs)
	}
	reply := e.mock.Requests[len(e.mock.Requests)-2]
	if !strings.Contains(reply.System, "not yet their settled view") {
		t.Fatal("the philosopher was not told")
	}
	// an ordinary message gets no such note
	e2 := newEnv(t, llm.Say(judgeOK), llm.Say("A reply."), llm.Say(gateOK), llm.Say("t"))
	e2.send(t, "Hello there")
	if strings.Contains(e2.mock.Requests[1].System, "settled view") {
		t.Fatal("an ordinary message was treated as tentative")
	}
}

func TestAChosenSuggestionIsJudgedLikeAnyOtherMessageAndARefusedOneKeepsNothing(t *testing.T) {
	e := newEnv(t, llm.Say(verdict("harm_to_others", "0.9", "hurt him")))
	res, err := e.svc.SendSuggested(bg, e.conv.ID, "I would hurt him")
	if err != nil || res.Outcome != OutcomeFixed {
		t.Fatalf("%+v %v", res, err)
	}
	for _, m := range e.messages(t) {
		if m.Role != "marker" {
			t.Fatalf("a %s message was kept", m.Role)
		}
	}
}

func TestWhatIsNotUnlockedCannotBeUsedAndTheBetaGrantsEverything(t *testing.T) {
	e := newEnv(t)
	for _, id := range []string{"socrates", "diogenes", "plato", "aristotle"} {
		if _, err := e.svc.Start(bg, id); err != nil {
			t.Fatalf("beta: %s: %v", id, err)
		}
	}
	e.svc.SetEntitlements(entitlements.Static{entitlements.Philosopher("socrates"): true})
	if _, err := e.svc.Start(bg, "plato"); !errors.Is(err, ErrLocked) {
		t.Fatalf("a locked philosopher: %v", err)
	}
	if _, err := e.svc.Start(bg, "socrates"); err != nil {
		t.Fatal(err)
	}
	// a conversation that was started while Plato was open cannot go on once he is locked, and no model is called
	conv, _ := e.st.CreateConversation(bg, "plato")
	if _, err := e.svc.Send(bg, conv.ID, "hello"); !errors.Is(err, ErrLocked) {
		t.Fatalf("sending to a locked philosopher: %v", err)
	}
	// suggestions are a feature of their own
	if _, err := e.svc.Suggest(bg, e.conv.ID, nil); !errors.Is(err, ErrNotEntitled) {
		t.Fatalf("suggestions locked: %v", err)
	}
	if len(e.mock.Requests) != 0 {
		t.Fatalf("%d model calls for things that are locked", len(e.mock.Requests))
	}
	// a source that cannot tell counts as no
	e.svc.SetEntitlements(failing{})
	if _, err := e.svc.Start(bg, "socrates"); !errors.Is(err, ErrLocked) {
		t.Fatalf("an unreadable source: %v", err)
	}
}

type failing struct{}

func (failing) Allowed(context.Context, entitlements.Feature) (bool, error) {
	return true, errors.New("could not read")
}
