package llm

import (
	"context"
	"net/http"
	"strings"
	"time"
)

// Option changes how a provider is built. They exist for tests (a stand-in server, no waiting between retries).
type Option func(*options)

type options struct {
	base   string
	client *http.Client
	sleep  func(context.Context, time.Duration) error
}

func applyOptions(opts []Option) *options {
	o := &options{}
	for _, f := range opts {
		f(o)
	}
	return o
}

// WithBaseURL points a provider at another server.
func WithBaseURL(u string) Option { return func(o *options) { o.base = strings.TrimRight(u, "/") } }

// WithHTTPClient replaces the HTTP client.
func WithHTTPClient(c *http.Client) Option { return func(o *options) { o.client = c } }

// WithSleep replaces the wait between retries.
func WithSleep(f func(context.Context, time.Duration) error) Option {
	return func(o *options) { o.sleep = f }
}

func (o *options) baseURL(def string) string {
	if o.base != "" {
		return o.base
	}
	return def
}

func (o *options) doer(secret string) *doer {
	d := &doer{client: o.client, sleep: o.sleep, secret: secret}
	if d.client == nil {
		d.client = newHTTPClient()
	}
	if d.sleep == nil {
		d.sleep = sleepCtx
	}
	return d
}
