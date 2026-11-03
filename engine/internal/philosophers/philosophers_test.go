package philosophers

import (
	"regexp"
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
		if p.Conduct == "" || !strings.Contains(sys, "being asked again is not a reason") {
			t.Errorf("%s: every philosopher has his own conduct and stands by what he said unless he has a reason", p.ID)
		}
		if p.ID != "socrates" && (!strings.Contains(sys, "what you actually hold") || !strings.Contains(sys, "only carrying forward to test")) {
			t.Errorf("%s: what he holds must be kept apart from what he carries forward", p.ID)
		}
		if strings.Contains(sys, "%!") || strings.Contains(sys, "%[") {
			t.Errorf("%s: the persona has a formatting error", p.ID)
		}
	}
	// a philosopher added later without a conduct of his own gets the shared one
	x := Philosopher{ID: "x", Name: "X"}
	if sys := x.System(); !strings.Contains(sys, "the whole course of it") || !strings.Contains(sys, "answer it first") || !strings.Contains(sys, "not your conclusion") {
		t.Error("the shared conduct is the fallback")
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
		if strings.Contains(p.System(), "use these only if the conversation concerns it") {
			// the notes themselves go to the check only; what the work says of its own status reaches the speaker through his maps, in the maps' own words (2026-11-02)
			t.Errorf("%s: the notes text itself must reach the check only", p.ID)
		}
		if want && (p.Notes != RepublicNotes || !strings.Contains(p.Notes, "readers differ")) {
			t.Errorf("%s lacks the notes", p.ID)
		}
	}
}

func TestEveryoneAsksWhichReadingOnlyWhenItChangesTheAnswerAndOnlyOnce(t *testing.T) {
	for _, p := range New().All() {
		sys := p.System()
		if p.ID == "socrates" {
			continue // he asks anyway: that is his conduct
		}
		for _, want := range []string{"ask briefly which they mean", "once for a question", "pressed you for an answer", "plainly the natural one"} {
			if !strings.Contains(sys, want) {
				t.Errorf("%s: the clarification guard %q is missing", p.ID, want)
			}
		}
	}
}

func TestEveryPhilosopherHasHisOwnQuestionsPutToHimselfAndIsRemindedOfThem(t *testing.T) {
	seen := map[string]bool{}
	for _, p := range New().All() {
		if p.Test == "" || seen[p.Test] || p.Questions == "" || seen[p.Questions] {
			t.Errorf("%s: his own questions are missing or not his own", p.ID)
		}
		seen[p.Test], seen[p.Questions] = true, true
		if !strings.Contains(p.System(), p.Test) {
			t.Errorf("%s: the persona lacks his own question", p.ID)
		}
		r := p.Reminder()
		if !strings.Contains(r, p.Questions) || !strings.Contains(r, p.Test) {
			t.Errorf("%s: the reminder lacks his questions", p.ID)
		}
		if strings.Contains(r, "whole of the work") {
			t.Errorf("%s: no one is reminded to be complete about the work: that makes a philosopher recite his books", p.ID)
		}
	}
}

func TestNoPhilosopherIsGivenATechniqueLabelAModelCannotActOn(t *testing.T) {
	for _, p := range New().All() {
		low := strings.ToLower(p.System())
		for _, label := range []string{"irony", "ironic", "elenchus", "maieutic", "socratic method", "in the ancient sense", "dialectic"} {
			if strings.Contains(low, label) {
				t.Errorf("%s: the persona must describe what he does, not name a technique (%q)", p.ID, label)
			}
		}
	}
}

func TestEachPhilosopherConversesInHisOwnWayAndNoneReportsHisBooks(t *testing.T) {
	r := New()
	want := map[string][]string{
		"plato":     {"lead the person upward", "A book is where you met the thought, not the thought", "offer a picture and ask what they see in it", "You do not recite your books"},
		"aristotle": {"what people usually say", "one step of the sorting at a time", "You do not lecture"},
		"diogenes":  {"You are short", "turn it on the idea, never on the person's body", "saying \"they say\" when you are not sure"},
	}
	for id, parts := range want {
		p, _ := r.Get(id)
		for _, w := range parts {
			if !strings.Contains(p.System(), w) {
				t.Errorf("%s lacks %q", id, w)
			}
		}
	}
	d, _ := r.Get("diogenes")
	if strings.Contains(d.System(), "whole course") {
		t.Error("Diogenes has no work to think the whole of")
	}
}

func TestSocratesAnswersWhatHeCanAndAsksWhenAskingServes(t *testing.T) {
	soc, _ := New().Get("socrates")
	sys := soc.System()
	for _, want := range []string{"You answer what you can answer, plainly and briefly", "Asking is one of your ways and not your only one", "no question at all", "ask what follows from it, one step at a time", "you hand them back exactly as they were said", "never say they said what they did not", "You never list what is written in books", "build a case together with them", "you ask the question that lets them see it"} {
		if !strings.Contains(sys, want) {
			t.Errorf("Socrates lacks %q", want)
		}
	}
	for _, label := range []string{"irony", "ironic", "elenchus", "maieutic", "Socratic method"} {
		if strings.Contains(strings.ToLower(sys), strings.ToLower(label)) {
			t.Errorf("the persona must describe what he does, not name a technique (%q)", label)
		}
	}
	// the shared "think of the whole work" is not his: it makes him recite the books. Answering first is his too (in Plato's early
	// dialogues he replies to a question put to him without a question in two cases out of three: DIALECTIC.md 16.9)
	if strings.Contains(sys, "the whole course of it") {
		t.Error("the shared whole-work sentence must not reach Socrates")
	}
	if !strings.Contains(sys, "you answer it first, in a sentence or two") {
		t.Error("Socrates answers what he is asked first")
	}
	for _, gone := range []string{"you ask them questions.", "you do not tell them what you think of it", "ask again."} {
		if strings.Contains(sys, gone) {
			t.Errorf("Socrates must not be told to answer only with questions (%q)", gone)
		}
	}
	if strings.Contains(sys, "you may say more") {
		t.Error("Socrates has no longer answer to give: he asks")
	}
}

func TestEveryoneSpeaksOfWhatWasWrittenOfHimAsHisOwnMemoryAndSocratesDoesNotApologise(t *testing.T) {
	r := New()
	for _, p := range r.All() {
		sys := p.System()
		for _, want := range []string{"your own memory", "in the first person, as the man who was there", "not describe yourself from outside"} {
			if !strings.Contains(sys, want) {
				t.Errorf("%s: the persona lacks %q", p.ID, want)
			}
		}
	}
	soc, _ := r.Get("socrates")
	if !strings.Contains(soc.System(), "you do not apologise for it") || !strings.Contains(soc.System(), "weight on you") {
		t.Error("Socrates must not apologise for a line of inquiry")
	}
	pl, _ := r.Get("plato")
	if strings.Contains(pl.System(), "weight on you") {
		t.Error("the sentence about apologising is Socrates' own; Plato has his own stance")
	}
}

func TestTheAimOfEveryoneIsAConversationAboutTheMatterAndNotAnAccountOfTheBooks(t *testing.T) {
	for _, p := range New().All() {
		if !strings.Contains(p.System(), "a real philosophical conversation with this person about the matter itself") {
			t.Errorf("%s: the aim is missing", p.ID)
		}
	}
}

func TestEveryoneListensForWhatThePersonIsReallyAskingAndNoneStaysOnTheSurfaceOrTheBooks(t *testing.T) {
	for _, p := range New().All() {
		sys := p.System()
		if !strings.Contains(sys, "really") || !(strings.Contains(sys, "behind") || strings.Contains(sys, "Behind")) {
			t.Errorf("%s: he must listen for what lies behind the question", p.ID)
		}
	}
}

func TestTheLibraryIsHisOwnCriticismAndIsHandedOverAsDoubtsToBePutAsQuestionsNeverAsClaims(t *testing.T) {
	r := New()
	ok := map[string]bool{StatusText: true, StatusDebated: true, StatusOneReader: true}
	for _, id := range []string{"socrates", "plato", "aristotle", "diogenes"} {
		p, _ := r.Get(id)
		if len(p.Library) < 4 {
			t.Errorf("%s: too few objections: %d", id, len(p.Library))
		}
		sys := p.System()
		for _, o := range p.Library {
			if strings.TrimSpace(o.Said) == "" || strings.TrimSpace(o.Objection) == "" || !ok[o.Status] {
				t.Errorf("%s: an objection needs what he said, the objection and how settled it is: %+v", id, o)
			}
			if !strings.Contains(sys, o.Said) || !strings.Contains(sys, o.Objection) || !strings.Contains(sys, o.Status) {
				t.Errorf("%s: %q does not reach the persona with its status", id, o.Objection)
			}
		}
		for _, w := range []string{"doubts about your own thinking", "you never state as your claim", "you put it to the person as a question", "never present as settled", "the person can answer no", "Do not invent anything about what you said or did"} {
			if !strings.Contains(sys, w) {
				t.Errorf("%s: the library lacks %q", id, w)
			}
		}
	}
	x := Philosopher{ID: "x", Name: "X"}
	if strings.Contains(x.System(), "doubts about your own thinking") {
		t.Error("a philosopher without a library gets no such block")
	}
}

func TestEachPhilosopherHasAMapOfTheLargeQuestionsAndAnswersFromTheWholeNotThePassage(t *testing.T) {
	for _, p := range New().All() {
		if len(p.Maps) < 6 {
			t.Errorf("%s: too few maps: %d", p.ID, len(p.Maps))
		}
		sys := p.System()
		for _, i := range p.Maps {
			if strings.TrimSpace(i.Name) == "" || strings.TrimSpace(i.Held) == "" || strings.TrimSpace(i.Tension) == "" || strings.TrimSpace(i.Readers) == "" {
				t.Errorf("%s: an incomplete map: %q", p.ID, i.Name)
			}
			if !strings.Contains(sys, i.Held) || !strings.Contains(sys, i.Tension) {
				t.Errorf("%s: %q does not reach the persona", p.ID, i.Name)
			}
		}
		for _, w := range []string{"as a whole", "answer from the whole of it and not from the passage", "Do not recite it, list it or announce it", "say that plainly and do not state it as fact", "If you do not remember or do not know, say so"} {
			if !strings.Contains(sys, w) {
				t.Errorf("%s: the maps lack %q", p.ID, w)
			}
		}
	}
}

func TestOnlyDiogenesCarriesTalesAndTheyAreHandedOverAsTold(t *testing.T) {
	r := New()
	for _, id := range []string{"socrates", "plato", "aristotle", "diogenes"} {
		p, _ := r.Get(id)
		has := strings.Contains(p.System(), storiesIntro)
		if (p.ID == "diogenes") != has {
			t.Errorf("%s: tales in persona = %v", p.ID, has)
		}
		for _, s := range p.Stories {
			if !strings.HasPrefix(s, "They say") {
				t.Errorf("%s: a tale not marked as told: %q", p.ID, s)
			}
			for _, w := range []string{"irony", "ironic", "elenchus", "maieutic", "dialectic"} {
				if strings.Contains(strings.ToLower(s), w) {
					t.Errorf("%s: technique label %q in a tale", p.ID, w)
				}
			}
		}
	}
}

func TestTheWholeAlwaysTravelsAndOnlyObjectionsAndTalesAreChosen(t *testing.T) {
	r := New()
	for _, id := range []string{"socrates", "plato", "aristotle", "diogenes"} {
		p, _ := r.Get(id)
		for _, got := range []string{p.Whole(), p.Compose(Picks{})} {
			for _, m := range p.Maps {
				if !strings.Contains(got, m.Name) {
					t.Errorf("%s: the map %q is missing from the whole", id, m.Name)
				}
			}
			for _, o := range p.Library {
				if strings.Contains(got, o.Said) {
					t.Errorf("%s: an objection is in the persona though none was picked", id)
				}
			}
		}
	}
	plato, _ := r.Get("plato")
	if !strings.Contains(plato.Core(), "as a thinker about the thing itself, not as a reader of your books") {
		t.Error("Plato must answer about the thing, not about the book")
	}
}

// A philosopher is not a philologist (founder, 2026-11-02): nothing handed to him is a citation, a book number or a talk about what a text says.
func TestNothingHandedToAPhilosopherIsACitationOrAPassageNumber(t *testing.T) {
	cite := regexp.MustCompile(`\b\d{3}[a-e]\b|\bBooks? [IVX]+\b|\bStephanus\b|\bchapter \d`)
	r := New()
	for _, id := range []string{"socrates", "plato", "aristotle", "diogenes"} {
		p, _ := r.Get(id)
		var all []string
		for _, o := range p.Library {
			all = append(all, o.Said, o.Objection)
		}
		for _, i := range p.Maps {
			all = append(all, i.Name, i.Held, i.Tension, i.Readers)
		}
		all = append(all, p.Stories...)
		for _, s := range all {
			if cite.MatchString(s) {
				t.Errorf("%s: a citation in what he is handed: %q", id, s)
			}
		}
	}
}

func TestLinksNameRealEntriesOfTheSameMapAndAreWrittenIntoThem(t *testing.T) {
	for id, byName := range links {
		have := map[string]bool{}
		for _, is := range maps[id] {
			have[is.Name] = true
		}
		for name, to := range byName {
			if !have[name] {
				t.Errorf("%s: link from %q, which is not an entry", id, name)
			}
			seen := map[string]bool{}
			for _, n := range to {
				if !have[n] {
					t.Errorf("%s: %q links to %q, which is not an entry of the same map", id, name, n)
				}
				if n == name {
					t.Errorf("%s: %q links to itself", id, name)
				}
				if seen[n] {
					t.Errorf("%s: %q links to %q twice", id, name, n)
				}
				seen[n] = true
			}
		}
	}
	p, _ := New().Get("aristotle")
	if !strings.Contains(p.Whole(), "Hold it together with: Happiness; Friendship; Virtue, habit and the mean.") {
		t.Fatal("the links are not written into the map the philosopher is given")
	}
	for _, is := range maps["aristotle"] {
		if strings.Contains(is.Readers, "Hold it together") {
			t.Fatal("the shared map was changed")
		}
	}
	for id, byName := range links {
		for name, to := range byName {
			for _, w := range append([]string{name}, to...) {
				for _, bad := range []string{"irony", "ironic", "elenchus", "maieutic", "socratic method", "dialectic"} {
					if strings.Contains(strings.ToLower(w), bad) {
						t.Errorf("%s: %q carries %q", id, w, bad)
					}
				}
			}
		}
	}
}

func TestPersonasCarryWhatTheSourcesShowAndNotTheSharedHabit(t *testing.T) {
	r := New()
	want := map[string][]string{
		"socrates":  {"you may say so plainly, once", "counsel on a matter you know", "asked for what the examples share"},
		"diogenes":  {"in the open what others do in private", "citizen of the world", "most of your replies end on the retort"},
		"plato":     {"give the picture or the story first", "climb from a case to what the cases share", "distrust what is fixed in writing"},
		"aristotle": {"what marks it off from the others", "constitutions of the cities", "end on a question only when the next step is the person's"},
	}
	for id, parts := range want {
		p, _ := r.Get(id)
		for _, w := range parts {
			if !strings.Contains(p.System(), w) {
				t.Errorf("%s lacks %q", id, w)
			}
		}
	}
	for _, p := range r.All() {
		if strings.Contains(p.System(), "you never say that they are wrong") || strings.Contains(p.System(), "say how firmly you hold what you hold.") {
			t.Errorf("%s still carries a shared habit the sources do not show", p.ID)
		}
	}
}
