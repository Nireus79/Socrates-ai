package cards

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image/png"
	"strings"
	"time"
)

// The card of the current format. A card describes itself in the data file, as data and never as code: a layout from a short fixed list, a
// credit line, an optional message, a logo (the built-in Hermes mark or a small PNG embedded in the file itself, so that nothing is ever
// fetched from a sponsor) and up to four link buttons. What the file can never change, because the screens draw it themselves: the label
// "Sponsored", the Hide button, how links open and that only https links are kept. A card that fails any check is dropped on its own.
const (
	LayoutCreditRow  = "credit-row"  // the credit line with the logo on the left, the buttons on the right (below on a narrow screen)
	LayoutLogoCenter = "logo-center" // one big logo in the middle, the buttons under it
	LayoutLogoText   = "logo-text"   // the logo beside a message (above it on a narrow screen), the buttons last

	maxImageBytes  = 24 << 10 // the decoded PNG
	maxImageWidth  = 1200
	maxImageHeight = 400
	pngPrefix      = "data:image/png;base64,"
	dateLayout     = "2006-01-02"
)

// Logo is the picture of a card: the built-in mark of Hermes Software, or a PNG embedded in the file (a data address).
type Logo struct {
	Mark  string `json:"mark,omitempty"`  // "hermes"
	Image string `json:"image,omitempty"` // "data:image/png;base64,..."
	Alt   string `json:"alt,omitempty"`   // required with an image
	Tint  bool   `json:"tint,omitempty"`  // black line art that turns light on the dark theme
}

// Card is one card of the current format.
type Card struct {
	ID      string `json:"id"`
	Layout  string `json:"layout"`
	Credit  string `json:"credit,omitempty"`
	Text    string `json:"text,omitempty"`
	Logo    *Logo  `json:"logo,omitempty"`
	Buttons []Link `json:"buttons"`
	From    string `json:"from,omitempty"`  // first day shown (YYYY-MM-DD), optional
	Until   string `json:"until,omitempty"` // last day shown, optional
	Active  bool   `json:"active"`
}

func validImage(s string) bool {
	if !strings.HasPrefix(s, pngPrefix) || len(s) > maxImageBytes*4/3+len(pngPrefix)+8 {
		return false
	}
	b, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(s, pngPrefix))
	if err != nil || len(b) == 0 || len(b) > maxImageBytes {
		return false
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(b)) // the real header, not the name
	return err == nil && cfg.Width > 0 && cfg.Height > 0 && cfg.Width <= maxImageWidth && cfg.Height <= maxImageHeight
}

func parseDay(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, true
	}
	d, err := time.Parse(dateLayout, s)
	return d, err == nil
}

func parseCard(raw json.RawMessage) (Card, bool) {
	var in struct {
		ID, Layout, Credit, Text, From, Until string
		Logo                                  *Logo
		Buttons                               []Link
		Active                                *bool
	}
	if json.Unmarshal(raw, &in) != nil || in.Active == nil || !fits(in.ID, 64) {
		return Card{}, false
	}
	c := Card{ID: in.ID, Layout: in.Layout, Active: *in.Active, Buttons: []Link{}}
	if in.Credit != "" && !fits(in.Credit, 40) || in.Text != "" && !fits(in.Text, 280) {
		return Card{}, false
	}
	c.Credit, c.Text = in.Credit, in.Text
	if in.Logo != nil {
		l := *in.Logo
		switch {
		case l.Mark == "hermes" && l.Image == "":
			c.Logo = &Logo{Mark: "hermes", Tint: false}
		case l.Mark == "" && l.Image != "" && validImage(l.Image) && fits(l.Alt, 100):
			c.Logo = &Logo{Image: l.Image, Alt: l.Alt, Tint: l.Tint}
		default:
			return Card{}, false
		}
	}
	from, ok1 := parseDay(in.From)
	until, ok2 := parseDay(in.Until)
	if !ok1 || !ok2 || (in.From != "" && in.Until != "" && until.Before(from)) {
		return Card{}, false
	}
	c.From, c.Until = in.From, in.Until
	switch c.Layout {
	case LayoutCreditRow:
		if c.Credit == "" {
			return Card{}, false
		}
	case LayoutLogoCenter:
		if c.Logo == nil {
			return Card{}, false
		}
		c.Credit, c.Text = "", "" // nothing but the logo and the buttons
	case LayoutLogoText:
		if c.Logo == nil || c.Text == "" {
			return Card{}, false
		}
	default:
		return Card{}, false
	}
	for _, l := range in.Buttons {
		if len(c.Buttons) < maxLinks && fits(l.Text, 30) && safeURL(l.URL) {
			c.Buttons = append(c.Buttons, l)
		}
	}
	return c, true
}

// Current are the cards that are active and whose days include now (UTC); the order of the file is kept.
func Current(cards []Card, now time.Time) []Card {
	today := now.UTC().Truncate(24 * time.Hour)
	out := []Card{}
	for _, c := range cards {
		if !c.Active {
			continue
		}
		if from, _ := parseDay(c.From); c.From != "" && today.Before(from) {
			continue
		}
		if until, _ := parseDay(c.Until); c.Until != "" && today.After(until) {
			continue
		}
		out = append(out, c)
	}
	return out
}
