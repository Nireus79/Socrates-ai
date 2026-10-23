package api

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestExportHoldsTheConversationsAndNeverTheKeyThenImportAddsThemBack(t *testing.T) {
	a := newApp(t, newScript(t, okJudge, "What do you mean by it?", okGate, "On courage", okLedger))
	a.setUp(t)
	_, conv := a.do(t, "POST", "/api/conversations", `{"philosopherId":"socrates"}`)
	id := conv["id"].(string)
	a.do(t, "POST", "/api/conversations/"+id+"/messages", `{"text":"What is courage?"}`)
	a.svc.Wait()

	code, ex := a.do(t, "GET", "/api/profile/export", "")
	raw := mustJSON(ex)
	if code != 200 || ex["format"] != "socrates-export-1" || !strings.Contains(raw, "What is courage?") || !strings.Contains(raw, "On courage") {
		t.Fatalf("%d %s", code, raw)
	}
	if strings.Contains(raw, claudeKey) || strings.Contains(raw, "apiKey") || strings.Contains(raw, "age.confirmed") {
		t.Fatal("the export holds settings or the key")
	}

	code, res := a.do(t, "POST", "/api/profile/import", raw)
	if code != 200 || res["conversations"] != float64(1) || res["messages"] != float64(2) {
		t.Fatalf("%d %v", code, res)
	}
	_, list := a.do(t, "GET", "/api/conversations", "")
	cs := list["conversations"].([]any)
	if len(cs) != 2 {
		t.Fatalf("the import must add, not replace: %v", list)
	}
	if cs[0].(map[string]any)["id"] == cs[1].(map[string]any)["id"] {
		t.Fatal("the imported conversation reused an id")
	}
}

func TestAnImportWithAnythingWrongIsRefusedAsAWhole(t *testing.T) {
	a := newApp(t, newScript(t))
	good := exportConversation{PhilosopherID: "socrates", Messages: []exportMessage{{Role: "user", Text: "hello"}}}
	for name, f := range map[string]exportFile{
		"wrong format":        {Format: "other", Conversations: []exportConversation{good}},
		"unknown philosopher": {Format: exportFormat, Conversations: []exportConversation{good, {PhilosopherID: "nobody"}}},
		"unknown role":        {Format: exportFormat, Conversations: []exportConversation{good, {PhilosopherID: "plato", Messages: []exportMessage{{Role: "system", Text: "x"}}}}},
		"unknown marker":      {Format: exportFormat, Conversations: []exportConversation{good, {PhilosopherID: "plato", Messages: []exportMessage{{Role: "marker", Text: "made_up"}}}}},
		"empty message":       {Format: exportFormat, Conversations: []exportConversation{good, {PhilosopherID: "plato", Messages: []exportMessage{{Role: "user"}}}}},
	} {
		b, _ := json.Marshal(f)
		if code, _ := a.do(t, "POST", "/api/profile/import", string(b)); code != 400 {
			t.Errorf("%s: %d", name, code)
		}
	}
	if code, _ := a.do(t, "POST", "/api/profile/import", "not json"); code != 400 {
		t.Errorf("not json: %d", code)
	}
	_, list := a.do(t, "GET", "/api/conversations", "")
	if len(list["conversations"].([]any)) != 0 {
		t.Fatal("a refused import left something behind")
	}
}

func TestDeletingTheProfileLeavesNothingAndNeedsTheWord(t *testing.T) {
	a := newApp(t, newScript(t))
	a.setUp(t)
	a.do(t, "POST", "/api/conversations", `{"philosopherId":"socrates"}`)
	if code, _ := a.do(t, "DELETE", "/api/profile", `{}`); code != 400 {
		t.Fatalf("without the word: %d", code)
	}
	if _, l := a.do(t, "GET", "/api/conversations", ""); len(l["conversations"].([]any)) != 1 {
		t.Fatal("a refused delete removed something")
	}
	if code, _ := a.do(t, "DELETE", "/api/profile", `{"confirm":"delete"}`); code != 204 {
		t.Fatalf("%d", code)
	}
	if _, l := a.do(t, "GET", "/api/conversations", ""); len(l["conversations"].([]any)) != 0 {
		t.Fatal("conversations remain")
	}
	_, p := a.do(t, "GET", "/api/profile", "")
	if p["ageConfirmed"] != false || p["hasProvider"] != false {
		t.Fatalf("settings remain: %v", p)
	}
}
