package store

import (
	"context"
	"time"
)

// Grants are what the person has bought, been given or earned for a while: product ids from a source (a store, a licence, an ad). The
// engine does not check them against anything: whoever holds the token can write them (the shell of the app does, after the store or the ad
// network has said so), and the person owns the file; see THREAT_MODEL.md.

const maxGrants = 200

// ReplaceGrants makes the standing (non-expiring) grants of a source exactly this list; the passes that expire are left alone.
func (o *Ops) ReplaceGrants(ctx context.Context, source string, products []string) error {
	if source == "" || len(products) > maxGrants {
		return ErrBadEntry
	}
	if _, err := o.q.ExecContext(ctx, "DELETE FROM grants WHERE source = ? AND expires_at IS NULL", source); err != nil {
		return err
	}
	for _, p := range products {
		if p == "" {
			return ErrBadEntry
		}
		if _, err := o.q.ExecContext(ctx, "INSERT OR IGNORE INTO grants(product_id, source, expires_at) VALUES (?, ?, NULL)", p, source); err != nil {
			return err
		}
	}
	return nil
}

// AddPass grants a product until a time (a rewarded ad, for instance). A later pass for the same product and source can only move the end later, never earlier.
func (o *Ops) AddPass(ctx context.Context, source, product string, until time.Time) error {
	if source == "" || product == "" {
		return ErrBadEntry
	}
	_, err := o.q.ExecContext(ctx, "INSERT INTO grants(product_id, source, expires_at) VALUES (?, ?, ?) ON CONFLICT(product_id, source) DO UPDATE SET expires_at = MAX(grants.expires_at, excluded.expires_at) WHERE grants.expires_at IS NOT NULL", product, source, until.Unix())
	return err
}

// ActiveGrants are the product ids in force at a time, without repeats. It only reads; PurgeGrants removes the expired passes.
func (o *Ops) ActiveGrants(ctx context.Context, now time.Time) ([]string, error) {
	rows, err := o.q.QueryContext(ctx, "SELECT DISTINCT product_id FROM grants WHERE expires_at IS NULL OR expires_at > ? ORDER BY product_id", now.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// PurgeGrants removes the passes that have ended.
func (o *Ops) PurgeGrants(ctx context.Context, now time.Time) error {
	_, err := o.q.ExecContext(ctx, "DELETE FROM grants WHERE expires_at IS NOT NULL AND expires_at <= ?", now.Unix())
	return err
}
