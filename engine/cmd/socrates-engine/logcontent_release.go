//go:build !dev

package main

import "socrates/internal/privacy"

// enableContentLogging: in a release build the masking is always on, whatever the environment says.
func enableContentLogging() { privacy.Default.Mask() }
