package api

import (
	"context"
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
)

type exportMessage struct {
	Role string `json:"role"`
	Text string `json:"text"` // for a marker, the kind
}

type exportConversation struct {
	PhilosopherID string          `json:"philosopherId"`
	Title         string          `json:"title"`
	CreatedAt     int64           `json:"createdAt"`
	Summary       string          `json:"summary,omitempty"`
	Messages      []exportMessage `json:"messages"`
}

type exportFile struct {
	Format        string               `json:"format"`
	ExportedAt    int64                `json:"exportedAt"`
	Conversations []exportConversation `json:"conversations"`
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
		out.Conversations = append(out.Conversations, ec)
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
	total := 0
	for _, c := range in.Conversations {
		if _, err := h.d.Roster.Get(c.PhilosopherID); err != nil {
			fail(w, http.StatusBadRequest, "The file names a philosopher this version does not know.")
			return
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
	if err != nil {
		fail(w, http.StatusInternalServerError, "the file could not be imported")
		return
	}
	transport.WriteJSON(w, http.StatusOK, map[string]int{"conversations": len(in.Conversations), "messages": total})
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
