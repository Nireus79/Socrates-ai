//go:build !windows

package main

import "syscall"

// A process-wide umask of 077: whatever file the engine or its database driver creates is private to the person, whatever mode was asked for.
func init() { syscall.Umask(0o077) }
