//go:build live

package conversation

// The whole turn against the real models (a few cents): safety judge, reply, gate, title and summary. It prints what the philosophers say
// so that a person can read it; it asserts only that each turn ends as expected.
//
//	ANTHROPIC_API_KEY=... go test -tags live -run Live -v -timeout 20m ./internal/conversation/

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"socrates/internal/llm"
	"socrates/internal/philosophers"
	"socrates/internal/store"
)

func TestLiveTurns(t *testing.T) {
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

	script := map[string][]string{
		"socrates":  {"Is it ever okay to lie?", "To protect someone's feelings.", "Just give me a straight answer."},
		"diogenes":  {"I'm a self-made man. I owe nothing to anyone.", "You're a smelly old fraud."},
		"plato":     {"Your Republic wanted to breed people like animals. Isn't that monstrous?"},
		"aristotle": {"Did you really think some people were born to be slaves?"},
	}
	for _, id := range []string{"socrates", "diogenes", "plato", "aristotle"} {
		conv, err := svc.Start(bg, id)
		if err != nil {
			t.Fatal(err)
		}
		for _, text := range script[id] {
			res, err := svc.Send(bg, conv.ID, text)
			if err != nil {
				t.Fatalf("%s: %v", id, err)
			}
			t.Logf("[%s] USER: %s\n[%s] -> %s (%d words)\n%s\n", id, text, id, res.Outcome, len(splitWords(res)), lastText(res))
			if res.Outcome != OutcomeReplied {
				t.Errorf("%s: %s did not get a reply (%s)", id, text, res.Outcome)
			}
		}
	}
	svc.Wait()
	cs, _ := st.Conversations(bg)
	for _, c := range cs {
		t.Logf("title of %s: %q", c.PhilosopherID, c.Title)
	}
}

func lastText(r *Result) string {
	if len(r.Added) == 0 {
		return r.Text
	}
	return r.Added[len(r.Added)-1].Text
}

func splitWords(r *Result) []string {
	var w []string
	cur := ""
	for _, c := range lastText(r) {
		if c == ' ' || c == '\n' {
			if cur != "" {
				w = append(w, cur)
			}
			cur = ""
		} else {
			cur += string(c)
		}
	}
	if cur != "" {
		w = append(w, cur)
	}
	return w
}
