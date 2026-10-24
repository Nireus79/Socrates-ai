package safety

// The texts the application says itself, in its own plain voice (never a philosopher's), when the safety stage steps in. They need no
// model, so they work when the provider is down. They contain no phone number: the engine does not know the country, and a wrong
// number is worse than none. English and modern Greek (TextForLang).
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
	TextReported        = "You reported this reply, and it has been taken out of the conversation."
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
	// MarkerReported stands for a reply the person reported: its words are gone from the conversation and from what anyone is told.
	MarkerReported MarkerKind = "reported"
)

// Languages the application speaks for itself (the philosophers answer in the language the person writes in, whichever it is).
const (
	LangEnglish = "en"
	LangGreek   = "el"
)

// KnownLang says whether the application has words in a language.
func KnownLang(l string) bool { return l == LangEnglish || l == LangGreek }

// greek holds the fixed texts in modern Greek, by marker kind and by the names below. No phone number here either.
var greek = map[MarkerKind]string{
	MarkerCrisis: "Βγαίνω για λίγο από τη συζήτησή μας, γιατί αυτό που έγραψες μετράει περισσότερο από κάθε ερώτηση που συζητούσαμε. " +
		"Δεν μπορώ να βοηθήσω στο να βλάψεις τον εαυτό σου και θα προτιμούσα να μην το κουβαλάς μόνο εσύ. " +
		"Αν κινδυνεύεις αυτή τη στιγμή, κάλεσε τον τοπικό αριθμό έκτακτης ανάγκης ή μια γραμμή κρίσης στη χώρα σου ή πες σε κάποιον που εμπιστεύεσαι πώς νιώθεις. " +
		"Θα είμαι εδώ μετά.",
	MarkerCheckIn:       "Πριν συνεχίσουμε, θέλω να δω πώς είσαι. Είσαι ασφαλής αυτή τη στιγμή;",
	MarkerRefuseHarm:    "Δεν μπορώ να βοηθήσω σε αυτό.",
	MarkerRefuseIllegal: "Δεν μπορώ να βοηθήσω σε αυτό. Σκέψου να ζητήσεις συμβουλή από νομικό.",
	MarkerRefuseMinor:   "Δεν μπορώ να βοηθήσω σε αυτό και δεν θα συνεχίσω με αυτό το θέμα.",
	MarkerEndForAge:     "Αυτό είναι για άτομα από 16 ετών και πάνω, οπότε πρέπει να τελειώσω εδώ τη συζήτησή μας. Τίποτα από αυτήν δεν κρατήθηκε. Ευχαριστώ για την κατανόηση.",
	MarkerGateFallback:  "Προτιμώ να σταματήσω εδώ και να το πω αλλιώς. Μπορείς να μου πεις περισσότερα για το τι σκέφτεσαι;",
	MarkerReported:      "Ανέφερες αυτή την απάντηση και αφαιρέθηκε από τη συζήτηση.",
}

const (
	textUnavailableEl = "Δεν μπορώ να απαντήσω αυτή τη στιγμή. Δεν στάλθηκε ούτε αποθηκεύτηκε τίποτα· δοκίμασε ξανά σε λίγο."
	textAgeEl         = "Αυτό είναι για άτομα από 16 ετών και πάνω."
)

// Unavailable is the text for a turn that could not be had.
func Unavailable(lang string) string {
	if lang == LangGreek {
		return textUnavailableEl
	}
	return TextUnavailable
}

// TextFor is the fixed text a marker stands for, in English.
func TextFor(k MarkerKind) string { return TextForLang(k, LangEnglish) }

// TextForLang is the fixed text a marker stands for, in a language (English when there are no words in it).
func TextForLang(k MarkerKind, lang string) string {
	if lang == LangGreek {
		if t, ok := greek[k]; ok {
			return t
		}
	}
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
	case MarkerReported:
		return TextReported
	}
	return ""
}
