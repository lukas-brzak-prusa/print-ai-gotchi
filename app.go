package main

// The pet as a whole: its profile, its eyes on the fleet, its brain, and the
// routes the page talks to.
//
// A brain on a Raspberry Pi takes tens of seconds to answer, longer than the
// host lets one request take. So a chat is two calls: POST /chat starts an
// answer and returns its id at once, and GET /chat/{id} reads it while the
// brain writes it, which is also what lets the page show the pet typing.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prusa3d/prusa-connect/sdk/deviceapi"
	"github.com/prusa3d/prusa-connect/sdk/fault"
	"github.com/prusa3d/prusa-connect/sdk/routesapi"
	"github.com/prusa3d/prusa-connect/sdk/stateapi"
)

const (
	keyProfile = stateapi.Key("profile")
	keyNotices = stateapi.Key("notices")
	keyVitals  = stateapi.Key("vitals")
)

// Bounds on what a person sends.
const (
	maxTurns       = 10
	maxTurnRunes   = 2000
	answerTokens   = 320
	quipTokens     = 60
	roastTokens    = 180
	lifeEvery      = 30 * time.Second
	answerLifetime = 15 * time.Minute
)

// App is everything the module holds.
type App struct {
	settings atomic.Pointer[Settings]
	records  stateapi.Records
	devices  Devices
	watcher  *Watcher
	brain    *Brain
	client   *http.Client

	// thinking lets one answer run at a time: a small machine has one brain.
	thinking chan struct{}

	mu      sync.Mutex
	profile *Profile
	chosen  *Choice
	started bool
	replies map[string]*Reply
	vitals  Vitals
	// alive is set once the vitals were read from the store, or the store
	// said there were none. Until then the pet neither grows nor gets hungry,
	// so a slow store does not hatch it again.
	alive    bool
	roasting bool
	now      func() time.Time

	saving chan struct{}
}

// Reply is one answer the brain is writing or has written.
type Reply struct {
	ID string `json:"id"`
	// State is queued, writing, done or failed.
	State string    `json:"state"`
	Text  string    `json:"text"`
	Error string    `json:"error,omitempty"`
	At    time.Time `json:"at"`
}

func newApp(records stateapi.Records, devices Devices, watcher *Watcher, brain *Brain) *App {
	app := &App{
		records: records, devices: devices, watcher: watcher, brain: brain,
		client:   &http.Client{Timeout: 5 * time.Minute},
		thinking: make(chan struct{}, 1),
		replies:  map[string]*Reply{},
		saving:   make(chan struct{}, 1),
		now:      time.Now,
	}

	s := defaults()
	app.settings.Store(&s)
	watcher.onNotice = app.noticed

	return app
}

func logf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, name+": "+format+"\n", args...)
}

// Start restores what the store kept and runs the eyes and the saver until
// ctx ends.
func (a *App) Start(ctx context.Context) {
	a.restore(ctx)

	a.mu.Lock()
	a.started = true
	a.mu.Unlock()

	// The store answers by now, so the person's choice of brain is read and
	// the local server started only when it is the one chosen.
	a.follow(a.choice(ctx))

	go a.watcher.Run(ctx)
	go a.saver(ctx)
	go a.life(ctx)
}

// restore reads the notices an earlier run kept. The store may not answer in
// the first moment after a start, so it is asked a few times.
func (a *App) restore(ctx context.Context) {
	a.restoreKey(ctx, keyNotices, func(raw []byte, found bool) {
		var kept []Notice
		if !found {
			return
		}

		if err := json.Unmarshal(raw, &kept); err != nil {
			logf("the kept notices cannot be read: %v", err)

			return
		}

		a.watcher.Restore(kept)
	})

	a.restoreVitals(ctx)
}

// restoreVitals reads the pet's body, or hatches it when the store has none.
func (a *App) restoreVitals(ctx context.Context) {
	a.restoreKey(ctx, keyVitals, func(raw []byte, found bool) {
		vitals := newVitals(a.now())
		if found {
			if err := json.Unmarshal(raw, &vitals); err != nil {
				logf("the pet's vitals cannot be read, it hatches again: %v", err)

				vitals = newVitals(a.now())
			}
		}

		a.mu.Lock()
		a.vitals, a.alive = vitals, true
		a.mu.Unlock()

		if !found {
			a.announce(petNotice(vitals.Born, "hatched", severityInfo, ""))
		}
	})
}

// restoreKey reads one record and hands it to use, with found false when the
// store has none. It gives up after a few attempts and calls nothing then.
func (a *App) restoreKey(ctx context.Context, key stateapi.Key, use func(raw []byte, found bool)) {
	for attempt := range 5 {
		value, err := a.records.Get(ctx, key)

		switch {
		case err == nil:
			use(value.Bytes, true)

			return
		case errors.Is(err, stateapi.ErrNoRecord):
			use(nil, false)

			return
		}

		logf("the kept %s could not be read (attempt %d): %v", key, attempt+1, err)

		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Duration(attempt+1) * time.Second):
		}
	}
}

// Configure takes the operator's settings and the data directory.
func (a *App) Configure(s Settings, dataDir string) error {
	s, err := s.normalise()
	if err != nil {
		return err
	}

	a.settings.Store(&s)
	a.brain.Configure(s, dataDir)

	a.mu.Lock()
	a.chosen = nil // the operator's defaults may have changed
	started := a.started
	a.mu.Unlock()

	if started {
		go a.follow(a.choice(context.Background()))
	}

	return nil
}

// Profile is the pet, read once from the store and held after that.
func (a *App) Profile(ctx context.Context) Profile {
	a.mu.Lock()
	held := a.profile
	a.mu.Unlock()

	if held != nil {
		return *held
	}

	profile := defaultProfile()

	value, err := a.records.Get(ctx, keyProfile)

	switch {
	case err == nil:
		var stored Profile
		if json.Unmarshal(value.Bytes, &stored) == nil {
			if cleaned, err := stored.clean(); err == nil {
				profile = cleaned
			}
		}
	case !errors.Is(err, stateapi.ErrNoRecord):
		// The store did not answer. Use the default this once and ask again
		// next time, rather than forgetting the pet the person chose.
		logf("the profile could not be read: %v", err)

		return profile
	}

	a.mu.Lock()
	a.profile = &profile
	a.mu.Unlock()

	return profile
}

func (a *App) saveProfile(ctx context.Context, p Profile) error {
	raw, err := json.Marshal(p)
	if err != nil {
		return err
	}

	if _, err := a.records.Put(ctx, keyProfile, raw); err != nil {
		return err
	}

	a.mu.Lock()
	a.profile = &p
	a.mu.Unlock()

	return nil
}

// noticed is called for every new notice. It asks the brain for a comment
// when the brain is free, and never waits for it.
func (a *App) noticed(n Notice) {
	a.mu.Lock()
	var follow []Notice
	if a.alive {
		follow = a.vitals.record(n)
	}
	a.mu.Unlock()

	a.save()
	a.comment(n)

	for _, about := range follow {
		a.announce(about)
	}
}

// announce keeps a notice about the pet itself and lets the pet comment.
func (a *App) announce(n Notice) {
	n = a.watcher.Add(n)
	a.save()
	a.comment(n)
}

// comment asks the brain for a one-line comment on n when the brain is free,
// and never waits for it.
func (a *App) comment(n Notice) {
	if !a.settings.Load().Quips {
		return
	}

	go func() {
		select {
		case a.thinking <- struct{}{}:
		default:
			return // the brain is busy with a person, who matters more
		}
		defer func() { <-a.thinking }()

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		target, provider, ok := a.target(ctx)
		if !ok {
			return
		}

		profile := a.Profile(ctx)
		target.MaxTokens, target.Temperature = quipTokens, 0.9
		target.Messages = []Message{
			{Role: roleSystem, Content: system(profile, "(only the event below)\n"+a.vitalsText())},
			{Role: roleUser, Content: quipPrompt(n, profile)},
		}

		quip, err := a.speak(ctx, target, provider, func(string) {})
		if err != nil {
			logf("no quip for %s: %v", n.ID, err)

			return
		}

		a.watcher.SetQuip(n.ID, oneLine(quip))
		a.save()
	}()
}

// oneLine keeps the first line of what a small model wrote, without quotes.
func oneLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i > 0 {
		s = s[:i]
	}

	return strings.Trim(s, `"„“ `)
}

// save asks the saver to write the notices. Bursts collapse into one write.
func (a *App) save() {
	select {
	case a.saving <- struct{}{}:
	default:
	}
}

func (a *App) saver(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-a.saving:
		}

		a.mu.Lock()
		alive, vitals := a.alive, a.vitals
		a.mu.Unlock()

		if alive {
			raw, _ := json.Marshal(vitals)
			if _, err := a.records.Put(ctx, keyVitals, raw); err != nil {
				logf("the pet's vitals could not be kept: %v", err)
			}
		}

		kept := a.watcher.Kept()

		for {
			raw, _ := json.Marshal(kept)
			if len(raw) < 60<<10 || len(kept) == 0 {
				if _, err := a.records.Put(ctx, keyNotices, raw); err != nil {
					logf("the notices could not be kept: %v", err)
				}

				break
			}

			kept = kept[len(kept)/4:]
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
}

// visible is the printers the person behind r may see, read from the host as
// that person, so the pet never tells anybody about a printer they cannot see.
func (a *App) visible(r *http.Request) ([]deviceapi.Device, map[string]bool, error) {
	var printers []deviceapi.Device

	principal := routesapi.PrincipalOf(r)

	for offset := 0; ; {
		page, err := a.devices.List(r.Context(), deviceapi.ListParams{Principal: principal, Limit: 200, Offset: offset})
		if err != nil {
			return nil, nil, err
		}

		printers = append(printers, page.Items...)
		offset += len(page.Items)

		if len(page.Items) == 0 || offset >= page.Total {
			break
		}
	}

	ids := make(map[string]bool, len(printers))
	for _, d := range printers {
		ids[d.ID] = true
	}

	return printers, ids, nil
}

// Ask starts one answer and returns at once.
func (a *App) Ask(r *http.Request, turns []Message) (*Reply, error) {
	if _, _, ok := a.target(r.Context()); !ok {
		return nil, errBrainAsleep
	}

	printers, ids, err := a.visible(r)
	if err != nil {
		return nil, err
	}

	profile := a.Profile(r.Context())
	messages := append([]Message{{
		Role:    roleSystem,
		Content: system(profile, Picture(printers, a.watcher.Notices(ids), time.Now())+a.vitalsText()),
	}}, turns...)

	reply := &Reply{ID: newID(), State: "queued", At: time.Now()}

	a.mu.Lock()
	a.prune()
	a.replies[reply.ID] = reply
	a.mu.Unlock()

	go a.answer(reply, messages)

	return a.copyOf(reply.ID), nil
}

func (a *App) answer(reply *Reply, messages []Message) {
	a.thinking <- struct{}{}
	defer func() { <-a.thinking }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	target, provider, ok := a.target(ctx)
	if !ok {
		a.finish(reply, errBrainAsleep)

		return
	}

	a.mu.Lock()
	reply.State = "writing"
	a.mu.Unlock()

	target.MaxTokens, target.Temperature, target.Messages = answerTokens, 0.5, messages

	_, err := a.speak(ctx, target, provider, func(piece string) {
		a.mu.Lock()
		reply.Text += piece
		a.mu.Unlock()
	})

	a.finish(reply, err)
}

func (a *App) finish(reply *Reply, err error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	reply.State = "done"
	reply.Text = strings.TrimSpace(reply.Text)

	if err != nil {
		reply.State, reply.Error = "failed", err.Error()
	}
}

func (a *App) copyOf(id string) *Reply {
	a.mu.Lock()
	defer a.mu.Unlock()

	held, ok := a.replies[id]
	if !ok {
		return nil
	}

	out := *held

	return &out
}

// prune drops answers nobody read for a while. The caller holds mu.
func (a *App) prune() {
	for id, reply := range a.replies {
		if time.Since(reply.At) > answerLifetime {
			delete(a.replies, id)
		}
	}
}

func newID() string {
	raw := make([]byte, 9)
	_, _ = rand.Read(raw)

	return hex.EncodeToString(raw)
}

// cleanTurns keeps the last turns a person sent, each a user or an assistant
// turn of bounded length, ending with the person's question.
func cleanTurns(turns []Message) ([]Message, error) {
	if len(turns) == 0 || turns[len(turns)-1].Role != roleUser || strings.TrimSpace(turns[len(turns)-1].Content) == "" {
		return nil, errors.New("the conversation must end with a question")
	}

	if len(turns) > maxTurns {
		turns = turns[len(turns)-maxTurns:]
	}

	out := make([]Message, 0, len(turns))

	for _, turn := range turns {
		if turn.Role != roleUser && turn.Role != roleAssistant {
			return nil, fmt.Errorf("a turn is %q, and it is user or assistant", turn.Role)
		}

		if r := []rune(turn.Content); len(r) > maxTurnRunes {
			turn.Content = string(r[:maxTurnRunes])
		}

		out = append(out, turn)
	}

	return out, nil
}

// PetView is what GET /pet answers.
type PetView struct {
	Profile Profile  `json:"profile"`
	Mood    Mood     `json:"mood"`
	Notices []Notice `json:"notices"`
	Brain   Summary  `json:"brain"`
	Choices Choices  `json:"choices"`
	// Vitals is absent until the pet's body was read from the store.
	Vitals *VitalsView `json:"vitals,omitempty"`
}

// Choices is what the profile form offers.
type Choices struct {
	Personalities []string `json:"personalities"`
	Languages     []string `json:"languages"`
	Species       []string `json:"species"`
}

func choices() Choices {
	out := Choices{Species: species}
	for _, key := range []string{"grumpy", "intern", "zen", "commentator", "cat"} {
		out.Personalities = append(out.Personalities, key)
	}

	for _, key := range []string{"en", "cs", "sk", "de", "pl", "fr", "es", "it"} {
		out.Languages = append(out.Languages, key)
	}

	return out
}

// routes is the surface this module serves. The paths carry no prefix.
func (a *App) routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /pet", func(w http.ResponseWriter, r *http.Request) {
		_, ids, err := a.visible(r)
		if err != nil {
			refuse(w, r, FleetUnavailable.New(err.Error()))

			return
		}

		vitals := a.vitalsView()

		answer(w, http.StatusOK, PetView{
			Profile: a.Profile(r.Context()),
			Mood:    moodWith(a.watcher.Mood(ids), vitals),
			Notices: a.watcher.Notices(ids),
			Brain:   a.summary(r.Context()),
			Choices: choices(),
			Vitals:  vitals,
		})
	})

	mux.HandleFunc("PUT /profile", func(w http.ResponseWriter, r *http.Request) {
		var p Profile
		if err := readJSON(r.Body, &p); err != nil {
			refuse(w, r, InvalidBody.New(err.Error()))

			return
		}

		p, err := p.clean()
		if err != nil {
			refuse(w, r, InvalidBody.New(err.Error()))

			return
		}

		if err := a.saveProfile(r.Context(), p); err != nil {
			refuse(w, r, StoreUnavailable.New(err.Error()))

			return
		}

		answer(w, http.StatusOK, p)
	})

	a.mindRoutes(mux)

	mux.HandleFunc("POST /chat", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []Message `json:"messages"`
		}

		if err := readJSON(r.Body, &body); err != nil {
			refuse(w, r, InvalidBody.New(err.Error()))

			return
		}

		turns, err := cleanTurns(body.Messages)
		if err != nil {
			refuse(w, r, InvalidBody.New(err.Error()))

			return
		}

		reply, err := a.Ask(r, turns)
		if err != nil {
			refuse(w, r, faultOf(err))

			return
		}

		answer(w, http.StatusAccepted, reply)
	})

	mux.HandleFunc("GET /chat/{id}", func(w http.ResponseWriter, r *http.Request) {
		reply := a.copyOf(r.PathValue("id"))
		if reply == nil {
			refuse(w, r, ReplyNotFound.New("no answer has the id "+r.PathValue("id")).
				With(fault.StringParam("id", r.PathValue("id"))))

			return
		}

		answer(w, http.StatusOK, reply)
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		refuse(w, r, RouteNotFound.
			New("this module serves no route at "+r.URL.Path).
			With(fault.StringParam("path", r.URL.Path)))
	})

	return mux
}

func readJSON(body io.Reader, into any) error {
	decoder := json.NewDecoder(io.LimitReader(body, 256<<10))
	decoder.DisallowUnknownFields()

	return decoder.Decode(into)
}

// answer writes one JSON body.
func answer(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)

	_ = json.NewEncoder(w).Encode(body)
}
