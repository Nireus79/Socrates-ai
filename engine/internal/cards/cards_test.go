package cards

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const good = `{"version":1,"entries":[{"id":"a","kind":"sponsor","title":"T","text":"Body","active":true,"links":[{"text":"Go","url":"https://example.org/x","primary":true}]}]}`

func TestParseKeepsOnlyWhatPasses(t *testing.T) {
	long := strings.Repeat("x", 300)
	cases := map[string]string{
		"version 2": `{"version":2,"entries":[]}`,
		"not json":  `<html>`,
		"too many":  `{"version":1,"entries":[` + strings.Repeat(`{},`, 10) + `{}]}`,
	}
	for name, in := range cases {
		if _, ok := Parse([]byte(in)); ok {
			t.Errorf("%s accepted", name)
		}
	}
	e, ok := Parse([]byte(`{"version":1,"entries":[
	 {"id":"a","kind":"sponsor","title":"T","text":"Body","active":true,"links":[
	   {"text":"ok","url":"https://example.org/x"},{"text":"http","url":"http://example.org/"},{"text":"js","url":"javascript:alert(1)"},
	   {"text":"user","url":"https://u:p@example.org/"},{"text":"port","url":"https://example.org:8443/"},{"text":"","url":"https://example.org/"},
	   {"text":"two","url":"https://example.org/2"},{"text":"three","url":"https://example.org/3"},{"text":"four","url":"https://example.org/4"},{"text":"five","url":"https://example.org/5"}]},
	 {"id":"b","kind":"other","title":"T","text":"Body","active":true},
	 {"id":"c","kind":"sponsor","title":"T","text":"` + long + `","active":true},
	 {"id":"d","kind":"sponsor","title":"T","text":"Body"},
	 {"id":"","kind":"sponsor","title":"T","text":"Body","active":true}]}`))
	if !ok || len(e) != 1 || e[0].ID != "a" {
		t.Fatalf("%v %+v", ok, e)
	}
	if len(e[0].Links) != 4 || e[0].Links[0].URL != "https://example.org/x" || e[0].Links[3].URL != "https://example.org/4" {
		t.Fatalf("only https links without user name or port, at most four: %+v", e[0].Links)
	}
}

func server(t *testing.T, body string, code int, hits *int32) *http.Client {
	t.Helper()
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(hits, 1)
		if len(r.Cookies()) != 0 || r.Header.Get("Referer") != "" || r.Header.Get("Authorization") != "" {
			t.Error("the request carried something about the person")
		}
		w.WriteHeader(code)
		w.Write([]byte(body))
	}))
	t.Cleanup(s.Close)
	c := s.Client()
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	feedURL = s.URL
	return c
}

var feedURL string

func TestTheFeedIsFetchedOncePerDayAndAFailureKeepsTheLastGoodAnswer(t *testing.T) {
	var hits int32
	c := server(t, good, 200, &hits)
	f := NewFeedWith(feedURL, c)
	now := time.Now()
	f.now = func() time.Time { return now }
	if e := f.Entries(context.Background()); len(e) != 1 {
		t.Fatalf("%+v", e)
	}
	f.Entries(context.Background())
	if hits != 1 {
		t.Fatalf("fetched %d times within a day", hits)
	}
	now = now.Add(25 * time.Hour)
	f.Entries(context.Background())
	if hits != 2 {
		t.Fatalf("not refreshed after a day: %d", hits)
	}
	// the file turns bad: the last good answer stays, and there is no new attempt within the hour
	var bad int32
	c2 := server(t, `{"version":9}`, 200, &bad)
	f.client, f.url = c2, feedURL
	now = now.Add(25 * time.Hour)
	if e := f.Entries(context.Background()); len(e) != 1 {
		t.Fatalf("the last good answer was lost: %+v", e)
	}
	f.Entries(context.Background())
	if bad != 1 {
		t.Fatalf("a failed fetch was repeated: %d", bad)
	}
}

func TestNothingValidMeansNothing(t *testing.T) {
	var hits int32
	for _, tc := range []struct {
		body string
		code int
	}{{good, 404}, {`{"version":1,"entries":[]}` + strings.Repeat(" ", 70000), 200}, {"nope", 200}, {good, 302}} {
		f := NewFeedWith("", server(t, tc.body, tc.code, &hits))
		f.url = feedURL
		if e := f.Entries(context.Background()); e != nil {
			t.Errorf("%d %.20q gave %+v", tc.code, tc.body, e)
		}
	}
}

func TestOnlyHTTPSIsFetched(t *testing.T) {
	var hits int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { atomic.AddInt32(&hits, 1); w.Write([]byte(good)) }))
	defer s.Close()
	if e := NewFeed(s.URL).Entries(context.Background()); e != nil || hits != 0 {
		t.Fatalf("an http address was used: %v %d", e, hits)
	}
	var nilFeed *Feed
	if nilFeed.Entries(context.Background()) != nil {
		t.Fatal("no feed means no entries")
	}
}
