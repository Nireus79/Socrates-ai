// Package privacy keeps what the user said out of the logs.
//
// The engine may log the course of a turn, and a log line could quote the user's message, a claim saved in the ledger or a
// philosopher's reply. The first rule is that the code does not log content at all; this package is the second line of defence: the
// output of the standard logger goes through a Scrubber, whatever the engine learns about the user's content is registered, and every
// registered string is replaced by a mask in every line written afterwards. SOCRATES_LOG_CONTENT=1 turns the masking off for debugging
// (the engine ignores it in production).
package privacy

import (
	"io"
	"os"
	"sort"
	"strings"
	"sync"
)

const (
	mask       = "[…]"
	minLen     = 3    // shorter strings would garble the log
	maxEntries = 5000 // the oldest are dropped first
)

// Scrubber is an io.Writer that masks registered strings.
type Scrubber struct {
	mu       sync.RWMutex
	out      io.Writer
	entries  []string
	set      map[string]struct{}
	replacer *strings.Replacer
	off      bool
}

// NewScrubber wraps out. Masking is off when SOCRATES_LOG_CONTENT=1.
func NewScrubber(out io.Writer) *Scrubber {
	return &Scrubber{out: out, set: map[string]struct{}{}, off: os.Getenv("SOCRATES_LOG_CONTENT") == "1"}
}

// Default is the scrubber of the standard logger; Register and Forget work on it.
var Default = NewScrubber(os.Stderr)

// Mask turns the masking on, whatever SOCRATES_LOG_CONTENT said when the scrubber was made.
func (s *Scrubber) Mask() {
	s.mu.Lock()
	s.off = false
	s.mu.Unlock()
}

// Register adds strings to mask in the log from now on. Empty and very short strings are ignored.
func (s *Scrubber) Register(values ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := false
	for _, v := range values {
		v = strings.TrimSpace(v)
		if len([]rune(v)) < minLen {
			continue
		}
		if _, ok := s.set[v]; ok {
			continue
		}
		s.set[v] = struct{}{}
		s.entries = append(s.entries, v)
		changed = true
		if len(s.entries) > maxEntries {
			delete(s.set, s.entries[0])
			s.entries = s.entries[1:]
		}
	}
	if changed {
		s.rebuild()
	}
}

// Forget drops everything registered (used when a profile is deleted, so the process holds nothing of the user).
func (s *Scrubber) Forget() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries, s.set, s.replacer = nil, map[string]struct{}{}, nil
}

// rebuild makes the replacer; longer strings first, so a phrase is masked as a whole before the words inside it.
func (s *Scrubber) rebuild() {
	list := append([]string(nil), s.entries...)
	sort.Slice(list, func(i, j int) bool { return len(list[i]) > len(list[j]) })
	pairs := make([]string, 0, len(list)*2)
	for _, e := range list {
		pairs = append(pairs, e, mask)
	}
	s.replacer = strings.NewReplacer(pairs...)
}

// Write masks the registered strings and writes the line.
func (s *Scrubber) Write(p []byte) (int, error) {
	s.mu.RLock()
	r, off := s.replacer, s.off
	s.mu.RUnlock()
	if off || r == nil {
		return s.out.Write(p)
	}
	if _, err := s.out.Write([]byte(r.Replace(string(p)))); err != nil {
		return 0, err
	}
	return len(p), nil
}

// Register adds strings to the default scrubber.
func Register(values ...string) { Default.Register(values...) }

// Forget clears the default scrubber.
func Forget() { Default.Forget() }
