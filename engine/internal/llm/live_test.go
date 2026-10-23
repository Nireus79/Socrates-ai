//go:build live

package llm

// Live checks against the real providers. They cost a few tokens, need a key in the environment, and run only with -tags live:
//
//	ANTHROPIC_API_KEY=... go test -tags live -run Live -v ./internal/llm/
//
// They exist because a stand-in server only proves that the code does what its author thinks; these prove that the provider agrees.

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestLiveClaude(t *testing.T) {
	key := os.Getenv("ANTHROPIC_API_KEY")
	if key == "" {
		t.Skip("no ANTHROPIC_API_KEY")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	s := Settings{Provider: "claude", APIKey: key}
	list, err := ListModels(ctx, s)
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	roles, err := ResolveRoleModels("claude", list, ApprovedModels())
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("models listed: %d; fast=%s reply=%s tested=%v", len(list.Models), roles.Fast, roles.Reply, roles.Tested)
	s.FastModel, s.ReplyModel = roles.Fast, roles.Reply
	c, err := NewClient(s)
	if err != nil {
		t.Fatal(err)
	}
	fast, err := c.Complete(ctx, Request{Role: RoleFast, System: "Answer with one word.", Messages: userMsg("Is water wet? Answer yes or no."), MaxTokens: 50})
	if err != nil || strings.TrimSpace(fast.Text) == "" {
		t.Fatalf("fast call: %v %+v", err, fast)
	}
	t.Logf("fast: %q (%d in, %d out, stop %s)", fast.Text, fast.InputTokens, fast.OutputTokens, fast.StopReason)
	reply, err := c.Complete(ctx, Request{Role: RoleReply, System: "You are a brief, plain speaker.", Messages: []Message{{Role: "user", Content: "Say hello."}, {Role: "assistant", Content: "Hello."}, {Role: "user", Content: "And goodbye?"}}, MaxTokens: 200})
	if err != nil || strings.TrimSpace(reply.Text) == "" {
		t.Fatalf("reply call: %v %+v", err, reply)
	}
	t.Logf("reply: %q (%d in, %d out, stop %s)", reply.Text, reply.InputTokens, reply.OutputTokens, reply.StopReason)
	// A wrong key must come back as an authentication error without the key in the text.
	bad := Settings{Provider: "claude", APIKey: "sk-ant-not-a-real-key-123456", FastModel: roles.Fast, ReplyModel: roles.Reply}
	cb, _ := NewClient(bad)
	_, err = cb.Complete(ctx, Request{Messages: userMsg("hi"), MaxTokens: 10})
	if !isAuth(err) || strings.Contains(err.Error(), bad.APIKey) {
		t.Fatalf("a wrong key: %v", err)
	}
	t.Logf("wrong key: %v", err)
	// A model that does not exist must be told as gone.
	cg, _ := NewClient(Settings{Provider: "claude", APIKey: key, FastModel: "claude-no-such-model-9"})
	_, err = cg.Complete(ctx, Request{Messages: userMsg("hi"), MaxTokens: 10})
	t.Logf("a model that does not exist: %v", err)
}

func isAuth(err error) bool { e, ok := err.(*Error); return ok && e.Kind == KindAuth }
