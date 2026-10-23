// Package api is the engine's HTTP surface once the database is open. Every route needs the token (see package transport) except the
// liveness check. Nothing here ever returns a credential.
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

// Deps is what the routes need.
type Deps struct {
	Store      *store.Store
	Roster     *philosophers.Registry // nil: the standard roster
	Version    string
	LLMOptions []llm.Option // only tests change these (a stand-in provider, no waiting between retries)
}

// New builds the routes. The caller adds the shutdown route and puts the door (transport.Policy) and the rate limit in front.
// It also returns the conversation service, so that the program can wait for its background work (titles and summaries) when it stops.
func New(d Deps) (*http.ServeMux, *conversation.Service) {
	if d.Roster == nil {
		d.Roster = philosophers.New()
	}
	h := &handler{d: d}
	h.conv = conversation.New(d.Store, h.models, d.Roster)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/status", h.status)
	mux.HandleFunc("GET /api/settings/llm", h.getLLM)
	mux.HandleFunc("PUT /api/settings/llm", h.putLLM)
	mux.HandleFunc("DELETE /api/settings/llm/key", h.deleteKey)
	mux.HandleFunc("POST /api/settings/llm/test", h.testLLM)
	mux.HandleFunc("GET /api/profile", h.profile)
	mux.HandleFunc("POST /api/profile/age", h.confirmAge)
	mux.HandleFunc("GET /api/philosophers", h.philosophers)
	mux.HandleFunc("POST /api/conversations", h.startConversation)
	mux.HandleFunc("GET /api/conversations", h.listConversations)
	mux.HandleFunc("GET /api/conversations/{id}", h.getConversation)
	mux.HandleFunc("DELETE /api/conversations/{id}", h.deleteConversation)
	mux.HandleFunc("POST /api/conversations/{id}/messages", h.sendMessage)
	return mux, h.conv
}

type handler struct {
	d    Deps
	conv *conversation.Service
}

func (h *handler) status(w http.ResponseWriter, _ *http.Request) {
	transport.WriteJSON(w, http.StatusOK, map[string]string{"status": "ready", "version": h.d.Version})
}

func fail(w http.ResponseWriter, code int, msg string) {
	transport.WriteJSON(w, code, map[string]string{"error": msg})
}

// failLLM turns a provider failure into an answer the screens can show: the kind tells them what to do, the text is plain and
// carries no key and nothing the user wrote.
func failLLM(w http.ResponseWriter, err error) {
	var e *llm.Error
	if !errors.As(err, &e) {
		log.Printf("[api] unexpected failure: %T", err)
		fail(w, http.StatusInternalServerError, "something went wrong")
		return
	}
	code, msg := http.StatusBadGateway, "The AI provider could not be reached or failed."
	switch e.Kind {
	case llm.KindAuth:
		code, msg = http.StatusUnprocessableEntity, "The provider did not accept this key."
	case llm.KindRateLimit:
		code, msg = http.StatusTooManyRequests, "The provider says there have been too many requests, or the account has no credit left."
	case llm.KindOverloaded:
		code, msg = http.StatusServiceUnavailable, "The provider is busy. Try again in a moment."
	case llm.KindModelGone:
		code, msg = http.StatusConflict, "The chosen model is no longer available. The models will be chosen again."
	case llm.KindInvalid, llm.KindBadRequest:
		code, msg = http.StatusBadRequest, "The provider settings are not valid."
	case llm.KindEmpty:
		code, msg = http.StatusBadGateway, "The model gave no answer."
	}
	transport.WriteJSON(w, code, map[string]string{"error": msg, "kind": string(e.Kind)})
}

func background(r *http.Request) context.Context { return r.Context() }
