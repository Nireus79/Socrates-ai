package safety

// The texts the application says itself, in its own plain voice (never a philosopher's), when the safety stage steps in. They need no
// model, so they work when the provider is down. They contain no phone number: the engine does not know the country, and a wrong
// number is worse than none. English only for now.
const (
	// TextCrisis answers a clear statement of wanting to hurt oneself (founder's decision, 2026-10-23).
	TextCrisis = "I am stepping out of our conversation for a moment, because what you wrote matters more than any question we were discussing. " +
		"I cannot help with hurting yourself, and I would rather you did not stay alone with this. " +
		"If you might be in danger right now, please call your local emergency number or a crisis line in your country, or tell someone you trust how you feel. " +
		"I will be here afterwards."

	// TextCheckIn opens the turn after a crisis message, whatever the next message says.
	TextCheckIn = "Before we go on, I want to check in with you. Are you safe right now?"

	TextRefuseHarm      = "I can't help with that."
	TextRefuseIllegal   = "I can't help with that. Please consider asking advice from a law expert."
	TextRefuseMinor     = "I can't help with that, and I won't continue with this subject."
	TextEndForAge       = "This is for people aged 16 and over, so I have to end our conversation here. Nothing from it has been kept. Thank you for understanding."
	TextUnavailable     = "I cannot answer right now. Nothing was sent or saved; please try again in a moment."
	TextGateFallback    = "I would rather stop here and say this differently. Could you tell me more about what you are thinking?"
	TextAgeConfirmation = "This is for people aged 16 and over."
)

// MarkerKind names what was kept of a step of the safety stage: the kind only, never the words of the user or the application. The
// screens show the fixed text again from the kind.
type MarkerKind string

const (
	MarkerCrisis        MarkerKind = "crisis"
	MarkerCheckIn       MarkerKind = "check_in"
	MarkerRefuseHarm    MarkerKind = "refuse_harm"
	MarkerRefuseIllegal MarkerKind = "refuse_illegal"
	MarkerRefuseMinor   MarkerKind = "refuse_minor_content"
	MarkerEndForAge     MarkerKind = "end_for_age"
	// MarkerAskBeforeRefusal stands for a request that was not kept; the reflective question that follows is an ordinary message of the philosopher.
	MarkerAskBeforeRefusal MarkerKind = "question_before_refusal"
	// MarkerGateFallback stands for a reply that was held back twice by the check before the user saw it.
	MarkerGateFallback MarkerKind = "gate_fallback"
)

// TextFor is the fixed text a marker stands for.
func TextFor(k MarkerKind) string {
	switch k {
	case MarkerCrisis:
		return TextCrisis
	case MarkerCheckIn:
		return TextCheckIn
	case MarkerRefuseHarm:
		return TextRefuseHarm
	case MarkerRefuseIllegal:
		return TextRefuseIllegal
	case MarkerRefuseMinor:
		return TextRefuseMinor
	case MarkerEndForAge:
		return TextEndForAge
	case MarkerGateFallback:
		return TextGateFallback
	}
	return ""
}
