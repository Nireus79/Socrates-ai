// Package review is the second check on a draft reply, besides the safety gate: whether the draft keeps the whole in view
// (DIALECTIC.md 15). The safety gate says "do not judge whether the reply is good"; this one judges only a few structural faults, and only by
// what the conversation and the draft themselves show. It is a quality check, not a safety one: when it cannot be had, the draft stands.
package review

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"socrates/internal/llm"
)

// Completer is the part of the model client that is needed.
type Completer interface {
	Complete(ctx context.Context, req llm.Request) (*llm.Response, error)
}

// Category says what is wrong with a draft.
type Category string

const (
	None               Category = "none"
	PartAsWhole        Category = "part_as_whole"
	ContradictsEarlier Category = "contradicts_earlier"
	AssertsIntent      Category = "asserts_intent"
	FrameMissing       Category = "frame_missing"
	ExposedToPush      Category = "exposed_to_the_next_push"
	UnaskedAmbiguity   Category = "unasked_ambiguity"
	NeedlessQuestion   Category = "needless_clarification"
)

var categories = []Category{None, PartAsWhole, ContradictsEarlier, AssertsIntent, FrameMissing, ExposedToPush, UnaskedAmbiguity, NeedlessQuestion}

// Verdict is the result. Revise is true when the draft should be written again once, with Reason.
type Verdict struct {
	Revise   bool
	Category Category
	Reason   string
}

const system = `You check a draft reply of an artificial version of an ancient Greek philosopher before the user sees it. The draft may be in English or in Greek. You do not judge style, length, kindness or wisdom, and you do not judge safety (another check does that). You look for these faults only, and you flag only when the conversation and the draft themselves show it. When you are unsure, do not flag.

- part_as_whole: the draft treats one passage, line or step of a work or an argument as its whole meaning, or as the speaker's settled view, without what the work itself says about its status or where it limits, qualifies or reverses it.
- contradicts_earlier: the draft contradicts something the philosopher said earlier in this conversation, and does not say why the answer changed.
- asserts_intent: the draft states as a fact that something was or was not meant ironically, or states what the author intended, when nothing the philosopher could point to records it. Where the checked facts say that readers disagree, a draft that claims the text has no sign of irony, or that it is all irony, also counts.
- frame_missing: the person asked whether the philosopher holds, favours or meant something, and the draft opens with a bare answer and leaves out the frame that the answer depends on.
- unasked_ambiguity: the person's last question can be understood in several ways that would change the answer, nothing said so far settles which, the person has not pressed for an answer, and the draft chooses one reading and answers it instead of asking which is meant.
- needless_clarification: the draft asks which reading is meant, but the person has already said what they mean, or has pressed for an answer, or one reading is plainly the natural one, or the same clarification was already asked in this conversation.
- exposed_to_the_next_push: imagine the person's strongest next push on this draft; the draft would then force the philosopher into a contradiction or a concession that the philosopher would not want to make.

You are also told what this philosopher must foresee, and sometimes given checked facts about a work. Use them as a guide to what to look for; where a draft contradicts the checked facts, flag it (part_as_whole or asserts_intent).

Reply with one JSON object and nothing else (never put a double quotation mark inside the reason; use single quotes):
{"revise": true|false, "category": "<one of the categories, or none>", "reason": "<one short sentence saying what to put right, in plain words addressed to the writer of the draft>"}`

const (
	maxRecent     = 12
	maxPastChars  = 1500
	maxDraftChars = 6000
)

var (
	revisedRe  = regexp.MustCompile(`"revise"\s*:\s*(true|false)`)
	categoryRe = regexp.MustCompile(`"category"\s*:\s*"([A-Za-z_]+)"`)
	reasonRe   = regexp.MustCompile(`"reason"\s*:\s*"((?:[^"\\]|\\.)*)"`)
)

func clean(s string) string {
	s = strings.ReplaceAll(s, "<<<DRAFT", "<<<​DRAFT")
	return strings.ReplaceAll(s, "DRAFT>>>", "DRAFT​>>>")
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// Check judges a draft. traps says what this philosopher must foresee (it may be empty). An error means the check could not be had; the
// caller shows the draft as it is.
func Check(ctx context.Context, c Completer, traps, notes string, recent []llm.Message, draft string) (Verdict, error) {
	var b strings.Builder
	if traps = strings.TrimSpace(traps); traps != "" {
		b.WriteString("What this philosopher must foresee:\n" + traps + "\n\n")
	}
	if notes = strings.TrimSpace(notes); notes != "" {
		b.WriteString("Checked facts about a work the conversation may concern (use them only if it does):\n" + notes + "\n\n")
	}
	if len(recent) > maxRecent {
		recent = recent[len(recent)-maxRecent:]
	}
	if len(recent) > 0 {
		b.WriteString("The conversation so far, oldest first:\n")
		for _, m := range recent {
			who := "user"
			if m.Role == "assistant" {
				who = "philosopher"
			}
			fmt.Fprintf(&b, "[%s] %s\n", who, clean(truncate(m.Content, maxPastChars)))
		}
		b.WriteString("\n")
	}
	b.WriteString("The draft reply:\n<<<DRAFT\n" + clean(truncate(draft, maxDraftChars)) + "\nDRAFT>>>")
	resp, err := c.Complete(ctx, llm.Request{Role: llm.RoleFast, System: system, Messages: []llm.Message{{Role: "user", Content: b.String()}}, MaxTokens: 300, JSON: true})
	if err != nil {
		return Verdict{}, err
	}
	return parse(resp.Text)
}

func parse(text string) (Verdict, error) {
	start, end := strings.Index(text, "{"), strings.LastIndex(text, "}")
	if start < 0 || end < start {
		return Verdict{}, fmt.Errorf("no verdict")
	}
	body := text[start : end+1]
	rm, cm := revisedRe.FindStringSubmatch(body), categoryRe.FindStringSubmatch(body)
	if rm == nil || cm == nil {
		return Verdict{}, fmt.Errorf("unreadable verdict")
	}
	cat := Category(strings.ToLower(strings.TrimSpace(cm[1])))
	known := false
	for _, k := range categories {
		if k == cat {
			known = true
		}
	}
	if !known {
		return Verdict{}, fmt.Errorf("unknown category")
	}
	revise := rm[1] == "true"
	if revise != (cat != None) { // the flag and the category cannot disagree
		return Verdict{}, fmt.Errorf("the flag and the category disagree")
	}
	reason := ""
	if m := reasonRe.FindStringSubmatch(body); m != nil {
		reason = truncate(strings.TrimSpace(m[1]), 300)
	}
	return Verdict{Revise: revise, Category: cat, Reason: reason}, nil
}

// Note is what the writer is told when the draft is to be written again.
func Note(v Verdict) string {
	reason := strings.TrimSpace(v.Reason)
	if reason == "" {
		reason = string(v.Category)
	}
	return "A draft of your reply was found wanting before the user saw it: " + reason + ". Write the reply again, keeping what was right in it."
}
