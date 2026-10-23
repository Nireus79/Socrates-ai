package api

import (
	"context"
	"net/http"
	"strings"

	"socrates/internal/llm"
	"socrates/internal/store"
	"socrates/internal/transport"
)

// The provider settings live in the encrypted store, under these keys. The API key is one of them and is never returned.
const (
	kProvider = "llm.provider"
	kKey      = "llm.key"
	kEndpoint = "llm.ollama_endpoint"
	kFast     = "llm.fast_model"
	kReply    = "llm.reply_model"
	kTested   = "llm.tested"
)

// LLMSettingsView is what the screens may see.
type LLMSettingsView struct {
	Provider       string `json:"provider"`
	OllamaEndpoint string `json:"ollamaEndpoint"`
	FastModel      string `json:"fastModel"`
	ReplyModel     string `json:"replyModel"`
	HasKey         bool   `json:"hasKey"`
	Tested         bool   `json:"tested"` // both models were measured on the program's batteries
}

func (h *handler) load(r *http.Request) (llm.Settings, bool, error) { return h.loadCtx(r.Context()) }

func (h *handler) loadCtx(ctx context.Context) (llm.Settings, bool, error) {
	var s llm.Settings
	get := func(k string) (string, error) {
		v, _, err := h.d.Store.Setting(ctx, k)
		return v, err
	}
	var err error
	if s.Provider, err = get(kProvider); err != nil {
		return s, false, err
	}
	if s.APIKey, err = get(kKey); err != nil {
		return s, false, err
	}
	if s.OllamaEndpoint, err = get(kEndpoint); err != nil {
		return s, false, err
	}
	if s.FastModel, err = get(kFast); err != nil {
		return s, false, err
	}
	if s.ReplyModel, err = get(kReply); err != nil {
		return s, false, err
	}
	t, err := get(kTested)
	return s, t == "1", err
}

func view(s llm.Settings, tested bool) LLMSettingsView {
	return LLMSettingsView{Provider: s.Provider, OllamaEndpoint: s.OllamaEndpoint, FastModel: s.FastModel, ReplyModel: s.ReplyModel, HasKey: s.APIKey != "", Tested: tested}
}

func (h *handler) getLLM(w http.ResponseWriter, r *http.Request) {
	s, tested, err := h.load(r)
	if err != nil {
		fail(w, http.StatusInternalServerError, "the settings could not be read")
		return
	}
	transport.WriteJSON(w, http.StatusOK, view(s, tested))
}

// putLLM changes the provider, the key or the Ollama address. The new settings are tried first (the provider is asked which models the
// key can use) and saved only when that works, so a wrong key is never stored. An omitted apiKey keeps the stored one.
func (h *handler) putLLM(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Provider       string  `json:"provider"`
		APIKey         *string `json:"apiKey"`
		OllamaEndpoint string  `json:"ollamaEndpoint"`
	}
	if !transport.ReadBody(w, r, &body) {
		return
	}
	s, _, err := h.load(r)
	if err != nil {
		fail(w, http.StatusInternalServerError, "the settings could not be read")
		return
	}
	provider := strings.ToLower(strings.TrimSpace(body.Provider))
	if provider != "claude" && provider != "openai" && provider != "ollama" {
		fail(w, http.StatusBadRequest, "The provider must be claude, openai or ollama.")
		return
	}
	next := llm.Settings{Provider: provider, APIKey: s.APIKey, OllamaEndpoint: strings.TrimSpace(body.OllamaEndpoint)}
	if provider != s.Provider {
		next.APIKey = "" // a key belongs to one provider
	}
	if body.APIKey != nil {
		next.APIKey = strings.TrimSpace(*body.APIKey)
	}
	if provider == "ollama" {
		next.APIKey = ""
		if next.OllamaEndpoint == "" {
			next.OllamaEndpoint = s.OllamaEndpoint
		}
		if e, err := llm.NormalizeOllamaEndpoint(next.OllamaEndpoint); err != nil {
			fail(w, http.StatusBadRequest, err.Error())
			return
		} else {
			next.OllamaEndpoint = e
		}
	} else if next.APIKey == "" {
		fail(w, http.StatusBadRequest, "An API key is needed for this provider.")
		return
	}
	list, err := llm.ListModels(background(r), next, h.d.LLMOptions...)
	if err != nil {
		failLLM(w, err)
		return
	}
	roles, err := llm.ResolveRoleModels(provider, list, llm.ApprovedModels())
	if err != nil {
		fail(w, http.StatusUnprocessableEntity, "The provider lists no model this key can use.")
		return
	}
	next.FastModel, next.ReplyModel = roles.Fast, roles.Reply
	tested := "0"
	if roles.Tested {
		tested = "1"
	}
	saved := map[string]string{kProvider: next.Provider, kKey: next.APIKey, kEndpoint: next.OllamaEndpoint, kFast: next.FastModel, kReply: next.ReplyModel, kTested: tested}
	err = h.d.Store.Update(r.Context(), func(o *store.Ops) error {
		for k, v := range saved {
			if v == "" {
				if err := o.DeleteSetting(r.Context(), k); err != nil {
					return err
				}
				continue
			}
			if err := o.SetSetting(r.Context(), k, v); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		fail(w, http.StatusInternalServerError, "the settings could not be saved")
		return
	}
	transport.WriteJSON(w, http.StatusOK, view(next, roles.Tested))
}

func (h *handler) deleteKey(w http.ResponseWriter, r *http.Request) {
	if err := h.d.Store.DeleteSetting(r.Context(), kKey); err != nil {
		fail(w, http.StatusInternalServerError, "the key could not be removed")
		return
	}
	s, tested, _ := h.load(r)
	transport.WriteJSON(w, http.StatusOK, view(s, tested))
}

// testLLM makes one tiny call with the stored settings, to tell the user whether the provider answers.
func (h *handler) testLLM(w http.ResponseWriter, r *http.Request) {
	s, _, err := h.load(r)
	if err != nil {
		fail(w, http.StatusInternalServerError, "the settings could not be read")
		return
	}
	if s.Provider == "" {
		fail(w, http.StatusConflict, "No provider has been chosen yet.")
		return
	}
	c, err := llm.NewClient(s, h.d.LLMOptions...)
	if err != nil {
		failLLM(w, err)
		return
	}
	if _, err := c.Complete(r.Context(), llm.Request{Role: llm.RoleFast, System: "Answer with one word.", Messages: []llm.Message{{Role: "user", Content: "Say ok."}}, MaxTokens: 20}); err != nil {
		failLLM(w, err)
		return
	}
	transport.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
