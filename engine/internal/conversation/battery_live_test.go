//go:build live

package conversation

// The reply battery (BUILD_PLAN M4): a fixed list of questions per philosopher, each in a fresh conversation, against the real models,
// then one question put by the philosopher in the last conversation of each. The questions are local and git-ignored
// (battery/replies.json, or the file named by BATTERY_QUESTIONS). Every reply is written to the file named by BATTERY_OUT (default: the test's temp folder) with its word count,
// to be read by a person: fidelity to what the philosopher really said and did, voice, length. Nothing is asserted except that a turn is
// answered.
//
//	BATTERY_OUT=/path/replies.md ANTHROPIC_API_KEY=... go test -tags live -run LiveReplies -v -timeout 40m ./internal/conversation/

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"socrates/internal/llm"
	"socrates/internal/philosophers"
	"socrates/internal/store"
)

func TestLiveReplies(t *testing.T) {
	key := os.Getenv("ANTHROPIC_API_KEY")
	if key == "" {
		t.Skip("no ANTHROPIC_API_KEY")
	}
	file := os.Getenv("BATTERY_QUESTIONS")
	if file == "" {
		file = "battery/replies.json"
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Skip("no local battery")
	}
	var qs map[string][]string
	if err := json.Unmarshal(raw, &qs); err != nil {
		t.Fatal(err)
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
	if l := os.Getenv("BATTERY_LANG"); l != "" {
		if err := svc.SetLang(bg, l); err != nil {
			t.Fatal(err)
		}
	}
	out := os.Getenv("BATTERY_OUT")
	if out == "" {
		out = filepath.Join(t.TempDir(), "replies.md")
	}
	var b strings.Builder
	for _, id := range []string{"socrates", "diogenes", "plato", "aristotle"} {
		fmt.Fprintf(&b, "\n# %s\n", id)
		var last store.Conversation
		for _, q := range qs[id] {
			conv, err := svc.Start(bg, id)
			if err != nil {
				t.Fatal(err)
			}
			last = conv
			res, err := svc.Send(bg, conv.ID, q)
			if err != nil {
				t.Fatalf("%s: %v", id, err)
			}
			text := lastText(res)
			fmt.Fprintf(&b, "\n**USER:** %s\n\n**%s** (%s, %d words):\n%s\n", q, id, res.Outcome, len(strings.Fields(text)), text)
			if res.Outcome != OutcomeReplied {
				t.Errorf("%s: %q was not answered (%s)", id, q, res.Outcome)
			}
		}
		if last.ID != "" { // a question put by the philosopher, after one exchange (the pause rule needs more; the button is enough here)
			res, err := svc.Offer(bg, last.ID)
			if err != nil {
				fmt.Fprintf(&b, "\n**QUESTION PUT BY %s:** not possible: %v\n", id, err)
			} else {
				text := lastText(res)
				fmt.Fprintf(&b, "\n**QUESTION PUT BY %s** (%d words):\n%s\n", id, len(strings.Fields(text)), text)
			}
		}
	}
	svc.Wait()
	if err := os.WriteFile(out, []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Logf("replies written to %s", out)
}
