package cards

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"math/rand"
	"os"
	"strings"
	"testing"
	"time"
)

func pngURI(t *testing.T, w, h int) string {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	return pngPrefix + base64.StdEncoding.EncodeToString(b.Bytes())
}

func feedWith(card string) []byte {
	return []byte(`{"version":1,"entries":[],"cards":[` + card + `]}`)
}

func one(t *testing.T, card string) (Card, bool) {
	t.Helper()
	c, ok := ParseFeed(feedWith(card))
	if !ok || len(c.Cards) == 0 {
		return Card{}, false
	}
	return c.Cards[0], true
}

const btn = `"buttons":[{"text":"Go","url":"https://example.org/x","primary":true}],"active":true`

func TestEachLayoutNeedsWhatItDraws(t *testing.T) {
	img := pngURI(t, 200, 80)
	logo := `"logo":{"image":"` + img + `","alt":"Acme"}`
	good := map[string]string{
		"credit row with the mark": `{"id":"a","layout":"credit-row","credit":"Created by","logo":{"mark":"hermes"},` + btn + `}`,
		"credit row, no logo":      `{"id":"a","layout":"credit-row","credit":"Created by",` + btn + `}`,
		"logo centre":              `{"id":"a","layout":"logo-center",` + logo + `,` + btn + `}`,
		"logo and text":            `{"id":"a","layout":"logo-text","text":"Hello",` + logo + `,` + btn + `}`,
	}
	for name, c := range good {
		if _, ok := one(t, c); !ok {
			t.Errorf("%s was dropped", name)
		}
	}
	bad := map[string]string{
		"unknown layout":             `{"id":"a","layout":"banner","credit":"x",` + btn + `}`,
		"credit row without credit":  `{"id":"a","layout":"credit-row",` + btn + `}`,
		"logo centre without logo":   `{"id":"a","layout":"logo-center",` + btn + `}`,
		"logo and text without text": `{"id":"a","layout":"logo-text",` + logo + `,` + btn + `}`,
		"no active flag":             `{"id":"a","layout":"credit-row","credit":"x","buttons":[]}`,
		"empty id":                   `{"id":"","layout":"credit-row","credit":"x",` + btn + `}`,
		"overlong text":              `{"id":"a","layout":"logo-text","text":"` + strings.Repeat("x", 281) + `",` + logo + `,` + btn + `}`,
	}
	for name, c := range bad {
		if _, ok := one(t, c); ok {
			t.Errorf("%s was accepted", name)
		}
	}
	// a centred logo shows nothing but the logo and the buttons, whatever else the file says
	c, _ := one(t, `{"id":"a","layout":"logo-center","credit":"Ignored","text":"Ignored",`+logo+`,`+btn+`}`)
	if c.Credit != "" || c.Text != "" {
		t.Errorf("a centred logo kept text: %+v", c)
	}
}

func TestTheLogoIsCheckedByWhatItReallyIs(t *testing.T) {
	big := pngURI(t, 1201, 10)
	tall := pngURI(t, 10, 401)
	notPNG := pngPrefix + base64.StdEncoding.EncodeToString([]byte("GIF89a not a png at all"))
	svg := "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(`<svg onload="alert(1)"/>`))
	var huge bytes.Buffer
	noisy := image.NewRGBA(image.Rect(0, 0, 100, 80))
	rand.New(rand.NewSource(1)).Read(noisy.Pix) // noise does not compress
	png.Encode(&huge, noisy)
	tooMany := pngPrefix + base64.StdEncoding.EncodeToString(huge.Bytes())
	for name, uri := range map[string]string{"too wide": big, "too tall": tall, "not a PNG": notPNG, "an SVG": svg, "over 24 KB": tooMany, "http": "http://example.org/x.png", "empty": pngPrefix} {
		if _, ok := one(t, `{"id":"a","layout":"logo-center","logo":{"image":"`+uri+`","alt":"x"},`+btn+`}`); ok {
			t.Errorf("%s was accepted", name)
		}
	}
	if len(huge.Bytes()) <= maxImageBytes {
		t.Fatalf("the test image is not over the limit: %d", len(huge.Bytes()))
	}
	good := pngURI(t, 1200, 400)
	if _, ok := one(t, `{"id":"a","layout":"logo-center","logo":{"image":"`+good+`","alt":"x"},`+btn+`}`); !ok {
		t.Error("an image at the limits was refused")
	}
	for name, logo := range map[string]string{
		"no alt text":     `{"image":"` + good + `"}`,
		"mark and image":  `{"mark":"hermes","image":"` + good + `","alt":"x"}`,
		"an unknown mark": `{"mark":"acme"}`,
		"an empty logo":   `{}`,
		"over-long alt":   `{"image":"` + good + `","alt":"` + strings.Repeat("x", 101) + `"}`,
	} {
		if _, ok := one(t, `{"id":"a","layout":"logo-center","logo":`+logo+`,`+btn+`}`); ok {
			t.Errorf("%s was accepted", name)
		}
	}
}

func TestOneBadCardDoesNotTakeTheOthersDown(t *testing.T) {
	c, ok := ParseFeed([]byte(`{"version":1,"entries":[],"cards":[
	  {"id":"bad","layout":"nope","credit":"x","buttons":[],"active":true},
	  "not even an object",
	  {"id":"ok","layout":"credit-row","credit":"Created by","logo":{"mark":"hermes"},"active":true,
	   "buttons":[{"text":"ok","url":"https://example.org/x"},{"text":"http","url":"http://example.org/"},{"text":"js","url":"javascript:alert(1)"},{"text":"port","url":"https://example.org:81/"}]}]}`))
	if !ok || len(c.Cards) != 1 || c.Cards[0].ID != "ok" {
		t.Fatalf("%v %+v", ok, c.Cards)
	}
	if len(c.Cards[0].Buttons) != 1 || c.Cards[0].Buttons[0].URL != "https://example.org/x" {
		t.Fatalf("only https buttons without a user name or port: %+v", c.Cards[0].Buttons)
	}
}

func TestTheFirstFormatStillWorksBesideTheNewOne(t *testing.T) {
	c, ok := ParseFeed([]byte(`{"version":1,
	 "entries":[{"id":"old","kind":"sponsor","title":"Created by","text":"Free software.","marks":["hermes"],"active":true,"links":[{"text":"Go","url":"https://example.org/x"}]}],
	 "cards":[{"id":"new","layout":"credit-row","credit":"Created by","logo":{"mark":"hermes"},"active":true,"buttons":[]}]}`))
	if !ok || len(c.Entries) != 1 || len(c.Cards) != 1 {
		t.Fatalf("%v %+v", ok, c)
	}
	// a file with only entries, and a file with only cards, are both fine
	if c, ok := ParseFeed([]byte(good)); !ok || len(c.Entries) != 1 || len(c.Cards) != 0 {
		t.Fatalf("%v %+v", ok, c)
	}
	if _, ok := ParseFeed([]byte(`{"version":1,"cards":[` + strings.Repeat(`{},`, 10) + `{}]}`)); ok {
		t.Fatal("too many cards were accepted")
	}
}

func TestCardsAreShownOnTheirDaysOnly(t *testing.T) {
	mk := func(id, from, until string, active bool) Card {
		return Card{ID: id, From: from, Until: until, Active: active}
	}
	now := time.Date(2026, 12, 15, 23, 59, 0, 0, time.UTC)
	got := Current([]Card{
		mk("always", "", "", true), mk("not yet", "2026-12-16", "", true), mk("last day", "", "2026-12-15", true),
		mk("over", "", "2026-12-14", true), mk("window", "2026-12-01", "2026-12-31", true), mk("off", "", "", false), mk("first day", "2026-12-15", "", true),
	}, now)
	var ids []string
	for _, c := range got {
		ids = append(ids, c.ID)
	}
	if strings.Join(ids, ",") != "always,last day,window,first day" {
		t.Fatalf("%v", ids)
	}
	for name, c := range map[string]string{
		"a bad date":        `"from":"15/12/2026"`,
		"until before from": `"from":"2026-12-10","until":"2026-12-01"`,
	} {
		if _, ok := one(t, `{"id":"a","layout":"credit-row","credit":"x",`+c+`,`+btn+`}`); ok {
			t.Errorf("%s was accepted", name)
		}
	}
}

// The file in this repository is what the apps in people's hands will read: it must pass, with a card for the new apps and an entry for the old ones.
func TestTheShippedFeedIsValid(t *testing.T) {
	b, err := os.ReadFile("../../../site/cards.json")
	if err != nil {
		t.Fatal(err)
	}
	c, ok := ParseFeed(b)
	if !ok || len(c.Cards) == 0 || len(c.Entries) == 0 {
		t.Fatalf("site/cards.json: ok=%v, %d cards, %d entries (it must hold both, so that old and new apps show a card)", ok, len(c.Cards), len(c.Entries))
	}
	var raw struct{ Cards, Entries []map[string]any }
	_ = raw
	if len(Current(c.Cards, time.Now())) == 0 {
		t.Fatal("no card is shown today")
	}
}
