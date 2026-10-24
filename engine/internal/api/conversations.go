package api

import (
	"context"
	"errors"
	"log"
	"net/http"

	"socrates/internal/conversation"
	"socrates/internal/entitlements"
	"socrates/internal/llm"
	"socrates/internal/philosophers"
	"socrates/internal/progress"
	"socrates/internal/safety"
	"socrates/internal/store"
	"socrates/internal/suggest"
	"socrates/internal/transport"
)

// knownProvider is false for an empty name and for one this version no longer offers (a file written when a local model was possible).
func knownProvider(p string) bool { return p == "claude" || p == "openai" }

// models builds the client from the user's stored settings, for each turn (a changed key or provider applies at once).
func (h *handler) models(ctx context.Context) (conversation.Completer, error) {
	s, _, err := h.loadCtx(ctx)
	if err != nil {
		return nil, err
	}
	if !knownProvider(s.Provider) || s.APIKey == "" || s.FastModel == "" {
		return nil, conversation.ErrNoProvider
	}
	return llm.NewClient(s, h.d.LLMOptions...)
}

func (h *handler) profile(w http.ResponseWriter, r *http.Request) {
	age, err := h.conv.AgeConfirmed(r.Context())
	if err != nil {
		fail(w, http.StatusInternalServerError, "the profile could not be read")
		return
	}
	s, _, err := h.load(r)
	if err != nil {
		fail(w, http.StatusInternalServerError, "the profile could not be read")
		return
	}
	transport.WriteJSON(w, http.StatusOK, map[string]any{
		"ageConfirmed": age,
		"hasProvider":  knownProvider(s.Provider) && s.APIKey != "",
	})
}

func (h *handler) confirmAge(w http.ResponseWriter, r *http.Request) {
	if err := h.conv.ConfirmAge(r.Context()); err != nil {
		fail(w, http.StatusInternalServerError, "the confirmation could not be saved")
		return
	}
	transport.WriteJSON(w, http.StatusOK, map[string]bool{"ageConfirmed": true})
}

func (h *handler) philosophers(w http.ResponseWriter, r *http.Request) {
	type need struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Done     int    `json:"done"`
		Required int    `json:"required"`
	}
	type view struct {
		philosophers.Philosopher
		Unlocked   bool   `json:"unlocked"`   // may the person talk with him now
		Challenges int    `json:"challenges"` // finished with him
		Earned     bool   `json:"earned"`     // unlocked by progress alone, whether or not the rules are enforced
		Needs      []need `json:"needs,omitempty"`
	}
	lang := h.conv.Lang(r.Context())
	roster := h.d.Roster.All()
	counts, err := h.d.Store.ChallengeCounts(r.Context())
	if err != nil {
		fail(w, http.StatusInternalServerError, "the roster could not be read")
		return
	}
	var out []view
	for i, st := range progress.StatusOf(roster, counts) {
		v := view{Philosopher: roster[i].Localized(lang), Unlocked: h.conv.Allowed(r.Context(), entitlements.Philosopher(st.ID)), Challenges: st.Challenges, Earned: st.Unlocked}
		for _, m := range st.Missing {
			p, _ := h.d.Roster.Get(m.Philosopher)
			v.Needs = append(v.Needs, need{ID: m.Philosopher, Name: p.Localized(lang).Name, Done: m.Done, Required: progress.Required})
		}
		out = append(out, v)
	}
	transport.WriteJSON(w, http.StatusOK, map[string]any{"philosophers": out})
}

type conversationJSON struct {
	ID            string `json:"id"`
	PhilosopherID string `json:"philosopherId"`
	Title         string `json:"title"`
	CreatedAt     int64  `json:"createdAt"`
	UpdatedAt     int64  `json:"updatedAt"`
}

func toJSON(c store.Conversation) conversationJSON {
	return conversationJSON{ID: c.ID, PhilosopherID: c.PhilosopherID, Title: c.Title, CreatedAt: c.CreatedAt.Unix(), UpdatedAt: c.UpdatedAt.Unix()}
}

func (h *handler) startConversation(w http.ResponseWriter, r *http.Request) {
	var body struct {
		PhilosopherID string `json:"philosopherId"`
	}
	if !transport.ReadBody(w, r, &body) {
		return
	}
	c, err := h.conv.Start(r.Context(), body.PhilosopherID)
	switch {
	case errors.Is(err, philosophers.ErrUnknown):
		fail(w, http.StatusBadRequest, "There is no such philosopher.")
	case errors.Is(err, conversation.ErrLocked):
		fail(w, http.StatusForbidden, "This philosopher is not unlocked yet.")
	case err != nil:
		fail(w, http.StatusInternalServerError, "the conversation could not be started")
	default:
		transport.WriteJSON(w, http.StatusCreated, toJSON(c))
	}
}

func (h *handler) listConversations(w http.ResponseWriter, r *http.Request) {
	list, err := h.d.Store.Conversations(r.Context())
	if err != nil {
		fail(w, http.StatusInternalServerError, "the conversations could not be read")
		return
	}
	out := make([]conversationJSON, 0, len(list))
	for _, c := range list {
		out = append(out, toJSON(c))
	}
	transport.WriteJSON(w, http.StatusOK, map[string]any{"conversations": out})
}

func (h *handler) getConversation(w http.ResponseWriter, r *http.Request) {
	c, msgs, err := h.conv.History(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		fail(w, http.StatusNotFound, "There is no such conversation.")
		return
	}
	if err != nil {
		fail(w, http.StatusInternalServerError, "the conversation could not be read")
		return
	}
	h.localizeViews(h.conv.Lang(r.Context()), msgs)
	transport.WriteJSON(w, http.StatusOK, map[string]any{"conversation": toJSON(c), "messages": msgs, "offer": h.conv.OfferState(r.Context(), c.ID), "learned": h.conv.Learned(r.Context(), c.ID)})
}

func (h *handler) deleteConversation(w http.ResponseWriter, r *http.Request) {
	// by default what the philosophers learnt in it goes too; ?keep=memory lets them keep it
	err := h.conv.Delete(r.Context(), r.PathValue("id"), r.URL.Query().Get("keep") == "memory")
	switch {
	case errors.Is(err, store.ErrNotFound):
		fail(w, http.StatusNotFound, "There is no such conversation.")
	case err != nil:
		fail(w, http.StatusInternalServerError, "the conversation could not be deleted")
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *handler) sendMessage(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Text      string `json:"text"`
		Suggested bool   `json:"suggested"`
	}
	if !transport.ReadBody(w, r, &body) {
		return
	}
	send := h.conv.Send
	if body.Suggested {
		send = h.conv.SendSuggested
	}
	res, err := send(r.Context(), r.PathValue("id"), body.Text)
	h.respondTurn(w, r, res, err)
}

// respondTurn answers a request that ran a turn (a message, or the taking up of a challenge).
func (h *handler) respondTurn(w http.ResponseWriter, r *http.Request, res *conversation.Result, err error) {
	switch {
	case errors.Is(err, conversation.ErrEmpty):
		fail(w, http.StatusBadRequest, "The message is empty.")
	case errors.Is(err, conversation.ErrTooLong):
		fail(w, http.StatusRequestEntityTooLarge, "The message is too long.")
	case errors.Is(err, conversation.ErrAgeNotConfirmed):
		transport.WriteJSON(w, http.StatusForbidden, map[string]string{"error": "The age limit has to be confirmed first.", "kind": "age"})
	case errors.Is(err, conversation.ErrNoProvider):
		transport.WriteJSON(w, http.StatusConflict, map[string]string{"error": "Set up an AI provider first.", "kind": "no_provider"})
	case errors.Is(err, conversation.ErrLocked):
		transport.WriteJSON(w, http.StatusForbidden, map[string]string{"error": "This philosopher is not unlocked yet.", "kind": "locked"})
	case errors.Is(err, conversation.ErrNoOffer):
		transport.WriteJSON(w, http.StatusConflict, map[string]string{"error": "There is no question waiting.", "kind": "no_offer"})
	case errors.Is(err, conversation.ErrNotNow):
		transport.WriteJSON(w, http.StatusConflict, map[string]string{"error": "A question cannot be put just now.", "kind": "not_now"})
	case errors.Is(err, store.ErrNotFound):
		fail(w, http.StatusNotFound, "There is no such conversation.")
	case err != nil:
		log.Printf("[api] a turn failed: %T", err)
		fail(w, http.StatusInternalServerError, "the message could not be handled")
	case res.Outcome == conversation.OutcomeUnavailable:
		h.localize(r.Context(), res)
		transport.WriteJSON(w, http.StatusServiceUnavailable, res)
	default:
		h.localize(r.Context(), res)
		transport.WriteJSON(w, http.StatusOK, res)
	}
}

// localize puts the application's own texts of a turn into the person's language: a marker is stored as its kind only and says its words
// when it is shown. Replies of the philosophers are never touched.
func (h *handler) localize(ctx context.Context, res *conversation.Result) {
	lang := h.conv.Lang(ctx)
	h.localizeViews(lang, res.Added)
	switch res.Outcome {
	case conversation.OutcomeUnavailable:
		res.Text = safety.Unavailable(lang)
	case conversation.OutcomeEnded:
		res.Text = safety.TextForLang(safety.MarkerEndForAge, lang)
	case conversation.OutcomeFixed:
		for i := len(res.Added) - 1; i >= 0; i-- {
			if res.Added[i].Role == "marker" && res.Added[i].Kind != "" {
				res.Text = res.Added[i].Text
				break
			}
		}
	}
}

func (h *handler) localizeViews(lang string, views []conversation.View) {
	for i := range views {
		if views[i].Role == "marker" && views[i].Kind != "" {
			if t := safety.TextForLang(safety.MarkerKind(views[i].Kind), lang); t != "" {
				views[i].Text = t
			}
		}
	}
}

// challenge asks the philosopher to pose a challenge (the person pressed the button, or took up his offer to hear one).
func (h *handler) challenge(w http.ResponseWriter, r *http.Request) {
	res, err := h.conv.Offer(r.Context(), r.PathValue("id"))
	h.respondTurn(w, r, res, err)
}

func (h *handler) acceptChallenge(w http.ResponseWriter, r *http.Request) {
	res, err := h.conv.Accept(r.Context(), r.PathValue("id"))
	h.respondTurn(w, r, res, err)
}

func (h *handler) declineChallenge(w http.ResponseWriter, r *http.Request) {
	switch err := h.conv.Decline(r.Context(), r.PathValue("id")); {
	case errors.Is(err, conversation.ErrNoOffer):
		transport.WriteJSON(w, http.StatusConflict, map[string]string{"error": "There is no question waiting.", "kind": "no_offer"})
	case errors.Is(err, store.ErrNotFound):
		fail(w, http.StatusNotFound, "There is no such conversation.")
	case err != nil:
		fail(w, http.StatusInternalServerError, "the question could not be left")
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *handler) suggestions(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Exclude []string `json:"exclude"`
	}
	if r.ContentLength != 0 && !transport.ReadBody(w, r, &body) {
		return
	}
	res, err := h.conv.Suggest(r.Context(), r.PathValue("id"), body.Exclude)
	var le *llm.Error
	switch {
	case errors.Is(err, conversation.ErrAgeNotConfirmed):
		transport.WriteJSON(w, http.StatusForbidden, map[string]string{"error": "The age limit has to be confirmed first.", "kind": "age"})
	case errors.Is(err, conversation.ErrNotEntitled):
		transport.WriteJSON(w, http.StatusForbidden, map[string]string{"error": "Suggestions are not unlocked.", "kind": "locked"})
	case errors.Is(err, conversation.ErrNoProvider):
		transport.WriteJSON(w, http.StatusConflict, map[string]string{"error": "Set up an AI provider first.", "kind": "no_provider"})
	case errors.Is(err, store.ErrNotFound):
		fail(w, http.StatusNotFound, "There is no such conversation.")
	case errors.Is(err, conversation.ErrNotNow), errors.Is(err, suggest.ErrNothingToAnswer):
		fail(w, http.StatusConflict, "There is nothing to answer just now.")
	case errors.As(err, &le):
		transport.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "I cannot offer suggestions right now. Please try again in a moment.", "providerKind": string(le.Kind)})
	case err != nil:
		log.Printf("[api] suggestions failed: %T", err)
		fail(w, http.StatusInternalServerError, "the suggestions could not be made")
	default:
		transport.WriteJSON(w, http.StatusOK, res)
	}
}
