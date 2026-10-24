package llm

import (
	"context"
	"errors"
	"testing"
	"time"
)

type deadlineSpy struct{ deadline time.Duration }

func (d *deadlineSpy) Complete(ctx context.Context, model string, req Request) (*Response, error) {
	if dl, ok := ctx.Deadline(); ok {
		d.deadline = time.Until(dl)
	} else {
		d.deadline = -1
	}
	return &Response{Text: "ok", Model: model}, nil
}

func TestEachRoleGoesToItsOwnModel(t *testing.T) {
	m := NewMock(Say("a"), Say("b"), Say("c"))
	c := NewClientWith(m, Settings{FastModel: "small", ReplyModel: "big"})
	c.Complete(context.Background(), Request{Role: RoleFast, Messages: userMsg("x")})
	c.Complete(context.Background(), Request{Role: RoleReply, Messages: userMsg("x")})
	c.Complete(context.Background(), Request{Messages: userMsg("x")}) // no role: a small check
	if m.Models[0] != "small" || m.Models[1] != "big" || m.Models[2] != "small" {
		t.Fatalf("%v", m.Models)
	}
}

func TestAClientWithOneModelUsesItForBothRolesAndWithNoneRefuses(t *testing.T) {
	m := NewMock(Say("a"), Say("b"))
	c := NewClientWith(m, Settings{ReplyModel: "only"})
	c.Complete(context.Background(), Request{Role: RoleFast, Messages: userMsg("x")})
	c.Complete(context.Background(), Request{Role: RoleReply, Messages: userMsg("x")})
	if m.Models[0] != "only" || m.Models[1] != "only" {
		t.Fatalf("%v", m.Models)
	}
	if _, err := NewClientWith(NewMock(), Settings{}).Complete(context.Background(), Request{Messages: userMsg("x")}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("no model: %v", err)
	}
}

func TestACallWithoutADeadlineGetsOneThatFitsItsRole(t *testing.T) {
	spy := &deadlineSpy{}
	c := NewClientWith(spy, Settings{Provider: "claude", FastModel: "a", ReplyModel: "b"})
	c.Complete(context.Background(), Request{Role: RoleFast, Messages: userMsg("x")})
	if spy.deadline < 80*time.Second || spy.deadline > 91*time.Second {
		t.Fatalf("fast: %v", spy.deadline)
	}
	c.Complete(context.Background(), Request{Role: RoleReply, Messages: userMsg("x")})
	if spy.deadline < 9*time.Minute || spy.deadline > 10*time.Minute+time.Second {
		t.Fatalf("reply: %v", spy.deadline)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c.Complete(ctx, Request{Role: RoleReply, Messages: userMsg("x")})
	if spy.deadline > 6*time.Second {
		t.Fatalf("the caller's own deadline was replaced: %v", spy.deadline)
	}
}

func TestNewClientBuildsTheNamedProviderAndRefusesAnUnknownOne(t *testing.T) {
	for _, p := range []string{"claude", "Claude", " openai "} {
		if _, err := NewClient(Settings{Provider: p, APIKey: "k"}); err != nil {
			t.Errorf("%q: %v", p, err)
		}
	}
	for _, p := range []string{"", "gemini", "claude-cli", "ollama"} {
		if _, err := NewClient(Settings{Provider: p}); !errors.Is(err, ErrInvalid) {
			t.Errorf("%q: %v", p, err)
		}
	}
}

func TestTheMockFailsLoudlyWhenTheScriptRunsOutAndRecordsEverything(t *testing.T) {
	m := NewMock(Say("one"), Fail(&Error{Kind: KindOverloaded, Reason: "busy"}))
	c := NewClientWith(m, Settings{FastModel: "x"})
	if r, err := c.Complete(context.Background(), Request{Messages: userMsg("q1")}); err != nil || r.Text != "one" {
		t.Fatal(err)
	}
	if _, err := c.Complete(context.Background(), Request{Messages: userMsg("q2")}); !errors.Is(err, ErrOverloaded) {
		t.Fatalf("%v", err)
	}
	if _, err := c.Complete(context.Background(), Request{Messages: userMsg("q3")}); err == nil {
		t.Fatal("a call beyond the script succeeded")
	}
	if len(m.Requests) != 3 || m.Requests[1].Messages[0].Content != "q2" || m.Remaining() != 0 {
		t.Fatalf("%d requests, %d remaining", len(m.Requests), m.Remaining())
	}
}
