// Package cards is the data of the card at the foot of every screen: a labelled, static message from Socrates or from a sponsor, which the
// person can hide. The messages can be updated without a new release by a data file in the project's repository (site/cards.json). It is
// data only, never code: fetched at most once a day with a plain request that carries no cookies, no referrer and nothing about the person,
// checked field by field (whatever does not pass is ignored), and shown by the screens as text with https links only. When nothing valid is
// available the screens show the messages built into them. The same rules as the sponsor card of Moly.
package cards

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"
	"unicode/utf8"
)

// DefaultURL is where the messages are published: a file of this repository, so that changing them is a commit and needs no store.
const DefaultURL = "https://raw.githubusercontent.com/Nireus79/Socrates-ai/master/site/cards.json"

const (
	maxBody    = 64 << 10
	maxEntries = 10
	maxLinks   = 4
	maxAge     = 24 * time.Hour
	retryAfter = time.Hour // after a failed fetch, not sooner
)

// Link is one button of a card.
type Link struct {
	Text    string `json:"text"`
	URL     string `json:"url"`
	Primary bool   `json:"primary,omitempty"`
}

// Entry is one message. Kind is "socrates" (the project's own) or "sponsor"; every card is labelled "Sponsored" on screen, the project's own too.
type Entry struct {
	ID     string   `json:"id"`
	Kind   string   `json:"kind"`
	Title  string   `json:"title"`
	Text   string   `json:"text"`
	Marks  []string `json:"marks,omitempty"` // "hermes": the logo of Hermes Software at the top of the card
	Links  []Link   `json:"links"`
	Active bool     `json:"active"`
}

func fits(s string, max int) bool {
	return s != "" && utf8.RuneCountInString(s) <= max && utf8.ValidString(s)
}

func safeURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && u.Port() == "" && utf8.RuneCountInString(raw) <= 300
}

// Parse reads a feed file ({"version": 1, "entries": [...]}) and returns the entries that pass; ok is false when the file is not a feed.
func Parse(b []byte) (out []Entry, ok bool) {
	var f struct {
		Version int `json:"version"`
		Entries []struct {
			ID, Kind, Title, Text string
			Links                 []Link
			Marks                 []string
			Active                *bool
		} `json:"entries"`
	}
	if len(b) > maxBody || json.Unmarshal(b, &f) != nil || f.Version != 1 || len(f.Entries) > maxEntries {
		return nil, false
	}
	out = []Entry{}
	for _, e := range f.Entries {
		if !fits(e.ID, 64) || !fits(e.Title, 80) || !fits(e.Text, 280) || (e.Kind != "socrates" && e.Kind != "sponsor") || e.Active == nil {
			continue
		}
		var links []Link
		for _, l := range e.Links {
			if len(links) < maxLinks && fits(l.Text, 30) && safeURL(l.URL) {
				links = append(links, l)
			}
		}
		var marks []string
		for _, m := range e.Marks {
			if m == "hermes" && len(marks) == 0 {
				marks = append(marks, m)
			}
		}
		out = append(out, Entry{ID: e.ID, Kind: e.Kind, Title: e.Title, Text: e.Text, Marks: marks, Links: links, Active: *e.Active})
	}
	return out, true
}

// Feed fetches and keeps the messages.
type Feed struct {
	url    string
	client *http.Client
	now    func() time.Time

	mu      sync.Mutex
	entries []Entry // the last valid answer, nil if there was none
	at      time.Time
	tried   time.Time
}

// NewFeed makes a feed for an https address. The client has no cookie jar, follows no redirect and gives up after five seconds.
func NewFeed(rawURL string) *Feed {
	return NewFeedWith(rawURL, &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }})
}

// NewFeedWith is NewFeed with the caller's client (for tests).
func NewFeedWith(rawURL string, c *http.Client) *Feed {
	return &Feed{url: rawURL, client: c, now: time.Now}
}

// Entries are the messages from the file, fetched if the last good answer is older than a day; nil when there is none (the screens then show
// theirs). A failed fetch is not repeated within the hour and keeps the last good answer.
func (f *Feed) Entries(ctx context.Context) []Entry {
	if f == nil {
		return nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	now := f.now()
	if f.entries != nil && now.Sub(f.at) < maxAge {
		return f.entries
	}
	if !f.tried.IsZero() && now.Sub(f.tried) < retryAfter {
		return f.entries
	}
	f.tried = now
	if e, ok := f.fetch(ctx); ok {
		f.entries, f.at = e, now
	}
	return f.entries
}

func (f *Feed) fetch(ctx context.Context) ([]Entry, bool) {
	if u, err := url.Parse(f.url); err != nil || u.Scheme != "https" {
		return nil, false
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.url, nil)
	if err != nil {
		return nil, false
	}
	req.Header.Set("User-Agent", "Socrates")
	req.Header.Set("Cache-Control", "no-cache")
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, false
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil || len(b) > maxBody {
		return nil, false
	}
	return Parse(b)
}
