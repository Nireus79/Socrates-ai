package llm

import (
	"context"
	"errors"
	"strings"
	"testing"
)

const openAIKey = "sk-proj-SECRET-OPENAI-654321"
const openAIText = `{"model":"gpt-5","choices":[{"message":{"content":"Greetings."},"finish_reason":"stop"}],"usage":{"prompt_tokens":12,"completion_tokens":4}}`

func callOpenAI(t *testing.T, f *fakeAPI, model string, req Request) (*Response, error) {
	t.Helper()
	return NewOpenAI(openAIKey, f.opts()...).Complete(context.Background(), model, req)
}

func TestOpenAICallHasTheRightShape(t *testing.T) {
	f := newFake(t, ok(openAIText))
	resp, err := callOpenAI(t, f, "gpt-4o", Request{Role: RoleReply, System: "persona", Messages: []Message{{Role: "user", Content: "hi"}, {Role: "assistant", Content: "hello"}, {Role: "user", Content: "again"}}})
	if err != nil || resp.Text != "Greetings." || resp.InputTokens != 12 || resp.OutputTokens != 4 {
		t.Fatalf("%v %+v", err, resp)
	}
	got := f.last()
	if got.Path != "/v1/chat/completions" || got.Headers.Get("Authorization") != "Bearer "+openAIKey {
		t.Fatalf("%s %v", got.Path, got.Headers)
	}
	msgs, _ := got.Body["messages"].([]any)
	if len(msgs) != 4 || msgs[0].(map[string]any)["role"] != "system" || msgs[0].(map[string]any)["content"] != "persona" || msgs[3].(map[string]any)["content"] != "again" {
		t.Fatalf("messages: %v", msgs)
	}
	for _, k := range []string{"temperature", "reasoning_effort", "max_tokens"} {
		if _, has := got.Body[k]; has {
			t.Fatalf("%s was sent to a model that does not need it", k)
		}
	}
	if mt, _ := got.Body["max_completion_tokens"].(float64); mt < 8192 {
		t.Fatalf("max_completion_tokens %v", mt)
	}
}

func TestAReasoningModelGetsAnEffortAndRoomToThink(t *testing.T) {
	f := newFake(t, ok(openAIText), ok(openAIText), ok(openAIText))
	callOpenAI(t, f, "gpt-5", Request{Role: RoleFast, Messages: userMsg("x")})
	if f.last().Body["reasoning_effort"] != "low" {
		t.Fatalf("fast: %v", f.last().Body["reasoning_effort"])
	}
	lowMax := f.last().Body["max_completion_tokens"].(float64)
	callOpenAI(t, f, "o3", Request{Role: RoleReply, Messages: userMsg("x")})
	if f.last().Body["reasoning_effort"] != "medium" {
		t.Fatalf("reply: %v", f.last().Body["reasoning_effort"])
	}
	if f.last().Body["max_completion_tokens"].(float64) <= lowMax {
		t.Fatal("a higher effort needs more room")
	}
	callOpenAI(t, f, "gpt-5-mini", Request{Role: RoleReply, Effort: EffortHigh, JSON: true, Messages: userMsg("x")})
	if f.last().Body["reasoning_effort"] != "high" {
		t.Fatalf("explicit: %v", f.last().Body["reasoning_effort"])
	}
	if rf, _ := f.last().Body["response_format"].(map[string]any); rf["type"] != "json_object" {
		t.Fatalf("json mode: %v", f.last().Body["response_format"])
	}
}

func TestAModelThatRefusesTheEffortOrTheLimitIsAskedAgainWithoutIt(t *testing.T) {
	f := newFake(t, status(400, `{"error":{"message":"Unsupported parameter: 'reasoning_effort' is not supported with this model."}}`), ok(openAIText))
	if _, err := callOpenAI(t, f, "gpt-5", Request{Role: RoleReply, Messages: userMsg("x")}); err != nil || len(f.Seen) != 2 {
		t.Fatalf("%v %d", err, len(f.Seen))
	}
	if _, has := f.last().Body["reasoning_effort"]; has {
		t.Fatal("the effort was sent again")
	}
	f = newFake(t, status(400, `{"error":{"message":"max_completion_tokens is too large: 20000. This model supports at most 4096."}}`), ok(openAIText))
	if _, err := callOpenAI(t, f, "gpt-4o", Request{Messages: userMsg("x")}); err != nil || len(f.Seen) != 2 {
		t.Fatalf("%v %d", err, len(f.Seen))
	}
	if f.last().Body["max_completion_tokens"].(float64) != 4096 {
		t.Fatalf("%v", f.last().Body["max_completion_tokens"])
	}
}

func TestOpenAIErrorsAreClassifiedAndNeverShowTheKey(t *testing.T) {
	f := newFake(t, status(401, `{"error":{"message":"Incorrect API key provided: `+openAIKey+`","code":"invalid_api_key"}}`))
	_, err := callOpenAI(t, f, "gpt-5", Request{Messages: userMsg("x")})
	if !errors.Is(err, ErrAuth) || strings.Contains(err.Error(), openAIKey) {
		t.Fatalf("%v", err)
	}
	f = newFake(t, status(404, `{"error":{"message":"The model `+"`gpt-old`"+` does not exist","code":"model_not_found"}}`))
	if _, err := callOpenAI(t, f, "gpt-old", Request{Messages: userMsg("x")}); !errors.Is(err, ErrModelGone) {
		t.Fatalf("%v", err)
	}
	f = newFake(t, status(429, `{"error":{"message":"You exceeded your current quota"}}`), status(429, `{"error":{"message":"quota"}}`), status(429, `{"error":{"message":"quota"}}`))
	if _, err := callOpenAI(t, f, "gpt-5", Request{Messages: userMsg("x")}); !errors.Is(err, ErrRateLimit) || len(f.Seen) != 3 {
		t.Fatalf("%v %d", err, len(f.Seen))
	}
}

func TestAReasoningModelThatSpentEverythingOnThinkingIsAnEmptyAnswer(t *testing.T) {
	f := newFake(t, ok(`{"choices":[{"message":{"content":""},"finish_reason":"length"}],"usage":{}}`))
	if _, err := callOpenAI(t, f, "gpt-5", Request{Messages: userMsg("x")}); !errors.Is(err, ErrEmpty) {
		t.Fatalf("%v", err)
	}
	f = newFake(t, ok(`{"choices":[]}`))
	if _, err := callOpenAI(t, f, "gpt-5", Request{Messages: userMsg("x")}); !errors.Is(err, ErrServer) {
		t.Fatalf("%v", err)
	}
}

func TestOpenAINeedsAKeyAndAValidRequest(t *testing.T) {
	f := newFake(t, ok(openAIText))
	if _, err := NewOpenAI("", f.opts()...).Complete(context.Background(), "gpt-5", Request{Messages: userMsg("x")}); !errors.Is(err, ErrAuth) {
		t.Fatalf("%v", err)
	}
	if _, err := callOpenAI(t, f, "gpt-5", Request{}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("%v", err)
	}
	if len(f.Seen) != 0 {
		t.Fatal("something was sent")
	}
}
