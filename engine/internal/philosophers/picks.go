package philosophers

import (
	"fmt"
	"strings"
)

// Picks are the objections and tales of a philosopher chosen for one reply, by number from 1 as Catalogue shows them. The choice is made by a
// small model call each turn (conversation/select.go), never by words in a list: the persona grows with every entry, and what is not about the
// present talk only crowds out what is.
type Picks struct {
	Objections []int `json:"objections"`
	Stories    []int `json:"tales"`
}

// Limits on one reply.
const (
	MaxObjections = 2
	MaxStories    = 1
)

// clean keeps numbers in 1..n once each, in order.
func clean(ns []int, n int) []int {
	seen := map[int]bool{}
	var out []int
	for _, v := range ns {
		if v >= 1 && v <= n && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

// Limit trims picks to what one reply may carry and to numbers that exist.
func (p Philosopher) Limit(pk Picks) Picks {
	return Picks{Cut(pk.Objections, len(p.Library), MaxObjections), Cut(pk.Stories, len(p.Stories), MaxStories)}
}

// Cut is clean followed by a maximum.
func Cut(ns []int, n, max int) []int {
	c := clean(ns, n)
	if len(c) > max {
		c = c[:max]
	}
	return c
}

func short(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}

// Catalogue is the numbered list the selector reads: the names of the issues and the first words of each objection and tale. Entries whose key
// is in skip (kind letter and number, "i3", "o2", "t1") are left out, so that nothing is picked twice in a conversation. It is empty when
// nothing is left to pick.
func (p Philosopher) Catalogue(skip map[string]bool) string {
	var b strings.Builder
	add := func(head string, n int, kind string, label func(i int) string) {
		var lines []string
		for i := 0; i < n; i++ {
			if !skip[fmt.Sprintf("%s%d", kind, i+1)] {
				lines = append(lines, fmt.Sprintf("%d. %s", i+1, label(i)))
			}
		}
		if len(lines) > 0 {
			b.WriteString(head + "\n" + strings.Join(lines, "\n") + "\n\n")
		}
	}
	add("OBJECTIONS (doubts raised about his work)", len(p.Library), "o", func(i int) string { return short(p.Library[i].Said, 110) })
	add("TALES (told about him)", len(p.Stories), "t", func(i int) string { return short(p.Stories[i], 100) })
	return strings.TrimSpace(b.String())
}
