//go:build testprovider

package main

import (
	"os"
	"path/filepath"
	"strings"

	"socrates/internal/llm"
)

// Only the test build of the engine (go build -tags testprovider: the whole-journey test and the emulator runs) can be pointed at a
// stand-in provider, by SOCRATES_TEST_PROVIDER_URL or, where a phone's service cannot set one, by a file test-provider-url in the data
// folder. The shipped engine has no such door, so a key can never be sent to another address by an environment variable or a file.
func llmOptions(dataDir string) []llm.Option {
	u := os.Getenv("SOCRATES_TEST_PROVIDER_URL")
	if u == "" {
		if b, err := os.ReadFile(filepath.Join(dataDir, "test-provider-url")); err == nil {
			u = strings.TrimSpace(string(b))
		}
	}
	if u != "" {
		return []llm.Option{llm.WithBaseURL(u)}
	}
	return nil
}
