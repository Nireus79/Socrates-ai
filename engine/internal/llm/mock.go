package llm

import (
	"context"
	"fmt"
	"sync"
)

// Mock is a scripted provider for tests: it answers from a queue, in order, and records every request. When the script runs out it fails
// loudly, so a test never passes by accident because a call was made that nobody expected.
type Mock struct {
	mu       sync.Mutex
	script   []MockStep
	Requests []Request
	Models   []string
}

// MockStep is one scripted answer: Text, or Err.
type MockStep struct {
	Text string
	Err  error
}

func NewMock(steps ...MockStep) *Mock { return &Mock{script: steps} }

// Say is a step that answers with text.
func Say(text string) MockStep { return MockStep{Text: text} }

// Fail is a step that fails.
func Fail(err error) MockStep { return MockStep{Err: err} }

func (m *Mock) Complete(_ context.Context, model string, req Request) (*Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Requests = append(m.Requests, req)
	m.Models = append(m.Models, model)
	if len(m.script) == 0 {
		return nil, fmt.Errorf("the mock provider was called %d times but the script has fewer steps", len(m.Requests))
	}
	step := m.script[0]
	m.script = m.script[1:]
	if step.Err != nil {
		return nil, step.Err
	}
	return &Response{Text: step.Text, Model: model, StopReason: "end_turn"}, nil
}

// Remaining is how many scripted steps were not used.
func (m *Mock) Remaining() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.script)
}
