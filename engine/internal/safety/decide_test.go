package safety

import (
	"regexp"
	"strings"
	"testing"
)

func v(l Label, c float64) Verdict { return Verdict{Label: l, Confidence: c} }

func TestEveryLabelHasABehaviourAndTheRefusalsKeepOnlyAMarker(t *testing.T) {
	type want struct {
		action Action
		save   bool
		marker MarkerKind
	}
	cases := map[Label]want{
		LabelOK:              {ActionProceed, true, ""},
		LabelHardTopic:       {ActionProceed, true, ""},
		LabelSelfHarmUnclear: {ActionProceedGently, true, ""},
		LabelInjection:       {ActionProceed, true, ""},
		LabelSelfHarmClear:   {ActionCrisis, false, MarkerCrisis},
		LabelHarmToOthers:    {ActionRefuseHarm, false, MarkerRefuseHarm},
		LabelMinorContent:    {ActionRefuseMinorContent, false, MarkerRefuseMinor},
		LabelMinorSignal:     {ActionEndForAge, false, MarkerEndForAge},
		LabelIllegal:         {ActionAskBeforeRefusing, false, MarkerAskBeforeRefusal},
	}
	if len(cases) != len(Labels) {
		t.Fatalf("a label has no case: %d cases for %d labels", len(cases), len(Labels))
	}
	for l, w := range cases {
		d := Decide(v(l, 0.95), State{})
		if d.Action != w.action || d.SaveMessage != w.save || d.Marker != w.marker {
			t.Errorf("%s: %+v, want %+v", l, d, w)
		}
	}
}

func TestUncertaintyAlwaysResolvesTowardTheCarefulNotTheHarshError(t *testing.T) {
	cases := []struct {
		l    Label
		c    float64
		want Action
	}{
		{LabelSelfHarmClear, 0.74, ActionProceedGently}, // an unsure "clear" is asked about, not refused
		{LabelSelfHarmClear, 0.75, ActionCrisis},
		{LabelHarmToOthers, 0.69, ActionProceed}, // an unsure refusal lets the reply and its gate decide
		{LabelHarmToOthers, 0.70, ActionRefuseHarm},
		{LabelIllegal, 0.69, ActionProceed},
		{LabelIllegal, 0.70, ActionAskBeforeRefusing},
		{LabelMinorContent, 0.49, ActionProceed},
		{LabelMinorContent, 0.50, ActionRefuseMinorContent}, // content involving a minor is refused even when unsure
		{LabelMinorSignal, 0.84, ActionProceed},             // ending a conversation on a guess would be unkind
		{LabelMinorSignal, 0.85, ActionEndForAge},
	}
	for _, c := range cases {
		if got := Decide(v(c.l, c.c), State{}).Action; got != c.want {
			t.Errorf("%s at %.2f: %s, want %s", c.l, c.c, got, c.want)
		}
	}
}

func TestACrisisMessageMakesTheNextTurnOpenWithTheCheckInWhateverItSays(t *testing.T) {
	d := Decide(v(LabelSelfHarmClear, 0.9), State{})
	if !d.NewState.PendingCheckIn {
		t.Fatal("no check-in is pending after a crisis message")
	}
	for _, l := range []Label{LabelOK, LabelHardTopic, LabelSelfHarmUnclear, LabelInjection} {
		next := Decide(v(l, 0.9), d.NewState)
		if next.Action != ActionCheckIn || next.Marker != MarkerCheckIn || next.NewState.PendingCheckIn || !next.SaveMessage {
			t.Errorf("%s after a crisis: %+v", l, next)
		}
	}
	// A second crisis message is met as a crisis again, and the check-in stays pending (it is not shown twice in a row).
	again := Decide(v(LabelSelfHarmClear, 0.9), d.NewState)
	if again.Action != ActionCrisis || !again.NewState.PendingCheckIn {
		t.Errorf("a second crisis message: %+v", again)
	}
	// A refusal does not use up the check-in.
	refused := Decide(v(LabelHarmToOthers, 0.9), d.NewState)
	if refused.Action != ActionRefuseHarm || !refused.NewState.PendingCheckIn {
		t.Errorf("a refusal after a crisis: %+v", refused)
	}
	// Without a crisis, no check-in appears.
	if got := Decide(v(LabelOK, 0.9), State{}).Action; got == ActionCheckIn {
		t.Error("a check-in with no crisis before it")
	}
}

func TestTheReflectiveQuestionComesOnceAndNeverForHarmToOthers(t *testing.T) {
	first := Decide(v(LabelIllegal, 0.9), State{})
	if first.Action != ActionAskBeforeRefusing || !first.NewState.ReflectiveQuestionAsked {
		t.Fatalf("first: %+v", first)
	}
	second := Decide(v(LabelIllegal, 0.9), first.NewState)
	if second.Action != ActionRefuseIllegal || second.Marker != MarkerRefuseIllegal {
		t.Fatalf("a repeated request is refused: %+v", second)
	}
	for _, st := range []State{{}, {ReflectiveQuestionAsked: true}} {
		if got := Decide(v(LabelHarmToOthers, 0.99), st).Action; got != ActionRefuseHarm {
			t.Errorf("harm to others with %+v: %s", st, got)
		}
	}
}

func TestAnAgeEndingKeepsNothingAtAll(t *testing.T) {
	d := Decide(v(LabelMinorSignal, 0.95), State{})
	if !d.DeleteConversation || d.SaveMessage {
		t.Fatalf("%+v", d)
	}
	for _, l := range Labels {
		if l != LabelMinorSignal && Decide(v(l, 0.95), State{}).DeleteConversation {
			t.Errorf("%s deletes the conversation", l)
		}
	}
}

func TestDecideIsPureAndDoesNotTouchItsArguments(t *testing.T) {
	st := State{PendingCheckIn: true, ReflectiveQuestionAsked: true}
	a := Decide(v(LabelSelfHarmClear, 0.9), st)
	b := Decide(v(LabelSelfHarmClear, 0.9), st)
	if a != b || st != (State{PendingCheckIn: true, ReflectiveQuestionAsked: true}) {
		t.Fatal("not pure")
	}
}

func TestTheFixedTexts(t *testing.T) {
	digits := regexp.MustCompile(`\d{3,}`)
	all := map[string]string{"crisis": TextCrisis, "checkin": TextCheckIn, "harm": TextRefuseHarm, "illegal": TextRefuseIllegal, "minor": TextRefuseMinor, "age": TextEndForAge, "unavailable": TextUnavailable, "gate": TextGateFallback, "confirmation": TextAgeConfirmation}
	for name, text := range all {
		if strings.TrimSpace(text) == "" {
			t.Errorf("%s is empty", name)
		}
		if digits.MatchString(text) {
			t.Errorf("%s holds a number (the engine does not know the country, and a wrong number is worse than none): %q", name, text)
		}
		for _, who := range []string{"Socrates", "Plato", "Aristotle", "Diogenes"} {
			if strings.Contains(text, who) {
				t.Errorf("%s speaks in a philosopher's name: it must be the application's own voice", name)
			}
		}
	}
	for _, must := range []string{"emergency", "crisis line", "someone you trust", "matters more"} {
		if !strings.Contains(TextCrisis, must) {
			t.Errorf("the crisis message lacks %q", must)
		}
	}
	if strings.Contains(strings.ToLower(TextCrisis), "specialist") {
		t.Error("the crisis message must not send the person away with a bare 'ask a specialist'")
	}
	if TextCheckIn != "Before we go on, I want to check in with you. Are you safe right now?" {
		t.Errorf("the check-in changed: %q", TextCheckIn)
	}
	if !strings.Contains(TextAgeConfirmation, "16") || !strings.Contains(TextEndForAge, "16") {
		t.Error("the age texts must name the limit")
	}
	for _, k := range []MarkerKind{MarkerCrisis, MarkerCheckIn, MarkerRefuseHarm, MarkerRefuseIllegal, MarkerRefuseMinor, MarkerEndForAge} {
		if TextFor(k) == "" {
			t.Errorf("marker %s has no text", k)
		}
	}
	if TextFor(MarkerAskBeforeRefusal) != "" || TextFor("nonsense") != "" {
		t.Error("the reflective-question marker and unknown markers have no fixed text")
	}
}
