package shell

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTrimLogKeepsTheTailAtALineStart(t *testing.T) {
	p := filepath.Join(t.TempDir(), "socrates.log")
	line := strings.Repeat("x", 99) + "\n"
	os.WriteFile(p, []byte(strings.Repeat(line, 6000)), 0o600) // 600 KB, over the limit
	trimLog(p)
	b, _ := os.ReadFile(p)
	if len(b) > logKeep || len(b) < logKeep-200 {
		t.Fatalf("kept %d bytes", len(b))
	}
	if !bytes.HasPrefix(b, []byte(strings.Repeat("x", 99)+"\n")) {
		t.Fatal("the kept part must start at a whole line")
	}
}

func TestSmallLogIsLeftAlone(t *testing.T) {
	p := filepath.Join(t.TempDir(), "socrates.log")
	os.WriteFile(p, []byte("a\nb\n"), 0o600)
	trimLog(p)
	if b, _ := os.ReadFile(p); string(b) != "a\nb\n" {
		t.Fatal("a small log must not change")
	}
}
