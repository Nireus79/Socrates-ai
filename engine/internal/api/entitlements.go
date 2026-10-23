package api

import (
	"errors"
	"net/http"
	"time"

	"socrates/internal/catalog"
	"socrates/internal/conversation"
	"socrates/internal/store"
	"socrates/internal/transport"
)

// What the person has bought, been given or earned for a while is told to the engine by the shell of the app (after the store, a licence check
// or an ad network said so). The engine cannot check it: see THREAT_MODEL.md.

const maxGrantList = 50

// putGrants makes the standing products of a source exactly the given list.
func (h *handler) putGrants(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Source   string   `json:"source"`
		Products []string `json:"products"`
	}
	if !transport.ReadBody(w, r, &body) {
		return
	}
	roster := h.d.Roster.All()
	if !catalog.Sources[body.Source] || len(body.Products) > maxGrantList {
		fail(w, http.StatusBadRequest, "That source or list is not accepted.")
		return
	}
	for _, p := range body.Products {
		if !catalog.Known(p, roster) || catalog.IsPass(p) {
			fail(w, http.StatusBadRequest, "A product in the list is not known to this version.")
			return
		}
	}
	if err := h.d.Store.Update(r.Context(), func(o *store.Ops) error { return o.ReplaceGrants(r.Context(), body.Source, body.Products) }); err != nil {
		fail(w, http.StatusInternalServerError, "the products could not be saved")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// addPass grants a pass product for a limited number of hours.
func (h *handler) addPass(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Source  string `json:"source"`
		Product string `json:"product"`
		Hours   int    `json:"hours"`
	}
	if !transport.ReadBody(w, r, &body) {
		return
	}
	if !catalog.Sources[body.Source] || !catalog.IsPass(body.Product) || !catalog.Known(body.Product, h.d.Roster.All()) || body.Hours < 1 || body.Hours > catalog.MaxPassHours {
		fail(w, http.StatusBadRequest, "That pass is not accepted.")
		return
	}
	until := time.Now().Add(time.Duration(body.Hours) * time.Hour)
	if err := h.d.Store.Update(r.Context(), func(o *store.Ops) error { return o.AddPass(r.Context(), body.Source, body.Product, until) }); err != nil {
		fail(w, http.StatusInternalServerError, "the pass could not be saved")
		return
	}
	transport.WriteJSON(w, http.StatusOK, map[string]int64{"until": until.Unix()})
}

// getGrants lists what is in force now.
func (h *handler) getGrants(w http.ResponseWriter, r *http.Request) {
	products, err := h.d.Store.ActiveGrants(r.Context(), time.Now())
	if err != nil {
		fail(w, http.StatusInternalServerError, "the products could not be read")
		return
	}
	if products == nil {
		products = []string{}
	}
	transport.WriteJSON(w, http.StatusOK, map[string]any{"products": products})
}

// categories are the reasons a reply can be reported for.
var reportCategories = map[string]bool{"harmful": true, "hateful": true, "sexual": true, "untrue": true, "other": true}

// reportReply takes a reply out of a conversation at the person's request and gives its words back once.
func (h *handler) reportReply(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Category string `json:"category"`
	}
	if !transport.ReadBody(w, r, &body) {
		return
	}
	if !reportCategories[body.Category] {
		fail(w, http.StatusBadRequest, "Choose one of the reasons.")
		return
	}
	text, err := h.conv.Report(r.Context(), r.PathValue("id"), r.PathValue("messageId"))
	switch {
	case errors.Is(err, store.ErrNotFound):
		fail(w, http.StatusNotFound, "There is no such reply.")
	case errors.Is(err, store.ErrNotAReply):
		fail(w, http.StatusBadRequest, "Only a philosopher's reply can be reported.")
	case err != nil:
		fail(w, http.StatusInternalServerError, "the reply could not be reported")
	default:
		transport.WriteJSON(w, http.StatusOK, map[string]string{"category": body.Category, "reply": text, "marker": conversation.ReportedText})
	}
}
