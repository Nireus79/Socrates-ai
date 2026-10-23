//go:build dev

package main

// enableContentLogging: a developer build honours SOCRATES_LOG_CONTENT=1 (the scrubber read it when it was made).
func enableContentLogging() {}
