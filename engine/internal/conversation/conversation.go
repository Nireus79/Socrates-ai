// Package conversation is the turn: what happens when the user sends a message to a philosopher.
//
//	safety judgement of the message -> a plain decision -> (the reply of the philosopher -> the check before the user sees it) -> one transaction
//
// A turn that fails keeps nothing. The words of a refused message or of a crisis are never kept, only a marker of the kind. Nothing here
// logs what anyone wrote.
package conversation

import (
	"context"
	"errors"
	"log"
	"strings"
	"sync"
	"time"

	"socrates/internal/llm"
	"socrates/internal/philosophers"
	"socrates/internal/safety"
	"socrates/internal/store"
	"socrates/internal/suggest"
)

const (
	MaxMessageChars  = 6000 // the longest message a user may send
	contextMessages  = 24   // how many earlier messages the philosopher is given
	storedForContext = 40   // how many are read from the store to build that context and the judge's
	summaryEvery     = 8    // new messages before the running summary is written again
	// AgeKey is the setting that records the user's one-time confirmation of the age limit.
	AgeKey = "age.confirmed"
)

var (
	ErrEmpty           = errors.New("the message is empty")
	ErrTooLong         = errors.New("the message is too long")
	ErrAgeNotConfirmed = errors.New("the age limit has not been confirmed")
	ErrNoProvider      = errors.New("no AI provider has been set up")
	ErrNotNow          = errors.New("suggestions are not offered now")
	ErrNotFound        = store.ErrNotFound
	ErrLocked          = errors.New("this philosopher is not unlocked")
)

// Completer is what the turn needs of the model client (the llm.Client, or a mock in tests).
type Completer interface {
	Complete(ctx context.Context, req llm.Request) (*llm.Response, error)
}

// ModelSource gives the client built from the user's settings, or ErrNoProvider.
type ModelSource func(ctx context.Context) (Completer, error)

// Service runs the turns.
type Service struct {
	st     *store.Store
	models ModelSource
	roster *philosophers.Registry

	mu    sync.Mutex
	locks map[string]*sync.Mutex
	bg    sync.WaitGroup
}

func New(st *store.Store, models ModelSource, roster *philosophers.Registry) *Service {
	return &Service{st: st, models: models, roster: roster, locks: map[string]*sync.Mutex{}}
}

// Wait returns when the background work of finished turns (titles, summaries) is done. Used by tests and by a clean shutdown.
func (s *Service) Wait() { s.bg.Wait() }

func (s *Service) lock(id string) func() {
	s.mu.Lock()
	m := s.locks[id]
	if m == nil {
		m = &sync.Mutex{}
		s.locks[id] = m
	}
	s.mu.Unlock()
	m.Lock()
	return m.Unlock
}

// ---- the age limit

func (s *Service) AgeConfirmed(ctx context.Context) (bool, error) {
	v, ok, err := s.st.Setting(ctx, AgeKey)
	return ok && v == "1", err
}

func (s *Service) ConfirmAge(ctx context.Context) error { return s.st.SetSetting(ctx, AgeKey, "1") }

// ---- conversations

// View is a message as the screens see it. A marker carries the fixed text it stands for.
type View struct {
	ID        string    `json:"id"`
	Seq       int       `json:"seq"`
	Role      string    `json:"role"` // "user", "philosopher" or "marker"
	Text      string    `json:"text"`
	Kind      string    `json:"kind,omitempty"` // for a marker: what it stands for
	Suggested bool      `json:"suggested,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

func view(m store.Message) View {
	v := View{ID: m.ID, Seq: m.Seq, Role: m.Role, Text: m.Content, CreatedAt: m.CreatedAt, Suggested: m.Suggested}
	if m.Role == "marker" {
		v.Kind, v.Text = m.Content, safety.TextFor(safety.MarkerKind(m.Content))
	}
	return v
}

// Start opens a conversation with a philosopher.
func (s *Service) Start(ctx context.Context, philosopherID string) (store.Conversation, error) {
	p, err := s.roster.Get(philosopherID)
	if err != nil {
		return store.Conversation{}, err
	}
	if !p.Unlocked {
		return store.Conversation{}, ErrLocked
	}
	return s.st.CreateConversation(ctx, p.ID)
}

// History is a conversation with its messages.
func (s *Service) History(ctx context.Context, id string) (store.Conversation, []View, error) {
	c, err := s.st.Conversation(ctx, id)
	if err != nil {
		return store.Conversation{}, nil, err
	}
	msgs, err := s.st.Messages(ctx, id)
	if err != nil {
		return store.Conversation{}, nil, err
	}
	out := make([]View, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, view(m))
	}
	return c, out, nil
}

func (s *Service) Delete(ctx context.Context, id string) error {
	defer s.lock(id)()
	return s.st.DeleteConversation(ctx, id)
}

// ---- the turn

// Outcome says what the turn came to.
type Outcome string

const (
	OutcomeReplied     Outcome = "replied"     // the philosopher answered
	OutcomeFixed       Outcome = "fixed"       // the application answered with a fixed text (a refusal, the crisis message, the check-in)
	OutcomeUnavailable Outcome = "unavailable" // no answer could be had; nothing was saved
	OutcomeEnded       Outcome = "ended"       // the conversation was ended and deleted
)

// Result is what the turn did.
type Result struct {
	Outcome Outcome `json:"outcome"`
	Added   []View  `json:"added"`          // the messages the turn kept, in order
	Text    string  `json:"text,omitempty"` // the fixed text, when nothing was kept to show it
	// ProviderKind, for an unavailable turn: what went wrong with the provider (auth, rate_limit, overloaded, ...), when it is known.
	ProviderKind string `json:"providerKind,omitempty"`
}

func unavailable(err error) *Result {
	r := &Result{Outcome: OutcomeUnavailable, Added: []View{}, Text: safety.TextUnavailable}
	var le *llm.Error
	if errors.As(err, &le) {
		r.ProviderKind = string(le.Kind)
	}
	return r
}

func toLLM(msgs []store.Message) []llm.Message {
	var out []llm.Message
	for _, m := range msgs {
		switch m.Role {
		case "user":
			out = append(out, llm.Message{Role: "user", Content: m.Content})
		case "philosopher":
			out = append(out, llm.Message{Role: "assistant", Content: m.Content})
		}
	}
	return out
}

// contextFor builds what the philosopher reads: the recent turns (markers left out), starting with the user and ending with the newest
// message, with neighbours of the same speaker joined (a refused message leaves two replies side by side).
func contextFor(msgs []store.Message, newest string) []llm.Message {
	all := toLLM(msgs)
	for len(all) > 0 && all[0].Role != "user" {
		all = all[1:]
	}
	if len(all) > contextMessages {
		all = all[len(all)-contextMessages:]
		for len(all) > 0 && all[0].Role != "user" {
			all = all[1:]
		}
	}
	all = append(all, llm.Message{Role: "user", Content: newest})
	var out []llm.Message
	for _, m := range all {
		if n := len(out); n > 0 && out[n-1].Role == m.Role {
			out[n-1].Content += "\n\n" + m.Content
			continue
		}
		out = append(out, m)
	}
	return out
}

// Send runs one turn.
func (s *Service) Send(ctx context.Context, conversationID, text string) (*Result, error) {
	return s.send(ctx, conversationID, text, false)
}

// SendSuggested runs a turn for a message the user chose (or started from) among the suggestions: it is kept marked as such, and the
// philosopher is told to take it as tentative.
func (s *Service) SendSuggested(ctx context.Context, conversationID, text string) (*Result, error) {
	return s.send(ctx, conversationID, text, true)
}

const suggestedNote = `The person's newest message was one of several wordings you offered to help them answer, which they chose or started from. It is a starting point, not yet their settled view: take it seriously, and when it matters you may ask whether it is really what they think.`

func (s *Service) send(ctx context.Context, conversationID, text string, suggested bool) (*Result, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, ErrEmpty
	}
	if len([]rune(text)) > MaxMessageChars {
		return nil, ErrTooLong
	}
	if ok, err := s.AgeConfirmed(ctx); err != nil {
		return nil, err
	} else if !ok {
		return nil, ErrAgeNotConfirmed
	}
	client, err := s.models(ctx)
	if err != nil {
		return nil, err
	}
	defer s.lock(conversationID)()

	conv, err := s.st.Conversation(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	ph, err := s.roster.Get(conv.PhilosopherID)
	if err != nil {
		return nil, err
	}
	ss, err := s.st.SafetyState(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	state := safety.State{PendingCheckIn: ss.PendingCheckin, ReflectiveQuestionAsked: ss.ReflectiveQuestionAsked}
	history, err := s.st.RecentMessages(ctx, conversationID, storedForContext)
	if err != nil {
		return nil, err
	}

	// 1. The safety judgement comes first and fails closed.
	verdict, err := safety.Judge(ctx, client, toLLM(history), text)
	if err != nil {
		return unavailable(err), nil
	}
	d := safety.Decide(verdict, state)

	// 2. What the decision says.
	switch d.Action {
	case safety.ActionEndForAge:
		if err := s.st.DeleteConversation(ctx, conversationID); err != nil {
			return nil, err
		}
		return &Result{Outcome: OutcomeEnded, Added: []View{}, Text: safety.TextEndForAge}, nil

	case safety.ActionCrisis, safety.ActionRefuseHarm, safety.ActionRefuseMinorContent, safety.ActionRefuseIllegal:
		return s.fixed(ctx, conversationID, d, nil, false)

	case safety.ActionCheckIn:
		return s.fixed(ctx, conversationID, d, &text, suggested)

	case safety.ActionAskBeforeRefusing:
		q, err := s.guarded(ctx, client, ph.System()+"\n\n"+philosophers.ReflectiveQuestion, contextFor(history, text), toLLM(history), text)
		if err != nil && !errors.Is(err, errHeldBack) {
			return unavailable(err), nil
		}
		if err != nil { // the question could not be made safe: the plain refusal stands
			d.Action, d.Marker = safety.ActionRefuseIllegal, safety.MarkerRefuseIllegal
			return s.fixed(ctx, conversationID, d, nil, false)
		}
		return s.commit(ctx, conversationID, d, nil, q, false)

	default: // proceed, proceed gently
		system := ph.System()
		if d.Action == safety.ActionProceedGently {
			system += "\n\n" + philosophers.Gentle
		}
		if suggested {
			system += "\n\n" + suggestedNote
		}
		if sum, _, ok, _ := s.st.Summary(ctx, conversationID); ok {
			system += "\n\nWhat has been said earlier in this conversation, in short:\n" + sum
		}
		reply, err := s.guarded(ctx, client, system, contextFor(history, text), toLLM(history), text)
		switch {
		case errors.Is(err, errHeldBack): // held back twice: the user's message is kept, the reply is a fixed text
			d.Marker = safety.MarkerGateFallback
			return s.commit(ctx, conversationID, d, &text, "", suggested)
		case err != nil:
			return unavailable(err), nil
		}
		res, err := s.commit(ctx, conversationID, d, &text, reply, suggested)
		if err == nil && res.Outcome == OutcomeReplied {
			s.afterTurn(client, conv)
		}
		return res, err
	}
}

// fixed keeps only what a refusal, a crisis or a check-in may keep and answers with the fixed text.
func (s *Service) fixed(ctx context.Context, id string, d safety.Decision, userText *string, suggested bool) (*Result, error) {
	res, err := s.commit(ctx, id, d, userText, "", suggested)
	if err == nil {
		res.Outcome = OutcomeFixed
		res.Text = safety.TextFor(d.Marker)
	}
	return res, err
}

// commit writes the turn in one transaction: the user's message when the decision allows it, the marker when there is one, the reply
// when there is one, and the safety state. When the transaction fails nothing is kept.
func (s *Service) commit(ctx context.Context, id string, d safety.Decision, userText *string, reply string, suggested bool) (*Result, error) {
	res := &Result{Outcome: OutcomeReplied, Added: []View{}}
	err := s.st.Update(ctx, func(o *store.Ops) error {
		add := func(role, content string) error {
			m, err := o.AppendMessage(ctx, id, role, content)
			if err == nil {
				res.Added = append(res.Added, view(m))
			}
			return err
		}
		if d.SaveMessage && userText != nil {
			var m store.Message
			var err error
			if suggested {
				m, err = o.AppendSuggested(ctx, id, *userText)
			} else {
				m, err = o.AppendMessage(ctx, id, "user", *userText)
			}
			if err != nil {
				return err
			}
			res.Added = append(res.Added, view(m))
		}
		if d.Marker != "" {
			if err := add("marker", string(d.Marker)); err != nil {
				return err
			}
		}
		if reply != "" {
			if err := add("philosopher", reply); err != nil {
				return err
			}
		}
		return o.SetSafetyState(ctx, id, store.SafetyState{PendingCheckin: d.NewState.PendingCheckIn, ReflectiveQuestionAsked: d.NewState.ReflectiveQuestionAsked})
	})
	if err != nil {
		return nil, err
	}
	if reply == "" {
		res.Outcome = OutcomeFixed
		res.Text = safety.TextFor(d.Marker)
	}
	return res, nil
}

var errHeldBack = errors.New("the reply was held back twice")

// guarded writes a reply and checks it before anyone sees it. A draft that is flagged is written again once, with the reason; flagged
// twice, errHeldBack. A draft that cannot be checked, or a model that fails, is an error of the provider kind: nothing is shown.
func (s *Service) guarded(ctx context.Context, client Completer, system string, msgs []llm.Message, judgeCtx []llm.Message, newest string) (string, error) {
	write := func(extra string) (string, error) {
		sys := system
		if extra != "" {
			sys += "\n\n" + extra
		}
		resp, err := client.Complete(ctx, llm.Request{Role: llm.RoleReply, System: sys, Messages: msgs, MaxTokens: 4000})
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(resp.Text), nil
	}
	gateContext := append(append([]llm.Message(nil), judgeCtx...), llm.Message{Role: "user", Content: newest})
	extra := ""
	for attempt := 0; attempt < 2; attempt++ {
		draft, err := write(extra)
		if err != nil {
			return "", err
		}
		gv, err := safety.CheckReply(ctx, client, gateContext, draft)
		if err != nil {
			return "", err
		}
		if !gv.Flagged {
			return draft, nil
		}
		log.Printf("[conversation] a draft was held back (%s)", gv.Category) // the category only, never the draft or the reason
		extra = philosophers.HeldBack(gv.Reason)
	}
	return "", errHeldBack
}

// afterTurn writes the title after the first exchange and the running summary every few messages. It runs in the background, after the
// turn is kept, and a failure changes nothing about the turn.
func (s *Service) afterTurn(client Completer, conv store.Conversation) {
	s.bg.Add(1)
	go func() {
		defer s.bg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		unlock := s.lock(conv.ID)
		defer unlock()
		msgs, err := s.st.Messages(ctx, conv.ID)
		if err != nil || len(msgs) == 0 {
			return
		}
		if c, err := s.st.Conversation(ctx, conv.ID); err == nil && c.Title == "" {
			s.title(ctx, client, c.ID, msgs)
		}
		s.summary(ctx, client, conv.ID, msgs)
	}()
}

func (s *Service) title(ctx context.Context, client Completer, id string, msgs []store.Message) {
	var first string
	for _, m := range msgs {
		if m.Role == "user" {
			first = m.Content
			break
		}
	}
	if first == "" {
		return
	}
	resp, err := client.Complete(ctx, llm.Request{Role: llm.RoleFast, System: "Write a title of at most six words for a conversation that begins with the user's message below. Use the language of the message. No quotation marks and no full stop. Write only the title.", Messages: []llm.Message{{Role: "user", Content: first}}, MaxTokens: 60})
	if err != nil {
		log.Printf("[conversation] no title could be written")
		return
	}
	t := strings.Trim(strings.TrimSpace(resp.Text), "\"“”'.")
	if r := []rune(t); len(r) > 80 {
		t = string(r[:80])
	}
	if t != "" {
		s.st.SetTitle(ctx, id, t)
	}
}

func (s *Service) summary(ctx context.Context, client Completer, id string, msgs []store.Message) {
	last := msgs[len(msgs)-1].Seq
	prev, through, has, err := s.st.Summary(ctx, id)
	if err != nil || (has && last-through < summaryEvery) || (!has && last < summaryEvery) {
		return
	}
	var b strings.Builder
	if has {
		b.WriteString("The summary so far:\n" + prev + "\n\nWhat was said after it:\n")
	} else {
		b.WriteString("The conversation:\n")
	}
	for _, m := range msgs {
		if m.Seq <= through || m.Role == "marker" {
			continue
		}
		who := "User"
		if m.Role == "philosopher" {
			who = "Philosopher"
		}
		b.WriteString(who + ": " + m.Content + "\n")
	}
	resp, err := client.Complete(ctx, llm.Request{Role: llm.RoleFast, System: "You keep the running summary of a conversation between a person and a philosopher. Write it in plain prose, at most 150 words: what has been discussed, what the person has said about themselves and what they believe, and where the conversation stands. Write only the summary.", Messages: []llm.Message{{Role: "user", Content: b.String()}}, MaxTokens: 400})
	if err != nil {
		log.Printf("[conversation] no summary could be written")
		return
	}
	if t := strings.TrimSpace(resp.Text); t != "" {
		s.st.SetSummary(ctx, id, t, last)
	}
}

// Suggest offers a few ways to answer the philosopher's last message. It is not offered after a crisis step, a refusal or while a check-in
// is pending, and only when the conversation ends with the philosopher's own message.
func (s *Service) Suggest(ctx context.Context, conversationID string, exclude []string) (*suggest.Result, error) {
	if ok, err := s.AgeConfirmed(ctx); err != nil {
		return nil, err
	} else if !ok {
		return nil, ErrAgeNotConfirmed
	}
	client, err := s.models(ctx)
	if err != nil {
		return nil, err
	}
	defer s.lock(conversationID)()
	if _, err := s.st.Conversation(ctx, conversationID); err != nil {
		return nil, err
	}
	ss, err := s.st.SafetyState(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	history, err := s.st.RecentMessages(ctx, conversationID, storedForContext)
	if err != nil {
		return nil, err
	}
	if ss.PendingCheckin || len(history) == 0 || history[len(history)-1].Role != "philosopher" {
		if ss.PendingCheckin || (len(history) > 0 && history[len(history)-1].Role == "marker") {
			return nil, ErrNotNow
		}
		return nil, suggest.ErrNothingToAnswer
	}
	msgs := contextFor(history, "")
	msgs = msgs[:len(msgs)-1] // contextFor appends the newest user message; there is none here
	return suggest.Generate(ctx, client, msgs, exclude)
}
