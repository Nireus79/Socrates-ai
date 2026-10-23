package llm

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func listOf(ids ...string) *ModelList {
	l := &ModelList{}
	for _, id := range ids {
		line := ""
		for _, f := range []string{"haiku", "sonnet", "opus"} {
			if strings.Contains(id, f) {
				line = f
			}
		}
		l.Models = append(l.Models, ModelInfo{ID: id, Label: id, Line: line})
	}
	if len(l.Models) > 0 {
		l.Suggested = l.Models[0].ID
	}
	return l
}

func TestTheApprovedTableShipsAndNamesTheClaudeRoles(t *testing.T) {
	table := ApprovedModels()
	if len(table["claude"]["fast"]) == 0 || len(table["claude"]["reply"]) == 0 {
		t.Fatalf("%v", table)
	}
	if _, has := table["_comment"]; has {
		t.Fatal("the comment is not a provider")
	}
}

func TestAnApprovedModelThatIsListedWinsAndTheResultIsTestedOnlyWhenBothAre(t *testing.T) {
	table := ApprovedTable{"claude": {"fast": {{ID: "f1", Approved: "2026-10-01"}}, "reply": {{ID: "r1", Approved: "2026-10-01"}}}}
	got, err := ResolveRoleModels("claude", listOf("opus-x", "r1", "f1"), table)
	if err != nil || got.Fast != "f1" || got.Reply != "r1" || !got.Tested {
		t.Fatalf("%+v %v", got, err)
	}
	table["claude"]["reply"][0].Approved = ""
	got, _ = ResolveRoleModels("claude", listOf("r1", "f1"), table)
	if got.Reply != "r1" || got.Tested {
		t.Fatalf("a listed but unmeasured model must not count as tested: %+v", got)
	}
}

func TestAnApprovedModelThatWasRetiredIsSkippedForTheNextOne(t *testing.T) {
	table := ApprovedTable{"claude": {"fast": {{ID: "gone", Approved: "2026-10-01"}, {ID: "f2", Approved: "2026-09-01"}}, "reply": {{ID: "r1", Approved: "2026-10-01"}}}}
	got, _ := ResolveRoleModels("claude", listOf("f2", "r1"), table)
	if got.Fast != "f2" {
		t.Fatalf("%+v", got)
	}
}

func TestAClaudeModelNotInTheTableIsFoundByItsFamily(t *testing.T) {
	got, err := ResolveRoleModels("claude", listOf("claude-opus-9", "claude-sonnet-9", "claude-haiku-9", "claude-sonnet-8"), ApprovedTable{})
	if err != nil || got.Reply != "claude-sonnet-9" || got.Fast != "claude-haiku-9" || got.Tested {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestWhenNothingFitsTheProvidersSuggestionIsUsedAndAnEmptyListIsAnError(t *testing.T) {
	got, err := ResolveRoleModels("ollama", listOf("mistral:latest", "llama3"), ApprovedTable{})
	if err != nil || got.Fast != "mistral:latest" || got.Reply != "mistral:latest" || got.Tested {
		t.Fatalf("%+v %v", got, err)
	}
	if _, err := ResolveRoleModels("claude", &ModelList{}, ApprovedTable{}); err == nil {
		t.Fatal("an empty list was accepted")
	}
	if _, err := ResolveRoleModels("claude", nil, ApprovedTable{}); err == nil {
		t.Fatal("no list was accepted")
	}
}

func TestOpenAIModelsAreFoundByVersionAndSize(t *testing.T) {
	l := listOf("gpt-6-pro", "gpt-5.5", "gpt-5.5-2026-08-01", "gpt-5.5-mini", "gpt-5", "gpt-5-codex", "gpt-4o", "o3", "gpt-5-search-preview")
	got, err := ResolveRoleModels("openai", l, ApprovedTable{})
	if err != nil || got.Reply != "gpt-5.5" || got.Fast != "gpt-5.5-mini" {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestListClaudeModelsFollowsPagesAndDropsRetiredOnes(t *testing.T) {
	f := newFake(t,
		ok(`{"data":[{"id":"claude-opus-5-5","display_name":"Opus 5.5","line":"opus","lifecycle":"active"},{"id":"claude-old","display_name":"Old","line":"sonnet","lifecycle":"retired"}],"has_more":true,"last_id":"claude-old"}`),
		ok(`{"data":[{"id":"claude-sonnet-5-5","display_name":"Sonnet 5.5","line":"sonnet","lifecycle":"active"},{"id":"claude-haiku-5-5","line":"haiku"}],"has_more":false}`))
	list, err := ListModels(context.Background(), Settings{Provider: "claude", APIKey: secretKey}, f.opts()...)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Seen) != 2 || !strings.Contains(f.Seen[1].Path, "after_id=claude-old") {
		t.Fatalf("pages: %v", f.Seen)
	}
	var ids []string
	for _, m := range list.Models {
		ids = append(ids, m.ID)
	}
	if strings.Join(ids, ",") != "claude-opus-5-5,claude-sonnet-5-5,claude-haiku-5-5" || list.Suggested != "claude-sonnet-5-5" {
		t.Fatalf("%v suggested %s", ids, list.Suggested)
	}
}

func TestListOpenAIModelsDropsWhatIsNotChatAndSortsNewestFirst(t *testing.T) {
	f := newFake(t, ok(`{"data":[{"id":"gpt-4o","created":100},{"id":"text-embedding-3-large","created":500},{"id":"gpt-5","created":300},{"id":"whisper-1","created":400},{"id":"gpt-5-mini","created":200}]}`))
	list, err := ListModels(context.Background(), Settings{Provider: "openai", APIKey: openAIKey}, f.opts()...)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, m := range list.Models {
		ids = append(ids, m.ID)
	}
	if strings.Join(ids, ",") != "gpt-5,gpt-5-mini,gpt-4o" || list.Suggested != "gpt-5" {
		t.Fatalf("%v", ids)
	}
	if f.last().Headers.Get("Authorization") != "Bearer "+openAIKey {
		t.Fatal("no key sent")
	}
}

func TestListOllamaModelsNeedsNoKey(t *testing.T) {
	f := newFake(t, ok(`{"models":[{"name":"mistral:latest"},{"name":"llama3:8b"}]}`))
	list, err := ListModels(context.Background(), Settings{Provider: "ollama", OllamaEndpoint: f.srv.URL}, f.opts()...)
	if err != nil || len(list.Models) != 2 || list.Suggested != "mistral:latest" {
		t.Fatalf("%v %+v", err, list)
	}
}

func TestListingNeedsAKeyForTheCloudProvidersAndRefusesAnUnknownOne(t *testing.T) {
	for _, p := range []string{"claude", "openai"} {
		if _, err := ListModels(context.Background(), Settings{Provider: p}); !errors.Is(err, ErrAuth) {
			t.Errorf("%s without a key: %v", p, err)
		}
	}
	if _, err := ListModels(context.Background(), Settings{Provider: "gemini", APIKey: "k"}); !errors.Is(err, ErrInvalid) {
		t.Errorf("unknown provider: %v", err)
	}
	f := newFake(t, status(401, `{"error":{"message":"bad key `+secretKey+`"}}`))
	_, err := ListModels(context.Background(), Settings{Provider: "claude", APIKey: secretKey}, f.opts()...)
	if !errors.Is(err, ErrAuth) || strings.Contains(err.Error(), secretKey) {
		t.Errorf("a refused key: %v", err)
	}
}
