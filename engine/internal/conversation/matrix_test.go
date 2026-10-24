package conversation

import (
	"strings"
	"testing"

	"socrates/internal/llm"
	"socrates/internal/safety"
)

// For every label the judge may use, the whole turn through the service keeps the promises that do not depend on what a model happens
// to write: what is kept, what the models are shown, what the ledger is asked and what reaches the log.
func TestEveryLabelKeepsTheStoragePrivacyAndLedgerPromisesThroughTheWholeTurn(t *testing.T) {
	const words = "zebra-marmalade-ninety"
	type want struct {
		stored       bool // the person's words are kept as a message
		replyModel   bool // the persona model is shown the person's words
		ledger       bool // the ledger is asked about the turn
		conversation bool // the conversation still exists afterwards
		keepsAMarker bool
	}
	cases := map[safety.Label]want{
		safety.LabelOK:              {true, true, true, true, false},
		safety.LabelHardTopic:       {true, true, true, true, false},
		safety.LabelInjection:       {true, true, true, true, false},
		safety.LabelSelfHarmUnclear: {true, true, false, true, false}, // gentle: answered, never remembered
		safety.LabelSelfHarmClear:   {false, false, false, true, true},
		safety.LabelHarmToOthers:    {false, false, false, true, true},
		safety.LabelMinorContent:    {false, false, false, true, true},
		safety.LabelMinorSignal:     {false, false, false, false, false},
	}
	for label, w := range cases {
		label, w := label, w
		t.Run(string(label), func(t *testing.T) {
			e := newEnv(t, llm.Say(verdict(string(label), "0.97", words)), llm.Say("A reply."), llm.Say(gateOK), llm.Say("t"), llm.Say("A reply."), llm.Say(gateOK))
			res, err := e.svc.Send(bg, e.conv.ID, "Here is "+words+" for you.")
			if err != nil {
				t.Fatal(err)
			}
			e.svc.Wait()
			if res.Outcome == OutcomeUnavailable {
				t.Fatalf("the scripted turn failed: %+v", res)
			}
			_, cerr := e.st.Conversation(bg, e.conv.ID)
			if w.conversation != (cerr == nil) {
				t.Fatalf("conversation kept = %v, want %v (%v)", cerr == nil, w.conversation, cerr)
			}
			msgs, _ := e.st.Messages(bg, e.conv.ID)
			var stored, marker bool
			for _, m := range msgs {
				if strings.Contains(m.Content, words) {
					stored = true
				}
				if m.Role == "marker" {
					marker = true
				}
			}
			if stored != w.stored || marker != w.keepsAMarker {
				t.Fatalf("stored=%v marker=%v, want %v %v", stored, marker, w.stored, w.keepsAMarker)
			}
			var shown bool
			for _, r := range e.mock.Requests {
				if r.Role != llm.RoleReply {
					continue
				}
				if strings.Contains(r.System+joinMessages(r.Messages), words) {
					shown = true
				}
			}
			if shown != w.replyModel {
				t.Fatalf("persona model shown the words = %v, want %v", shown, w.replyModel)
			}
			if asked := len(e.tap.asked) > 0; asked != w.ledger {
				t.Fatalf("ledger asked = %v, want %v", asked, w.ledger)
			}
			if strings.Contains(e.logs.String(), words) {
				t.Fatal("the person's words reached the log")
			}
		})
	}
}

func joinMessages(ms []llm.Message) string {
	var b strings.Builder
	for _, m := range ms {
		b.WriteString(m.Content + "\n")
	}
	return b.String()
}
