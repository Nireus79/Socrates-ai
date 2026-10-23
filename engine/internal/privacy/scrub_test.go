package privacy

import (
	"bytes"
	"strings"
	"testing"
)

func TestScrubberMasksRegisteredContentOnly(t *testing.T) {
	var buf bytes.Buffer
	s := NewScrubber(&buf)
	s.off = false
	s.Register("courage is knowing what to fear", "Glaucon", "ab", "  ")
	s.Write([]byte(`[turn] claim saved: "courage is knowing what to fear" for Glaucon (ab) ok` + "\n"))
	got := buf.String()
	if strings.Contains(got, "Glaucon") || strings.Contains(got, "courage") {
		t.Fatalf("registered content is masked: %q", got)
	}
	if !strings.Contains(got, "[turn] claim saved") || !strings.Contains(got, "(ab)") {
		t.Fatalf("the rest of the line is kept, and a too-short string is not registered: %q", got)
	}
	s.Forget()
	buf.Reset()
	s.Write([]byte("Glaucon again\n"))
	if !strings.Contains(buf.String(), "Glaucon") {
		t.Fatalf("after Forget nothing is masked")
	}
}

func TestScrubberCanBeSwitchedOffForDebugging(t *testing.T) {
	var buf bytes.Buffer
	s := NewScrubber(&buf)
	s.off = true
	s.Register("private claim")
	s.Write([]byte("the private claim\n"))
	if !strings.Contains(buf.String(), "private claim") {
		t.Fatal("SOCRATES_LOG_CONTENT=1 shows the content")
	}
}

func TestLongerPhrasesAreMaskedBeforeTheWordsInsideThem(t *testing.T) {
	var buf bytes.Buffer
	s := NewScrubber(&buf)
	s.off = false
	s.Register("Phaedo", "tell Phaedo the truth")
	s.Write([]byte("claim: tell Phaedo the truth\n"))
	if got := buf.String(); strings.Count(got, "[…]") != 1 {
		t.Fatalf("the phrase is masked as one piece: %q", got)
	}
}
