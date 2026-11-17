package main

import (
	"fmt"
	"strings"
)

// Settings is what an operator writes under this instance's config block. The
// host merges it over the defaults module.yaml declares and sends the result on
// plugin.configure. A key this struct does not carry refuses the start.
//
// The brain settings are where the pet starts. A person with operate rights
// on the module chooses another brain on the page, unless page_setup is off.
type Settings struct {
	// Provider is where the pet thinks at first, an id from providers.go.
	Provider string `json:"provider"`
	// Model is the local model the pet starts with, a key of brains.
	Model string `json:"model"`
	// BaseURL, APIKey and RemoteModel preset the remote provider.
	BaseURL     string `json:"base_url"`
	APIKey      string `json:"api_key"`
	RemoteModel string `json:"remote_model"`
	// LlamaServer is the absolute path of a llama-server already on this
	// machine. Empty downloads the pinned build for this platform.
	LlamaServer string `json:"llama_server"`
	// Threads is how many CPU threads the local brain uses. 0 lets it choose.
	Threads int `json:"threads"`
	// Quips turns on the pet's own comment on each new notice.
	Quips bool `json:"quips"`
	// PageSetup lets a person change the brain on the page.
	PageSetup bool `json:"page_setup"`
	// RoastHour is the hour of the server's day from which the pet writes its
	// morning roast of the day before. -1 turns the roast off.
	RoastHour int `json:"roast_hour"`
}

// The defaults module.yaml publishes. TestTheManifestPublishesTheCodesDefaults
// holds the two to each other.
const (
	defaultProvider = "local"
	defaultModel    = "gemma3-4b"
	defaultRoast    = 8
)

// defaults is what the module runs on before the host sends anything.
func defaults() Settings {
	return Settings{Provider: defaultProvider, Model: defaultModel, Quips: true, PageSetup: true, RoastHour: defaultRoast}
}

// normalise fills what the operator left empty and refuses what cannot work,
// so a typo stops the start instead of the first chat.
func (s Settings) normalise() (Settings, error) {
	if s.Provider == "" {
		s.Provider = defaultProvider
	}

	if s.Model == "" {
		s.Model = defaultModel
	}

	if _, ok := providerByID(s.Provider); !ok {
		ids := make([]string, 0, len(providers))
		for _, p := range providers {
			ids = append(ids, p.ID)
		}

		return s, fmt.Errorf("provider is %q, and it is one of %s", s.Provider, strings.Join(ids, ", "))
	}

	if _, ok := brains[s.Model]; !ok {
		return s, fmt.Errorf("model is %q, and it is one of %s", s.Model, strings.Join(brainKeys(), ", "))
	}

	if s.BaseURL != "" {
		u, err := cleanURL(s.BaseURL)
		if err != nil {
			return s, fmt.Errorf("base_url: %w", err)
		}

		s.BaseURL = u
	}

	if s.RoastHour < -1 || s.RoastHour > 23 {
		return s, fmt.Errorf("roast_hour is %d, and it is an hour from 0 to 23, or -1 for no roast", s.RoastHour)
	}

	if s.Threads < 0 {
		return s, fmt.Errorf("threads is %d, and it is 0 or more", s.Threads)
	}

	return s, nil
}

// initialChoice is the brain the operator's settings describe, used until a
// person chooses another on the page.
func (s Settings) initialChoice() Choice {
	c := Choice{
		Provider: s.Provider, LocalModel: s.Model,
		BaseURL: map[string]string{}, Model: map[string]string{}, Keys: map[string]string{},
	}

	if s.BaseURL != "" {
		c.BaseURL[s.Provider] = s.BaseURL
	}

	if s.RemoteModel != "" {
		c.Model[s.Provider] = s.RemoteModel
	}

	if s.APIKey != "" {
		c.Keys[s.Provider] = s.APIKey
	}

	return c
}
