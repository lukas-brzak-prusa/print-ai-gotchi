package main

// Where the pet can think: on this server, on a machine on the network, or in
// a cloud service. Every remote provider except Anthropic speaks the OpenAI
// chat API, so one client serves them; Claude goes through Anthropic's own SDK.

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// The kinds of client a provider needs.
const (
	kindLocal     = "local"     // the bundled llama.cpp, run by this module
	kindOpenAI    = "openai"    // any server that speaks the OpenAI chat API
	kindAnthropic = "anthropic" // the Messages API, through the Anthropic SDK
)

// Provider is one place the pet can think.
type Provider struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Kind  string `json:"kind"`
	// BaseURL is where the API is, with its version path. Empty for local.
	BaseURL string `json:"base_url"`
	// NeedsKey says the provider refuses a request without an API key.
	NeedsKey bool `json:"needs_key"`
	// OnPremises says nothing leaves the person's network: the server runs on
	// this machine or on another one the person runs.
	OnPremises bool `json:"on_premises"`
	// EditableURL says a person writes the address, as for a server on the network.
	EditableURL bool `json:"editable_url"`
	// KeyURL is where a person gets a key.
	KeyURL string `json:"key_url,omitempty"`
}

//nolint:gochecknoglobals // a closed table.
var providers = []Provider{
	{ID: "local", Title: "This server", Kind: kindLocal, OnPremises: true},
	{ID: "ollama", Title: "Ollama", Kind: kindOpenAI, BaseURL: "http://127.0.0.1:11434/v1", OnPremises: true, EditableURL: true},
	{ID: "lmstudio", Title: "LM Studio", Kind: kindOpenAI, BaseURL: "http://127.0.0.1:1234/v1", OnPremises: true, EditableURL: true},
	{ID: "anthropic", Title: "Anthropic Claude", Kind: kindAnthropic, BaseURL: "https://api.anthropic.com", NeedsKey: true, KeyURL: "https://platform.claude.com/settings/keys"},
	{ID: "openai", Title: "OpenAI", Kind: kindOpenAI, BaseURL: "https://api.openai.com/v1", NeedsKey: true, KeyURL: "https://platform.openai.com/api-keys"},
	{ID: "gemini", Title: "Google Gemini", Kind: kindOpenAI, BaseURL: "https://generativelanguage.googleapis.com/v1beta/openai", NeedsKey: true, KeyURL: "https://aistudio.google.com/apikey"},
	{ID: "openrouter", Title: "OpenRouter", Kind: kindOpenAI, BaseURL: "https://openrouter.ai/api/v1", NeedsKey: true, KeyURL: "https://openrouter.ai/keys"},
	{ID: "groq", Title: "Groq", Kind: kindOpenAI, BaseURL: "https://api.groq.com/openai/v1", NeedsKey: true, KeyURL: "https://console.groq.com/keys"},
	{ID: "mistral", Title: "Mistral", Kind: kindOpenAI, BaseURL: "https://api.mistral.ai/v1", NeedsKey: true, KeyURL: "https://console.mistral.ai/api-keys"},
	{ID: "custom", Title: "Another OpenAI-compatible server", Kind: kindOpenAI, EditableURL: true},
}

func providerByID(id string) (Provider, bool) {
	for _, p := range providers {
		if p.ID == id {
			return p, true
		}
	}

	return Provider{}, false
}

// Choice is the brain a person chose on the page, kept in the record store.
// Keys are kept per provider, so switching back and forth does not lose one.
type Choice struct {
	Provider   string            `json:"provider"`
	LocalModel string            `json:"local_model"`
	BaseURL    map[string]string `json:"base_url,omitempty"`
	Model      map[string]string `json:"model,omitempty"`
	Keys       map[string]string `json:"keys,omitempty"`
}

// ChoiceView is the choice as the page reads it: never a key, only whether one is set.
type ChoiceView struct {
	Provider   string            `json:"provider"`
	LocalModel string            `json:"local_model"`
	BaseURL    map[string]string `json:"base_url"`
	Model      map[string]string `json:"model"`
	HasKey     map[string]bool   `json:"has_key"`
}

func (c Choice) view() ChoiceView {
	out := ChoiceView{
		Provider: c.Provider, LocalModel: c.LocalModel,
		BaseURL: map[string]string{}, Model: map[string]string{}, HasKey: map[string]bool{},
	}

	for _, p := range providers {
		out.BaseURL[p.ID] = c.urlFor(p)
		out.Model[p.ID] = c.Model[p.ID]
		out.HasKey[p.ID] = c.Keys[p.ID] != ""
	}

	return out
}

func (c Choice) urlFor(p Provider) string {
	if u := c.BaseURL[p.ID]; u != "" && p.EditableURL {
		return u
	}

	return p.BaseURL
}

// ChoiceChange is what PUT /brain takes. A nil key keeps the stored one, an
// empty one removes it.
type ChoiceChange struct {
	Provider   string  `json:"provider"`
	LocalModel string  `json:"local_model,omitempty"`
	BaseURL    string  `json:"base_url,omitempty"`
	Model      string  `json:"model,omitempty"`
	Key        *string `json:"key,omitempty"`
}

var errBadChoice = errors.New("the brain choice is not valid")

// apply writes one change onto the choice, after checking it.
func (c Choice) apply(change ChoiceChange) (Choice, error) {
	p, ok := providerByID(change.Provider)
	if !ok {
		return c, fmt.Errorf("%w: there is no provider %q", errBadChoice, change.Provider)
	}

	next := Choice{
		Provider: p.ID, LocalModel: c.LocalModel,
		BaseURL: clone(c.BaseURL), Model: clone(c.Model), Keys: clone(c.Keys),
	}

	if p.Kind == kindLocal {
		if change.LocalModel != "" {
			if _, ok := brains[change.LocalModel]; !ok {
				return c, fmt.Errorf("%w: there is no local model %q", errBadChoice, change.LocalModel)
			}

			next.LocalModel = change.LocalModel
		}

		return next, nil
	}

	if p.EditableURL && change.BaseURL != "" {
		u, err := cleanURL(change.BaseURL)
		if err != nil {
			return c, err
		}

		next.BaseURL[p.ID] = u
	}

	if next.urlFor(p) == "" {
		return c, fmt.Errorf("%w: %s needs an address", errBadChoice, p.Title)
	}

	if m := strings.TrimSpace(change.Model); m != "" {
		if len(m) > 200 {
			return c, fmt.Errorf("%w: the model name is too long", errBadChoice)
		}

		next.Model[p.ID] = m
	}

	if change.Key != nil {
		key := strings.TrimSpace(*change.Key)
		if len(key) > 500 {
			return c, fmt.Errorf("%w: the key is too long", errBadChoice)
		}

		if key == "" {
			delete(next.Keys, p.ID)
		} else {
			next.Keys[p.ID] = key
		}
	}

	return next, nil
}

func clone(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}

	return out
}

// cleanURL takes an http or https address and drops a trailing slash.
func cleanURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("%w: %q is not an http or https address", errBadChoice, raw)
	}

	u.RawQuery, u.Fragment = "", ""

	return strings.TrimSuffix(u.String(), "/"), nil
}
