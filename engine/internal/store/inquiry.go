package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// An inquiry is the question a conversation is examining. It is quoted from the person, it can deepen into a new inquiry linked to its
// parent, and it banks the best maturity it reached. A completed inquiry is a challenge.

// Inquiry is one question under examination.
type Inquiry struct {
	ID             string
	PhilosopherID  string
	ConversationID string
	ParentID       string
	Question       string
	Quote          string
	Banked         int // the highest maturity reached, in thousandths
	Completed      bool
	Announced      bool // the person has been told it was finished
	Challenge      bool // it began as a challenge the philosopher posed and the person accepted
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

const inquiryColumns = "id, philosopher_id, conversation_id, parent_id, question, quote, banked, completed, announced, created_at, updated_at, challenge"

func (o *Ops) scanInquiry(sc interface{ Scan(...any) error }) (Inquiry, error) {
	var in Inquiry
	var parent sql.NullString
	var q, quote []byte
	var done, ann, created, updated, chal int64
	if err := sc.Scan(&in.ID, &in.PhilosopherID, &in.ConversationID, &parent, &q, &quote, &in.Banked, &done, &ann, &created, &updated, &chal); err != nil {
		return Inquiry{}, err
	}
	var err error
	var p []byte
	if p, err = o.c.open("inquiry", "question", in.ID, q); err != nil {
		return Inquiry{}, err
	}
	in.Question = string(p)
	if p, err = o.c.open("inquiry", "quote", in.ID, quote); err != nil {
		return Inquiry{}, err
	}
	in.Quote = string(p)
	in.ParentID, in.Completed, in.Announced, in.Challenge = parent.String, done == 1, ann == 1, chal == 1
	in.CreatedAt, in.UpdatedAt = time.Unix(created, 0), time.Unix(updated, 0)
	return in, nil
}

// CreateInquiry starts an inquiry in a conversation.
func (o *Ops) CreateInquiry(ctx context.Context, in Inquiry) (Inquiry, error) {
	if in.PhilosopherID == "" || in.ConversationID == "" || in.Question == "" || in.Quote == "" ||
		len([]rune(in.Question)) > MaxEntryText || len([]rune(in.Quote)) > MaxEntryQuote {
		return Inquiry{}, ErrBadEntry
	}
	in.ID = newID()
	now := o.stamp()
	q, err := o.c.seal("inquiry", "question", in.ID, []byte(in.Question))
	if err != nil {
		return Inquiry{}, err
	}
	quote, err := o.c.seal("inquiry", "quote", in.ID, []byte(in.Quote))
	if err != nil {
		return Inquiry{}, err
	}
	chal := 0
	if in.Challenge {
		chal = 1
	}
	_, err = o.q.ExecContext(ctx, `INSERT INTO inquiry(id, philosopher_id, conversation_id, parent_id, question, quote, created_at, updated_at, challenge)
		VALUES (?, ?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?)`, in.ID, in.PhilosopherID, in.ConversationID, in.ParentID, q, quote, now, now, chal)
	if err != nil {
		return Inquiry{}, err
	}
	in.CreatedAt, in.UpdatedAt = time.Unix(now, 0), time.Unix(now, 0)
	return in, nil
}

// CurrentInquiry is the newest inquiry of a conversation.
func (o *Ops) CurrentInquiry(ctx context.Context, conversationID string) (Inquiry, bool, error) {
	in, err := o.scanInquiry(o.q.QueryRowContext(ctx, "SELECT "+inquiryColumns+" FROM inquiry WHERE conversation_id = ? ORDER BY created_at DESC, rowid DESC LIMIT 1", conversationID))
	if errors.Is(err, sql.ErrNoRows) {
		return Inquiry{}, false, nil
	}
	return in, err == nil, err
}

// HasInquiry says whether a conversation has had any inquiry.
func (o *Ops) HasInquiry(ctx context.Context, conversationID string) (bool, error) {
	var n int
	err := o.q.QueryRowContext(ctx, "SELECT COUNT(*) FROM inquiry WHERE conversation_id = ?", conversationID).Scan(&n)
	return n > 0, err
}

// LinkEntries ties ledger entries to an inquiry (those it rests on).
func (o *Ops) LinkEntries(ctx context.Context, inquiryID string, entryIDs ...string) error {
	for _, id := range entryIDs {
		if _, err := o.q.ExecContext(ctx, "INSERT OR IGNORE INTO inquiry_entry(inquiry_id, entry_id) SELECT ?, id FROM ledger WHERE id = ?", inquiryID, id); err != nil {
			return err
		}
	}
	return nil
}

// LinkInherited ties to a new inquiry entries it takes over from earlier work; they are marked, and the score of a deepening leaves them out
// when it decides whether the deepening is finished.
func (o *Ops) LinkInherited(ctx context.Context, inquiryID string, entryIDs ...string) error {
	for _, id := range entryIDs {
		if _, err := o.q.ExecContext(ctx, "INSERT OR IGNORE INTO inquiry_entry(inquiry_id, entry_id, inherited) SELECT ?, id, 1 FROM ledger WHERE id = ?", inquiryID, id); err != nil {
			return err
		}
	}
	return nil
}

// InheritedEntryIDs is the set of entries an inquiry took over from earlier work.
func (o *Ops) InheritedEntryIDs(ctx context.Context, inquiryID string) (map[string]bool, error) {
	rows, err := o.q.QueryContext(ctx, "SELECT entry_id FROM inquiry_entry WHERE inquiry_id = ? AND inherited = 1", inquiryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

// LinkOrphans ties to an inquiry the entries of its conversation that belong to no inquiry yet (the first inquiry of a conversation
// takes what was noted before it was recognised).
func (o *Ops) LinkOrphans(ctx context.Context, inquiryID, conversationID string) error {
	_, err := o.q.ExecContext(ctx, `INSERT OR IGNORE INTO inquiry_entry(inquiry_id, entry_id)
		SELECT ?, id FROM ledger WHERE conversation_id = ? AND id NOT IN (SELECT entry_id FROM inquiry_entry)`, inquiryID, conversationID)
	return err
}

// InquiryEntries are the entries an inquiry rests on, in all statuses, oldest first.
func (o *Ops) InquiryEntries(ctx context.Context, inquiryID string) ([]Entry, error) {
	rows, err := o.q.QueryContext(ctx, "SELECT "+entryColumns+" FROM ledger WHERE id IN (SELECT entry_id FROM inquiry_entry WHERE inquiry_id = ?) ORDER BY created_at, rowid", inquiryID)
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

// InquiryEntryIDs is the set of entry ids linked to an inquiry.
func (o *Ops) InquiryEntryIDs(ctx context.Context, inquiryID string) (map[string]bool, error) {
	rows, err := o.q.QueryContext(ctx, "SELECT entry_id FROM inquiry_entry WHERE inquiry_id = ?", inquiryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

// Bank raises the banked maturity of an inquiry (never lowers it).
func (o *Ops) Bank(ctx context.Context, inquiryID string, thousandths int) error {
	_, err := o.q.ExecContext(ctx, "UPDATE inquiry SET banked = MAX(banked, ?), updated_at = ? WHERE id = ?", thousandths, o.stamp(), inquiryID)
	return err
}

// CompleteInquiry marks an inquiry finished and records the challenge. It does so once: a second call changes nothing.
func (o *Ops) CompleteInquiry(ctx context.Context, inquiryID string) error {
	res, err := o.q.ExecContext(ctx, "UPDATE inquiry SET completed = 1, updated_at = ? WHERE id = ? AND completed = 0", o.stamp(), inquiryID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil
	}
	_, err = o.q.ExecContext(ctx, "INSERT INTO challenge(id, philosopher_id, inquiry_id, created_at) SELECT ?, philosopher_id, id, ? FROM inquiry WHERE id = ?", newID(), o.stamp(), inquiryID)
	return err
}

// ChallengesSince counts the challenges recorded at or after a time.
func (o *Ops) ChallengesSince(ctx context.Context, since time.Time) (int, error) {
	var n int
	err := o.q.QueryRowContext(ctx, "SELECT COUNT(*) FROM challenge WHERE created_at >= ?", since.Unix()).Scan(&n)
	return n, err
}

// ChallengeCounts is the number of challenges finished with each philosopher.
func (o *Ops) ChallengeCounts(ctx context.Context) (map[string]int, error) {
	rows, err := o.q.QueryContext(ctx, "SELECT philosopher_id, COUNT(*) FROM challenge GROUP BY philosopher_id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var id string
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, rows.Err()
}

// Challenge is a finished challenge as it travels in an export: no text, only whom it was with and when.
type Challenge struct {
	PhilosopherID string
	CreatedAt     time.Time
}

// Challenges lists every finished challenge.
func (o *Ops) Challenges(ctx context.Context) ([]Challenge, error) {
	rows, err := o.q.QueryContext(ctx, "SELECT philosopher_id, created_at FROM challenge ORDER BY created_at, rowid")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Challenge
	for rows.Next() {
		var c Challenge
		var t int64
		if err := rows.Scan(&c.PhilosopherID, &t); err != nil {
			return nil, err
		}
		c.CreatedAt = time.Unix(t, 0)
		out = append(out, c)
	}
	return out, rows.Err()
}

// AddChallenge records a challenge brought in by an import.
func (o *Ops) AddChallenge(ctx context.Context, c Challenge) error {
	id := newID()
	_, err := o.q.ExecContext(ctx, "INSERT INTO challenge(id, philosopher_id, inquiry_id, created_at) VALUES (?, ?, ?, ?)", id, c.PhilosopherID, "imported:"+id, c.CreatedAt.Unix())
	return err
}

// TakeAnnouncements marks the finished inquiries of a conversation as told and returns how many were not told yet.
func (o *Ops) TakeAnnouncements(ctx context.Context, conversationID string) (int, error) {
	res, err := o.q.ExecContext(ctx, "UPDATE inquiry SET announced = 1 WHERE conversation_id = ? AND completed = 1 AND announced = 0", conversationID)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// DeleteInquiry removes an inquiry (used to undo one that was begun for a turn that then failed).
func (o *Ops) DeleteInquiry(ctx context.Context, id string) error {
	res, err := o.q.ExecContext(ctx, "DELETE FROM inquiry WHERE id = ?", id)
	return affected(res, err)
}
