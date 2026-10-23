package llm

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

const secretKey = "sk-ant-SECRET-KEY-123456"

func call(t *testing.T, f *fakeAPI, model string, req Request) (*Response, error) {
	t.Helper()
	return NewAnthropic(secretKey, f.opts()...).Complete(context.Background(), model, req)
}

func TestClaudeFastCallHasTheRightShapeAndNoTemperature(t *testing.T) {
	f := newFake(t, ok(claudeText))
	resp, err := call(t, f, "claude-haiku-5-5", Request{Role: RoleFast, System: "judge", Messages: userMsg("is this a crisis?"), MaxTokens: 300})
	if err != nil || resp.Text != "Hello." {
		t.Fatalf("%v %+v", err, resp)
	}
	got := f.last()
	if got.Path != "/v1/messages" || got.Headers.Get("x-api-key") != secretKey || got.Headers.Get("anthropic-version") != "2023-06-01" {
		t.Fatalf("request line or headers: %s %v", got.Path, got.Headers)
	}
	b := got.Body
	if b["model"] != "claude-haiku-5-5" || b["system"] != "judge" {
		t.Fatalf("body: %v", b)
	}
	if _, has := b["temperature"]; has {
		t.Fatal("a temperature was sent (Claude 5 refuses it)")
	}
	if th, _ := b["thinking"].(map[string]any); th["type"] != "disabled" {
		t.Fatalf("a fast call must not think: %v", b["thinking"])
	}
	if _, has := b["output_config"]; has {
		t.Fatal("a fast call asked for an effort")
	}
	if mt, _ := b["max_tokens"].(float64); mt < 16000 {
		t.Fatalf("the output ceiling must not cut an answer off: %v", b["max_tokens"])
	}
}

func TestClaudeReplyCallThinksAtMediumEffortAndAnEffortOverrides(t *testing.T) {
	f := newFake(t, ok(claudeText), ok(claudeText))
	call(t, f, "claude-sonnet-5-5", Request{Role: RoleReply, Messages: userMsg("What is courage?")})
	b := f.last().Body
	if oc, _ := b["output_config"].(map[string]any); oc["effort"] != "medium" {
		t.Fatalf("reply effort: %v", b["output_config"])
	}
	if _, has := b["thinking"]; has {
		t.Fatalf("a reply must not switch thinking off: %v", b["thinking"])
	}
	if mt, _ := b["max_tokens"].(float64); mt < 18000 {
		t.Fatalf("thinking needs room above the ceiling: %v", b["max_tokens"])
	}
	call(t, f, "claude-sonnet-5-5", Request{Role: RoleReply, Effort: EffortHigh, Messages: userMsg("x")})
	if oc, _ := f.last().Body["output_config"].(map[string]any); oc["effort"] != "high" {
		t.Fatalf("explicit effort: %v", f.last().Body["output_config"])
	}
}

func TestAnOlderClaudeModelGetsNeitherThinkingNorEffort(t *testing.T) {
	f := newFake(t, ok(claudeText))
	call(t, f, "claude-3-5-sonnet-20241022", Request{Role: RoleReply, Messages: userMsg("x")})
	b := f.last().Body
	for _, k := range []string{"thinking", "output_config", "temperature"} {
		if _, has := b[k]; has {
			t.Fatalf("%s was sent to a model that does not think", k)
		}
	}
}

func TestTheAnswerIsTheTextBlocksAndTheUsageIsRead(t *testing.T) {
	f := newFake(t, ok(`{"content":[{"type":"thinking","thinking":"private reasoning"},{"type":"text","text":"One. "},{"type":"text","text":"Two."}],"stop_reason":"end_turn","usage":{"input_tokens":5,"output_tokens":9}}`))
	resp, err := call(t, f, "claude-sonnet-5-5", Request{Role: RoleReply, Messages: userMsg("x")})
	if err != nil || resp.Text != "One. Two." || strings.Contains(resp.Text, "private reasoning") {
		t.Fatalf("%v %+v", err, resp)
	}
	if resp.InputTokens != 5 || resp.OutputTokens != 9 || resp.StopReason != "end_turn" || resp.Model != "claude-sonnet-5-5" {
		t.Fatalf("%+v", resp)
	}
}

func TestAnAnswerWithoutTextIsAnError(t *testing.T) {
	f := newFake(t, ok(`{"content":[{"type":"thinking","thinking":"..."}],"stop_reason":"max_tokens","usage":{}}`))
	if _, err := call(t, f, "claude-sonnet-5-5", Request{Role: RoleReply, Messages: userMsg("x")}); !errors.Is(err, ErrEmpty) {
		t.Fatalf("%v", err)
	}
}

func TestTheKeyNeverAppearsInAnErrorEvenWhenTheProviderEchoesIt(t *testing.T) {
	f := newFake(t, status(401, `{"error":{"type":"authentication_error","message":"invalid x-api-key `+secretKey+`"}}`))
	_, err := call(t, f, "claude-sonnet-5-5", Request{Role: RoleReply, Messages: userMsg("x")})
	if !errors.Is(err, ErrAuth) {
		t.Fatalf("%v", err)
	}
	if strings.Contains(err.Error(), secretKey) || !strings.Contains(err.Error(), "[key]") {
		t.Fatalf("the key reached the error text: %v", err)
	}
	if len(f.Seen) != 1 {
		t.Fatalf("an authentication failure was retried (%d requests)", len(f.Seen))
	}
}

func TestAModelThatIsGoneIsTold(t *testing.T) {
	f := newFake(t, status(404, `{"error":{"type":"not_found_error","message":"model: claude-old-1"}}`))
	if _, err := call(t, f, "claude-old-1", Request{Messages: userMsg("x")}); !errors.Is(err, ErrModelGone) {
		t.Fatalf("%v", err)
	}
}

func TestARateLimitIsRetriedAfterTheTimeTheProviderAsked(t *testing.T) {
	f := newFake(t, fakeStep{status: 429, body: `{"error":{"message":"slow down"}}`, headers: map[string]string{"Retry-After": "2"}}, ok(claudeText))
	resp, err := call(t, f, "claude-sonnet-5-5", Request{Role: RoleReply, Messages: userMsg("x")})
	if err != nil || resp.Text != "Hello." || len(f.Seen) != 2 {
		t.Fatalf("%v %d", err, len(f.Seen))
	}
	if len(f.Sleeps) != 1 || f.Sleeps[0] < 2*time.Second || f.Sleeps[0] > 3*time.Second {
		t.Fatalf("waits: %v", f.Sleeps)
	}
}

func TestAnOverloadedProviderIsTriedThreeTimesThenGivenUp(t *testing.T) {
	f := newFake(t, status(529, `{"error":{"message":"overloaded"}}`), status(529, `{"error":{"message":"overloaded"}}`), status(529, `{"error":{"message":"overloaded"}}`), ok(claudeText))
	_, err := call(t, f, "claude-sonnet-5-5", Request{Role: RoleReply, Messages: userMsg("x")})
	if !errors.Is(err, ErrOverloaded) || len(f.Seen) != 3 || len(f.Sleeps) != 2 {
		t.Fatalf("%v, %d requests, %v", err, len(f.Seen), f.Sleeps)
	}
}

func TestAServerFailureIsRetriedAndAClientErrorIsNot(t *testing.T) {
	f := newFake(t, status(500, "<html>bad gateway</html>"), ok(claudeText))
	if _, err := call(t, f, "claude-sonnet-5-5", Request{Messages: userMsg("x")}); err != nil || len(f.Seen) != 2 {
		t.Fatalf("a 500: %v %d", err, len(f.Seen))
	}
	f = newFake(t, status(400, `{"error":{"message":"messages: text content blocks must be non-empty"}}`), ok(claudeText))
	_, err := call(t, f, "claude-sonnet-5-5", Request{Messages: userMsg("x")})
	if !errors.Is(err, ErrBadRequest) || len(f.Seen) != 1 {
		t.Fatalf("a 400: %v %d", err, len(f.Seen))
	}
}

func TestARawErrorBodyIsNeverPassedOn(t *testing.T) {
	f := newFake(t, status(400, `<html>the user wrote: PRIVATE-WORDS</html>`))
	_, err := call(t, f, "claude-sonnet-5-5", Request{Messages: userMsg("x")})
	if err == nil || strings.Contains(err.Error(), "PRIVATE-WORDS") || strings.Contains(err.Error(), "<html>") {
		t.Fatalf("%v", err)
	}
}

func TestAModelThatRefusesThinkingOffIsAskedForLowEffortInstead(t *testing.T) {
	f := newFake(t, status(400, `{"error":{"message":"thinking: disabled is not supported for this model"}}`), ok(claudeText))
	if _, err := call(t, f, "claude-haiku-5-5", Request{Role: RoleFast, Messages: userMsg("x")}); err != nil {
		t.Fatal(err)
	}
	if len(f.Seen) != 2 {
		t.Fatalf("%d requests", len(f.Seen))
	}
	b := f.last().Body
	if _, has := b["thinking"]; has {
		t.Fatal("thinking was sent again")
	}
	if oc, _ := b["output_config"].(map[string]any); oc["effort"] != "low" {
		t.Fatalf("%v", b["output_config"])
	}
}

func TestAModelThatRefusesAnEffortIsAskedWithoutIt(t *testing.T) {
	f := newFake(t, status(400, `{"error":{"message":"output_config.effort is not supported"}}`), ok(claudeText))
	if _, err := call(t, f, "claude-sonnet-5-5", Request{Role: RoleReply, Messages: userMsg("x")}); err != nil || len(f.Seen) != 2 {
		t.Fatalf("%v %d", err, len(f.Seen))
	}
	if _, has := f.last().Body["output_config"]; has {
		t.Fatal("the effort was sent again")
	}
}

func TestAnAnswerLargerThanAnyRealAnswerIsRefused(t *testing.T) {
	f := newFake(t, ok(`{"content":[{"type":"text","text":"`+strings.Repeat("a", 9<<20)+`"}]}`))
	if _, err := call(t, f, "claude-sonnet-5-5", Request{Messages: userMsg("x")}); !errors.Is(err, ErrServer) {
		t.Fatalf("%v", err)
	}
}

func TestACancelledCallStopsWaitingBetweenRetries(t *testing.T) {
	f := newFake(t, status(529, `{"error":{"message":"overloaded"}}`), ok(claudeText))
	f.sleepErr = context.Canceled
	_, err := call(t, f, "claude-sonnet-5-5", Request{Messages: userMsg("x")})
	if !errors.Is(err, ErrNetwork) || len(f.Seen) != 1 {
		t.Fatalf("%v %d", err, len(f.Seen))
	}
}

func TestABadRequestOrAMissingKeyIsRefusedBeforeAnythingIsSent(t *testing.T) {
	f := newFake(t, ok(claudeText))
	bad := []Request{
		{Messages: nil},
		{Messages: []Message{{Role: "assistant", Content: "first"}}},
		{Messages: []Message{{Role: "user", Content: "  "}}},
		{Messages: []Message{{Role: "system", Content: "x"}}},
	}
	for i, r := range bad {
		if _, err := call(t, f, "claude-sonnet-5-5", r); !errors.Is(err, ErrInvalid) {
			t.Fatalf("request %d: %v", i, err)
		}
	}
	if _, err := NewAnthropic("", f.opts()...).Complete(context.Background(), "claude-sonnet-5-5", Request{Messages: userMsg("x")}); !errors.Is(err, ErrAuth) {
		t.Fatalf("no key: %v", err)
	}
	if len(f.Seen) != 0 {
		t.Fatalf("%d requests were sent", len(f.Seen))
	}
}
