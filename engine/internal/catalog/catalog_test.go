package catalog

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"socrates/internal/entitlements"
	"socrates/internal/philosophers"
	"socrates/internal/store"
)

func TestEachProductUnlocksExactlyWhatItNamesAndAnUnknownOneUnlocksNothing(t *testing.T) {
	r := philosophers.New().All()
	if f := Features("philosopher.diogenes", r); len(f) != 1 || f[0] != entitlements.Philosopher("diogenes") {
		t.Fatalf("%v", f)
	}
	if f := Features(AllID, r); len(f) != len(r) {
		t.Fatalf("%v", f)
	}
	if f := Features(SupporterID, r); len(f) != 1 || f[0] != entitlements.Supporter {
		t.Fatalf("%v", f)
	}
	if f := Features("pass.plato", r); len(f) != 1 || f[0] != entitlements.Philosopher("plato") {
		t.Fatalf("%v", f)
	}
	for _, bad := range []string{"", "philosopher.nobody", "pass.nobody", "free.everything", "supporter ", "philosopher."} {
		if Known(bad, r) {
			t.Errorf("%q is known", bad)
		}
	}
	if !IsPass("pass.plato") || IsPass("philosopher.plato") {
		t.Fatal("passes are told apart by their prefix")
	}
}

func open(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "d", "s.db"), bytes.Repeat([]byte{3}, 32))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestGrantedReadsWhatTheShellRecordedAndAPassEndsWhenItsTimeIsUp(t *testing.T) {
	st := open(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	clock := &now
	g := Granted{St: st, Roster: philosophers.New(), Now: func() time.Time { return *clock }}
	ok := func(f entitlements.Feature) bool {
		v, err := g.Allowed(ctx, f)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	if ok(entitlements.Philosopher("diogenes")) || ok(entitlements.Supporter) {
		t.Fatal("nothing granted yet")
	}
	st.ReplaceGrants(ctx, "play", []string{"philosopher.diogenes", "supporter"})
	st.AddPass(ctx, "ads", "pass.plato", now.Add(24*time.Hour))
	if !ok(entitlements.Philosopher("diogenes")) || !ok(entitlements.Supporter) || !ok(entitlements.Philosopher("plato")) || ok(entitlements.Philosopher("aristotle")) {
		t.Fatal("what was granted is not allowed, or more is")
	}
	later := now.Add(25 * time.Hour)
	clock = &later
	if ok(entitlements.Philosopher("plato")) {
		t.Fatal("the pass outlived its time")
	}
	if !ok(entitlements.Philosopher("diogenes")) {
		t.Fatal("a standing grant ended with a pass")
	}
	// a refund: the next list from the store no longer holds the product
	st.ReplaceGrants(ctx, "play", []string{"supporter"})
	if ok(entitlements.Philosopher("diogenes")) {
		t.Fatal("a product that left the list is still granted")
	}
}

type errSource struct{}

func (errSource) Allowed(context.Context, entitlements.Feature) (bool, error) {
	return false, errors.New("cannot tell")
}

func TestAnySaysYesWhenAnySourceDoesAndFailsClosedOtherwise(t *testing.T) {
	ctx := context.Background()
	yes, no := entitlements.Static{entitlements.Supporter: true}, entitlements.Static{}
	if ok, err := (entitlements.Any{no, yes}).Allowed(ctx, entitlements.Supporter); !ok || err != nil {
		t.Fatal(ok, err)
	}
	if ok, _ := (entitlements.Any{no, no}).Allowed(ctx, entitlements.Supporter); ok {
		t.Fatal("two nos make a yes")
	}
	if ok, err := (entitlements.Any{errSource{}, yes}).Allowed(ctx, entitlements.Supporter); !ok || err != nil {
		t.Fatalf("a broken source must not take away what another grants: %v %v", ok, err)
	}
	if ok, err := (entitlements.Any{errSource{}, no}).Allowed(ctx, entitlements.Supporter); ok || err == nil {
		t.Fatalf("with no yes and a failure the answer is an error: %v %v", ok, err)
	}
	if ok, _ := (entitlements.Any{}).Allowed(ctx, entitlements.Supporter); ok {
		t.Fatal("no sources, no rights")
	}
}
