package api

import (
	"context"
	"errors"
	"log"
	"net/http"

	"socrates/internal/conversation"
	"socrates/internal/llm"
	"socrates/internal/philosophers"
	"socrates/internal/store"
	"socrates/internal/transport"
)

// models builds the client from the user's stored settings, for each turn (a changed key or provider applies at once).
func (h *handler) models(ctx context.Context) (conversation.Completer, error) {
	s, _, err := h.loadCtx(ctx)
	if err != nil {
		return nil, err
	}
	if s.Provider == "" || (s.Provider != "ollama" && s.APIKey == "") || s.FastModel == "" {
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
		"hasProvider":  s.Provider != "" && (s.Provider == "ollama" || s.APIKey != ""),
	})
}

func (h *handler) confirmAge(w http.ResponseWriter, r *http.Request) {
	if err := h.conv.ConfirmAge(r.Context()); err != nil {
		fail(w, http.StatusInternalServerError, "the confirmation could not be saved")
		return
	}
	transport.WriteJSON(w, http.StatusOK, map[string]bool{"ageConfirmed": true})
}

func (h *handler) philosophers(w http.ResponseWriter, _ *http.Request) {
	transport.WriteJSON(w, http.StatusOK, map[string]any{"philosophers": h.d.Roster.All()})
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
	transport.WriteJSON(w, http.StatusOK, map[string]any{"conversation": toJSON(c), "messages": msgs})
}

func (h *handler) deleteConversation(w http.ResponseWriter, r *http.Request) {
	err := h.conv.Delete(r.Context(), r.PathValue("id"))
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
		Text string `json:"text"`
	}
	if !transport.ReadBody(w, r, &body) {
		return
	}
	res, err := h.conv.Send(r.Context(), r.PathValue("id"), body.Text)
	switch {
	case errors.Is(err, conversation.ErrEmpty):
		fail(w, http.StatusBadRequest, "The message is empty.")
	case errors.Is(err, conversation.ErrTooLong):
		fail(w, http.StatusRequestEntityTooLarge, "The message is too long.")
	case errors.Is(err, conversation.ErrAgeNotConfirmed):
		transport.WriteJSON(w, http.StatusForbidden, map[string]string{"error": "The age limit has to be confirmed first.", "kind": "age"})
	case errors.Is(err, conversation.ErrNoProvider):
		transport.WriteJSON(w, http.StatusConflict, map[string]string{"error": "Set up an AI provider first.", "kind": "no_provider"})
	case errors.Is(err, store.ErrNotFound):
		fail(w, http.StatusNotFound, "There is no such conversation.")
	case err != nil:
		log.Printf("[api] a turn failed: %T", err)
		fail(w, http.StatusInternalServerError, "the message could not be handled")
	case res.Outcome == conversation.OutcomeUnavailable:
		transport.WriteJSON(w, http.StatusServiceUnavailable, res)
	default:
		transport.WriteJSON(w, http.StatusOK, res)
	}
}
