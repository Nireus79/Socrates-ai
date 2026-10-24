package api

import (
	"strings"
	"testing"
)

func TestTheLanguageIsChosenOnceAndAnythingElseIsRefused(t *testing.T) {
	a := newApp(t, newScript(t))
	a.setUp(t)
	if _, r := a.do(t, "GET", "/api/settings/language", ""); r["language"] != "" {
		t.Fatalf("a language before any choice: %v", r)
	}
	if code, _ := a.do(t, "PUT", "/api/settings/language", `{"language":"klingon"}`); code != 400 {
		t.Fatalf("%d", code)
	}
	if code, _ := a.do(t, "PUT", "/api/settings/language", `{"language":"el"}`); code != 200 {
		t.Fatalf("%d", code)
	}
	if _, r := a.do(t, "GET", "/api/settings/language", ""); r["language"] != "el" {
		t.Fatalf("%v", r)
	}
}

func TestInGreekTheRosterTheFixedTextsAndTheReportSpeakGreekButTheStoredWordsDoNot(t *testing.T) {
	a := newApp(t, newScript(t, `{"label":"self_harm_clear","confidence":0.97,"evidence":"τελειώσω τη ζωή μου","reason":"x"}`))
	a.setUp(t)
	a.do(t, "PUT", "/api/settings/language", `{"language":"el"}`)
	_, r := a.do(t, "GET", "/api/philosophers", "")
	first := r["philosophers"].([]any)[0].(map[string]any)
	if first["name"] != "Σωκράτης" || !strings.Contains(first["opening"].(string), "τεχνητός Σωκράτης") || !strings.Contains(first["era"].(string), "π.Χ.") {
		t.Fatalf("%v", first)
	}
	_, conv := a.do(t, "POST", "/api/conversations", `{"philosopherId":"socrates"}`)
	id := conv["id"].(string)
	code, res := a.do(t, "POST", "/api/conversations/"+id+"/messages", `{"text":"Θέλω να τελειώσω τη ζωή μου"}`)
	if code != 200 || res["outcome"] != "fixed" || !strings.Contains(res["text"].(string), "έκτακτης ανάγκης") {
		t.Fatalf("%d %v", code, res)
	}
	_, h := a.do(t, "GET", "/api/conversations/"+id, "")
	m := h["messages"].([]any)[0].(map[string]any)
	if m["kind"] != "crisis" || !strings.Contains(m["text"].(string), "έκτακτης ανάγκης") {
		t.Fatalf("%v", m)
	}
	if strings.Contains(mustJSON(h), "τελειώσω") {
		t.Fatal("the user's words are in the history")
	}
	// back in English the same stored marker speaks English
	a.do(t, "PUT", "/api/settings/language", `{"language":"en"}`)
	_, h = a.do(t, "GET", "/api/conversations/"+id, "")
	if m := h["messages"].([]any)[0].(map[string]any); !strings.Contains(m["text"].(string), "emergency number") {
		t.Fatalf("%v", m)
	}
}
