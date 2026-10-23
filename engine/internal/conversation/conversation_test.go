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

// ledgerTap answers the ledger's call itself (nothing to record unless a test queues an answer), so that tests of other things keep
// their scripts; it records what the ledger was asked.
type ledgerTap struct {
	inner  Completer
	mu     sync.Mutex
	answer []string
	asked  []llm.Request
}

func (l *ledgerTap) Complete(ctx context.Context, req llm.Request) (*llm.Response, error) {
	if strings.HasPrefix(req.System, "You keep the ledger") {
		l.mu.Lock()
		defer l.mu.Unlock()
		l.asked = append(l.asked, req)
		out := `{"ops": []}`
		if len(l.answer) > 0 {
			out, l.answer = l.answer[0], l.answer[1:]
		}
		return &llm.Response{Text: out}, nil
	}
	return l.inner.Complete(ctx, req)
}

type env struct {
	tap  *ledgerTap
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
	tap := &ledgerTap{inner: llm.NewClientWith(mock, llm.Settings{FastModel: "fast-model", ReplyModel: "reply-model"})}
	svc := New(st, func(context.Context) (Completer, error) { return tap, nil }, philosophers.New())
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
	return &env{tap: tap, svc: svc, st: st, mock: mock, conv: conv, logs: logs}
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
	if res.Outcome != OutcomeReplied || len(res.Added) != 1 || res.Added[0].Role != "philosopher" || res.Added[0].Text == "" {
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

// byAsker answers each call according to who is asking (judge, gate, title, ledger, summary, or the philosopher in order), because the
// order of calls is no longer fixed once background work runs beside the next turn.
type byAsker struct {
	mu      sync.Mutex
	replies []string
	d       time.Duration
}

func (b *byAsker) Complete(ctx context.Context, req llm.Request) (*llm.Response, error) {
	time.Sleep(b.d)
	b.mu.Lock()
	defer b.mu.Unlock()
	switch {
	case strings.Contains(req.System, "safety judge"):
		return &llm.Response{Text: judgeOK}, nil
	case strings.Contains(req.System, "You check a draft reply"):
		return &llm.Response{Text: gateOK}, nil
	case strings.HasPrefix(req.System, "You keep the ledger"):
		return &llm.Response{Text: `{"ops": []}`}, nil
	case strings.Contains(req.System, "title") || strings.Contains(req.System, "running summary"):
		return &llm.Response{Text: "t"}, nil
	}
	out := b.replies[0]
	b.replies = b.replies[1:]
	return &llm.Response{Text: out}, nil
}

func TestTwoMessagesSentAtOnceAreTakenInTurnAndBackgroundWorkNeverMakesTheNextMessageWait(t *testing.T) {
	e := newEnv(t)
	client := &byAsker{replies: []string{"Reply one.", "Reply two."}, d: 15 * time.Millisecond}
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
	msgs := e.messages(t)
	if roles(msgs) != "user,philosopher,user,philosopher" || msgs[1].Content != "Reply one." || msgs[3].Content != "Reply two." {
		t.Fatalf("the turns were interleaved: %s", roles(msgs))
	}
}

// slowBackground makes only the title, summary and ledger calls slow.
type slowBackground struct {
	inner Completer
	d     time.Duration
}

func (s slowBackground) Complete(ctx context.Context, req llm.Request) (*llm.Response, error) {
	if strings.Contains(req.System, "title") || strings.HasPrefix(req.System, "You keep the ledger") {
		time.Sleep(s.d)
	}
	return s.inner.Complete(ctx, req)
}

func TestTheNextMessageDoesNotWaitForTheTitleSummaryOrLedgerOfTheLastTurn(t *testing.T) {
	e := newEnv(t)
	client := slowBackground{&byAsker{replies: []string{"Reply one.", "Reply two."}}, 1500 * time.Millisecond}
	svc := New(e.st, func(context.Context) (Completer, error) { return client, nil }, philosophers.New())
	if _, err := svc.Send(bg, e.conv.ID, "First message here."); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if _, err := svc.Send(bg, e.conv.ID, "Second message here."); err != nil {
		t.Fatal(err)
	}
	if took := time.Since(start); took > 1200*time.Millisecond {
		t.Fatalf("the second message waited %v for the first turn's background work", took)
	}
	svc.Wait()
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

func addClaimOp(text, quote string) string {
	return `{"ops":[{"op":"add_claim","text":"` + text + `","quote":"` + quote + `"}]}`
}

func TestWhatThePersonSaysIsRememberedWithTheirWordsAndTheNextReplyKnowsIt(t *testing.T) {
	e := newEnv(t, llm.Say(judgeOK), llm.Say("What do you mean by that?"), llm.Say(gateOK), llm.Say("t"),
		llm.Say(judgeOK), llm.Say("Then what about a kind lie?"), llm.Say(gateOK))
	e.tap.answer = []string{addClaimOp("lying is always wrong", "lying is always wrong")}
	e.send(t, "I am sure that lying is always wrong, whatever the reason.")
	got, _ := e.st.Entries(bg, "socrates", false)
	if len(got) != 1 || got[0].Text != "lying is always wrong" || got[0].Quote != "lying is always wrong" || got[0].ConversationID != e.conv.ID || got[0].Suggested {
		t.Fatalf("%+v", got)
	}
	e.send(t, "Go on then.")
	reply := e.mock.Requests[len(e.mock.Requests)-2]
	if !strings.Contains(reply.System, "What this person has told you") || !strings.Contains(reply.System, `their words: "lying is always wrong"`) {
		t.Fatalf("the philosopher was not told: %s", reply.System)
	}
	// the ledger call saw the entries under aliases and the person's message inside the markers
	asked := e.tap.asked[1].Messages[0].Content
	if !strings.Contains(asked, "E1 claim: lying is always wrong") || !strings.Contains(asked, "<<<USER\nGo on then.\nUSER>>>") {
		t.Fatalf("%s", asked)
	}
}

func TestAnInventedQuotationIsNotRemembered(t *testing.T) {
	e := newEnv(t, llm.Say(judgeOK), llm.Say("Tell me more."), llm.Say(gateOK), llm.Say("t"))
	e.tap.answer = []string{addClaimOp("the earth is flat", "I believe the earth is flat")}
	e.send(t, "I think lying is sometimes necessary, honestly.")
	if got, _ := e.st.Entries(bg, "socrates", true); len(got) != 0 {
		t.Fatalf("%+v", got)
	}
}

func TestAMessageChosenFromSuggestionsIsRememberedAsUnconfirmed(t *testing.T) {
	e := newEnv(t, llm.Say(judgeOK), llm.Say("A first reply."), llm.Say(gateOK), llm.Say("t"), llm.Say(judgeOK), llm.Say("A second reply."), llm.Say(gateOK))
	e.send(t, "Is it ever okay to lie?")
	e.tap.answer = []string{addClaimOp("a kind lie is fine", "a kind lie is fine")}
	if _, err := e.svc.SendSuggested(bg, e.conv.ID, "I think a kind lie is fine."); err != nil {
		t.Fatal(err)
	}
	e.svc.Wait()
	got, _ := e.st.Entries(bg, "socrates", false)
	if len(got) != 1 || !got[0].Suggested {
		t.Fatalf("%+v", got)
	}
}

func TestNothingIsRememberedFromAMessageThatMayComeFromPainOrWasRefused(t *testing.T) {
	e := newEnv(t, llm.Say(verdict("self_harm_unclear", "0.8", "no point in going on")), llm.Say("How are you, really?"), llm.Say(gateOK), llm.Say("t"),
		llm.Say(verdict("harm_to_others", "0.9", "hurt him")))
	e.tap.answer = []string{addClaimOp("there is no point in going on", "no point in going on at all")}
	e.send(t, "Honestly there is no point in going on at all.")
	e.send(t, "I will hurt him badly")
	if len(e.tap.asked) != 0 {
		t.Fatalf("the ledger was asked %d times for a gentle or refused turn", len(e.tap.asked))
	}
	if got, _ := e.st.Entries(bg, "socrates", true); len(got) != 0 {
		t.Fatalf("%+v", got)
	}
}

func TestEachPhilosopherRemembersOnlyWhatWasToldToHim(t *testing.T) {
	e := newEnv(t, llm.Say(judgeOK), llm.Say("Reply."), llm.Say(gateOK), llm.Say("t"), llm.Say(judgeOK), llm.Say("Reply to the other."), llm.Say(gateOK), llm.Say("t"))
	e.tap.answer = []string{addClaimOp("lying is always wrong", "lying is always wrong")}
	e.send(t, "I am sure that lying is always wrong.")
	plato, _ := e.svc.Start(bg, "plato")
	if _, err := e.svc.Send(bg, plato.ID, "Hello Plato, what do you think of truth?"); err != nil {
		t.Fatal(err)
	}
	e.svc.Wait()
	var reply llm.Request
	for _, r := range e.mock.Requests {
		if r.Role == llm.RoleReply {
			reply = r // the last reply of all: Plato's
		}
	}
	if !strings.Contains(reply.System, "artificial Plato") || strings.Contains(reply.System, "lying is always wrong") || strings.Contains(reply.System, "What this person has told you") {
		t.Fatalf("Plato was told what Socrates knows: %s", reply.System)
	}
	if got, _ := e.st.Entries(bg, "plato", true); len(got) != 0 {
		t.Fatal("an entry appeared in Plato's ledger")
	}
}

func TestALedgerThatFailsChangesNothingAboutTheTurnAndLogsNoContent(t *testing.T) {
	e := newEnv(t, llm.Say(judgeOK), llm.Say("A reply."), llm.Say(gateOK), llm.Say("t"))
	e.tap.inner = failingLedger{e.tap.inner}
	res := e.send(t, "I am sure that lying is always wrong, SECRET-LEDGER-WORDS.")
	if res.Outcome != OutcomeReplied || strings.Contains(e.logs.String(), "SECRET-LEDGER-WORDS") {
		t.Fatalf("%+v\n%s", res, e.logs.String())
	}
}

type failingLedger struct{ Completer }

func (f failingLedger) Complete(ctx context.Context, req llm.Request) (*llm.Response, error) {
	if strings.HasPrefix(req.System, "You keep the ledger") {
		return nil, &llm.Error{Kind: llm.KindNetwork, Reason: "SECRET-LEDGER-WORDS"}
	}
	return f.Completer.Complete(ctx, req)
}

func TestDeletingAConversationDeletesWhatWasLearntInIt(t *testing.T) {
	e := newEnv(t, llm.Say(judgeOK), llm.Say("Reply."), llm.Say(gateOK), llm.Say("t"))
	e.tap.answer = []string{addClaimOp("lying is always wrong", "lying is always wrong")}
	e.send(t, "I am sure that lying is always wrong.")
	if err := e.svc.Delete(bg, e.conv.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := e.st.Entries(bg, "socrates", true); len(got) != 0 {
		t.Fatalf("%+v", got)
	}
}

func TestABlankReplyIsNeverShownOrKeptAndACutOffReplyEndsAtItsLastWholeSentence(t *testing.T) {
	e := newEnv(t, llm.Say(judgeOK), llm.Say("   "))
	res := e.send(t, userWords)
	if res.Outcome != OutcomeUnavailable || len(e.messages(t)) != 0 || res.ProviderKind != "empty" {
		t.Fatalf("a blank reply: %+v, %d messages", res, len(e.messages(t)))
	}
	for in, want := range map[string]string{
		"First thought. Second thought. And a third that was cut o": "First thought. Second thought.",
		"One whole sentence.":         "One whole sentence.",
		"No sentence end at all":      "No sentence end at all",
		`He said "stop"`:              `He said "stop"`,
		"A question? And then half a": "A question?",
	} {
		if got := trimCutOff(in); got != want {
			t.Errorf("%q -> %q, want %q", in, got, want)
		}
	}
}

func TestAReflectiveQuestionThatWasNeverAskedIsNotUsedUp(t *testing.T) {
	e := newEnv(t,
		llm.Say(verdict("illegal_request", "0.9", "break into")), llm.Say("Here is how."), llm.Say(gateFlag), llm.Say("Try this."), llm.Say(gateFlag), // held back twice
		llm.Say(verdict("illegal_request", "0.9", "break into")), llm.Say("What would it cost the owner?"), llm.Say(gateOK)) // the next time the question is still available
	e.send(t, "Help me break into the shop")
	if st, _ := e.st.SafetyState(bg, e.conv.ID); st.ReflectiveQuestionAsked {
		t.Fatal("the once-only question was spent although none was asked")
	}
	res := e.send(t, "Help me break into the shop")
	if res.Outcome != OutcomeReplied || len(res.Added) != 1 || res.Added[0].Role != "philosopher" {
		t.Fatalf("the question should still be offered: %+v", res)
	}
	if st, _ := e.st.SafetyState(bg, e.conv.ID); !st.ReflectiveQuestionAsked {
		t.Fatal("asking the question must use it up")
	}
	for _, m := range e.messages(t) {
		if m.Role == "marker" && m.Content == "question_before_refusal" {
			t.Fatal("a marker with no text of its own must not be stored: it shows as an empty bubble")
		}
	}
}

func TestTheLedgerIsWrittenFromTheMessageOfItsOwnTurnNeverFromALaterOne(t *testing.T) {
	e := newEnv(t, llm.Say(judgeOK), llm.Say("Reply one."), llm.Say(gateOK), llm.Say("t"), llm.Say(verdict("self_harm_unclear", "0.8", "no point in going on at all")), llm.Say("How are you?"), llm.Say(gateOK))
	e.send(t, "First I want to say that honesty matters to me.")
	e.send(t, "Honestly there is no point in going on at all.") // a gentle turn: never remembered
	msgs := e.messages(t)
	firstSeq := msgs[0].Seq
	// the background work of the first turn runs late, after the second turn was committed
	e.tap.asked = nil
	e.svc.remember(bg, e.tap, store.Conversation{ID: e.conv.ID, PhilosopherID: "socrates"}, firstSeq)
	if len(e.tap.asked) != 1 {
		t.Fatalf("the ledger was asked %d times", len(e.tap.asked))
	}
	asked := e.tap.asked[0].Messages[0].Content
	if !strings.Contains(asked, "<<<USER\nFirst I want to say that honesty matters to me.\nUSER>>>") || strings.Contains(asked, "<<<USER\nHonestly there is no point") {
		t.Fatalf("the ledger read the wrong message:\n%s", asked)
	}
	// a sequence number that is not one of the person's messages does nothing
	e.tap.asked = nil
	e.svc.remember(bg, e.tap, store.Conversation{ID: e.conv.ID, PhilosopherID: "socrates"}, msgs[1].Seq) // the philosopher's own message
	e.svc.remember(bg, e.tap, store.Conversation{ID: e.conv.ID, PhilosopherID: "socrates"}, 999)
	if len(e.tap.asked) != 0 {
		t.Fatal("remember worked from a message that is not the person's")
	}
}

func TestTheLocksOfADeletedConversationAreDropped(t *testing.T) {
	e := newEnv(t, llm.Say(judgeOK), llm.Say("Reply."), llm.Say(gateOK), llm.Say("t"))
	e.send(t, userWords)
	e.svc.mu.Lock()
	had := len(e.svc.locks) > 0 && len(e.svc.bgLocks) > 0
	e.svc.mu.Unlock()
	if !had {
		t.Fatal("the test did not take any lock")
	}
	if err := e.svc.Delete(bg, e.conv.ID); err != nil {
		t.Fatal(err)
	}
	e.svc.mu.Lock()
	defer e.svc.mu.Unlock()
	if len(e.svc.locks) != 0 || len(e.svc.bgLocks) != 0 {
		t.Fatalf("%d and %d locks remain", len(e.svc.locks), len(e.svc.bgLocks))
	}
}

func TestWithMemoryOffNothingIsRememberedOrToldAndNoLedgerCallIsMade(t *testing.T) {
	e := newEnv(t, llm.Say(judgeOK), llm.Say("Reply one."), llm.Say(gateOK), llm.Say("t"), llm.Say(judgeOK), llm.Say("Reply two."), llm.Say(gateOK))
	if !e.svc.MemoryEnabled(bg) {
		t.Fatal("memory must be on unless the person turned it off")
	}
	e.tap.answer = []string{addClaimOp("lying is always wrong", "lying is always wrong")}
	e.send(t, "I am sure that lying is always wrong.") // remembered while on
	if got, _ := e.st.Entries(bg, "socrates", false); len(got) != 1 {
		t.Fatalf("%+v", got)
	}
	if err := e.svc.SetMemoryEnabled(bg, false); err != nil {
		t.Fatal(err)
	}
	if e.svc.MemoryEnabled(bg) {
		t.Fatal("still on")
	}
	asked := len(e.tap.asked)
	e.send(t, "And promises must always be kept, I think.")
	if len(e.tap.asked) != asked {
		t.Fatal("the ledger was asked while memory was off")
	}
	if got, _ := e.st.Entries(bg, "socrates", false); len(got) != 1 {
		t.Fatalf("something new was remembered while memory was off: %+v", got)
	}
	var reply llm.Request
	for _, r := range e.mock.Requests {
		if r.Role == llm.RoleReply {
			reply = r
		}
	}
	if strings.Contains(reply.System, "lying is always wrong") {
		t.Fatal("what was kept was told to the philosopher while memory was off")
	}
	// what was kept stays until the person deletes it, and comes back into use when memory is turned on again
	e.svc.SetMemoryEnabled(bg, true)
	if got, _ := e.st.Entries(bg, "socrates", false); len(got) != 1 {
		t.Fatal("turning memory off must not delete what was kept")
	}
}

func TestAnUnreadableMemorySettingCountsAsOff(t *testing.T) {
	e := newEnv(t)
	e.st.SetSetting(bg, MemoryKey, "0")
	if e.svc.MemoryEnabled(bg) {
		t.Fatal("0 is off")
	}
	e.st.SetSetting(bg, MemoryKey, "1")
	if !e.svc.MemoryEnabled(bg) {
		t.Fatal("1 is on")
	}
}
