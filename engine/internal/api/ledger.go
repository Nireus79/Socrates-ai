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

func (h *handler) getMemory(w http.ResponseWriter, r *http.Request) {
	transport.WriteJSON(w, http.StatusOK, map[string]bool{"enabled": h.conv.MemoryEnabled(r.Context())})
}

func (h *handler) putMemory(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Enabled *bool `json:"enabled"`
	}
	if !transport.ReadBody(w, r, &body) {
		return
	}
	if body.Enabled == nil {
		fail(w, http.StatusBadRequest, "Say whether memory should be on or off.")
		return
	}
	if err := h.conv.SetMemoryEnabled(r.Context(), *body.Enabled); err != nil {
		fail(w, http.StatusInternalServerError, "the setting could not be saved")
		return
	}
	transport.WriteJSON(w, http.StatusOK, map[string]bool{"enabled": *body.Enabled})
}

func (h *handler) getLang(w http.ResponseWriter, r *http.Request) {
	l, _ := h.conv.LangChosen(r.Context()) // empty until chosen
	transport.WriteJSON(w, http.StatusOK, map[string]string{"language": l})
}

func (h *handler) putLang(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Language string `json:"language"`
	}
	if !transport.ReadBody(w, r, &body) {
		return
	}
	if err := h.conv.SetLang(r.Context(), body.Language); err != nil {
		fail(w, http.StatusBadRequest, "There are no words in that language yet.")
		return
	}
	transport.WriteJSON(w, http.StatusOK, map[string]string{"language": body.Language})
}

// The theme of the screens: "auto" follows the device, otherwise "light" or "dark". It is the person's own setting, kept with the profile.
const themeKey = "theme"

func (h *handler) getTheme(w http.ResponseWriter, r *http.Request) {
	v, ok, err := h.d.Store.Setting(r.Context(), themeKey)
	if err != nil || !ok || !knownTheme(v) {
		v = "auto"
	}
	transport.WriteJSON(w, http.StatusOK, map[string]string{"theme": v})
}

func (h *handler) putTheme(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Theme string `json:"theme"`
	}
	if !transport.ReadBody(w, r, &body) {
		return
	}
	if !knownTheme(body.Theme) {
		fail(w, http.StatusBadRequest, "The theme must be auto, light or dark.")
		return
	}
	if err := h.d.Store.SetSetting(r.Context(), themeKey, body.Theme); err != nil {
		fail(w, http.StatusInternalServerError, "That did not work.")
		return
	}
	transport.WriteJSON(w, http.StatusOK, map[string]string{"theme": body.Theme})
}

func knownTheme(v string) bool { return v == "auto" || v == "light" || v == "dark" }
