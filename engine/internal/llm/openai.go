package llm

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
)

const (
	openAIURL         = "https://api.openai.com"
	openAIOutputFloor = 8192
)

// openAIReasoning matches the models that think before they answer (o1, o3, o4-mini, gpt-5 and later): they fix the temperature and count
// the thinking inside the output limit. No temperature is ever sent.
var openAIReasoning = regexp.MustCompile(`^(o[0-9]|gpt-([5-9]|[1-9][0-9]))(-|\.|$)`)

func reasoningRoom(e Effort) int {
	switch e {
	case EffortHigh:
		return 24000
	case EffortMedium:
		return 12000
	}
	return 4000
}

// OpenAI is the OpenAI provider (the Chat Completions API).
type OpenAI struct {
	d       *doer
	baseURL string
	key     string
}

func NewOpenAI(apiKey string, opts ...Option) *OpenAI {
	o := applyOptions(opts)
	return &OpenAI{d: o.doer(apiKey), baseURL: o.baseURL(openAIURL), key: apiKey}
}

func (o *OpenAI) Complete(ctx context.Context, model string, req Request) (*Response, error) {
	if err := req.validate(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(o.key) == "" {
		return nil, &Error{Kind: KindAuth, Reason: "there is no API key"}
	}
	msgs := []map[string]string{}
	if req.System != "" {
		msgs = append(msgs, map[string]string{"role": "system", "content": req.System})
	}
	for _, m := range req.Messages {
		msgs = append(msgs, map[string]string{"role": m.Role, "content": m.Content})
	}
	max := ceiling(req.MaxTokens, openAIOutputFloor)
	body := map[string]any{"model": model, "messages": msgs}
	reasoning := openAIReasoning.MatchString(model)
	effort := req.Effort
	if reasoning {
		if effort == "" {
			effort = EffortLow
			if req.Role == RoleReply {
				effort = EffortMedium
			}
		}
		body["reasoning_effort"] = string(effort)
		max += reasoningRoom(effort)
	}
	body["max_completion_tokens"] = max
	if req.JSON {
		body["response_format"] = map[string]string{"type": "json_object"}
	}
	raw, _ := json.Marshal(body)
	handle400 := func(reason string) []byte {
		r := strings.ToLower(reason)
		switch {
		case body["reasoning_effort"] != nil && strings.Contains(r, "reasoning_effort"):
			delete(body, "reasoning_effort")
		case body["max_completion_tokens"].(int) > 4096 && (strings.Contains(r, "max_completion_tokens") || strings.Contains(r, "max_tokens")):
			body["max_completion_tokens"] = 4096 // the most every chat model takes
		default:
			return nil
		}
		out, _ := json.Marshal(body)
		return out
	}
	res, err := o.d.post(ctx, o.baseURL+"/v1/chat/completions", map[string]string{"Authorization": "Bearer " + o.key}, raw, handle400)
	if err != nil {
		return nil, err
	}
	var out struct {
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			In  int `json:"prompt_tokens"`
			Out int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(res.body, &out); err != nil || len(out.Choices) == 0 {
		return nil, &Error{Kind: KindServer, Status: res.status, Reason: "the answer could not be read"}
	}
	c := out.Choices[0]
	if strings.TrimSpace(c.Message.Content) == "" { // a model that thinks first can spend the whole limit on it
		return nil, &Error{Kind: KindEmpty, Status: res.status, Reason: "the model returned no text (finish reason: " + c.FinishReason + ")"}
	}
	if out.Model == "" {
		out.Model = model
	}
	return &Response{Text: c.Message.Content, Model: out.Model, StopReason: c.FinishReason, InputTokens: out.Usage.In, OutputTokens: out.Usage.Out}, nil
}
