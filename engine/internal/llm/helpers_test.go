package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// fakeAPI is a stand-in provider: it answers from a script and records what it received.
type fakeAPI struct {
	mu       sync.Mutex
	steps    []fakeStep
	Seen     []seenRequest
	Sleeps   []time.Duration
	srv      *httptest.Server
	sleepErr error
}

type fakeStep struct {
	status  int
	body    string
	headers map[string]string
}

type seenRequest struct {
	Path    string
	Headers http.Header
	Body    map[string]any
	Raw     string
}

func newFake(t *testing.T, steps ...fakeStep) *fakeAPI {
	t.Helper()
	f := &fakeAPI{steps: steps}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		var body map[string]any
		json.Unmarshal(data, &body)
		f.mu.Lock()
		f.Seen = append(f.Seen, seenRequest{Path: r.URL.RequestURI(), Headers: r.Header.Clone(), Body: body, Raw: string(data)})
		var s fakeStep
		if len(f.steps) > 0 {
			s, f.steps = f.steps[0], f.steps[1:]
		} else {
			s = fakeStep{status: 500, body: `{"error":{"message":"the script ran out"}}`}
		}
		f.mu.Unlock()
		for k, v := range s.headers {
			w.Header().Set(k, v)
		}
		w.WriteHeader(s.status)
		io.WriteString(w, s.body)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeAPI) opts() []Option {
	return []Option{WithBaseURL(f.srv.URL), WithSleep(func(_ context.Context, d time.Duration) error {
		f.mu.Lock()
		f.Sleeps = append(f.Sleeps, d)
		err := f.sleepErr
		f.mu.Unlock()
		return err
	})}
}

func (f *fakeAPI) last() seenRequest { return f.Seen[len(f.Seen)-1] }

func ok(body string) fakeStep { return fakeStep{status: 200, body: body} }
func status(code int, body string) fakeStep {
	return fakeStep{status: code, body: body}
}

const claudeText = `{"content":[{"type":"text","text":"Hello."}],"stop_reason":"end_turn","model":"claude-sonnet-5-5","usage":{"input_tokens":11,"output_tokens":3}}`

func userMsg(s string) []Message { return []Message{{Role: "user", Content: s}} }
