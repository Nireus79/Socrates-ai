//go:build !linux

package secrets

// HardenProcess is only implemented on Linux for now (macOS would use PT_DENY_ATTACH, Windows a restricted process
// token); elsewhere it does nothing.
func HardenProcess() error { return nil }
