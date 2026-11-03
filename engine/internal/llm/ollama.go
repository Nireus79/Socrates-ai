package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
)

const ollamaDefault = "http://127.0.0.1:11434"

// Ollama is a local model server. The endpoint is the user's choice (usually the machine itself), so it is checked: http or https, a host,
// no credentials, no query.
type Ollama struct {
	d       *doer
	baseURL string
}

// NormalizeOllamaEndpoint validates an endpoint and returns it without a trailing slash; "" gives the default.
func NormalizeOllamaEndpoint(endpoint string) (string, error) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return ollamaDefault, nil
	}
	u, err := url.Parse(endpoint)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("the Ollama address must look like http://127.0.0.1:11434")
	}
	return strings.TrimRight(u.String(), "/"), nil
}

func NewOllama(endpoint string, opts ...Option) (*Ollama, error) {
	e, err := NormalizeOllamaEndpoint(endpoint)
	if err != nil {
		return nil, err
	}
	o := applyOptions(opts)
	return &Ollama{d: o.doer(""), baseURL: e}, nil
}

func (o *Ollama) Complete(ctx context.Context, model string, req Request) (*Response, error) {
	if err := req.validate(); err != nil {
		return nil, err
	}
	msgs := []map[string]string{}
	if req.System != "" {
		msgs = append(msgs, map[string]string{"role": "system", "content": req.System})
	}
	for _, m := range req.Messages {
		msgs = append(msgs, map[string]string{"role": m.Role, "content": m.Content})
	}
	body := map[string]any{"model": model, "messages": msgs, "stream": false, "options": map[string]any{"num_predict": ceiling(req.MaxTokens, 2048)}}
	if req.JSON {
		body["format"] = "json"
	}
	raw, _ := json.Marshal(body)
	res, err := o.d.post(ctx, o.baseURL+"/api/chat", nil, raw, nil)
	if err != nil {
		return nil, err
	}
	var out struct {
		Model   string `json:"model"`
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		DoneReason string `json:"done_reason"`
		In         int    `json:"prompt_eval_count"`
		Out        int    `json:"eval_count"`
	}
	if err := json.Unmarshal(res.body, &out); err != nil {
		return nil, &Error{Kind: KindServer, Status: res.status, Reason: "the answer could not be read"}
	}
	if strings.TrimSpace(out.Message.Content) == "" {
		return nil, &Error{Kind: KindEmpty, Status: res.status, Reason: "the model returned no text"}
	}
	if out.Model == "" {
		out.Model = model
	}
	return &Response{Text: out.Message.Content, Model: out.Model, StopReason: out.DoneReason, InputTokens: out.In, OutputTokens: out.Out}, nil
}
