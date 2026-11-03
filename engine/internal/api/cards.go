package api

import (
	"net/http"

	"socrates/internal/cards"
	"socrates/internal/transport"
)

// getCards answers with the messages of the card, from the project's data file when it is available (never more than one fetch a day, and
// none at all unless a screen asks, which it does only while the card is shown), or an empty list: the screens then show their own.
func (h *handler) getCards(w http.ResponseWriter, r *http.Request) {
	e := h.d.Cards.Entries(r.Context())
	if e == nil {
		e = []cards.Entry{}
	}
	transport.WriteJSON(w, http.StatusOK, map[string]any{"entries": e})
}
