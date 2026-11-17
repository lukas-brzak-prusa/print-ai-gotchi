package main

import (
	"strings"
	"testing"
	"time"
)

func said(notices []Notice) string {
	out := make([]string, 0, len(notices))
	for _, n := range notices {
		out = append(out, n.Kind+":"+n.Detail)
	}

	return strings.Join(out, ",")
}

// The pet gets hungrier while nothing prints, says so once per level, and
// thanks whoever feeds it.
func TestThePetGetsHungryAndIsFed(t *testing.T) {
	born := time.Date(2026, 11, 17, 9, 0, 0, 0, time.Local)
	v := newVitals(born)

	steps := []struct {
		after    time.Duration
		printing int
		want     string
		hunger   string
	}{
		{11 * time.Hour, 0, "", hungerFull},
		{13 * time.Hour, 0, "", hungerPeckish}, // peckish is not worth a notice
		{25 * time.Hour, 0, "hungry:25", hungerHungry},
		{26 * time.Hour, 0, "", hungerHungry},
		{49 * time.Hour, 0, "starving:49", hungerStarving},
		{50 * time.Hour, 1, "fed:", hungerFull},
		{51 * time.Hour, 0, "", hungerFull},
	}

	for _, step := range steps {
		now := born.Add(step.after)
		if got := said(v.tick(now, step.printing)); got != step.want {
			t.Errorf("after %s the pet said %q, want %q", step.after, got, step.want)
		}

		if got := v.hunger(now); got != step.hunger {
			t.Errorf("after %s the pet is %s, want %s", step.after, got, step.hunger)
		}
	}
}

// A finished print feeds the pet; a failed one does not.
func TestAFinishedPrintFeedsThePet(t *testing.T) {
	born := time.Date(2026, 11, 17, 9, 0, 0, 0, time.Local)
	v := newVitals(born)
	v.tick(born.Add(30*time.Hour), 0)

	failed := born.Add(31 * time.Hour)
	if got := said(v.record(Notice{At: failed, Kind: "failed"})); got != "" || v.hunger(failed) != hungerHungry {
		t.Fatalf("a failed print fed the pet: %q, %s", got, v.hunger(failed))
	}

	done := born.Add(32 * time.Hour)
	if got := said(v.record(Notice{At: done, Kind: "finished"})); got != "fed:" {
		t.Fatalf("a finished print said %q", got)
	}

	if v.Finished != 1 || v.Failed != 1 || v.hunger(done) != hungerFull {
		t.Fatalf("the pet counts %d finished and %d failed, and is %s", v.Finished, v.Failed, v.hunger(done))
	}
}

// Printing time grows the pet, one stage at a time, and never back. A gap in
// the ticks does not count as printing.
func TestThePetGrowsWithPrintingHours(t *testing.T) {
	now := time.Date(2026, 11, 17, 9, 0, 0, 0, time.Local)
	v := newVitals(now)

	var heard []string

	// Five printers print for two hours, a tick a minute: ten printer-hours.
	for range 120 {
		now = now.Add(time.Minute)
		heard = append(heard, said(v.tick(now, 5)))
	}

	if v.Stage != "kid" || !strings.Contains(strings.Join(heard, ","), "evolved:kid") {
		t.Fatalf("after %.1f hours the pet is a %s and said %v", v.hours(), v.Stage, heard)
	}

	before := v.Seconds
	v.tick(now.Add(10*time.Hour), 1)

	if added := v.Seconds - before; added > maxAccrual.Seconds() {
		t.Fatalf("a ten-hour gap added %.0f seconds of printing", added)
	}

	v.Seconds = 260 * 3600
	if got := said(v.tick(now.Add(10*time.Hour+time.Minute), 0)); got != "evolved:adult,evolved:legend" {
		t.Fatalf("at 260 hours the pet said %q", got)
	}

	view := v.view(now.Add(10*time.Hour + time.Minute))
	if view.Stage != "legend" || view.Next != "" || view.Growth != 100 {
		t.Fatalf("a legend shows %+v", view)
	}
}

// The day rolls over at midnight, and the roast is due once a morning, for
// yesterday only.
func TestTheRoastIsDueOnceAMorning(t *testing.T) {
	evening := time.Date(2026, 11, 17, 20, 0, 0, 0, time.Local)
	v := newVitals(evening)
	v.record(Notice{At: evening, Kind: "failed"})
	v.record(Notice{At: evening, Kind: "attention"})

	early := time.Date(2026, 11, 18, 7, 0, 0, 0, time.Local)
	if v.roastDue(early, 8) {
		t.Fatal("the roast is due before its hour")
	}

	morning := time.Date(2026, 11, 18, 8, 5, 0, 0, time.Local)
	if !v.roastDue(morning, 8) {
		t.Fatal("the roast is not due in the morning")
	}

	if v.Yesterday.Date != "2026-11-17" || v.Yesterday.Failed != 1 || v.Yesterday.Attention != 1 || v.Today.Failed != 0 {
		t.Fatalf("the days are %+v and %+v", v.Yesterday, v.Today)
	}

	v.Roasted = v.Yesterday.Date
	if v.roastDue(morning.Add(time.Hour), 8) {
		t.Fatal("the roast is due twice")
	}

	if v.roastDue(time.Date(2026, 11, 19, 9, 0, 0, 0, time.Local), -1) {
		t.Fatal("a roast turned off is due")
	}

	// A week off: yesterday is an empty day, not the last day seen.
	v.rollDay(time.Date(2026, 11, 26, 9, 0, 0, 0, time.Local))
	if v.Yesterday.Date != "2026-11-25" || v.Yesterday.Failed != 0 {
		t.Fatalf("after a week off yesterday is %+v", v.Yesterday)
	}
}

// Hunger shows on the face only when nothing more urgent does.
func TestHungerShowsOnAQuietFace(t *testing.T) {
	hungry := &VitalsView{Hunger: hungerStarving}

	for face, want := range map[string]string{"bored": "hungry", "sleepy": "hungry", "alarmed": "alarmed", "busy": "busy"} {
		if got := moodWith(Mood{Face: face}, hungry).Face; got != want {
			t.Errorf("a starving pet that is %s shows %s, want %s", face, got, want)
		}
	}

	if got := moodWith(Mood{Face: "bored"}, nil).Face; got != "bored" {
		t.Errorf("a pet with no vitals yet shows %s", got)
	}
}

// What the brain reads about the pet, and the morning's numbers.
func TestTheBrainReadsThePetsBody(t *testing.T) {
	born := time.Date(2026, 11, 17, 9, 0, 0, 0, time.Local)
	v := newVitals(born)
	v.Seconds = 12 * 3600
	v.grow(born)

	text := v.text(born.Add(30 * time.Hour))
	for _, want := range []string{"young pet", "12 printing hours", "hungry", "30 hours"} {
		if !strings.Contains(text, want) {
			t.Errorf("the brain reads %q, without %q", text, want)
		}
	}

	if got := dayText(Day{Date: "2026-11-17", Failed: 3}); !strings.Contains(got, "3 prints failed with an error") {
		t.Errorf("the day reads %q", got)
	}

	if got := trimRoast("  \"Three failed.\n\nImpressive.\"  "); got != "Three failed. Impressive." {
		t.Errorf("the roast is kept as %q", got)
	}
}
