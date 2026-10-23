//go:build linux

package secrets

import "syscall"

const prSetDumpable = 4

// HardenProcess makes the process non-dumpable: other programs of the same user can no longer read its memory through
// ptrace or /proc/<pid>/mem, and no core file is written. It raises the bar for malware; it does not stop root.
func HardenProcess() error {
	_, _, errno := syscall.RawSyscall(syscall.SYS_PRCTL, prSetDumpable, 0, 0)
	if errno != 0 {
		return errno
	}
	return nil
}
