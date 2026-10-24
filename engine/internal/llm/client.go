package llm

import (
	"context"
	"strings"
	"time"
)

// Settings are the user's choices: which provider, with which key, and which model for each role (found once with ListModels and
// ResolveRoleModels, then kept).
type Settings struct {
	Provider   string // "claude" or "openai"
	APIKey     string
	FastModel  string
	ReplyModel string
}

// Client sends each call to the model of its role.
type Client struct {
	p        Provider
	settings Settings
}

// NewClient builds the provider the settings name. It makes no network call.
func NewClient(s Settings, opts ...Option) (*Client, error) {
	var p Provider
	switch strings.ToLower(strings.TrimSpace(s.Provider)) {
	case "claude":
		p = NewAnthropic(s.APIKey, opts...)
	case "openai":
		p = NewOpenAI(s.APIKey, opts...)
	default:
		return nil, &Error{Kind: KindInvalid, Reason: "unknown AI provider " + s.Provider}
	}
	return &Client{p: p, settings: s}, nil
}

// NewClientWith wraps any provider (the mock, in tests).
func NewClientWith(p Provider, s Settings) *Client { return &Client{p: p, settings: s} }

// Model is the model a role would use.
func (c *Client) Model(r Role) string {
	m := c.settings.FastModel
	if r == RoleReply {
		m = c.settings.ReplyModel
	}
	if m == "" { // a client with only one model uses it for both
		if r == RoleReply {
			return c.settings.FastModel
		}
		return c.settings.ReplyModel
	}
	return m
}

// Complete makes the call. Without a deadline of its own the call gets one that fits its role (a reply may think for minutes).
func (c *Client) Complete(ctx context.Context, req Request) (*Response, error) {
	if req.Role == "" {
		req.Role = RoleFast
	}
	model := c.Model(req.Role)
	if model == "" {
		return nil, &Error{Kind: KindInvalid, Reason: "no model has been chosen"}
	}
	if _, has := ctx.Deadline(); !has {
		d := 90 * time.Second
		if req.Role == RoleReply {
			d = 10 * time.Minute
		}
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, d)
		defer cancel()
	}
	return c.p.Complete(ctx, model, req)
}
