//go:build live

package conversation

// Long talks against the real models, with a second model playing a thoughtful person, to see how the maturity of an inquiry grows turn
// by turn and how soon a question counts as thought through (PLAN 16; the thresholds are guesses until this is read). The person is
// simulated and so is generous with its own words; the numbers say what the ledger and the inquiry rule do with a cooperative person.
//
//	ANTHROPIC_API_KEY=... go test -tags live -run LiveThresholds -v -timeout 60m ./internal/conversation/

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"socrates/internal/llm"
	"socrates/internal/philosophers"
	"socrates/internal/progress"
	"socrates/internal/store"
)

const personSim = `You play an ordinary, thoughtful adult talking with a philosopher in a chat app. Write only your next message: one to three plain sentences in the first person, in your own words, like a real person typing. Answer what the philosopher just asked. Sometimes state a firm opinion, sometimes give a definition of a word in your own terms, sometimes admit a doubt or notice that two things you said do not fit together. Never mention that you are playing a role.`

func TestLiveThresholds(t *testing.T) {
	key := os.Getenv("ANTHROPIC_API_KEY")
	if key == "" {
		t.Skip("no ANTHROPIC_API_KEY")
	}
	client, err := llm.NewClient(llm.Settings{Provider: "claude", APIKey: key, FastModel: "claude-haiku-5-5", ReplyModel: "claude-sonnet-5-5"})
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(t.TempDir(), "d", "s.db"), bytes.Repeat([]byte{9}, 32))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	svc := New(st, func(context.Context) (Completer, error) { return client, nil }, philosophers.New())
	svc.ConfirmAge(bg)
	seeds := map[string]string{
		"socrates": "Is it ever okay to lie to someone you love?",
		"diogenes": "I think success is the only thing that matters in life.",
		"plato":    "What does it mean to be a good citizen?",
	}
	for _, id := range []string{"socrates", "diogenes", "plato"} {
		if only := os.Getenv("THRESH_ONLY"); only != "" && only != id {
			continue
		}
		conv, err := svc.Start(bg, id)
		if err != nil {
			t.Fatal(err)
		}
		msg := seeds[id]
		var transcript []llm.Message
		for turn := 1; turn <= 12; turn++ {
			res, err := svc.Send(bg, conv.ID, msg)
			if err != nil {
				t.Fatal(err)
			}
			svc.Wait()
			reply := lastText(res)
			inq, ok, _ := st.CurrentInquiry(bg, conv.ID)
			counts, _ := st.ChallengeCounts(bg)
			banked := 0
			if ok {
				banked = inq.Banked
			}
			var reading progress.Result
			if ok {
				es, _ := st.InquiryEntries(bg, inq.ID)
				reading = progress.Score(es, progress.Default)
			}
			t.Logf("[%s] turn %2d outcome=%s inquiry=%v banked=%d now=%.2f settled=%d own=%d examined=%d turns=%d done=%v challenges=%d | user: %.70s", id, turn, res.Outcome, ok, banked, reading.Maturity, reading.Settled, reading.Own, reading.Examined, inq.Turns, inq.Completed, counts[id], msg)
			if res.Outcome != OutcomeReplied {
				break
			}
			transcript = append(transcript, llm.Message{Role: "assistant", Content: reply})
			// the simulated person answers the philosopher
			var b strings.Builder
			for _, m := range transcript {
				b.WriteString("Philosopher: " + m.Content + "\n")
			}
			jr, err := client.Complete(bg, llm.Request{Role: llm.RoleFast, System: personSim, Messages: []llm.Message{{Role: "user", Content: "The conversation so far (your earlier messages are not repeated, answer the last philosopher message):\n" + b.String()}}, MaxTokens: 300})
			if err != nil {
				t.Fatal(err)
			}
			msg = strings.TrimSpace(jr.Text)
			transcript = append(transcript, llm.Message{Role: "user", Content: msg})
		}
		ents, _ := st.Entries(bg, id, false)
		kinds := map[string]int{}
		for _, e := range ents {
			kinds[e.Kind]++
		}
		t.Logf("[%s] ledger after the talk: %v", id, kinds)
	}
}
