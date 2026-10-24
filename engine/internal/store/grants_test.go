package store

import (
	"testing"
	"time"
)

func TestAPassNeverShortensAndExpiredOnesAreNotActive(t *testing.T) {
	s, _, _ := open(t)
	now := time.Now()
	if err := s.AddPass(ctx, "ads", "pass.plato", now.Add(72*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := s.AddPass(ctx, "ads", "pass.plato", now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.ActiveGrants(ctx, now.Add(10*time.Hour)); len(got) != 1 {
		t.Fatalf("a later, shorter pass cut the first: %v", got)
	}
	if got, _ := s.ActiveGrants(ctx, now.Add(80*time.Hour)); len(got) != 0 {
		t.Fatalf("an ended pass is active: %v", got)
	}
	var n int
	s.db.QueryRow("SELECT COUNT(*) FROM grants").Scan(&n)
	if n != 1 {
		t.Fatal("reading must not write")
	}
	if err := s.Update(ctx, func(o *Ops) error { return o.PurgeGrants(ctx, now.Add(80*time.Hour)) }); err != nil {
		t.Fatal(err)
	}
	s.db.QueryRow("SELECT COUNT(*) FROM grants").Scan(&n)
	if n != 0 {
		t.Fatal(n)
	}
}
