// Package evaluate measures the plain, countable side of a set of replies from one philosopher: length, questions asked and how often replies
// open with the same words. It judges nothing about meaning (that is for a human or a labelled judge); it exists so that a change to a persona
// or a new model can be compared with the last one by numbers, without a person re-reading every transcript. Replies come from the local
// live runs and never from stored conversations.
package evaluate

import (
	"sort"
	"strings"
	"unicode"
)

// Report is what Measure finds.
type Report struct {
	Replies       int
	MedianWords   int
	LongestWords  int
	OverLimit     int      // replies longer than the limit given to Measure
	MoreThanOneQ  int      // replies with more than one question mark
	NoQuestion    int      // replies with none
	RepeatedStart []string // openings (first three words) that begin more than a quarter of the replies, when there are at least four replies
}

// Measure reads the replies. limit is the number of words above which a reply counts as long.
func Measure(replies []string, limit int) Report {
	r := Report{Replies: len(replies)}
	if len(replies) == 0 {
		return r
	}
	var counts []int
	starts := map[string]int{}
	for _, text := range replies {
		w := words(text)
		counts = append(counts, len(w))
		if len(w) > limit {
			r.OverLimit++
		}
		// a Greek question mark is written ";" (or U+037E), so those count too
		switch q := strings.Count(text, "?") + strings.Count(text, ";") + strings.Count(text, "\u037e"); {
		case q == 0:
			r.NoQuestion++
		case q > 1:
			r.MoreThanOneQ++
		}
		if len(w) >= 3 {
			starts[strings.ToLower(strings.Trim(strings.Join(w[:3], " "), ",.;:!?\"'"))]++
		}
	}
	sort.Ints(counts)
	r.MedianWords, r.LongestWords = counts[len(counts)/2], counts[len(counts)-1]
	if len(replies) >= 4 {
		for s, n := range starts {
			if n*4 > len(replies) {
				r.RepeatedStart = append(r.RepeatedStart, s)
			}
		}
		sort.Strings(r.RepeatedStart)
	}
	return r
}

func words(s string) []string {
	return strings.FieldsFunc(s, func(c rune) bool { return unicode.IsSpace(c) })
}

// EndingQuestions counts the replies whose last sentence is a question (a Greek question mark is ";" or U+037E), and OverWords those longer
// than the given number of words. Both are structural, for comparing runs without reading every transcript.
func EndingQuestions(replies []string) int {
	n := 0
	for _, r := range replies {
		t := strings.TrimRight(strings.TrimSpace(r), " .\"'»)]”")
		if strings.HasSuffix(t, "?") || strings.HasSuffix(t, ";") || strings.HasSuffix(t, "\u037e") {
			n++
		}
	}
	return n
}

func OverWords(replies []string, limit int) int {
	n := 0
	for _, r := range replies {
		if len(words(r)) > limit {
			n++
		}
	}
	return n
}
