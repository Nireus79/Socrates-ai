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

func TestReportingAReplyTakesItOutEverywhereAndGivesItsWordsBackOnce(t *testing.T) {
	a := newApp(t, newScript(t, okJudge, "An offensive reply.", okGate, "On courage", okLedger))
	a.setUp(t)
	_, conv := a.do(t, "POST", "/api/conversations", `{"philosopherId":"socrates"}`)
	id := conv["id"].(string)
	_, res := a.do(t, "POST", "/api/conversations/"+id+"/messages", `{"text":"What is courage?"}`)
	a.svc.Wait()
	added := res["added"].([]any)
	userMsg, reply := added[0].(map[string]any), added[1].(map[string]any)
	path := "/api/conversations/" + id + "/messages/" + reply["id"].(string) + "/report"
	if code, _ := a.do(t, "POST", path, `{"category":"nonsense"}`); code != 400 {
		t.Fatalf("an unknown reason: %d", code)
	}
	if code, _ := a.do(t, "POST", "/api/conversations/"+id+"/messages/"+userMsg["id"].(string)+"/report", `{"category":"other"}`); code != 400 {
		t.Fatalf("a person's own message cannot be reported: %d", code)
	}
	if code, _ := a.do(t, "POST", "/api/conversations/"+id+"/messages/none/report", `{"category":"other"}`); code != 404 {
		t.Fatalf("%d", code)
	}
	code, r := a.do(t, "POST", path, `{"category":"harmful"}`)
	if code != 200 || r["reply"] != "An offensive reply." || r["category"] != "harmful" || !strings.Contains(r["marker"].(string), "You reported this reply") {
		t.Fatalf("%d %v", code, r)
	}
	_, h := a.do(t, "GET", "/api/conversations/"+id, "")
	raw := mustJSON(h)
	msgs := h["messages"].([]any)
	last := msgs[len(msgs)-1].(map[string]any)
	if strings.Contains(raw, "offensive reply") || last["role"] != "marker" || last["kind"] != "reported" || !strings.Contains(last["text"].(string), "taken out of the conversation") {
		t.Fatalf("the reply is still in the conversation: %s", raw)
	}
	if code, _ := a.do(t, "POST", path, `{"category":"harmful"}`); code != 400 {
		t.Fatalf("a reply cannot be reported twice: %d", code)
	}
	_, ex := a.do(t, "GET", "/api/profile/export", "")
	if strings.Contains(mustJSON(ex), "offensive reply") {
		t.Fatal("the export holds the reported words")
	}
	b := newApp(t, newScript(t))
	b.setUp(t)
	if code, res := b.do(t, "POST", "/api/profile/import", mustJSON(ex)); code != 200 {
		t.Fatalf("a conversation with a reported reply must import: %d %v", code, res)
	}
}

func TestWhatTheShellReportsBoughtUnlocksThePhilosopherAndAPassEndsAndBadInputIsRefused(t *testing.T) {
	a := newAppEnforcing(t, newScript(t), true)
	a.setUp(t)
	if code, _ := a.do(t, "POST", "/api/conversations", `{"philosopherId":"plato"}`); code != 403 {
		t.Fatalf("%d", code)
	}
	for name, body := range map[string]string{
		"unknown source":     `{"source":"somewhere","products":["supporter"]}`,
		"unknown product":    `{"source":"play","products":["philosopher.zeno"]}`,
		"a pass as standing": `{"source":"play","products":["pass.plato"]}`,
		"too many":           `{"source":"play","products":[` + strings.Repeat(`"supporter",`, 60) + `"supporter"]}`,
	} {
		if code, _ := a.do(t, "PUT", "/api/entitlements/grants", body); code != 400 {
			t.Errorf("%s: %d", name, code)
		}
	}
	if code, _ := a.do(t, "PUT", "/api/entitlements/grants", `{"source":"play","products":["philosopher.plato","supporter"]}`); code != 204 {
		t.Fatalf("%d", code)
	}
	_, r := a.do(t, "GET", "/api/philosophers", "")
	if find(t, r, "plato")["unlocked"] != true || find(t, r, "plato")["earned"] != false || find(t, r, "aristotle")["unlocked"] != false {
		t.Fatalf("%v", r)
	}
	if code, _ := a.do(t, "POST", "/api/conversations", `{"philosopherId":"plato"}`); code != 201 {
		t.Fatalf("%d", code)
	}
	_, g := a.do(t, "GET", "/api/entitlements", "")
	if l := g["products"].([]any); len(l) != 2 || l[0] != "philosopher.plato" {
		t.Fatalf("%v", g)
	}
	for name, body := range map[string]string{
		"not a pass":     `{"source":"ads","product":"philosopher.plato","hours":24}`,
		"too long":       `{"source":"ads","product":"pass.plato","hours":73}`,
		"no time":        `{"source":"ads","product":"pass.plato","hours":0}`,
		"unknown source": `{"source":"x","product":"pass.plato","hours":1}`,
		"unknown person": `{"source":"ads","product":"pass.nobody","hours":1}`,
	} {
		if code, _ := a.do(t, "POST", "/api/entitlements/pass", body); code != 400 {
			t.Errorf("%s: %d", name, code)
		}
	}
	code, p := a.do(t, "POST", "/api/entitlements/pass", `{"source":"ads","product":"pass.aristotle","hours":24}`)
	if code != 200 || p["until"] == nil {
		t.Fatalf("%d %v", code, p)
	}
	if code, _ := a.do(t, "POST", "/api/conversations", `{"philosopherId":"aristotle"}`); code != 201 {
		t.Fatalf("a pass should open him: %d", code)
	}
	// a refund: the store's next list no longer has Plato
	a.do(t, "PUT", "/api/entitlements/grants", `{"source":"play","products":["supporter"]}`)
	if code, _ := a.do(t, "POST", "/api/conversations", `{"philosopherId":"plato"}`); code != 403 {
		t.Fatalf("a refunded product still unlocks: %d", code)
	}
}
