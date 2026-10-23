package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// The ledger is what a philosopher remembers of the person: claims they made, words they defined, tensions between what they said.
// Every entry carries the person's own words as evidence. The text columns are sealed like messages.

const (
	KindClaim   = "claim"
	KindTerm    = "term"
	KindTension = "tension"

	StatusActive    = "active"
	StatusRetracted = "retracted"
	StatusResolved  = "resolved"
)

// Entry is one line of the ledger.
type Entry struct {
	ID             string
	PhilosopherID  string
	ConversationID string // where it was learnt; the entry goes when that conversation is deleted
	Kind           string
	Text           string // a claim, a term, or the description of a tension
	Detail         string // for a term: the person's definition
	Quote          string // the person's own words that support it
	Suggested      bool   // it came from a message chosen among suggestions
	Examined       int    // 0 not yet; 1 the person has examined it in their own words (a term defined, a claim tested, a tension seen); 2 the same, from a wording chosen among suggestions
	Status         string
	A, B           string // for a tension: the two entries that cannot both hold
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// ErrBadEntry is returned for an entry that breaks the shape rules of the ledger.
var ErrBadEntry = errors.New("the ledger entry is not valid")

// Hard limits of the store; the rules about what is worth recording belong to the layer above.
const (
	MaxEntryText  = 400
	MaxEntryQuote = 600
	MaxLedgerSize = 500 // entries per philosopher
)

func (o *Ops) sealOpt(col, id, v string) ([]byte, error) {
	if v == "" {
		return nil, nil
	}
	return o.c.seal("ledger", col, id, []byte(v))
}

// AddEntry stores a new entry (status active) and returns it with its id.
func (o *Ops) AddEntry(ctx context.Context, e Entry) (Entry, error) {
	if e.PhilosopherID == "" || e.ConversationID == "" || e.Text == "" || e.Quote == "" ||
		(e.Kind != KindClaim && e.Kind != KindTerm && e.Kind != KindTension) ||
		len([]rune(e.Text)) > MaxEntryText || len([]rune(e.Detail)) > MaxEntryText || len([]rune(e.Quote)) > MaxEntryQuote {
		return Entry{}, ErrBadEntry
	}
	if e.Examined < 0 || e.Examined > 2 ||
		(e.Kind == KindTension) != (e.A != "" && e.B != "") || (e.Kind != KindTension && (e.A != "" || e.B != "")) || (e.Kind == KindTension && e.A == e.B) {
		return Entry{}, ErrBadEntry
	}
	var n int
	if err := o.q.QueryRowContext(ctx, "SELECT COUNT(*) FROM ledger WHERE philosopher_id = ?", e.PhilosopherID).Scan(&n); err != nil {
		return Entry{}, err
	}
	if n >= MaxLedgerSize {
		return Entry{}, fmt.Errorf("%w: the ledger is full", ErrBadEntry)
	}
	e.ID, e.Status = newID(), StatusActive
	now := o.stamp()
	e.CreatedAt, e.UpdatedAt = time.Unix(now, 0), time.Unix(now, 0)
	text, err := o.sealOpt("text", e.ID, e.Text)
	if err != nil {
		return Entry{}, err
	}
	detail, err := o.sealOpt("detail", e.ID, e.Detail)
	if err != nil {
		return Entry{}, err
	}
	quote, err := o.sealOpt("quote", e.ID, e.Quote)
	if err != nil {
		return Entry{}, err
	}
	flag := 0
	if e.Suggested {
		flag = 1
	}
	_, err = o.q.ExecContext(ctx, `INSERT INTO ledger(id, philosopher_id, conversation_id, kind, text, detail, quote, suggested, status, a_id, b_id, created_at, updated_at, examined)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'active', NULLIF(?, ''), NULLIF(?, ''), ?, ?, ?)`,
		e.ID, e.PhilosopherID, e.ConversationID, e.Kind, text, detail, quote, flag, e.A, e.B, now, now, e.Examined)
	if err != nil {
		return Entry{}, err
	}
	return e, nil
}

const entryColumns = "id, philosopher_id, conversation_id, kind, text, detail, quote, suggested, status, a_id, b_id, created_at, updated_at, examined"

func (o *Ops) scanEntry(sc interface{ Scan(...any) error }) (Entry, error) {
	var e Entry
	var text, detail, quote []byte
	var a, b sql.NullString
	var flag, created, updated int64
	if err := sc.Scan(&e.ID, &e.PhilosopherID, &e.ConversationID, &e.Kind, &text, &detail, &quote, &flag, &e.Status, &a, &b, &created, &updated, &e.Examined); err != nil {
		return Entry{}, err
	}
	open := func(col string, blob []byte) (string, error) {
		if blob == nil {
			return "", nil
		}
		p, err := o.c.open("ledger", col, e.ID, blob)
		return string(p), err
	}
	var err error
	if e.Text, err = open("text", text); err != nil {
		return Entry{}, fmt.Errorf("ledger %s: %w", e.ID, err)
	}
	if e.Detail, err = open("detail", detail); err != nil {
		return Entry{}, fmt.Errorf("ledger %s: %w", e.ID, err)
	}
	if e.Quote, err = open("quote", quote); err != nil {
		return Entry{}, fmt.Errorf("ledger %s: %w", e.ID, err)
	}
	e.Suggested, e.A, e.B = flag == 1, a.String, b.String
	e.CreatedAt, e.UpdatedAt = time.Unix(created, 0), time.Unix(updated, 0)
	return e, nil
}

// Entry returns one entry.
func (o *Ops) Entry(ctx context.Context, id string) (Entry, error) {
	e, err := o.scanEntry(o.q.QueryRowContext(ctx, "SELECT "+entryColumns+" FROM ledger WHERE id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return Entry{}, ErrNotFound
	}
	return e, err
}

// Entries returns a philosopher's entries, oldest first. Retracted and resolved ones are included only when asked for.
func (o *Ops) Entries(ctx context.Context, philosopherID string, all bool) ([]Entry, error) {
	q := "SELECT " + entryColumns + " FROM ledger WHERE philosopher_id = ?"
	if !all {
		q += " AND status = 'active'"
	}
	rows, err := o.q.QueryContext(ctx, q+" ORDER BY created_at, rowid", philosopherID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Entry
	for rows.Next() {
		e, err := o.scanEntry(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// EntriesForConversation returns every entry learnt in one conversation (for export).
func (o *Ops) EntriesForConversation(ctx context.Context, conversationID string) ([]Entry, error) {
	rows, err := o.q.QueryContext(ctx, "SELECT "+entryColumns+" FROM ledger WHERE conversation_id = ? ORDER BY created_at, rowid", conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Entry
	for rows.Next() {
		e, err := o.scanEntry(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ReviseEntry changes the text and detail of an entry and its status; the evidence stays as it was.
func (o *Ops) ReviseEntry(ctx context.Context, id, text, detail, status string) error {
	if text == "" || len([]rune(text)) > MaxEntryText || len([]rune(detail)) > MaxEntryText ||
		(status != StatusActive && status != StatusRetracted && status != StatusResolved) {
		return ErrBadEntry
	}
	t, err := o.sealOpt("text", id, text)
	if err != nil {
		return err
	}
	d, err := o.sealOpt("detail", id, detail)
	if err != nil {
		return err
	}
	res, err := o.q.ExecContext(ctx, "UPDATE ledger SET text = ?, detail = ?, status = ?, updated_at = ? WHERE id = ?", t, d, status, o.stamp(), id)
	return affected(res, err)
}

// DeleteEntry removes one entry for good, and any tension that rests on it.
func (o *Ops) DeleteEntry(ctx context.Context, id string) error {
	res, err := o.q.ExecContext(ctx, "DELETE FROM ledger WHERE id = ?", id)
	if err := affected(res, err); err != nil {
		return err
	}
	_, err = o.q.ExecContext(ctx, "DELETE FROM ledger WHERE kind = 'tension' AND (a_id = ? OR b_id = ?)", id, id)
	return err
}

// ClearLedger removes everything a philosopher remembers of the person.
func (o *Ops) ClearLedger(ctx context.Context, philosopherID string) error {
	_, err := o.q.ExecContext(ctx, "DELETE FROM ledger WHERE philosopher_id = ?", philosopherID)
	return err
}

// SetExamined records that the person has examined an entry (1 in their own words, 2 from a wording chosen among suggestions). It never
// lowers a mark: an entry examined in the person's own words stays so.
func (o *Ops) SetExamined(ctx context.Context, id string, level int) error {
	if level < 1 || level > 2 {
		return ErrBadEntry
	}
	res, err := o.q.ExecContext(ctx, `UPDATE ledger SET examined = CASE WHEN examined = 1 THEN 1 ELSE ? END, updated_at = ? WHERE id = ?`, level, o.stamp(), id)
	return affected(res, err)
}
