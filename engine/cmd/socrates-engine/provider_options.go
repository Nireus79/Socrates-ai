//go:build !testprovider

package main

import "socrates/internal/llm"

// The shipped engine always talks to the providers' own addresses.
func llmOptions(dataDir string) []llm.Option { return nil }
