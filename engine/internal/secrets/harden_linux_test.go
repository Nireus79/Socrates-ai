//go:build linux

package secrets

import (
	"syscall"
	"testing"
)

const prGetDumpable = 3

func TestHardenProcessMakesTheProcessNonDumpable(t *testing.T) {
	if err := HardenProcess(); err != nil {
		t.Fatal(err)
	}
	r, _, errno := syscall.RawSyscall(syscall.SYS_PRCTL, prGetDumpable, 0, 0)
	if errno != 0 {
		t.Fatal(errno)
	}
	if r != 0 {
		t.Fatalf("PR_GET_DUMPABLE = %d after HardenProcess, want 0", r)
	}
}
