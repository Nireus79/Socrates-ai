package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"socrates/internal/safety"
	"socrates/internal/store"
	"socrates/internal/transport"
)

// The export holds the user's conversations and nothing else: never the provider key, the settings or the database key.
const exportFormat = "socrates-export-1"

const (
	maxImportConversations = 500
	maxImportMessages      = 20000
	maxImportChallenges    = 2000
)

type exportMessage struct {
	Role string `json:"role"`
	Text string `json:"text"` // for a marker, the kind
}

// exportEntry is a ledger entry; for a tension, A and B are the 1-based positions of the two entries in the same list (0: none, which
// happens when one of them was learnt in another conversation).
type exportEntry struct {
	Kind      string `json:"kind"`
	Text      string `json:"text"`
	Detail    string `json:"detail,omitempty"`
	Quote     string `json:"quote"`
	Suggested bool   `json:"suggested,omitempty"`
	Status    string `json:"status"`
	Examined  int    `json:"examined,omitempty"`
	A         int    `json:"a,omitempty"`
	B         int    `json:"b,omitempty"`
}

type exportConversation struct {
	PhilosopherID string          `json:"philosopherId"`
	Title         string          `json:"title"`
	CreatedAt     int64           `json:"createdAt"`
	Summary       string          `json:"summary,omitempty"`
	Messages      []exportMessage `json:"messages"`
	Ledger        []exportEntry   `json:"ledger,omitempty"`
}

// toExportEntries lists entries for an export; a tension points at its two claims by their position in the same list (0 when one of them is
// not in it).
func toExportEntries(entries []store.Entry) []exportEntry {
	pos := map[string]int{}
	for i, e := range entries {
		pos[e.ID] = i + 1
	}
	var out []exportEntry
	for _, e := range entries {
		out = append(out, exportEntry{Kind: e.Kind, Text: e.Text, Detail: e.Detail, Quote: e.Quote, Suggested: e.Suggested, Status: e.Status, Examined: e.Examined, A: pos[e.A], B: pos[e.B]})
	}
	return out
}

// exportChallenge is a finished challenge: whom it was with and when, no text.
type exportChallenge struct {
	PhilosopherID string `json:"philosopherId"`
	At            int64  `json:"at"`
}

type exportFile struct {
	Format        string               `json:"format"`
	ExportedAt    int64                `json:"exportedAt"`
	Conversations []exportConversation `json:"conversations"`
	Challenges    []exportChallenge    `json:"challenges,omitempty"`
	// Memory is what the philosophers kept after the conversations it was learnt in were deleted.
	Memory []exportMemory `json:"memory,omitempty"`
}

type exportMemory struct {
	PhilosopherID string        `json:"philosopherId"`
	Ledger        []exportEntry `json:"ledger"`
}

func (h *handler) exportProfile(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	list, err := h.d.Store.Conversations(ctx)
	if err != nil {
		fail(w, http.StatusInternalServerError, "the profile could not be exported")
		return
	}
	out := exportFile{Format: exportFormat, ExportedAt: time.Now().Unix(), Conversations: []exportConversation{}}
	for _, c := range list {
		msgs, err := h.d.Store.Messages(ctx, c.ID)
		if err != nil {
			fail(w, http.StatusInternalServerError, "the profile could not be exported")
			return
		}
		ec := exportConversation{PhilosopherID: c.PhilosopherID, Title: c.Title, CreatedAt: c.CreatedAt.Unix(), Messages: []exportMessage{}}
		if sum, _, ok, _ := h.d.Store.Summary(ctx, c.ID); ok {
			ec.Summary = sum
		}
		for _, m := range msgs {
			ec.Messages = append(ec.Messages, exportMessage{Role: m.Role, Text: m.Content})
		}
		if entries, err := h.d.Store.EntriesForConversation(ctx, c.ID); err == nil {
			ec.Ledger = toExportEntries(entries)
		}
		out.Conversations = append(out.Conversations, ec)
	}
	if kept, err := h.d.Store.DetachedEntries(ctx); err == nil {
		byPhil := map[string][]store.Entry{}
		var order []string
		for _, e := range kept {
			if _, ok := byPhil[e.PhilosopherID]; !ok {
				order = append(order, e.PhilosopherID)
			}
			byPhil[e.PhilosopherID] = append(byPhil[e.PhilosopherID], e)
		}
		for _, id := range order {
			out.Memory = append(out.Memory, exportMemory{PhilosopherID: id, Ledger: toExportEntries(byPhil[id])})
		}
	}
	if cs, err := h.d.Store.Challenges(ctx); err == nil {
		for _, c := range cs {
			out.Challenges = append(out.Challenges, exportChallenge{PhilosopherID: c.PhilosopherID, At: c.CreatedAt.Unix()})
		}
	}
	w.Header().Set("Content-Disposition", `attachment; filename="socrates-export.json"`)
	transport.WriteJSON(w, http.StatusOK, out)
}

// importProfile adds the conversations of an export as new conversations; nothing existing is touched. A file with anything wrong in it
// is refused as a whole, so a half-imported profile cannot happen.
func (h *handler) importProfile(w http.ResponseWriter, r *http.Request) {
	var in exportFile
	if !transport.ReadBody(w, r, &in) {
		return
	}
	if in.Format != exportFormat {
		fail(w, http.StatusBadRequest, "This is not a Socrates export file.")
		return
	}
	if len(in.Conversations) > maxImportConversations {
		fail(w, http.StatusBadRequest, "The file holds too many conversations.")
		return
	}
	if len(in.Challenges) > maxImportChallenges {
		fail(w, http.StatusBadRequest, "The file holds too many finished questions.")
		return
	}
	for _, c := range in.Challenges {
		if _, err := h.d.Roster.Get(c.PhilosopherID); err != nil || c.At < 0 {
			fail(w, http.StatusBadRequest, "The file holds a finished question that cannot be read.")
			return
		}
	}
	if len(in.Memory) > maxImportConversations {
		fail(w, http.StatusBadRequest, "The file holds too much kept memory.")
		return
	}
	for _, m := range in.Memory {
		if _, err := h.d.Roster.Get(m.PhilosopherID); err != nil {
			fail(w, http.StatusBadRequest, "The file names a philosopher this version does not know.")
			return
		}
		if len(m.Ledger) > store.MaxLedgerSize {
			fail(w, http.StatusBadRequest, "The file holds too many memory entries.")
			return
		}
		for _, e := range m.Ledger {
			if !validEntry(m.Ledger, e) {
				fail(w, http.StatusBadRequest, "The file holds a memory entry that cannot be read.")
				return
			}
		}
	}
	total := 0
	for _, c := range in.Conversations {
		if _, err := h.d.Roster.Get(c.PhilosopherID); err != nil {
			fail(w, http.StatusBadRequest, "The file names a philosopher this version does not know.")
			return
		}
		if len(c.Ledger) > store.MaxLedgerSize {
			fail(w, http.StatusBadRequest, "The file holds too many memory entries.")
			return
		}
		for _, e := range c.Ledger {
			if !validEntry(c.Ledger, e) {
				fail(w, http.StatusBadRequest, "The file holds a memory entry that cannot be read.")
				return
			}
		}
		total += len(c.Messages)
		for _, m := range c.Messages {
			ok := m.Role == "user" || m.Role == "philosopher" || (m.Role == "marker" && safety.TextFor(safety.MarkerKind(m.Text)) != "")
			if !ok || (m.Role != "marker" && m.Text == "") {
				fail(w, http.StatusBadRequest, "The file holds a message that cannot be read.")
				return
			}
		}
	}
	if total > maxImportMessages {
		fail(w, http.StatusBadRequest, "The file holds too many messages.")
		return
	}
	ctx := r.Context()
	err := h.d.Store.Update(ctx, func(o *store.Ops) error { return importAll(ctx, o, in) })
	if errors.Is(err, store.ErrBadEntry) {
		fail(w, http.StatusBadRequest, "The memory in the file does not fit.")
		return
	}
	if err != nil {
		fail(w, http.StatusInternalServerError, "the file could not be imported")
		return
	}
	transport.WriteJSON(w, http.StatusOK, map[string]int{"conversations": len(in.Conversations), "messages": total})
}

// validEntry checks one imported ledger entry against the same rules the store applies, and that a tension points at two different claims
// of the same list.
func validEntry(list []exportEntry, e exportEntry) bool {
	if e.Text == "" || e.Quote == "" || len([]rune(e.Text)) > store.MaxEntryText || len([]rune(e.Detail)) > store.MaxEntryText || len([]rune(e.Quote)) > store.MaxEntryQuote {
		return false
	}
	if e.Examined < 0 || e.Examined > 2 {
		return false
	}
	if e.Status != store.StatusActive && e.Status != store.StatusRetracted && e.Status != store.StatusResolved {
		return false
	}
	linked := e.A != 0 || e.B != 0
	switch e.Kind {
	case store.KindClaim, store.KindTerm, store.KindFact:
		return !linked
	case store.KindTension:
		if !linked {
			return true // its claims were learnt in another conversation: kept in the file, skipped on import
		}
		in := func(n int) bool { return n >= 1 && n <= len(list) && list[n-1].Kind == store.KindClaim }
		return in(e.A) && in(e.B) && e.A != e.B
	}
	return false
}

func importAll(ctx context.Context, o *store.Ops, in exportFile) error {
	for _, c := range in.Conversations {
		conv, err := o.CreateConversation(ctx, c.PhilosopherID)
		if err != nil {
			return err
		}
		var last int
		for _, m := range c.Messages {
			msg, err := o.AppendMessage(ctx, conv.ID, m.Role, m.Text)
			if err != nil {
				return err
			}
			last = msg.Seq
		}
		if c.Title != "" {
			if err := o.SetTitle(ctx, conv.ID, c.Title); err != nil {
				return err
			}
		}
		if c.Summary != "" && last > 0 {
			if err := o.SetSummary(ctx, conv.ID, c.Summary, last); err != nil {
				return err
			}
		}
		if err := importEntries(ctx, o, c.PhilosopherID, conv.ID, c.Ledger); err != nil {
			return err
		}
	}
	for _, m := range in.Memory {
		if err := importEntries(ctx, o, m.PhilosopherID, "", m.Ledger); err != nil {
			return err
		}
	}
	for _, c := range in.Challenges {
		if err := o.AddChallenge(ctx, store.Challenge{PhilosopherID: c.PhilosopherID, CreatedAt: time.Unix(c.At, 0)}); err != nil {
			return err
		}
	}
	return nil
}

// deleteProfile removes everything the store holds: conversations, summaries, settings (with the provider key) and the age
// confirmation. It asks for the word "delete" so that a stray request cannot do it.
func (h *handler) deleteProfile(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Confirm string `json:"confirm"`
	}
	if !transport.ReadBody(w, r, &body) {
		return
	}
	if body.Confirm != "delete" {
		fail(w, http.StatusBadRequest, `Send {"confirm":"delete"} to delete the profile.`)
		return
	}
	h.conv.Wait()
	if err := h.d.Store.Wipe(r.Context()); err != nil {
		fail(w, http.StatusInternalServerError, "the profile could not be deleted")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// importEntries adds a list of exported entries for a philosopher, tied to a conversation or (empty id) to none. Claims, terms and facts
// go first so that a tension can point at them.
func importEntries(ctx context.Context, o *store.Ops, philosopherID, conversationID string, list []exportEntry) error {
	ids := make([]string, len(list))
	for i, e := range list { // claims and terms first, so that a tension can point at them
		if e.Kind == store.KindTension {
			continue
		}
		added, err := o.AddEntry(ctx, store.Entry{PhilosopherID: philosopherID, ConversationID: conversationID, Kind: e.Kind, Text: e.Text, Detail: e.Detail, Quote: e.Quote, Suggested: e.Suggested, Examined: e.Examined})
		if err != nil {
			return err
		}
		ids[i] = added.ID
		if e.Status != store.StatusActive {
			if err := o.ReviseEntry(ctx, added.ID, e.Text, e.Detail, e.Status); err != nil {
				return err
			}
		}
	}
	for i, e := range list {
		if e.Kind != store.KindTension {
			continue
		}
		if e.A == 0 || e.B == 0 {
			continue // a tension whose claims were learnt elsewhere cannot be rebuilt
		}
		added, err := o.AddEntry(ctx, store.Entry{PhilosopherID: philosopherID, ConversationID: conversationID, Kind: store.KindTension, Text: e.Text, Quote: e.Quote, Suggested: e.Suggested, Examined: e.Examined, A: ids[e.A-1], B: ids[e.B-1]})
		if err != nil {
			return err
		}
		ids[i] = added.ID
		if e.Status != store.StatusActive {
			if err := o.ReviseEntry(ctx, added.ID, e.Text, "", e.Status); err != nil {
				return err
			}
		}
	}
	return nil
}
