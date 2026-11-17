package main

// The pet's day: every little while it adds the printing time, gets hungrier
// or is fed, grows, and once a morning writes its roast of the day before.

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// life runs the pet's clock until ctx ends.
func (a *App) life(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(lifeEvery):
		}

		a.live(ctx)
	}
}

// live is one beat of the pet's clock.
func (a *App) live(ctx context.Context) {
	a.mu.Lock()
	alive := a.alive
	a.mu.Unlock()

	if !alive {
		a.restoreVitals(ctx)

		return
	}

	// Until the fleet was read once, nobody knows whether something prints,
	// and a pet that missed a long print would cry hunger by mistake.
	printing, listed := a.watcher.Printing()
	if !listed {
		return
	}

	now := a.now()

	a.mu.Lock()
	changed := a.vitals.tick(now, printing)
	due := !a.roasting && a.vitals.roastDue(now, a.settings.Load().RoastHour)
	if due {
		a.roasting = true
	}
	a.mu.Unlock()

	a.save()

	for _, n := range changed {
		a.announce(n)
	}

	if due {
		go a.roast()
	}
}

// roast writes the morning roast of yesterday, and tries again on the next
// beat when no brain can write it now.
func (a *App) roast() {
	defer func() {
		a.mu.Lock()
		a.roasting = false
		a.mu.Unlock()
	}()

	// The roast waits for a person's question to finish, not the other way.
	a.thinking <- struct{}{}
	defer func() { <-a.thinking }()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	target, provider, ok := a.target(ctx)
	if !ok {
		return
	}

	a.mu.Lock()
	day := a.vitals.Yesterday
	a.mu.Unlock()

	profile := a.Profile(ctx)
	target.MaxTokens, target.Temperature = roastTokens, 0.9
	target.Messages = []Message{
		{Role: roleSystem, Content: system(profile, dayText(day)+"\n"+a.vitalsText())},
		{Role: roleUser, Content: roastPrompt(profile)},
	}

	text, err := a.speak(ctx, target, provider, func(string) {})
	if err != nil {
		logf("no roast for %s: %v", day.Date, err)

		return
	}

	now := a.now()
	roast := &Roast{Date: day.Date, Text: trimRoast(text), At: now}

	a.mu.Lock()
	a.vitals.Roasted, a.vitals.Roast = day.Date, roast
	a.mu.Unlock()

	n := petNotice(now, "roast", severityInfo, day.Date)
	n.Quip = roast.Text
	a.watcher.Add(n)
	a.save()
}

// trimRoast keeps what a small model wrote to a few sentences on one line,
// without quotes around it.
func trimRoast(s string) string {
	runes := []rune(oneLine(strings.Join(strings.Fields(s), " ")))
	if len(runes) > 600 {
		runes = append(runes[:600], '…')
	}

	return string(runes)
}

// dayText is one day of the workshop in plain English, for the roast. Every
// line says what it counts, so a small model does not read a count as hours.
func dayText(d Day) string {
	return fmt.Sprintf(`Yesterday (%s) in the workshop, all printers together:
- %d prints finished well
- %d prints failed with an error
- %d prints were cancelled by a person
- %d times a printer stopped and waited for a person to answer it
- %d new problems appeared on printers
- the printers printed for %.1f hours in total`, d.Date, d.Finished, d.Failed, d.Stopped, d.Attention, d.Issues, d.Seconds/3600)
}

// roastPrompt asks for the morning roast.
func roastPrompt(p Profile) string {
	return fmt.Sprintf(`Write in %s only. It is morning. Write your daily roast of yesterday in the workshop: two or three short sentences, in character, teasing the people a little about the numbers above. Use only those numbers and invent nothing. If nothing happened, tease them about that. No greeting, no quotes, no list.`, languages[p.Language])
}

// vitalsText is the pet's body for the brain, empty until it is known.
func (a *App) vitalsText() string {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.alive {
		return ""
	}

	return "\n" + a.vitals.text(a.now()) + "\n"
}

// vitalsView is the pet's body for the page, nil until it is known.
func (a *App) vitalsView() *VitalsView {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.alive {
		return nil
	}

	view := a.vitals.view(a.now())

	return &view
}

// moodWith lets hunger show on a face that has nothing more urgent to show.
func moodWith(mood Mood, vitals *VitalsView) Mood {
	if vitals == nil {
		return mood
	}

	hungry := vitals.Hunger == hungerHungry || vitals.Hunger == hungerStarving
	if hungry && (mood.Face == "bored" || mood.Face == "sleepy") {
		mood.Face = "hungry"
	}

	return mood
}
