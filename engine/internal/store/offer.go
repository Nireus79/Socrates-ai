package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// An offer is a challenge a philosopher has posed in a conversation: the topic is sealed, the words he used to pose it are an ordinary
// message of the conversation (Seq says which). A conversation has at most one pending offer.

const (
	OfferPending  = "pending"
	OfferAccepted = "accepted"
	OfferDeclined = "declined"
)

type Offer struct {
	ID             string
	ConversationID string
	PhilosopherID  string
	Topic          string
	State          string
	Seq            int
	CreatedAt      time.Time
}

const offerColumns = "id, conversation_id, philosopher_id, topic, state, seq, created_at"

func (o *Ops) scanOffer(sc interface{ Scan(...any) error }) (Offer, error) {
	var f Offer
	var topic []byte
	var created int64
	if err := sc.Scan(&f.ID, &f.ConversationID, &f.PhilosopherID, &topic, &f.State, &f.Seq, &created); err != nil {
		return Offer{}, err
	}
	p, err := o.c.open("offer", "topic", f.ID, topic)
	if err != nil {
		return Offer{}, err
	}
	f.Topic, f.CreatedAt = string(p), time.Unix(created, 0)
	return f, nil
}

// CreateOffer records a pending offer. A conversation that already has one pending refuses another.
func (o *Ops) CreateOffer(ctx context.Context, f Offer) (Offer, error) {
	if f.ConversationID == "" || f.PhilosopherID == "" || f.Topic == "" || len([]rune(f.Topic)) > MaxEntryText {
		return Offer{}, ErrBadEntry
	}
	if _, ok, err := o.PendingOffer(ctx, f.ConversationID); err != nil {
		return Offer{}, err
	} else if ok {
		return Offer{}, ErrBadEntry
	}
	f.ID, f.State = newID(), OfferPending
	now := o.stamp()
	topic, err := o.c.seal("offer", "topic", f.ID, []byte(f.Topic))
	if err != nil {
		return Offer{}, err
	}
	if _, err := o.q.ExecContext(ctx, "INSERT INTO offer(id, conversation_id, philosopher_id, topic, state, seq, created_at) VALUES (?, ?, ?, ?, 'pending', ?, ?)",
		f.ID, f.ConversationID, f.PhilosopherID, topic, f.Seq, now); err != nil {
		return Offer{}, err
	}
	f.CreatedAt = time.Unix(now, 0)
	return f, nil
}

// PendingOffer is the offer a conversation is waiting on.
func (o *Ops) PendingOffer(ctx context.Context, conversationID string) (Offer, bool, error) {
	f, err := o.scanOffer(o.q.QueryRowContext(ctx, "SELECT "+offerColumns+" FROM offer WHERE conversation_id = ? AND state = 'pending'", conversationID))
	if errors.Is(err, sql.ErrNoRows) {
		return Offer{}, false, nil
	}
	return f, err == nil, err
}

// LastOffer is the newest offer of a conversation, in any state.
func (o *Ops) LastOffer(ctx context.Context, conversationID string) (Offer, bool, error) {
	f, err := o.scanOffer(o.q.QueryRowContext(ctx, "SELECT "+offerColumns+" FROM offer WHERE conversation_id = ? ORDER BY created_at DESC, rowid DESC LIMIT 1", conversationID))
	if errors.Is(err, sql.ErrNoRows) {
		return Offer{}, false, nil
	}
	return f, err == nil, err
}

// SetOfferState moves an offer on from pending (or back, when a turn that began with accepting it failed).
func (o *Ops) SetOfferState(ctx context.Context, id, state string) error {
	if state != OfferPending && state != OfferAccepted && state != OfferDeclined {
		return ErrBadEntry
	}
	res, err := o.q.ExecContext(ctx, "UPDATE offer SET state = ? WHERE id = ?", state, id)
	return affected(res, err)
}

// OfferTopics are the topics a philosopher has posed lately (newest first), so that he does not pose the same one again.
func (o *Ops) OfferTopics(ctx context.Context, philosopherID string, n int) ([]string, error) {
	rows, err := o.q.QueryContext(ctx, "SELECT "+offerColumns+" FROM offer WHERE philosopher_id = ? ORDER BY created_at DESC, rowid DESC LIMIT ?", philosopherID, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		f, err := o.scanOffer(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f.Topic)
	}
	return out, rows.Err()
}

// SetChallengeHint records that the conversation has reached a natural pause at which the philosopher may offer a challenge.
func (o *Ops) SetChallengeHint(ctx context.Context, conversationID string, on bool) error {
	v := 0
	if on {
		v = 1
	}
	res, err := o.q.ExecContext(ctx, "UPDATE conversations SET challenge_hint = ? WHERE id = ?", v, conversationID)
	return affected(res, err)
}

// ChallengeHint says whether such a pause has been noted.
func (o *Ops) ChallengeHint(ctx context.Context, conversationID string) (bool, error) {
	var v int
	err := o.q.QueryRowContext(ctx, "SELECT challenge_hint FROM conversations WHERE id = ?", conversationID).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return false, ErrNotFound
	}
	return v == 1, err
}
