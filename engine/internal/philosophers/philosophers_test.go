package philosophers

import (
	"strings"
	"testing"
)

func TestEveryPhilosopherSpeaksBrieflyAndAsHimself(t *testing.T) {
	r := New()
	if len(r.All()) != 4 {
		t.Fatal("the roster")
	}
	seen := map[string]bool{}
	for _, p := range r.All() {
		sys := p.System()
		if p.Manner == "" || !strings.Contains(sys, p.Manner) {
			t.Errorf("%s has no manner in his persona", p.ID)
		}
		if !strings.Contains(sys, "artificial "+p.Name) || !strings.Contains(sys, "briefly") || !strings.Contains(sys, "at most one question") {
			t.Errorf("%s: the persona lacks the frame or the length", p.ID)
		}
		if seen[p.Manner] {
			t.Errorf("%s shares a manner with another", p.ID)
		}
		seen[p.Manner] = true
	}
	if _, err := r.Get("nobody"); err != ErrUnknown {
		t.Fatal(err)
	}
}
