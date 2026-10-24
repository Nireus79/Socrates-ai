// Package llm talks to the AI provider the user chose (Claude, OpenAI, or a local Ollama) with the user's own key.
//
// The package knows the vendors' addresses and quirks and nothing else about Socrates. It never logs what it sends or receives, it never
// puts the key into an error, and it does not echo a provider's raw error body (only a short, cleaned reason).
package llm

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Role says what a call is for. Fast calls are the small checks (the safety judgement, a title, a summary): short answers, no thinking.
// Reply calls produce the text the user reads and may think before they answer.
type Role string

const (
	RoleFast  Role = "fast"
	RoleReply Role = "reply"
)

// Effort asks a model that thinks to think at this level. Empty: the provider's or the role's default.
type Effort string

const (
	EffortLow    Effort = "low"
	EffortMedium Effort = "medium"
	EffortHigh   Effort = "high"
)

// Message is one turn of the conversation. Role is "user" or "assistant".
type Message struct {
	Role    string
	Content string
}

// Request is one call.
type Request struct {
	Role      Role
	System    string
	Messages  []Message
	MaxTokens int    // how long an answer is expected to be; a floor is applied (a limit that cuts an answer off is worse than no limit)
	Effort    Effort // see Effort
	JSON      bool   // ask for valid JSON where the provider can
}

// Response is what came back.
type Response struct {
	Text         string
	Model        string
	StopReason   string
	InputTokens  int
	OutputTokens int
}

// Provider makes a call with a model chosen by the caller.
type Provider interface {
	Complete(ctx context.Context, model string, req Request) (*Response, error)
}

// Kind classifies a failure so that the caller can act on it.
type Kind string

const (
	KindAuth       Kind = "auth"        // the key is missing, wrong or without rights
	KindRateLimit  Kind = "rate_limit"  // too many requests, or the account's quota is used up
	KindOverloaded Kind = "overloaded"  // the provider is busy
	KindServer     Kind = "server"      // the provider failed
	KindModelGone  Kind = "model_gone"  // the model does not exist (retired, or the key has no access)
	KindBadRequest Kind = "bad_request" // the provider refused the request
	KindNetwork    Kind = "network"     // no answer
	KindEmpty      Kind = "empty"       // the answer had no text
	KindInvalid    Kind = "invalid"     // the request was wrong before it was sent
	KindRefused    Kind = "refused"     // the model declined to answer (stop reason "refusal")
)

// Error is a failed call.
type Error struct {
	Kind       Kind
	Status     int           // HTTP status, 0 when there was none
	Reason     string        // short and cleaned; never the key, never the request
	RetryAfter time.Duration // what the provider asked, when it did
}

func (e *Error) Error() string {
	if e.Status != 0 {
		return fmt.Sprintf("the AI provider failed (%s, status %d): %s", e.Kind, e.Status, e.Reason)
	}
	return fmt.Sprintf("the AI provider failed (%s): %s", e.Kind, e.Reason)
}

// Is lets errors.Is(err, llm.ErrAuth) and the like work on any *Error of that kind.
func (e *Error) Is(target error) bool {
	k, ok := target.(kindError)
	return ok && Kind(k) == e.Kind
}

type kindError Kind

func (k kindError) Error() string { return string(k) }

var (
	ErrAuth       error = kindError(KindAuth)
	ErrRateLimit  error = kindError(KindRateLimit)
	ErrOverloaded error = kindError(KindOverloaded)
	ErrServer     error = kindError(KindServer)
	ErrModelGone  error = kindError(KindModelGone)
	ErrBadRequest error = kindError(KindBadRequest)
	ErrNetwork    error = kindError(KindNetwork)
	ErrEmpty      error = kindError(KindEmpty)
	ErrInvalid    error = kindError(KindInvalid)
	ErrRefused    error = kindError(KindRefused)
)

func (r Request) validate() error {
	if len(r.Messages) == 0 {
		return &Error{Kind: KindInvalid, Reason: "no message to answer"}
	}
	if r.Messages[0].Role != "user" {
		return &Error{Kind: KindInvalid, Reason: "the conversation must start with the user"}
	}
	for _, m := range r.Messages {
		if (m.Role != "user" && m.Role != "assistant") || strings.TrimSpace(m.Content) == "" {
			return &Error{Kind: KindInvalid, Reason: "a message has an unknown role or no content"}
		}
	}
	return nil
}

func ceiling(hint, floor int) int {
	if hint < floor {
		return floor
	}
	return hint
}
