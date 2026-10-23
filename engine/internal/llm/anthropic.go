package llm

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
)

const (
	anthropicURL     = "https://api.anthropic.com"
	anthropicVersion = "2023-06-01"
	// The thinking of a Claude 5 model counts toward max_tokens, so the ceiling is generous: it costs nothing unless the model uses it.
	claudeOutputFloor = 16000
	claudeEffortRoom  = 2000
)

// claude5 matches the Claude 5 family (claude-opus-5-5, claude-sonnet-5-5, claude-haiku-5-5, ...): they take no temperature and think
// adaptively. No temperature is ever sent to any Claude model.
var claude5 = regexp.MustCompile(`^claude-[a-z]+-([5-9]|[1-9][0-9])(-|$)`)

// Anthropic is the Claude provider.
type Anthropic struct {
	d       *doer
	baseURL string
	key     string
}

func NewAnthropic(apiKey string, opts ...Option) *Anthropic {
	o := applyOptions(opts)
	return &Anthropic{d: o.doer(apiKey), baseURL: o.baseURL(anthropicURL), key: apiKey}
}

func (a *Anthropic) headers() map[string]string {
	return map[string]string{"x-api-key": a.key, "anthropic-version": anthropicVersion}
}

type claudeBody struct {
	Model        string            `json:"model"`
	MaxTokens    int               `json:"max_tokens"`
	System       string            `json:"system,omitempty"`
	Messages     []map[string]any  `json:"messages"`
	Thinking     map[string]string `json:"thinking,omitempty"`
	OutputConfig map[string]string `json:"output_config,omitempty"`
}

func (a *Anthropic) Complete(ctx context.Context, model string, req Request) (*Response, error) {
	if err := req.validate(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(a.key) == "" {
		return nil, &Error{Kind: KindAuth, Reason: "there is no API key"}
	}
	b := claudeBody{Model: model, System: req.System}
	for _, m := range req.Messages {
		b.Messages = append(b.Messages, map[string]any{"role": m.Role, "content": m.Content})
	}
	think := claude5.MatchString(model)
	effort := string(req.Effort)
	if effort == "" && req.Role == RoleReply {
		effort = string(EffortMedium)
	}
	switch {
	case think && effort != "":
		b.OutputConfig = map[string]string{"effort": effort} // adaptive thinking, steered by effort
	case think && req.Role != RoleReply:
		b.Thinking = map[string]string{"type": "disabled"} // short structured answers need no thinking
	}
	b.MaxTokens = ceiling(req.MaxTokens, claudeOutputFloor)
	if b.OutputConfig != nil {
		b.MaxTokens += claudeEffortRoom
	}
	body, _ := json.Marshal(b)
	// A model may refuse an optional part; the call is sent again without it.
	handle400 := func(reason string) []byte {
		r := strings.ToLower(reason)
		switch {
		case b.Thinking != nil && strings.Contains(r, "thinking"):
			b.Thinking, b.OutputConfig = nil, map[string]string{"effort": string(EffortLow)}
		case b.OutputConfig != nil && (strings.Contains(r, "effort") || strings.Contains(r, "output_config")):
			b.OutputConfig = nil
		default:
			return nil
		}
		out, _ := json.Marshal(b)
		return out
	}
	hdr := a.headers()
	res, err := a.d.post(ctx, a.baseURL+"/v1/messages", hdr, body, handle400)
	if err != nil {
		return nil, err
	}
	var out struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		StopReason string `json:"stop_reason"`
		Model      string `json:"model"`
		Usage      struct {
			In  int `json:"input_tokens"`
			Out int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(res.body, &out); err != nil {
		return nil, &Error{Kind: KindServer, Status: res.status, Reason: "the answer could not be read"}
	}
	var text strings.Builder
	for _, c := range out.Content {
		if c.Type == "text" { // a thinking block may come first and is not part of the answer
			text.WriteString(c.Text)
		}
	}
	if strings.TrimSpace(text.String()) == "" {
		return nil, &Error{Kind: KindEmpty, Status: res.status, Reason: "the model returned no text (stop reason: " + out.StopReason + ")"}
	}
	if out.Model == "" {
		out.Model = model
	}
	return &Response{Text: text.String(), Model: out.Model, StopReason: out.StopReason, InputTokens: out.Usage.In, OutputTokens: out.Usage.Out}, nil
}
