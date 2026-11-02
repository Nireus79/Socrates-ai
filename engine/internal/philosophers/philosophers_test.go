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

func TestEveryoneHoldsConstructionsAsMeansAndEachHasHisStanceOnUnpopularViews(t *testing.T) {
	for _, p := range New().All() {
		sys := p.System()
		if !strings.Contains(sys, "not your conclusion") || !strings.Contains(sys, "only for a reason you can state") {
			t.Errorf("%s: the shared principle on constructions and pressure is missing", p.ID)
		}
		if !strings.Contains(sys, "the whole course of it") || !strings.Contains(sys, "answer it first") {
			t.Errorf("%s: the whole-work and answer-first sentences are missing", p.ID)
		}
		if strings.Contains(sys, "%!") || strings.Contains(sys, "%[") {
			t.Errorf("%s: the persona has a formatting error", p.ID)
		}
	}
	r := New()
	soc, _ := r.Get("socrates")
	pl, _ := r.Get("plato")
	ar, _ := r.Get("aristotle")
	if strings.Contains(soc.System(), "why it seemed right to you then") || strings.Contains(pl.System(), "why it seemed right to you then") {
		t.Error("Socrates and Plato must not be told to explain why an unpopular belief seemed right; that presumes a sincere belief")
	}
	if !strings.Contains(ar.System(), "why it seemed right to you then") {
		t.Error("Aristotle keeps the shared stance")
	}
	if !strings.Contains(pl.System(), "do not invent an intent") {
		t.Error("Plato must not invent his intent")
	}
}

func TestEveryPhilosopherHasHisTrapsForTheSecondCheck(t *testing.T) {
	seen := map[string]bool{}
	for _, p := range New().All() {
		if strings.TrimSpace(p.Traps) == "" || seen[p.Traps] {
			t.Errorf("%s has no traps of his own", p.ID)
		}
		seen[p.Traps] = true
	}
}

func TestOnlyThoseWhoMayBeQuestionedOnTheRepublicHaveItsNotesAndTheyReachTheCheckNotTheSpeaker(t *testing.T) {
	r := New()
	for _, p := range r.All() {
		want := p.ID == "socrates" || p.ID == "plato"
		if strings.Contains(p.System(), "nowhere on earth") {
			t.Errorf("%s: the notes must reach the check only (stage B was tried and dropped)", p.ID)
		}
		if want && (p.Notes != RepublicNotes || !strings.Contains(p.Notes, "readers differ")) {
			t.Errorf("%s lacks the notes", p.ID)
		}
	}
}

func TestEveryoneAsksWhichReadingOnlyWhenItChangesTheAnswerAndOnlyOnce(t *testing.T) {
	for _, p := range New().All() {
		sys := p.System()
		for _, want := range []string{"ask briefly which they mean", "once for a question", "pressed you for an answer", "plainly the natural one"} {
			if !strings.Contains(sys, want) {
				t.Errorf("%s: the clarification guard %q is missing", p.ID, want)
			}
		}
	}
}

func TestEveryPhilosopherPutsTheSharedQuestionsAndHisOwnToHimselfAndIsRemindedOfThem(t *testing.T) {
	seen := map[string]bool{}
	for _, p := range New().All() {
		if p.Test == "" || seen[p.Test] {
			t.Errorf("%s: his own question is missing or not his own", p.ID)
		}
		seen[p.Test] = true
		for _, part := range []string{"whole of the work", "what you hold", "only because you were asked again", p.Test} {
			if !strings.Contains(p.System(), part) {
				t.Errorf("%s: the persona lacks %q", p.ID, part)
			}
		}
		r := p.Reminder()
		for _, part := range []string{"whole of the work", "how firmly", "I was asked again", p.Test} {
			if !strings.Contains(r, part) {
				t.Errorf("%s: the reminder lacks %q", p.ID, part)
			}
		}
		if strings.Contains(p.System(), "%!") {
			t.Errorf("%s: a formatting error in the persona", p.ID)
		}
	}
}
