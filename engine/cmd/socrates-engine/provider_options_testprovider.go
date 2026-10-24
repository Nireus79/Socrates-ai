//go:build testprovider

package main

import (
	"os"

	"socrates/internal/llm"
)

// Only the test build of the engine (go build -tags testprovider, used by the whole-journey test) can be pointed at a stand-in
// provider. The shipped engine has no such door, so a key can never be sent to another address by an environment variable.
func llmOptions() []llm.Option {
	if u := os.Getenv("SOCRATES_TEST_PROVIDER_URL"); u != "" {
		return []llm.Option{llm.WithBaseURL(u)}
	}
	return nil
}
