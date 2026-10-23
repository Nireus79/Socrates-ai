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

func TestEveryOpeningSaysHeIsArtificialAndOnlyThatAndNoOneGetsATagline(t *testing.T) {
	for _, p := range New().All() {
		if !strings.Contains(p.Opening, "artificial") {
			t.Errorf("%s: opening must say he is artificial: %q", p.ID, p.Opening)
		}
		if p.ID != "socrates" && strings.Count(p.Opening, ".") != 1 {
			t.Errorf("%s: only the approved Socrates line has more than the plain statement: %q", p.ID, p.Opening)
		}
	}
}
