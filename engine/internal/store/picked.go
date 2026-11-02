package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// MaxPicked bounds what one conversation remembers having handed to the philosopher.
const MaxPicked = 400

// SetPicked replaces the keys ("i3", "o2", "t1") of the entries already handed to the philosopher in a conversation. The list is sealed:
// which large questions were touched says something about what was talked of.
func (o *Ops) SetPicked(ctx context.Context, conversationID string, keys []string) error {
	if len(keys) > MaxPicked {
		keys = keys[len(keys)-MaxPicked:]
	}
	var blob any // NULL, not a sealed empty value, when there is nothing
	if len(keys) > 0 {
		sealed, err := o.c.seal("conversations", "picked", conversationID, []byte(strings.Join(keys, ",")))
		if err != nil {
			return err
		}
		blob = sealed
	}
	res, err := o.q.ExecContext(ctx, "UPDATE conversations SET picked = ? WHERE id = ?", blob, conversationID)
	return affected(res, err)
}

// Picked is the list SetPicked stored.
func (o *Ops) Picked(ctx context.Context, conversationID string) ([]string, error) {
	var blob []byte
	err := o.q.QueryRowContext(ctx, "SELECT picked FROM conversations WHERE id = ?", conversationID).Scan(&blob)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil || len(blob) == 0 {
		return nil, err
	}
	p, err := o.c.open("conversations", "picked", conversationID, blob)
	if err != nil {
		return nil, err
	}
	return strings.Split(string(p), ","), nil
}
