package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"socrates/internal/store"
)

var bg = context.Background()

// seed puts entries straight into the store for a conversation with the given philosopher and returns the conversation id.
func (a *app) seed(t *testing.T, philosopher string, entries ...store.Entry) (string, []store.Entry) {
	t.Helper()
	conv, err := a.st.CreateConversation(bg, philosopher)
	if err != nil {
		t.Fatal(err)
	}
	var out []store.Entry
	for _, e := range entries {
		e.PhilosopherID, e.ConversationID = philosopher, conv.ID
		got, err := a.st.AddEntry(bg, e)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, got)
	}
	return conv.ID, out
}

func TestTheLedgerIsListedPerPhilosopherWithTheUsersWordsAndCanBeDeleted(t *testing.T) {
	a := newApp(t, newScript(t))
	_, soc := a.seed(t, "socrates",
		store.Entry{Kind: store.KindClaim, Text: "lying is always wrong", Quote: "lying is always wrong"},
		store.Entry{Kind: store.KindTerm, Text: "lie", Detail: "saying what is false", Quote: "a lie is saying what is false", Suggested: true})
	a.seed(t, "plato", store.Entry{Kind: store.KindClaim, Text: "plato's own", Quote: "plato's own words here"})
	code, r := a.do(t, "GET", "/api/ledger?philosopherId=socrates", "")
	list, _ := r["entries"].([]any)
	if code != 200 || len(list) != 2 || strings.Contains(mustJSON(r), "plato's own") {
		t.Fatalf("%d %v", code, r)
	}
	first := list[0].(map[string]any)
	if first["quote"] != "lying is always wrong" || first["kind"] != "claim" || list[1].(map[string]any)["suggested"] != true {
		t.Fatalf("%v", list)
	}
	if code, _ := a.do(t, "GET", "/api/ledger?philosopherId=nobody", ""); code != 400 {
		t.Fatalf("unknown philosopher: %d", code)
	}
	if code, _ := a.do(t, "DELETE", "/api/ledger/"+soc[0].ID, ""); code != 204 {
		t.Fatalf("delete: %d", code)
	}
	if code, _ := a.do(t, "DELETE", "/api/ledger/"+soc[0].ID, ""); code != 404 {
		t.Fatalf("twice: %d", code)
	}
	if code, _ := a.do(t, "DELETE", "/api/ledger?philosopherId=socrates", ""); code != 204 {
		t.Fatalf("clear: %d", code)
	}
	_, r = a.do(t, "GET", "/api/ledger?philosopherId=socrates", "")
	if len(r["entries"].([]any)) != 0 {
		t.Fatal("clear left entries")
	}
	_, r = a.do(t, "GET", "/api/ledger?philosopherId=plato", "")
	if len(r["entries"].([]any)) != 1 {
		t.Fatal("clearing one philosopher's memory touched another's")
	}
}

func TestTheLedgerTravelsInTheExportWithItsTensionsAndComesBackOnImport(t *testing.T) {
	a := newApp(t, newScript(t))
	a.seed(t, "socrates")
	cid, es := a.seed(t, "socrates",
		store.Entry{Kind: store.KindClaim, Text: "one", Quote: "I said one thing here"},
		store.Entry{Kind: store.KindClaim, Text: "two", Quote: "I said two things here"},
		store.Entry{Kind: store.KindTerm, Text: "lie", Detail: "d", Quote: "a lie is saying d here", Suggested: true})
	tn, err := a.st.AddEntry(bg, store.Entry{PhilosopherID: "socrates", ConversationID: cid, Kind: store.KindTension, Text: "one versus two", Quote: "I said both here", A: es[0].ID, B: es[1].ID})
	if err != nil {
		t.Fatal(err)
	}
	a.st.ReviseEntry(bg, es[1].ID, "two", "", store.StatusRetracted)
	a.st.ReviseEntry(bg, tn.ID, tn.Text, "", store.StatusResolved)
	a.st.AppendMessage(bg, cid, "user", "hello")

	_, ex := a.do(t, "GET", "/api/profile/export", "")
	raw := mustJSON(ex)
	if !strings.Contains(raw, `"ledger"`) || !strings.Contains(raw, "one versus two") || strings.Contains(raw, es[0].ID) || strings.Contains(raw, "conversation_id") {
		t.Fatalf("export: %s", raw)
	}
	if code, res := a.do(t, "POST", "/api/profile/import", raw); code != 200 {
		t.Fatalf("%d %v", code, res)
	}
	all, _ := a.st.Entries(bg, "socrates", true)
	if len(all) != 8 {
		t.Fatalf("original 4 + imported 4, got %d", len(all))
	}
	var imported []store.Entry
	for _, e := range all {
		if e.ConversationID != cid {
			imported = append(imported, e)
		}
	}
	byText := map[string]store.Entry{}
	for _, e := range imported {
		byText[e.Text] = e
	}
	tension := byText["one versus two"]
	if tension.Status != store.StatusResolved || tension.A != byText["one"].ID || tension.B != byText["two"].ID || byText["two"].Status != store.StatusRetracted || !byText["lie"].Suggested || byText["lie"].Detail != "d" {
		t.Fatalf("%+v", imported)
	}
	if tension.A == es[0].ID {
		t.Fatal("an imported tension points into the original conversation")
	}
}

func TestAnImportWithABadMemoryEntryIsRefusedAsAWhole(t *testing.T) {
	a := newApp(t, newScript(t))
	good := exportEntry{Kind: "claim", Text: "x", Quote: "some quote", Status: "active"}
	conv := func(es ...exportEntry) exportFile {
		return exportFile{Format: exportFormat, Conversations: []exportConversation{{PhilosopherID: "socrates", Messages: []exportMessage{{Role: "user", Text: "hi"}}, Ledger: es}}}
	}
	long := strings.Repeat("x", store.MaxEntryText+1)
	bad := map[string]exportFile{
		"unknown kind":       conv(good, exportEntry{Kind: "belief", Text: "x", Quote: "q", Status: "active"}),
		"no quote":           conv(exportEntry{Kind: "claim", Text: "x", Status: "active"}),
		"too long":           conv(exportEntry{Kind: "claim", Text: long, Quote: "q", Status: "active"}),
		"unknown status":     conv(exportEntry{Kind: "claim", Text: "x", Quote: "q", Status: "weird"}),
		"a claim with link":  conv(good, exportEntry{Kind: "claim", Text: "y", Quote: "q", Status: "active", A: 1, B: 1}),
		"tension to itself":  conv(good, exportEntry{Kind: "tension", Text: "t", Quote: "q", Status: "active", A: 1, B: 1}),
		"tension to nothing": conv(good, exportEntry{Kind: "tension", Text: "t", Quote: "q", Status: "active", A: 1, B: 9}),
		"tension to a term":  conv(good, exportEntry{Kind: "term", Text: "w", Quote: "q", Status: "active"}, exportEntry{Kind: "tension", Text: "t", Quote: "q", Status: "active", A: 1, B: 2}),
	}
	for name, f := range bad {
		b, _ := json.Marshal(f)
		if code, _ := a.do(t, "POST", "/api/profile/import", string(b)); code != 400 {
			t.Errorf("%s: %d", name, code)
		}
	}
	if list, _ := a.st.Conversations(bg); len(list) != 0 {
		t.Fatal("a refused import left a conversation behind")
	}
	// a file whose memory would overflow what is already there is refused too, and nothing of it is kept
	var many []exportEntry
	for i := 0; i < store.MaxLedgerSize; i++ {
		many = append(many, exportEntry{Kind: "claim", Text: "c" + strings.Repeat("a", i%9) + string(rune('a'+i%26)), Quote: "some quote", Status: "active"})
	}
	b, _ := json.Marshal(conv(many...))
	if code, _ := a.do(t, "POST", "/api/profile/import", string(b)); code != 200 {
		t.Fatalf("a full ledger: %d", code)
	}
	if code, _ := a.do(t, "POST", "/api/profile/import", string(b)); code != 400 {
		t.Fatalf("a second full ledger must not fit: %d", code)
	}
	if list, _ := a.st.Conversations(bg); len(list) != 1 {
		t.Fatalf("the refused import left %d conversations", len(list))
	}
}

func TestTheMemorySettingCanBeReadAndChangedOverHTTP(t *testing.T) {
	a := newApp(t, newScript(t))
	_, r := a.do(t, "GET", "/api/settings/memory", "")
	if r["enabled"] != true {
		t.Fatalf("%v", r)
	}
	if code, r := a.do(t, "PUT", "/api/settings/memory", `{"enabled":false}`); code != 200 || r["enabled"] != false {
		t.Fatalf("%d %v", code, r)
	}
	_, r = a.do(t, "GET", "/api/settings/memory", "")
	if r["enabled"] != false {
		t.Fatalf("%v", r)
	}
	if code, _ := a.do(t, "PUT", "/api/settings/memory", `{}`); code != 400 {
		t.Fatalf("a missing value: %d", code)
	}
	if code, _ := a.do(t, "PUT", "/api/settings/memory", `{"enabled":true}`); code != 200 {
		t.Fatal(code)
	}
}
