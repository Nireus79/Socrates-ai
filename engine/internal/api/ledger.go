package api

import (
	"errors"
	"net/http"

	"socrates/internal/store"
	"socrates/internal/transport"
)

type entryJSON struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Text      string `json:"text"`
	Detail    string `json:"detail,omitempty"`
	Quote     string `json:"quote"`
	Suggested bool   `json:"suggested,omitempty"`
	CreatedAt int64  `json:"createdAt"`
}

// ledger lists what one philosopher remembers of the person (active entries), with the person's own words.
func (h *handler) ledger(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("philosopherId")
	if _, err := h.d.Roster.Get(id); err != nil {
		fail(w, http.StatusBadRequest, "There is no such philosopher.")
		return
	}
	list, err := h.d.Store.Entries(r.Context(), id, false)
	if err != nil {
		fail(w, http.StatusInternalServerError, "the memory could not be read")
		return
	}
	out := make([]entryJSON, 0, len(list))
	for _, e := range list {
		out = append(out, entryJSON{ID: e.ID, Kind: e.Kind, Text: e.Text, Detail: e.Detail, Quote: e.Quote, Suggested: e.Suggested, CreatedAt: e.CreatedAt.Unix()})
	}
	transport.WriteJSON(w, http.StatusOK, map[string]any{"entries": out})
}

func (h *handler) deleteEntry(w http.ResponseWriter, r *http.Request) {
	switch err := h.d.Store.DeleteEntry(r.Context(), r.PathValue("id")); {
	case errors.Is(err, store.ErrNotFound):
		fail(w, http.StatusNotFound, "There is no such entry.")
	case err != nil:
		fail(w, http.StatusInternalServerError, "the entry could not be deleted")
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *handler) clearLedger(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("philosopherId")
	if _, err := h.d.Roster.Get(id); err != nil {
		fail(w, http.StatusBadRequest, "There is no such philosopher.")
		return
	}
	if err := h.d.Store.ClearLedger(r.Context(), id); err != nil {
		fail(w, http.StatusInternalServerError, "the memory could not be cleared")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
