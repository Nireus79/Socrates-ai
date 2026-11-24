//go:build !linux && !windows

package shell

// macOS has Socrates.app: drag it to Applications. Nothing to make here.

func ShortcutNeeded() bool { return false }

func InstallShortcut(icon []byte, onDesktop bool) error { return ErrNoShortcut }
