package main

// The pet's mind: which brain it thinks with, and the one call that asks it.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/prusa3d/prusa-connect/sdk/fault"
	"github.com/prusa3d/prusa-connect/sdk/stateapi"
)

const keyBrain = stateapi.Key("brain")

// choice is the brain in use: the person's choice from the store, or the
// operator's settings until there is one.
func (a *App) choice(ctx context.Context) Choice {
	a.mu.Lock()
	held := a.chosen
	a.mu.Unlock()

	if held != nil {
		return *held
	}

	choice := a.settings.Load().initialChoice()

	value, err := a.records.Get(ctx, keyBrain)

	switch {
	case err == nil:
		var stored Choice
		if json.Unmarshal(value.Bytes, &stored) == nil {
			if _, ok := providerByID(stored.Provider); ok {
				if _, ok := brains[stored.LocalModel]; !ok {
					stored.LocalModel = choice.LocalModel
				}

				choice = stored
			}
		}
	case !errors.Is(err, stateapi.ErrNoRecord):
		logf("the brain choice could not be read: %v", err)

		return choice
	}

	a.mu.Lock()
	a.chosen = &choice
	a.mu.Unlock()

	return choice
}

func (a *App) saveChoice(ctx context.Context, c Choice) error {
	raw, err := json.Marshal(c)
	if err != nil {
		return err
	}

	if _, err := a.records.Put(ctx, keyBrain, raw); err != nil {
		return err
	}

	a.mu.Lock()
	a.chosen = &c
	a.mu.Unlock()

	a.follow(c)

	return nil
}

// follow starts the local server when the choice is local and stops it
// otherwise, so a model nobody uses does not hold the memory.
func (a *App) follow(c Choice) {
	if p, _ := providerByID(c.Provider); p.Kind == kindLocal {
		a.brain.Use(c.LocalModel)
	} else {
		a.brain.Use("")
	}
}

// target is where a question goes right now, and false when that brain cannot
// answer yet.
func (a *App) target(ctx context.Context) (completion, Provider, bool) {
	c := a.choice(ctx)
	p, _ := providerByID(c.Provider)

	if p.Kind == kindLocal {
		url, ok := a.brain.Endpoint()

		return completion{BaseURL: url, Model: c.LocalModel}, p, ok
	}

	out := completion{BaseURL: c.urlFor(p), Model: c.Model[p.ID], Key: c.Keys[p.ID]}
	if p.Kind == kindAnthropic && out.Model == "" {
		out.Model = defaultClaudeModel
	}

	ready := out.BaseURL != "" && out.Model != "" && (!p.NeedsKey || out.Key != "")

	return out, p, ready
}

// speak asks the brain in use and streams its answer.
func (a *App) speak(ctx context.Context, c completion, p Provider, onPiece func(string)) (string, error) {
	if p.Kind == kindAnthropic {
		return streamClaude(ctx, c, onPiece)
	}

	return stream(ctx, a.client, c, onPiece)
}

// MindView is what GET /brain answers.
type MindView struct {
	Choice    ChoiceView  `json:"choice"`
	Providers []Provider  `json:"providers"`
	Models    []brain     `json:"models"`
	Local     LocalStatus `json:"local"`
	// Ready says the chosen brain can answer now.
	Ready bool `json:"ready"`
	// Editable says a person may change the brain on the page.
	Editable bool `json:"editable"`
}

// Summary is the brain in one line, for the pet's header.
type Summary struct {
	Provider   string `json:"provider"`
	Title      string `json:"title"`
	Model      string `json:"model"`
	OnPremises bool   `json:"on_premises"`
	Ready      bool   `json:"ready"`
	// State is the local server's state when the provider is local.
	State string `json:"state,omitempty"`
}

func (a *App) mind(ctx context.Context) MindView {
	_, _, ready := a.target(ctx)

	return MindView{
		Choice: a.choice(ctx).view(), Providers: providers, Models: brainList(),
		Local: a.brain.Status(), Ready: ready, Editable: a.settings.Load().PageSetup,
	}
}

func (a *App) summary(ctx context.Context) Summary {
	c, p, ready := a.target(ctx)
	out := Summary{Provider: p.ID, Title: p.Title, Model: c.Model, OnPremises: p.OnPremises, Ready: ready}

	if p.Kind == kindLocal {
		out.Model = brains[c.Model].Title
		out.State = a.brain.Status().State
	}

	return out
}

// Probe is what POST /brain/probe takes: a provider to try before choosing it.
type Probe struct {
	Provider string  `json:"provider"`
	BaseURL  string  `json:"base_url,omitempty"`
	Key      *string `json:"key,omitempty"`
}

// ProbeResult is what the provider answered.
type ProbeResult struct {
	OK     bool     `json:"ok"`
	Models []string `json:"models"`
	Error  string   `json:"error,omitempty"`
}

func (a *App) probe(ctx context.Context, in Probe) (ProbeResult, error) {
	p, ok := providerByID(in.Provider)
	if !ok || p.Kind == kindLocal {
		return ProbeResult{}, errBadChoice
	}

	stored := a.choice(ctx)
	c := completion{BaseURL: stored.urlFor(p), Key: stored.Keys[p.ID]}

	if p.EditableURL && in.BaseURL != "" {
		u, err := cleanURL(in.BaseURL)
		if err != nil {
			return ProbeResult{}, err
		}

		c.BaseURL = u
	}

	if in.Key != nil {
		c.Key = *in.Key
	}

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	var (
		models []string
		err    error
	)

	if p.Kind == kindAnthropic {
		models, err = claudeModels(ctx, c)
	} else {
		models, err = openAIModels(ctx, a.client, c)
	}

	if err != nil {
		return ProbeResult{Error: err.Error(), Models: []string{}}, nil
	}

	return ProbeResult{OK: true, Models: models}, nil
}

// mindRoutes are the routes of the brain page.
func (a *App) mindRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /brain", func(w http.ResponseWriter, r *http.Request) {
		answer(w, http.StatusOK, a.mind(r.Context()))
	})

	mux.HandleFunc("PUT /brain", func(w http.ResponseWriter, r *http.Request) {
		if !a.settings.Load().PageSetup {
			refuse(w, r, BrainLocked.New("page_setup is off, so only the operator changes the brain"))

			return
		}

		var change ChoiceChange
		if err := readJSON(r.Body, &change); err != nil {
			refuse(w, r, InvalidBody.New(err.Error()))

			return
		}

		next, err := a.choice(r.Context()).apply(change)
		if err != nil {
			refuse(w, r, InvalidBody.New(err.Error()))

			return
		}

		if err := a.saveChoice(r.Context(), next); err != nil {
			refuse(w, r, StoreUnavailable.New(err.Error()))

			return
		}

		answer(w, http.StatusOK, a.mind(r.Context()))
	})

	mux.HandleFunc("POST /brain/probe", func(w http.ResponseWriter, r *http.Request) {
		var in Probe
		if err := readJSON(r.Body, &in); err != nil {
			refuse(w, r, InvalidBody.New(err.Error()))

			return
		}

		result, err := a.probe(r.Context(), in)
		if err != nil {
			refuse(w, r, InvalidBody.New(err.Error()))

			return
		}

		answer(w, http.StatusOK, result)
	})

	mux.HandleFunc("POST /brain/models/{model}/install", func(w http.ResponseWriter, r *http.Request) {
		if err := a.brain.Install(r.PathValue("model")); err != nil {
			refuse(w, r, faultOf(err))

			return
		}

		answer(w, http.StatusAccepted, a.mind(r.Context()))
	})

	mux.HandleFunc("DELETE /brain/models/{model}", func(w http.ResponseWriter, r *http.Request) {
		if err := a.brain.Remove(r.PathValue("model")); err != nil {
			refuse(w, r, faultOf(err).With(fault.StringParam("model", r.PathValue("model"))))

			return
		}

		answer(w, http.StatusOK, a.mind(r.Context()))
	})
}
