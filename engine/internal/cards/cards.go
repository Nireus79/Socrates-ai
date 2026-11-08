// Package cards is the data of the card at the foot of every screen: a static card, always labelled "Sponsored" on screen (decided 2026-11-08: the label is mandatory on every card), whose credit line reads its title ("Created by") followed by the logo, with link buttons and, optionally, a message; the
// person can hide it. The messages can be updated without a new release by a data file in the project's repository (site/cards.json). It is
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
	maxBody    = 128 << 10 // room for a logo or two embedded in the file
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

// Entry is one card. Kind is "socrates" (the project's own) or "sponsor"; the screens label both "Sponsored". Text is optional (the unified card of Socrates and Moly has none).
type Entry struct {
	ID     string   `json:"id"`
	Kind   string   `json:"kind"`
	Title  string   `json:"title"`
	Text   string   `json:"text"`
	Marks  []string `json:"marks,omitempty"` // "hermes": the logo of Hermes Software, drawn right after the title ("Created by")
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

// Content is what a feed file holds: the cards of the current format and, beside them, the entries of the first format, which the apps already
// in people's hands read (they ignore "cards", as these apps ignore nothing they understand). Either list may be empty.
type Content struct {
	Entries []Entry `json:"entries"`
	Cards   []Card  `json:"cards"`
}

// Parse reads the entries of a feed file ({"version": 1, "entries": [...]}) and returns the ones that pass; ok is false when the file is not a feed.
func Parse(b []byte) (out []Entry, ok bool) {
	c, ok := ParseFeed(b)
	if !ok {
		return nil, false
	}
	return c.Entries, true
}

// ParseFeed reads a whole feed file: the entries of the first format and the cards of the current one, each checked on its own.
func ParseFeed(b []byte) (Content, bool) {
	var f struct {
		Version int `json:"version"`
		Entries []struct {
			ID, Kind, Title, Text string
			Links                 []Link
			Marks                 []string
			Active                *bool
		} `json:"entries"`
		Cards []json.RawMessage `json:"cards"`
	}
	if len(b) > maxBody || json.Unmarshal(b, &f) != nil || f.Version != 1 || len(f.Entries) > maxEntries || len(f.Cards) > maxEntries {
		return Content{}, false
	}
	out := Content{Entries: []Entry{}, Cards: []Card{}}
	for _, e := range f.Entries {
		if !fits(e.ID, 64) || !fits(e.Title, 80) || (e.Text != "" && !fits(e.Text, 280)) || (e.Kind != "socrates" && e.Kind != "sponsor") || e.Active == nil {
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
		out.Entries = append(out.Entries, Entry{ID: e.ID, Kind: e.Kind, Title: e.Title, Text: e.Text, Marks: marks, Links: links, Active: *e.Active})
	}
	for _, raw := range f.Cards {
		if c, ok := parseCard(raw); ok {
			out.Cards = append(out.Cards, c)
		}
	}
	return out, true
}

// Feed fetches and keeps the messages.
type Feed struct {
	url    string
	client *http.Client
	now    func() time.Time

	mu      sync.Mutex
	content *Content // the last valid answer, nil if there was none
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

// Content is what the file holds, fetched if the last good answer is older than a day; nil when there is none (the screens then show
// theirs). A failed fetch is not repeated within the hour and keeps the last good answer.
func (f *Feed) Content(ctx context.Context) *Content {
	if f == nil {
		return nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	now := f.now()
	if f.content != nil && now.Sub(f.at) < maxAge {
		return f.content
	}
	if !f.tried.IsZero() && now.Sub(f.tried) < retryAfter {
		return f.content
	}
	f.tried = now
	if c, ok := f.fetch(ctx); ok {
		f.content, f.at = &c, now
	}
	return f.content
}

// Entries are the entries of the first format from the file; nil when there is no valid file.
func (f *Feed) Entries(ctx context.Context) []Entry {
	c := f.Content(ctx)
	if c == nil {
		return nil
	}
	return c.Entries
}

func (f *Feed) fetch(ctx context.Context) (Content, bool) {
	if u, err := url.Parse(f.url); err != nil || u.Scheme != "https" {
		return Content{}, false
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.url, nil)
	if err != nil {
		return Content{}, false
	}
	req.Header.Set("User-Agent", "Socrates")
	req.Header.Set("Cache-Control", "no-cache")
	resp, err := f.client.Do(req)
	if err != nil {
		return Content{}, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Content{}, false
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil || len(b) > maxBody {
		return Content{}, false
	}
	return ParseFeed(b)
}
