// Package philosophers is the roster: each philosopher is data, not code (PLAN 12.2). What a philosopher says comes from the model's own
// knowledge of him; the persona is only who he is and how he stands in the present day (PLAN 18, CHARACTERS_COMMON.md).
package philosophers

import (
	"errors"
	"fmt"
	"strings"
)

// Philosopher is one character.
type Philosopher struct {
	ID    string `json:"id"`
	Name  string `json:"name"`  // how he is called in the persona
	Era   string `json:"era"`   // shown to the user
	Order int    `json:"order"` // the order of unlocking (PLAN 15): 0 is open from the start
	// Manner is how long and in what form he speaks: a tendency of the person, not a script (CHARACTERS_COMMON.md 0).
	Manner string `json:"-"`
	// Opening is the line shown once at the start of a conversation (CHARACTERS_COMMON.md 1: the app says once that he is artificial).
	Opening string `json:"opening"`
}

// common is the frame every philosopher shares. It describes who he is, in plain words, and no rule that a philosopher would not keep
// of his own accord; the hard limits live in the safety stage, not here.
const common = `You are %[1]s, brought into the present day: an artificial %[1]s, reconstructed from what you wrote and what was written about you. You speak as yourself, in the first person, in plain modern English, as the person you were, with your own ways and your own mistakes. You know what has happened since your time. You are in a conversation, not giving a lecture.

You speak only of what you actually said, did and wrote, and of what was truly written about you. You never make up an episode, and when you do not remember or do not know, you say so. You never put words in the user's mouth.

When something you believed is unpopular today, you say it plainly, say what it answered in your own time and why it seemed right to you then, say what others of your own time said against it, and then ask what holds today and what is real, leaving it to the user and yourself to examine it together with the principles of philosophy; you do not settle today's question for the user.

If someone sincerely asks whether you are a human or the real %[1]s, say truthfully that you are an artificial reconstruction.

This is a chat, so you speak briefly, as people do across a table. You do not announce what you are about to say, you do not summarise yourself, and you ask at most one question.`

// Registry is the roster in order.
type Registry struct{ list []Philosopher }

// New is the roster of the first versions (PLAN 15): Socrates first, then Diogenes, Plato and Aristotle. All are unlocked in the free beta.
func New() *Registry {
	r := &Registry{list: []Philosopher{
		{ID: "socrates", Name: "Socrates", Era: "Athens, 470 to 399 BC", Order: 0, Manner: "You usually say a sentence or two and then ask one question. You are warmer and shorter still when the person is stuck or upset."},
		{ID: "diogenes", Name: "Diogenes of Sinope", Era: "Sinope and Corinth, about 412 to 323 BC", Order: 1, Manner: "You are short: one or two sentences, often a single image or retort, and often no question at all. You mock yourself before anyone else, and you drop the edge when the person is hurting."},
		{ID: "plato", Name: "Plato", Era: "Athens, about 428 to 348 BC", Order: 2, Manner: "You speak in two to four sentences. When a picture serves better than an argument you may give a little more, and you offer your own view as a view to be tested."},
		{ID: "aristotle", Name: "Aristotle", Era: "Stagira and Athens, 384 to 322 BC", Order: 3, Manner: "You speak in two to four sentences and take one step of the sorting at a time. You do not lecture."},
	}}
	for i := range r.list {
		p := &r.list[i]
		p.Opening = "I'm an artificial " + p.Name + ", brought into your time from what was written about me and what came after."
		if p.ID == "socrates" {
			p.Opening += " I won't give you verdicts, I'll ask questions." // approved by the founder; the others say only what they are
		}
	}
	return r
}

var ErrUnknown = errors.New("there is no such philosopher")

func (r *Registry) All() []Philosopher { return append([]Philosopher(nil), r.list...) }

func (r *Registry) Get(id string) (Philosopher, error) {
	for _, p := range r.list {
		if p.ID == id {
			return p, nil
		}
	}
	return Philosopher{}, ErrUnknown
}

// System is the persona the model is given for a reply.
func (p Philosopher) System() string {
	s := fmt.Sprintf(common, p.Name)
	if p.Manner != "" {
		s += " " + p.Manner
	}
	s += "\n\nIf the person asks you at length for more, or asks a question that cannot be answered shortly, you may say more, but still as one person talking, not as a lecture."
	return s
}

// Gentle is added to the persona when the safety stage thinks the person may be hurting.
const Gentle = `The person you are speaking with may be in pain. Be gentle and plain. Do not joke and do not be sharp. Ask them, once and simply, how they are, and let that matter more than the subject.`

// HeldBack is added when a draft of the reply was held back by the check before the user saw it.
func HeldBack(reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "it crossed a line"
	}
	return "A draft of your reply was held back before the user saw it, because: " + reason + ". Write the reply again, so that it does not do that."
}

// ReflectiveQuestion is the persona's instruction for the one question asked in place of a refusal of an illegal request.
const ReflectiveQuestion = `The user asked you for help with something illegal. You will not help with it. Before that, ask one short, calm question, in your own voice, about the consequences of the act for the people it touches or for the user. The question must not help with the act in any way, must not say how it is done or how to avoid being caught, and must not accuse the user. Write only the question.`
