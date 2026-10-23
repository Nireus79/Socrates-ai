package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	maxResponseBytes = 8 << 20
	maxAttempts      = 3
	maxRetryWait     = 30 * time.Second
)

// transport is shared by every provider: no proxy surprises, bounded connection set-up, and no overall timeout (the caller's context
// bounds a call, because a reply that thinks can take minutes).
func newHTTPClient() *http.Client {
	return &http.Client{Transport: &http.Transport{
		DialContext:         (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
		TLSHandshakeTimeout: 10 * time.Second,
		MaxIdleConns:        4,
		IdleConnTimeout:     60 * time.Second,
		ForceAttemptHTTP2:   true,
	}}
}

// doer sends requests with a few bounded retries.
type doer struct {
	client *http.Client
	sleep  func(context.Context, time.Duration) error
	secret string // the key, removed from anything that could reach an error text
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// result is a successful answer's body, or a classified failure.
type result struct {
	body   []byte
	status int
}

// post sends the JSON body. It retries a rate limit, an overload, a server failure and a lost connection, up to three attempts in all
// (the first wait is a second, then three; a Retry-After of the provider is honoured up to 30 seconds). Other failures are returned at
// once. handle400 gets the cleaned reason of a 400 and may ask for one more attempt with a changed body.
func (d *doer) post(ctx context.Context, url string, headers map[string]string, body []byte, handle400 func(reason string) []byte) (*result, error) {
	waits := []time.Duration{time.Second, 3 * time.Second}
	var last error
	for attempt := 0; attempt < maxAttempts+2; attempt++ { // two extra attempts are allowed only for the changed bodies of handle400
		res, err := d.once(ctx, url, headers, body)
		if err == nil {
			return res, nil
		}
		last = err
		var e *Error
		if !errors.As(err, &e) {
			return nil, err
		}
		if e.Kind == KindBadRequest && handle400 != nil {
			if changed := handle400(e.Reason); changed != nil {
				body = changed
				continue
			}
		}
		retryable := e.Kind == KindRateLimit || e.Kind == KindOverloaded || e.Kind == KindServer || e.Kind == KindNetwork
		if !retryable || len(waits) == 0 {
			return nil, err
		}
		wait := waits[0]
		waits = waits[1:]
		if e.RetryAfter > 0 {
			wait = min(e.RetryAfter, maxRetryWait)
		}
		wait += time.Duration(rand.Int64N(int64(wait/4) + 1)) // a little jitter
		if err := d.sleep(ctx, wait); err != nil {
			return nil, &Error{Kind: KindNetwork, Reason: "the call was cancelled"}
		}
	}
	return nil, last
}

func (d *doer) once(ctx context.Context, url string, headers map[string]string, body []byte) (*result, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, &Error{Kind: KindInvalid, Reason: d.clean("the request could not be made: " + err.Error())}
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := d.client.Do(req)
	if err != nil {
		reason := "no answer from the provider"
		if ctx.Err() != nil {
			reason = "the call was cancelled or ran out of time"
		}
		return nil, &Error{Kind: KindNetwork, Reason: reason}
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, &Error{Kind: KindNetwork, Status: resp.StatusCode, Reason: "the answer was cut off"}
	}
	if len(data) > maxResponseBytes {
		return nil, &Error{Kind: KindServer, Status: resp.StatusCode, Reason: "the answer is larger than any real answer"}
	}
	if resp.StatusCode == http.StatusOK {
		return &result{body: data, status: resp.StatusCode}, nil
	}
	return nil, d.classify(resp, data)
}

// classify turns an error answer into an *Error with a short cleaned reason.
func (d *doer) classify(resp *http.Response, data []byte) error {
	reason := d.clean(errorReason(data))
	e := &Error{Status: resp.StatusCode, Reason: reason}
	if s := resp.Header.Get("Retry-After"); s != "" {
		if secs, err := strconv.Atoi(strings.TrimSpace(s)); err == nil && secs >= 0 {
			e.RetryAfter = time.Duration(secs) * time.Second
		}
	}
	lower := strings.ToLower(string(data))
	switch {
	case modelGone(resp.StatusCode, lower):
		e.Kind = KindModelGone
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		e.Kind = KindAuth
	case resp.StatusCode == http.StatusTooManyRequests:
		e.Kind = KindRateLimit
	case resp.StatusCode == 529 || resp.StatusCode == http.StatusServiceUnavailable:
		e.Kind = KindOverloaded
	case resp.StatusCode >= 500:
		e.Kind = KindServer
	default:
		e.Kind = KindBadRequest
	}
	return e
}

// modelGone reports whether an error answer says that the model does not exist.
func modelGone(status int, lowerBody string) bool {
	switch {
	case strings.Contains(lowerBody, "model_not_found"):
		return true
	case status == http.StatusNotFound && strings.Contains(lowerBody, "model"):
		return true
	case strings.Contains(lowerBody, "model") && (strings.Contains(lowerBody, "does not exist") || strings.Contains(lowerBody, "has been deprecated") || strings.Contains(lowerBody, "has been retired")):
		return true
	}
	return false
}

// errorReason reads the human reason out of the common error shapes ({"error":{"message":..}} and {"error":"..."}), and
// otherwise says nothing: a raw body is never passed on.
func errorReason(data []byte) string {
	var shaped struct {
		Error json.RawMessage `json:"error"`
	}
	if json.Unmarshal(data, &shaped) != nil || len(shaped.Error) == 0 {
		return "the provider gave no reason"
	}
	var obj struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    any    `json:"code"`
	}
	if json.Unmarshal(shaped.Error, &obj) == nil && (obj.Message != "" || obj.Type != "") {
		if obj.Message != "" {
			return obj.Message
		}
		return obj.Type
	}
	var s string
	if json.Unmarshal(shaped.Error, &s) == nil && s != "" {
		return s
	}
	return "the provider gave no reason"
}

// clean removes the key, collapses white space and keeps the reason short.
func (d *doer) clean(s string) string {
	if d.secret != "" {
		s = strings.ReplaceAll(s, d.secret, "[key]")
	}
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > 200 {
		s = string(r[:200]) + "…"
	}
	return s
}
