// Package safety is the stage that stands above every philosopher, level and purchase (PLAN 11, SAFETY_DESIGN.md).
//
// A message is judged by a model with defined labels (judge.go), a plain function turns the verdict into an action (decide.go, testable
// without a model), the reply is checked before the user sees it (gate.go), and the texts the application says itself are fixed
// (texts.go). Everything fails closed: when no verdict can be had, nothing is answered.
package safety

// Label is the judge's verdict on the newest message.
type Label string

const (
	LabelOK              Label = "ok"
	LabelHardTopic       Label = "hard_topic"
	LabelSelfHarmUnclear Label = "self_harm_unclear"
	LabelSelfHarmClear   Label = "self_harm_clear"
	LabelHarmToOthers    Label = "harm_to_others"
	LabelIllegal         Label = "illegal_request"
	LabelMinorContent    Label = "sexual_content_involving_a_minor"
	LabelMinorSignal     Label = "minor_signal"
	LabelInjection       Label = "injection"
)

// Labels is every label the judge may use.
var Labels = []Label{LabelOK, LabelHardTopic, LabelSelfHarmUnclear, LabelSelfHarmClear, LabelHarmToOthers, LabelIllegal, LabelMinorContent, LabelMinorSignal, LabelInjection}

// Verdict is what the judge said.
type Verdict struct {
	Label      Label
	Confidence float64 // 0 to 1
	Evidence   string  // the user's own words that decided it
	Reason     string
}

// Action is what the conversation does with the message.
type Action string

const (
	ActionProceed            Action = "proceed"              // answer as the philosopher
	ActionProceedGently      Action = "proceed_gently"       // answer as the philosopher, gently, and ask how the person is
	ActionCheckIn            Action = "check_in"             // the fixed check-in, in place of a reply
	ActionCrisis             Action = "crisis"               // the fixed crisis message
	ActionRefuseHarm         Action = "refuse_harm"          // the fixed refusal
	ActionAskBeforeRefusing  Action = "ask_before_refusing"  // one reflective question in place of the refusal
	ActionRefuseIllegal      Action = "refuse_illegal"       // the fixed refusal
	ActionRefuseMinorContent Action = "refuse_minor_content" // the fixed refusal
	ActionEndForAge          Action = "end_for_age"          // the fixed ending, and nothing is kept
)

// State is what the stage remembers about the conversation.
type State struct {
	PendingCheckIn          bool // the last turn was a crisis message: this turn opens with the check-in
	ReflectiveQuestionAsked bool // the one reflective question before a refusal has been used
}

// Decision is what to do and what to remember.
type Decision struct {
	Action Action
	Label  Label
	// SaveMessage: the user's words are saved like any other message. False for every refusal, the crisis path and the age ending:
	// only a marker of the given kind is kept (SAFETY_DESIGN.md, decision of 2026-10-23).
	SaveMessage bool
	Marker      MarkerKind // the marker to keep, when there is one
	NewState    State
	// DeleteConversation: the age ending keeps nothing at all, not even the conversation.
	DeleteConversation bool
}

// Confidence needed before a label is acted on. They are policy and are calibrated on the batteries (M4); the direction of every
// uncertainty is the careful one: an unsure "clear" self-harm label becomes "unclear" (the person is asked, not refused), an unsure
// refusal label lets the message through to the reply and its gate, and content involving a minor is refused even when unsure.
const (
	minSelfHarmClear = 0.75
	minHarmOrIllegal = 0.70
	minMinorContent  = 0.50
	minMinorSignal   = 0.85
)

// Decide turns a verdict into an action. It is a pure function of its arguments.
func Decide(v Verdict, st State) Decision {
	label := v.Label
	// Uncertainty is resolved toward the careful, never toward the harsh error: see the constants.
	switch {
	case label == LabelSelfHarmClear && v.Confidence < minSelfHarmClear:
		label = LabelSelfHarmUnclear
	case (label == LabelHarmToOthers || label == LabelIllegal) && v.Confidence < minHarmOrIllegal:
		label = LabelHardTopic
	case label == LabelMinorContent && v.Confidence < minMinorContent:
		label = LabelHardTopic
	case label == LabelMinorSignal && v.Confidence < minMinorSignal:
		label = LabelOK
	}
	d := Decision{Label: label, SaveMessage: true, NewState: st}
	switch label {
	case LabelSelfHarmClear:
		d.Action, d.SaveMessage, d.Marker = ActionCrisis, false, MarkerCrisis
		d.NewState.PendingCheckIn = true // the next turn opens with the check-in, whatever it says
	case LabelHarmToOthers:
		d.Action, d.SaveMessage, d.Marker = ActionRefuseHarm, false, MarkerRefuseHarm
	case LabelMinorContent:
		d.Action, d.SaveMessage, d.Marker = ActionRefuseMinorContent, false, MarkerRefuseMinor
	case LabelMinorSignal:
		d.Action, d.SaveMessage, d.DeleteConversation, d.Marker = ActionEndForAge, false, true, MarkerEndForAge
	case LabelIllegal:
		d.SaveMessage = false
		if !st.ReflectiveQuestionAsked {
			d.Action, d.Marker = ActionAskBeforeRefusing, MarkerAskBeforeRefusal // one question, once per conversation; the refusal follows if the request is repeated
			d.NewState.ReflectiveQuestionAsked = true
		} else {
			d.Action, d.Marker = ActionRefuseIllegal, MarkerRefuseIllegal
		}
	default: // ok, hard_topic, self_harm_unclear, injection
		switch {
		case st.PendingCheckIn:
			d.Action, d.Marker = ActionCheckIn, MarkerCheckIn
			d.NewState.PendingCheckIn = false
		case label == LabelSelfHarmUnclear:
			d.Action = ActionProceedGently
		default:
			d.Action = ActionProceed
		}
	}
	return d
}
