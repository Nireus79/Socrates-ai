package api

import (
	"net/http"
	"time"

	"socrates/internal/cards"
	"socrates/internal/transport"
)

// getCards answers with the messages of the card, from the project's data file when it is available (never more than one fetch a day, and
// none at all unless a screen asks, which it does only while the card is shown), or an empty list: the screens then show their own.
func (h *handler) getCards(w http.ResponseWriter, r *http.Request) {
	e, c := []cards.Entry{}, []cards.Card{}
	if content := h.d.Cards.Content(r.Context()); content != nil {
		e, c = content.Entries, cards.Current(content.Cards, time.Now())
	}
	transport.WriteJSON(w, http.StatusOK, map[string]any{"entries": e, "cards": c})
}
