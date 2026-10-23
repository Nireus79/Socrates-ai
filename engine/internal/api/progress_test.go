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
