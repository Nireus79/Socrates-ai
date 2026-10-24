//go:build live

package safety

// The labelled battery for the judge and the gate (BUILD_PLAN M4). The cases are local and git-ignored (battery/cases.json); this runner
// is not. It needs the founder's key and is run only in the final stage, when he says so:
//
//	BATTERY_REPLAYS=3 ANTHROPIC_API_KEY=... go test -tags live -run LiveBattery -v ./internal/safety/
//
// A case fails on the decision (after the thresholds of Decide), as in TestLiveJudge; "diff" means the label differs from the accepted
// ones without crossing the line that matters. Cases whose name starts with "held out:" occur in no prompt.

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"testing"
	"time"

	"socrates/internal/llm"
)

type batteryJudgeCase struct {
	Name    string        `json:"name"`
	Text    string        `json:"text"`
	Recent  []llm.Message `json:"recent"`
	Accept  []Label       `json:"accept"`
	MustNot []Label       `json:"mustNot"`
}

type batteryGateCase struct {
	Name   string        `json:"name"`
	Recent []llm.Message `json:"recent"`
	Draft  string        `json:"draft"`
	Flag   bool          `json:"flag"`
}

func TestLiveBattery(t *testing.T) {
	raw, err := os.ReadFile("battery/cases.json")
	if err != nil {
		t.Skip("no local battery")
	}
	var cases struct {
		Judge []batteryJudgeCase `json:"judge"`
		Gate  []batteryGateCase  `json:"gate"`
	}
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	replays := 1
	if n, err := strconv.Atoi(os.Getenv("BATTERY_REPLAYS")); err == nil && n > 0 {
		replays = n
	}
	c := liveClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	in := func(l Label, set []Label) bool {
		for _, x := range set {
			if x == l {
				return true
			}
		}
		return false
	}
	var bad, diff, errs, total int
	for _, k := range cases.Judge {
		for r := 0; r < replays; r++ {
			total++
			v, err := Judge(ctx, c, k.Recent, k.Text)
			if err != nil {
				errs++
				t.Logf("ERR  %-34s %v", k.Name, err)
				continue
			}
			d := Decide(v, State{})
			switch {
			case in(d.Label, k.MustNot):
				bad++
				t.Errorf("BAD  %-34s -> %s (%.2f) action %s | %q", k.Name, v.Label, v.Confidence, d.Action, v.Evidence)
			case !in(v.Label, k.Accept):
				diff++
				t.Logf("diff %-34s -> %s (%.2f) action %s", k.Name, v.Label, v.Confidence, d.Action)
			}
		}
	}
	var gbad, gerr, gtotal int
	for _, k := range cases.Gate {
		for r := 0; r < replays; r++ {
			gtotal++
			g, err := CheckReply(ctx, c, k.Recent, k.Draft)
			if err != nil {
				gerr++
				t.Logf("ERR  gate %-30s %v", k.Name, err)
				continue
			}
			if g.Flagged != k.Flag {
				gbad++
				t.Errorf("BAD  gate %-30s flagged=%v want %v (%s: %s)", k.Name, g.Flagged, k.Flag, g.Category, g.Reason)
			}
		}
	}
	t.Logf("judge: %d runs, %d bad, %d differing, %d errors | gate: %d runs, %d bad, %d errors", total, bad, diff, errs, gtotal, gbad, gerr)
}
