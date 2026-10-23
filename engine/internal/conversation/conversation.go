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

	"socrates/internal/entitlements"
	"socrates/internal/ledger"
	"socrates/internal/llm"
	"socrates/internal/philosophers"
	"socrates/internal/progress"
	"socrates/internal/safety"
	"socrates/internal/store"
	"socrates/internal/suggest"
)

const (
	MaxMessageChars  = 6000 // the longest message a user may send
	contextMessages  = 24   // how many earlier messages the philosopher is given
	storedForContext = 40   // how many are read from the store to build that context and the judge's
	summaryEvery     = 8    // new messages before the running summary is written again
	// MemoryKey is the setting that says whether the philosophers keep a ledger of what the person tells them ("0" turns it off; it is on
	// unless the person turned it off).
	MemoryKey = "memory.enabled"
	// AgeKey is the setting that records the user's one-time confirmation of the age limit.
	AgeKey = "age.confirmed"
)

var (
	ErrEmpty           = errors.New("the message is empty")
	ErrTooLong         = errors.New("the message is too long")
	ErrAgeNotConfirmed = errors.New("the age limit has not been confirmed")
	ErrNoProvider      = errors.New("no AI provider has been set up")
	ErrNotNow          = errors.New("suggestions are not offered now")
	ErrNotEntitled     = errors.New("this is not part of what is unlocked")
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
	rights entitlements.Source

	mu      sync.Mutex
	locks   map[string]*sync.Mutex // one turn at a time per conversation
	now     func() time.Time       // the clock, replaceable in tests
	bgLocks map[string]*sync.Mutex // one piece of background work at a time per conversation; never held by a turn
	bg      sync.WaitGroup
}

func New(st *store.Store, models ModelSource, roster *philosophers.Registry) *Service {
	return &Service{st: st, models: models, roster: roster, rights: entitlements.Beta{}, now: time.Now, locks: map[string]*sync.Mutex{}, bgLocks: map[string]*sync.Mutex{}}
}

// SetEntitlements replaces the source that says what the person may use (the free beta grants everything).
func (s *Service) SetEntitlements(e entitlements.Source) { s.rights = e }

// Allowed reports whether a feature may be used; a source that cannot tell counts as no.
func (s *Service) Allowed(ctx context.Context, f entitlements.Feature) bool {
	ok, err := s.rights.Allowed(ctx, f)
	return err == nil && ok
}

// Wait returns when the background work of finished turns (titles, summaries) is done. Used by tests and by a clean shutdown.
func (s *Service) Wait() { s.bg.Wait() }

func (s *Service) lock(id string) func() { return s.take(s.locks, id) }

func (s *Service) take(m map[string]*sync.Mutex, id string) func() {
	s.mu.Lock()
	l := m[id]
	if l == nil {
		l = &sync.Mutex{}
		m[id] = l
	}
	s.mu.Unlock()
	l.Lock()
	return l.Unlock
}

// forget drops the locks of a conversation that no longer exists.
func (s *Service) forget(id string) {
	s.mu.Lock()
	delete(s.locks, id)
	delete(s.bgLocks, id)
	s.mu.Unlock()
}

// ---- the age limit

func (s *Service) AgeConfirmed(ctx context.Context) (bool, error) {
	v, ok, err := s.st.Setting(ctx, AgeKey)
	return ok && v == "1", err
}

// MemoryEnabled reports whether the ledger is kept. It is on unless the person turned it off, and unreadable counts as off.
func (s *Service) MemoryEnabled(ctx context.Context) bool {
	v, ok, err := s.st.Setting(ctx, MemoryKey)
	return err == nil && (!ok || v != "0")
}

// SetMemoryEnabled turns the ledger on or off. Turning it off does not delete what is kept (the person does that on the Memory screen),
// but nothing more is added and nothing kept is told to the philosophers.
func (s *Service) SetMemoryEnabled(ctx context.Context, on bool) error {
	v := "1"
	if !on {
		v = "0"
	}
	return s.st.SetSetting(ctx, MemoryKey, v)
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
	if !s.Allowed(ctx, entitlements.Philosopher(p.ID)) {
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

// Delete removes a conversation. By default everything the philosopher learnt in it goes too; with keepMemory the entries stay with him,
// no longer tied to the conversation, until the person deletes them on the Memory screen.
func (s *Service) Delete(ctx context.Context, id string, keepMemory bool) error {
	unlock := s.lock(id)
	var err error
	if keepMemory {
		err = s.st.DeleteConversationKeepingMemory(ctx, id)
	} else {
		err = s.st.DeleteConversation(ctx, id)
	}
	unlock()
	if err == nil {
		s.forget(id)
	}
	return err
}

// Learned is how many things the philosophers learnt in a conversation (the screens ask whether to keep them when it is deleted).
func (s *Service) Learned(ctx context.Context, id string) int {
	n, _ := s.st.CountEntriesForConversation(ctx, id)
	return n
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
	// Offer, when a challenge can be taken up or is waiting for an answer (see challenge.go).
	Offer *OfferView `json:"offer,omitempty"`
	// Milestone, when a challenge was finished since the person last heard of one: a plain sentence, never a number.
	Milestone string `json:"milestone,omitempty"`
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
	return s.send(ctx, conversationID, text, false, "")
}

// SendSuggested runs a turn for a message the user chose (or started from) among the suggestions: it is kept marked as such, and the
// philosopher is told to take it as tentative.
func (s *Service) SendSuggested(ctx context.Context, conversationID, text string) (*Result, error) {
	return s.send(ctx, conversationID, text, true, "")
}

const suggestedNote = `The person's newest message was one of several wordings you offered to help them answer, which they chose or started from. It is a starting point, not yet their settled view: take it seriously, and when it matters you may ask whether it is really what they think.`

// send is the turn. note, when given, is added to what the philosopher is told for this reply only.
func (s *Service) send(ctx context.Context, conversationID, text string, suggested bool, note string) (*Result, error) {
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
	if !s.Allowed(ctx, entitlements.Philosopher(ph.ID)) {
		return nil, ErrLocked
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
			d.NewState.ReflectiveQuestionAsked = state.ReflectiveQuestionAsked // no question was asked, so none is used up
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
		if note != "" {
			system += "\n\n" + note
		}
		if s.MemoryEnabled(ctx) {
			system += "\n\n" + ledger.GetToKnow
			if known, err := s.st.Entries(ctx, ph.ID, false); err == nil {
				if c := ledger.Context(known); c != "" {
					system += "\n\n" + c
				}
			}
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
			// what someone says while they may be hurting is not written into the ledger
			userSeq := 0
			if d.Action != safety.ActionProceedGently {
				for _, m := range res.Added {
					if m.Role == "user" {
						userSeq = m.Seq
					}
				}
			}
			s.afterTurn(client, conv, userSeq)
			res.Offer = s.OfferState(ctx, conv.ID)
			if n, err := s.st.TakeAnnouncements(ctx, conv.ID); err == nil && n > 0 {
				res.Milestone = "You and " + ph.Name + " thought one question all the way through."
			}
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
		if d.Marker != "" && safety.TextFor(d.Marker) != "" {
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

// trimCutOff ends a reply that the model stopped for length at its last whole sentence (when it has one).
func trimCutOff(t string) string {
	t = strings.TrimSpace(t)
	if t == "" || strings.ContainsAny(t[len(t)-1:], ".?!\"") {
		return t
	}
	if i := strings.LastIndexAny(t, ".?!"); i > 0 {
		return strings.TrimSpace(t[:i+1])
	}
	return t
}

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
		text := strings.TrimSpace(resp.Text)
		if resp.StopReason == "max_tokens" || resp.StopReason == "length" {
			text = trimCutOff(text)
		}
		if text == "" {
			return "", &llm.Error{Kind: llm.KindEmpty, Reason: "the model returned no text"} // a blank turn is never shown or kept
		}
		return text, nil
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

// afterTurn writes the title after the first exchange, the running summary every few messages, and the ledger from the message of this
// turn (userSeq, 0 for none: a turn where the person may be hurting is not remembered). It runs in the background after the turn is kept;
// a failure changes nothing about the turn. It never holds the lock of a turn, so the next message is never made to wait for it.
func (s *Service) afterTurn(client Completer, conv store.Conversation, userSeq int) {
	s.bg.Add(1)
	go func() {
		defer s.bg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		defer s.take(s.bgLocks, conv.ID)()
		if c, err := s.st.Conversation(ctx, conv.ID); err != nil {
			return // deleted meanwhile
		} else if c.Title == "" {
			if first, err := s.st.FirstMessages(ctx, conv.ID, 4); err == nil {
				s.title(ctx, client, c.ID, first)
			}
		}
		s.summary(ctx, client, conv.ID)
		if userSeq > 0 && s.MemoryEnabled(ctx) {
			s.remember(ctx, client, conv, userSeq)
		}
	}()
}

// remember updates the ledger from the person's message userSeq (not "the newest message", which may belong to a later turn). The model
// proposes, ledger.Validate disposes: only operations resting on the person's own words in their last few messages are applied, all in one
// transaction. Any failure changes nothing.
func (s *Service) remember(ctx context.Context, client Completer, conv store.Conversation, userSeq int) {
	recentMsgs, err := s.st.RecentMessages(ctx, conv.ID, 12)
	if err != nil {
		return
	}
	at := -1
	for i, m := range recentMsgs {
		if m.Seq == userSeq && m.Role == "user" {
			at = i
		}
	}
	if at < 0 {
		return
	}
	newest := recentMsgs[at]
	var userTexts []string
	for i := at; i >= 0 && len(userTexts) < 3; i-- {
		if recentMsgs[i].Role == "user" {
			userTexts = append(userTexts, recentMsgs[i].Content)
		}
	}
	from := at - 3
	if from < 0 {
		from = 0
	}
	recent := toLLM(recentMsgs[from:at])
	known, err := s.ledgerFor(ctx, conv.PhilosopherID)
	if err != nil {
		return
	}
	cur, haveCur, err := s.st.CurrentInquiry(ctx, conv.ID)
	if err != nil {
		return
	}
	var inInquiry map[string]bool
	current := ""
	if haveCur {
		if inInquiry, err = s.st.InquiryEntryIDs(ctx, cur.ID); err != nil {
			return
		}
		current = cur.Question
		known = inquiryLast(known, inInquiry) // the model sees the newest entries, so the ones the inquiry rests on go last
	}
	reading, shown, err := ledger.Extract(ctx, client, ledger.Input{Entries: known, InInquiry: inInquiry, Current: current, Recent: recent, Newest: newest.Content})
	if err != nil {
		log.Printf("[conversation] the ledger could not be updated")
		return
	}
	if reading.Offer {
		s.noteOffer(ctx, conv.ID, newest.Seq)
	}
	action := ledger.ValidateInquiry(reading.Inquiry, current, shown, inInquiry, userTexts)
	changes, _ := ledger.Validate(reading.Ops, shown, userTexts, newest.Content, newest.Suggested, conv.PhilosopherID, conv.ID)
	if action == nil && len(changes) == 0 {
		// nothing new was said, but an inquiry that qualified beyond the day's limit is counted on the first turn after it
		if haveCur && !cur.Completed && cur.Banked >= int(progress.Default.Threshold*1000) {
			if err := s.st.Update(ctx, func(o *store.Ops) error { return s.score(ctx, o, cur) }); err != nil {
				log.Printf("[conversation] the ledger could not be saved")
			}
		}
		return
	}
	err = s.st.Update(ctx, func(o *store.Ops) error {
		inq, have := cur, haveCur
		if action != nil {
			had, err := o.HasInquiry(ctx, conv.ID)
			if err != nil {
				return err
			}
			next := store.Inquiry{PhilosopherID: conv.PhilosopherID, ConversationID: conv.ID, Question: action.Question, Quote: action.Quote}
			if action.Deepening {
				next.ParentID = cur.ID
			}
			if inq, err = o.CreateInquiry(ctx, next); err != nil {
				return err
			}
			have = true
			if had { // a later inquiry builds on the entries it names
				for _, e := range action.Inherit {
					if err := o.LinkEntries(ctx, inq.ID, e.ID); err != nil {
						return err
					}
				}
			} else if err := o.LinkOrphans(ctx, inq.ID, conv.ID); err != nil { // the first one takes what was noted before it was recognised
				return err
			}
		}
		kindOf := map[string]string{}
		for _, e := range shown {
			kindOf[e.ID] = e.Kind
		}
		link := func(id, kind string) error { // facts are about the person, not about an inquiry
			if !have || kind == store.KindFact {
				return nil
			}
			return o.LinkEntries(ctx, inq.ID, id)
		}
		for _, c := range changes {
			var err error
			switch {
			case c.New != nil:
				var added store.Entry
				if added, err = o.AddEntry(ctx, *c.New); err == nil {
					err = link(added.ID, added.Kind)
				}
			case c.Revise != nil:
				if err = o.ReviseEntry(ctx, c.Revise.ID, c.Revise.Text, c.Revise.Detail, c.Revise.Status); err == nil && c.Revise.Examined > 0 {
					err = o.SetExamined(ctx, c.Revise.ID, c.Revise.Examined)
				}
				if err == nil {
					err = link(c.Revise.ID, kindOf[c.Revise.ID])
				}
			}
			if errors.Is(err, store.ErrBadEntry) || errors.Is(err, store.ErrNotFound) {
				continue // one bad change does not spoil the rest
			}
			if err != nil {
				return err
			}
		}
		if !have {
			return nil
		}
		return s.score(ctx, o, inq)
	})
	if err != nil {
		log.Printf("[conversation] the ledger could not be saved")
	}
}

// score reads the maturity of an inquiry, banks the best it has reached and, when it qualifies and the day's limit allows, records the
// challenge. An inquiry that qualifies beyond the limit is counted on a later turn.
func (s *Service) score(ctx context.Context, o *store.Ops, inq store.Inquiry) error {
	entries, err := o.InquiryEntries(ctx, inq.ID)
	if err != nil {
		return err
	}
	r := progress.Score(entries, progress.Default)
	if err := o.Bank(ctx, inq.ID, r.Thousandths()); err != nil {
		return err
	}
	if inq.Completed || !r.Complete(progress.Default) {
		return nil
	}
	today, err := o.ChallengesSince(ctx, progress.DayStart(s.now()))
	if err != nil || today >= progress.MaxPerDay {
		return err
	}
	return o.CompleteInquiry(ctx, inq.ID)
}

// inquiryLast puts the entries an inquiry rests on after the others, keeping the order within each group.
func inquiryLast(entries []store.Entry, in map[string]bool) []store.Entry {
	out := make([]store.Entry, 0, len(entries))
	for _, e := range entries {
		if !in[e.ID] {
			out = append(out, e)
		}
	}
	for _, e := range entries {
		if in[e.ID] {
			out = append(out, e)
		}
	}
	return out
}

func (s *Service) ledgerFor(ctx context.Context, philosopherID string) ([]store.Entry, error) {
	return s.st.Entries(ctx, philosopherID, false)
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

func (s *Service) summary(ctx context.Context, client Completer, id string) {
	prev, through, has, err := s.st.Summary(ctx, id)
	if err != nil {
		return
	}
	msgs, err := s.st.MessagesAfter(ctx, id, through)
	if err != nil || len(msgs) == 0 {
		return
	}
	last := msgs[len(msgs)-1].Seq
	if (has && last-through < summaryEvery) || (!has && last < summaryEvery) {
		return
	}
	var b strings.Builder
	if has {
		b.WriteString("The summary so far:\n" + prev + "\n\nWhat was said after it:\n")
	} else {
		b.WriteString("The conversation:\n")
	}
	for _, m := range msgs {
		if m.Role == "marker" {
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
	if !s.Allowed(ctx, entitlements.Suggestions) {
		return nil, ErrNotEntitled
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
