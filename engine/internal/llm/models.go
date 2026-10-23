package llm

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Which models exist is asked of the provider, with the user's own key, never assumed: a list written into the program is out of date
// the day a provider adds or retires a model. Which model serves which role is chosen from that list and a table shipped with the
// program, in this order, so that a retired or a new model never needs an update:
//  1. a model the program was tested with (the approved table), newest first;
//  2. a listed model of the table that has not been measured yet;
//  3. otherwise the newest model of the right kind, found from the list (the provider's own family words, never a full model name);
//  4. otherwise the provider's suggestion.

//go:embed approved_models.json
var approvedModelsJSON []byte

// ApprovedModel is one model of the table.
type ApprovedModel struct {
	ID       string `json:"id"`
	Approved string `json:"approved"` // the date the live batteries passed on it; empty: listed, not measured yet
	Notes    string `json:"notes"`
}

// ApprovedTable maps a provider to its models per role.
type ApprovedTable map[string]map[string][]ApprovedModel

// ApprovedModels is the table shipped with this version.
func ApprovedModels() ApprovedTable {
	var t ApprovedTable
	_ = json.Unmarshal(approvedModelsJSON, &t) // part of the program and tested; "_comment" is not a provider
	delete(t, "_comment")
	return t
}

// ModelInfo is one model a provider offers.
type ModelInfo struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Line  string `json:"line,omitempty"` // Claude: haiku, sonnet, opus, ... as the provider says
}

// ModelList is what a provider offers and the one to preselect when the user has not chosen.
type ModelList struct {
	Models    []ModelInfo `json:"models"`
	Suggested string      `json:"suggested"`
}

// RoleModels are the models chosen for the two roles.
type RoleModels struct {
	Fast   string `json:"fast"`
	Reply  string `json:"reply"`
	Tested bool   `json:"tested"` // both were measured; false: at least one is a listed or a guessed model
}

// ResolveRoleModels chooses the model of each role. An approved model that the list does not hold is skipped for the next one.
func ResolveRoleModels(provider string, list *ModelList, table ApprovedTable) (RoleModels, error) {
	if list == nil || len(list.Models) == 0 {
		return RoleModels{}, errors.New("the provider lists no model for this key")
	}
	available := map[string]bool{}
	for _, m := range list.Models {
		available[m.ID] = true
	}
	pick := func(role string) (string, bool) {
		for _, a := range table[strings.ToLower(provider)][role] {
			if a.Approved != "" && available[a.ID] {
				return a.ID, true
			}
		}
		for _, a := range table[strings.ToLower(provider)][role] {
			if available[a.ID] {
				return a.ID, false
			}
		}
		return dynamicPick(provider, role, list), false
	}
	var out RoleModels
	var fastOK, replyOK bool
	out.Fast, fastOK = pick("fast")
	out.Reply, replyOK = pick("reply")
	if out.Fast == "" {
		out.Fast = list.Suggested
	}
	if out.Reply == "" {
		out.Reply = list.Suggested
	}
	if out.Fast == "" || out.Reply == "" {
		return RoleModels{}, errors.New("the provider lists no usable model for this key")
	}
	out.Tested = fastOK && replyOK
	return out, nil
}

// dynamicPick chooses by the family words the providers themselves use: Claude "haiku" for fast and "sonnet" for replies; OpenAI the
// highest version of "mini" (or "nano") for fast and of the plain model for replies. Ollama has no families: the caller chooses.
func dynamicPick(provider, role string, list *ModelList) string {
	switch strings.ToLower(provider) {
	case "claude":
		want := "sonnet"
		if role == "fast" {
			want = "haiku"
		}
		for _, m := range list.Models { // the provider lists the newest first
			line := strings.ToLower(m.Line)
			if line == "" {
				line = strings.ToLower(m.ID)
			}
			if strings.Contains(line, want) {
				return m.ID
			}
		}
	case "openai":
		return pickOpenAI(role, list)
	}
	return ""
}

var openAIPlain = regexp.MustCompile(`^gpt-(\d+(?:\.\d+)?)(o)?(?:-(mini|nano))?$`)
var openAIDated = regexp.MustCompile(`^(.*)-\d{4}-\d{2}-\d{2}$|^(.*)-\d{4}$`)

func pickOpenAI(role string, list *ModelList) string {
	have := map[string]bool{}
	for _, m := range list.Models {
		have[m.ID] = true
	}
	type cand struct {
		id      string
		version float64
		small   int // 0 plain, 1 mini, 2 nano
	}
	var cands []cand
	for _, m := range list.Models {
		id := m.ID
		if g := openAIDated.FindStringSubmatch(id); g != nil {
			base := g[1]
			if base == "" {
				base = g[2]
			}
			if have[base] {
				continue // a dated snapshot of a model that has its own name
			}
			id = base
		}
		g := openAIPlain.FindStringSubmatch(id)
		if g == nil {
			continue // special products (pro, codex, preview, search, ...)
		}
		v, _ := strconv.ParseFloat(g[1], 64)
		small := 0
		switch g[3] {
		case "mini":
			small = 1
		case "nano":
			small = 2
		}
		cands = append(cands, cand{m.ID, v, small})
	}
	best := func(smalls ...int) string {
		bestID, bestV := "", -1.0
		for _, size := range smalls {
			for _, c := range cands {
				if c.small == size && c.version > bestV {
					bestID, bestV = c.id, c.version
				}
			}
			if bestID != "" {
				return bestID
			}
		}
		return ""
	}
	if role == "fast" {
		return best(1, 2, 0)
	}
	return best(0)
}

// ---- listing

// openAINotChat marks the OpenAI models that are not for chat (the list has no field that says what a model is for). A model that is
// left out wrongly can still be typed in.
var openAINotChat = regexp.MustCompile(`(?i)(embedding|whisper|tts|dall-e|moderation|image|audio|realtime|transcribe|search|davinci|babbage|instruct|computer-use)`)

// ListModels asks the provider which models the key can use. Ollama needs no key: the server's own list.
func ListModels(ctx context.Context, s Settings, opts ...Option) (*ModelList, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	o := applyOptions(opts)
	switch strings.ToLower(strings.TrimSpace(s.Provider)) {
	case "claude":
		return listClaude(ctx, o, s.APIKey)
	case "openai":
		return listOpenAI(ctx, o, s.APIKey)
	case "ollama":
		e, err := NormalizeOllamaEndpoint(s.OllamaEndpoint)
		if err != nil {
			return nil, err
		}
		return listOllama(ctx, o, e)
	}
	return nil, &Error{Kind: KindInvalid, Reason: "unknown AI provider " + strconv.Quote(s.Provider)}
}

func (d *doer) get(ctx context.Context, rawURL string, headers map[string]string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return &Error{Kind: KindInvalid, Reason: "the request could not be made"}
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return &Error{Kind: KindNetwork, Reason: "no answer from the provider"}
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusOK {
		return d.classify(resp, body)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return &Error{Kind: KindServer, Status: resp.StatusCode, Reason: "the list of models could not be read"}
	}
	return nil
}

func needKey(key string) error {
	if strings.TrimSpace(key) == "" {
		return &Error{Kind: KindAuth, Reason: "an API key is needed to list the models"}
	}
	return nil
}

func listClaude(ctx context.Context, o *options, key string) (*ModelList, error) {
	if err := needKey(key); err != nil {
		return nil, err
	}
	d := o.doer(key)
	base := o.baseURL(anthropicURL)
	list := &ModelList{Models: []ModelInfo{}}
	after := ""
	for page := 0; page < 5; page++ {
		q := url.Values{"limit": {"1000"}}
		if after != "" {
			q.Set("after_id", after)
		}
		var resp struct {
			Data []struct {
				ID          string `json:"id"`
				DisplayName string `json:"display_name"`
				Line        string `json:"line"`
				Lifecycle   string `json:"lifecycle"`
			} `json:"data"`
			HasMore bool   `json:"has_more"`
			LastID  string `json:"last_id"`
		}
		if err := d.get(ctx, base+"/v1/models?"+q.Encode(), map[string]string{"x-api-key": key, "anthropic-version": anthropicVersion}, &resp); err != nil {
			return nil, err
		}
		for _, m := range resp.Data {
			if m.ID == "" || (m.Lifecycle != "" && m.Lifecycle != "active") {
				continue // retired models are left out
			}
			label := m.DisplayName
			if label == "" {
				label = m.ID
			}
			list.Models = append(list.Models, ModelInfo{ID: m.ID, Label: label, Line: m.Line})
		}
		if !resp.HasMore || resp.LastID == "" {
			break
		}
		after = resp.LastID
	}
	for _, m := range list.Models {
		if m.Line == "sonnet" { // the balanced line: the newest overall is usually the dearest
			list.Suggested = m.ID
			break
		}
	}
	if list.Suggested == "" && len(list.Models) > 0 {
		list.Suggested = list.Models[0].ID
	}
	return list, nil
}

func listOpenAI(ctx context.Context, o *options, key string) (*ModelList, error) {
	if err := needKey(key); err != nil {
		return nil, err
	}
	d := o.doer(key)
	var resp struct {
		Data []struct {
			ID      string `json:"id"`
			Created int64  `json:"created"`
		} `json:"data"`
	}
	if err := d.get(ctx, o.baseURL(openAIURL)+"/v1/models", map[string]string{"Authorization": "Bearer " + key}, &resp); err != nil {
		return nil, err
	}
	type entry struct {
		id      string
		created int64
	}
	var entries []entry
	for _, m := range resp.Data {
		if m.ID != "" && !openAINotChat.MatchString(m.ID) {
			entries = append(entries, entry{m.ID, m.Created})
		}
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].created > entries[j].created })
	list := &ModelList{Models: []ModelInfo{}}
	for _, e := range entries {
		list.Models = append(list.Models, ModelInfo{ID: e.id, Label: e.id})
	}
	if len(list.Models) > 0 {
		list.Suggested = list.Models[0].ID
	}
	return list, nil
}

func listOllama(ctx context.Context, o *options, endpoint string) (*ModelList, error) {
	d := o.doer("")
	var resp struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := d.get(ctx, endpoint+"/api/tags", nil, &resp); err != nil {
		return nil, fmt.Errorf("Ollama did not answer at %s", endpoint)
	}
	list := &ModelList{Models: []ModelInfo{}}
	for _, m := range resp.Models {
		if m.Name != "" {
			list.Models = append(list.Models, ModelInfo{ID: m.Name, Label: m.Name})
		}
	}
	if len(list.Models) > 0 {
		list.Suggested = list.Models[0].ID
	}
	return list, nil
}
