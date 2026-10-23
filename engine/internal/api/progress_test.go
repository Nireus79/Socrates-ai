package api

import (
	"strings"
	"testing"
	"time"

	"socrates/internal/store"
)

func (a *app) finishChallenges(t *testing.T, philosopher string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		c, _ := a.st.CreateConversation(bg, philosopher)
		in, err := a.st.CreateInquiry(bg, store.Inquiry{PhilosopherID: philosopher, ConversationID: c.ID, Question: "q", Quote: "quote"})
		if err != nil {
			t.Fatal(err)
		}
		if err := a.st.CompleteInquiry(bg, in.ID); err != nil {
			t.Fatal(err)
		}
	}
}

func find(t *testing.T, r map[string]any, id string) map[string]any {
	t.Helper()
	for _, p := range r["philosophers"].([]any) {
		if m := p.(map[string]any); m["id"] == id {
			return m
		}
	}
	t.Fatalf("no %s", id)
	return nil
}

func TestTheBetaBypassesTheUnlockRulesButTheRosterStillReportsProgress(t *testing.T) {
	a := newApp(t, newScript(t))
	a.setUp(t)
	a.finishChallenges(t, "socrates", 2)
	_, r := a.do(t, "GET", "/api/philosophers", "")
	d := find(t, r, "diogenes")
	if d["unlocked"] != true || d["earned"] != false || find(t, r, "socrates")["challenges"] != float64(2) {
		t.Fatalf("%v", d)
	}
	needs := d["needs"].([]any)
	if len(needs) != 1 || needs[0].(map[string]any)["name"] != "Socrates" || needs[0].(map[string]any)["done"] != float64(2) || needs[0].(map[string]any)["required"] != float64(5) {
		t.Fatalf("%v", needs)
	}
	if code, _ := a.do(t, "POST", "/api/conversations", `{"philosopherId":"aristotle"}`); code != 201 {
		t.Fatalf("the beta should let anyone in: %d", code)
	}
}

func TestWithTheRulesEnforcedAPhilosopherOpensOnlyAfterFiveChallengesWithEachEarlierOne(t *testing.T) {
	a := newAppEnforcing(t, newScript(t), true)
	a.setUp(t)
	_, r := a.do(t, "GET", "/api/philosophers", "")
	if find(t, r, "socrates")["unlocked"] != true || find(t, r, "diogenes")["unlocked"] != false {
		t.Fatalf("%v", r)
	}
	if code, _ := a.do(t, "POST", "/api/conversations", `{"philosopherId":"diogenes"}`); code != 403 {
		t.Fatalf("a locked philosopher was opened: %d", code)
	}
	a.finishChallenges(t, "socrates", 5)
	_, r = a.do(t, "GET", "/api/philosophers", "")
	if d := find(t, r, "diogenes"); d["unlocked"] != true || d["earned"] != true || d["needs"] != nil {
		t.Fatalf("%v", d)
	}
	if find(t, r, "plato")["unlocked"] != false {
		t.Fatal("Plato needs Diogenes too")
	}
	if code, _ := a.do(t, "POST", "/api/conversations", `{"philosopherId":"diogenes"}`); code != 201 {
		t.Fatalf("%d", code)
	}
}

func TestChallengesTravelInAnExportWithoutAnyTextAndComeBackOnImport(t *testing.T) {
	a := newApp(t, newScript(t))
	a.setUp(t)
	a.finishChallenges(t, "socrates", 3)
	a.finishChallenges(t, "plato", 1)
	_, ex := a.do(t, "GET", "/api/profile/export", "")
	raw := mustJSON(ex)
	if !strings.Contains(raw, `"challenges"`) || strings.Count(raw, `"at":`) != 4 {
		t.Fatalf("%s", raw)
	}
	b := newApp(t, newScript(t))
	b.setUp(t)
	if code, res := b.do(t, "POST", "/api/profile/import", raw); code != 200 {
		t.Fatalf("%d %v", code, res)
	}
	counts, _ := b.st.ChallengeCounts(bg)
	if counts["socrates"] != 3 || counts["plato"] != 1 {
		t.Fatalf("%v", counts)
	}
	if cs, _ := b.st.Challenges(bg); cs[0].CreatedAt.After(time.Now().Add(time.Minute)) {
		t.Fatal("a date was invented")
	}
}

func TestAnImportWithAnUnknownPhilosopherOrTooManyChallengesIsRefused(t *testing.T) {
	a := newApp(t, newScript(t))
	a.setUp(t)
	body := func(challenges string) string {
		return `{"format":"socrates-export-1","conversations":[],"challenges":` + challenges + `}`
	}
	if code, _ := a.do(t, "POST", "/api/profile/import", body(`[{"philosopherId":"zeno","at":5}]`)); code != 400 {
		t.Fatalf("%d", code)
	}
	if code, _ := a.do(t, "POST", "/api/profile/import", body(`[{"philosopherId":"plato","at":-1}]`)); code != 400 {
		t.Fatalf("%d", code)
	}
	many := "[" + strings.Repeat(`{"philosopherId":"plato","at":5},`, maxImportChallenges) + `{"philosopherId":"plato","at":5}]`
	if code, _ := a.do(t, "POST", "/api/profile/import", body(many)); code != 400 {
		t.Fatalf("%d", code)
	}
	if n, _ := a.st.ChallengeCounts(bg); len(n) != 0 {
		t.Fatal("a refused file left something behind")
	}
}

func TestAChallengeOverHTTPIsPosedTakenUpOrLeftAndTheHistoryShowsWhatWaits(t *testing.T) {
	a := newApp(t, newScript(t, okJudge, "What do you mean by it?", okGate, "On courage", okLedger,
		`{"topic":"What is it to be brave when afraid?"}`, "Here is a question for you, take it or leave it.", okGate,
		okJudge, "Then what is bravery?", okGate, okLedger))
	a.setUp(t)
	_, conv := a.do(t, "POST", "/api/conversations", `{"philosopherId":"socrates"}`)
	id := conv["id"].(string)
	a.do(t, "POST", "/api/conversations/"+id+"/messages", `{"text":"What is courage?"}`)
	a.svc.Wait()

	code, res := a.do(t, "POST", "/api/conversations/"+id+"/challenge", "")
	if code != 200 || res["offer"].(map[string]any)["state"] != "pending" || len(res["added"].([]any)) != 1 {
		t.Fatalf("%d %v", code, res)
	}
	_, h := a.do(t, "GET", "/api/conversations/"+id, "")
	if h["offer"].(map[string]any)["state"] != "pending" {
		t.Fatalf("the history does not show what waits: %v", h)
	}
	if strings.Contains(mustJSON(h), "brave when afraid") {
		t.Fatal("the topic itself is not shown, only his words")
	}
	code, res = a.do(t, "POST", "/api/conversations/"+id+"/challenge/accept", "")
	a.svc.Wait()
	added, _ := res["added"].([]any)
	if code != 200 || len(added) != 2 || added[0].(map[string]any)["text"] != "I'll take up your question." {
		t.Fatalf("%d %v", code, res)
	}
	if code, r := a.do(t, "POST", "/api/conversations/"+id+"/challenge/accept", ""); code != 409 || r["kind"] != "no_offer" {
		t.Fatalf("%d %v", code, r)
	}
	if code, r := a.do(t, "POST", "/api/conversations/"+id+"/challenge/decline", ""); code != 409 || r["kind"] != "no_offer" {
		t.Fatalf("%d %v", code, r)
	}
	if code, _ := a.do(t, "POST", "/api/conversations/none/challenge", ""); code != 404 {
		t.Fatalf("%d", code)
	}
}

func TestLeavingAChallengeOverHTTP(t *testing.T) {
	a := newApp(t, newScript(t, okJudge, "What do you mean by it?", okGate, "On courage", okLedger,
		`{"topic":"What is it to be brave when afraid?"}`, "Here is a question for you.", okGate))
	a.setUp(t)
	_, conv := a.do(t, "POST", "/api/conversations", `{"philosopherId":"socrates"}`)
	id := conv["id"].(string)
	a.do(t, "POST", "/api/conversations/"+id+"/messages", `{"text":"What is courage?"}`)
	a.svc.Wait()
	a.do(t, "POST", "/api/conversations/"+id+"/challenge", "")
	if code, _ := a.do(t, "POST", "/api/conversations/"+id+"/challenge/decline", ""); code != 204 {
		t.Fatalf("%d", code)
	}
	_, h := a.do(t, "GET", "/api/conversations/"+id, "")
	if h["offer"] != nil {
		t.Fatalf("%v", h["offer"])
	}
}

func TestAChallengeFromALockedPhilosopherIsRefusedWhenTheRulesAreEnforced(t *testing.T) {
	a := newAppEnforcing(t, newScript(t), true)
	a.setUp(t)
	conv, _ := a.st.CreateConversation(bg, "plato") // as if it were begun before the rule applied
	if code, r := a.do(t, "POST", "/api/conversations/"+conv.ID+"/challenge", ""); code != 403 || r["kind"] != "locked" {
		t.Fatalf("%d %v", code, r)
	}
}

func TestFactsTravelInAnExportAndComeBackOnImport(t *testing.T) {
	a := newApp(t, newScript(t))
	a.setUp(t)
	a.seed(t, "socrates", store.Entry{Kind: store.KindFact, Text: "works as a nurse", Quote: "I work as a nurse"})
	_, ex := a.do(t, "GET", "/api/profile/export", "")
	raw := mustJSON(ex)
	if !strings.Contains(raw, `"kind":"fact"`) {
		t.Fatalf("%s", raw)
	}
	b := newApp(t, newScript(t))
	b.setUp(t)
	if code, r := b.do(t, "POST", "/api/profile/import", raw); code != 200 {
		t.Fatalf("%d %v", code, r)
	}
	es, _ := b.st.Entries(bg, "socrates", false)
	if len(es) != 1 || es[0].Kind != store.KindFact || es[0].Text != "works as a nurse" {
		t.Fatalf("%+v", es)
	}
}

func TestDeletingAConversationOverHTTPForgetsWhatWasLearntUnlessToldToKeepItAndTheKeptMemoryTravels(t *testing.T) {
	a := newApp(t, newScript(t))
	a.setUp(t)
	idA, _ := a.seed(t, "socrates", store.Entry{Kind: store.KindFact, Text: "works as a nurse", Quote: "I work as a nurse"})
	idB, _ := a.seed(t, "socrates", store.Entry{Kind: store.KindClaim, Text: "lying is wrong", Quote: "lying is always wrong"})
	_, h := a.do(t, "GET", "/api/conversations/"+idA, "")
	if h["learned"] != float64(1) {
		t.Fatalf("the screens cannot ask what to do without the count: %v", h["learned"])
	}
	if code, _ := a.do(t, "DELETE", "/api/conversations/"+idA+"?keep=memory", ""); code != 204 {
		t.Fatalf("%d", code)
	}
	if code, _ := a.do(t, "DELETE", "/api/conversations/"+idB, ""); code != 204 {
		t.Fatalf("%d", code)
	}
	_, r := a.do(t, "GET", "/api/ledger?philosopherId=socrates", "")
	list, _ := r["entries"].([]any)
	if len(list) != 1 || list[0].(map[string]any)["text"] != "works as a nurse" {
		t.Fatalf("the default must forget, the choice must keep: %v", list)
	}
	_, ex := a.do(t, "GET", "/api/profile/export", "")
	raw := mustJSON(ex)
	if !strings.Contains(raw, `"memory"`) || !strings.Contains(raw, "works as a nurse") {
		t.Fatalf("kept memory is not in the export: %s", raw)
	}
	b := newApp(t, newScript(t))
	b.setUp(t)
	if code, res := b.do(t, "POST", "/api/profile/import", raw); code != 200 {
		t.Fatalf("%d %v", code, res)
	}
	got, _ := b.st.Entries(bg, "socrates", false)
	if len(got) != 1 || got[0].Text != "works as a nurse" || got[0].ConversationID != "" {
		t.Fatalf("%+v", got)
	}
	bad := `{"format":"socrates-export-1","conversations":[],"memory":[{"philosopherId":"zeno","ledger":[]}]}`
	if code, _ := b.do(t, "POST", "/api/profile/import", bad); code != 400 {
		t.Fatalf("%d", code)
	}
	bad = `{"format":"socrates-export-1","conversations":[],"memory":[{"philosopherId":"plato","ledger":[{"kind":"fact","text":"","quote":"x","status":"active"}]}]}`
	if code, _ := b.do(t, "POST", "/api/profile/import", bad); code != 400 {
		t.Fatalf("%d", code)
	}
}
