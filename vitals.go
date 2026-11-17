package main

// The pet's body: it gets hungry when nothing prints, it grows with every hour
// the printers print, and every morning it remembers the day before. All of it
// is worked out here from the clock and from what the eyes saw, without a
// brain; the brain only puts words to it.

import (
	"fmt"
	"strconv"
	"time"
)

// Hunger levels, from fed to starving.
const (
	hungerFull     = "full"
	hungerPeckish  = "peckish"
	hungerHungry   = "hungry"
	hungerStarving = "starving"
)

// How long after the last print each hunger level begins.
const (
	peckishAfter  = 12 * time.Hour
	hungryAfter   = 24 * time.Hour
	starvingAfter = 48 * time.Hour
)

// growth is each stage and the printing hours, across the whole fleet, at
// which the pet reaches it. A stage once reached is kept.
var growth = []struct {
	Stage string
	Hours float64
}{
	{"baby", 0},
	{"kid", 10},
	{"adult", 50},
	{"legend", 250},
}

// maxAccrual bounds the printing time one tick may add, so a module that was
// stopped for a day does not count the whole gap as printing.
const maxAccrual = 2 * time.Minute

// Vitals is the pet's body, kept in the store.
type Vitals struct {
	Born time.Time `json:"born"`
	// Fed is the last moment a printer printed or finished a print.
	Fed time.Time `json:"fed"`
	// Seconds is the printing time of the whole fleet, one printer printing
	// for an hour counting 3600.
	Seconds  float64 `json:"seconds"`
	Finished int     `json:"finished"`
	Failed   int     `json:"failed"`
	// Stage is the highest stage reached.
	Stage string `json:"stage"`
	// Announced is the hunger level the pet last told people about.
	Announced string `json:"announced"`
	// Ticked is when printing time was last added.
	Ticked time.Time `json:"ticked"`
	// Today and Yesterday count what happened on each day, in the server's
	// time zone.
	Today     Day `json:"today"`
	Yesterday Day `json:"yesterday"`
	// Roasted is the day whose morning roast was written, as 2006-01-02.
	Roasted string `json:"roasted"`
	Roast   *Roast `json:"roast,omitempty"`
}

// Day counts one day of the workshop. It names no printer, so a person who
// may see only some printers learns nothing about the others from it.
type Day struct {
	Date      string  `json:"date"`
	Finished  int     `json:"finished"`
	Failed    int     `json:"failed"`
	Stopped   int     `json:"stopped"`
	Attention int     `json:"attention"`
	Issues    int     `json:"issues"`
	Seconds   float64 `json:"seconds"`
}

// Roast is the pet's morning take on the day before.
type Roast struct {
	Date string    `json:"date"`
	Text string    `json:"text"`
	At   time.Time `json:"at"`
}

// VitalsView is what GET /pet tells about the pet's body.
type VitalsView struct {
	Stage  string `json:"stage"`
	Hunger string `json:"hunger"`
	// Fullness runs from 100 just after a print to 0 when starving begins.
	Fullness int       `json:"fullness"`
	Hours    float64   `json:"hours"`
	Fed      time.Time `json:"fed"`
	Born     time.Time `json:"born"`
	// Next is the stage after this one and Growth the share of the way to
	// it, 0 to 100. Next is empty for the last stage.
	Next   string `json:"next,omitempty"`
	Growth int    `json:"growth"`
	Roast  *Roast `json:"roast,omitempty"`
}

func newVitals(now time.Time) Vitals {
	return Vitals{Born: now, Fed: now, Stage: growth[0].Stage, Announced: hungerFull, Ticked: now, Today: Day{Date: dateOf(now)}}
}

func dateOf(t time.Time) string { return t.Local().Format(time.DateOnly) }

func (v *Vitals) hours() float64 { return v.Seconds / 3600 }

// hunger is the hunger level at now.
func (v *Vitals) hunger(now time.Time) string {
	switch since := now.Sub(v.Fed); {
	case since >= starvingAfter:
		return hungerStarving
	case since >= hungryAfter:
		return hungerHungry
	case since >= peckishAfter:
		return hungerPeckish
	}

	return hungerFull
}

func hungerRank(level string) int {
	switch level {
	case hungerPeckish:
		return 1
	case hungerHungry:
		return 2
	case hungerStarving:
		return 3
	}

	return 0
}

func stageIndex(stage string) int {
	for i, step := range growth {
		if step.Stage == stage {
			return i
		}
	}

	return 0
}

// rollDay starts a new day when the date changed. Yesterday is the day before
// today only, so a module that was off for a week does not roast last week.
func (v *Vitals) rollDay(now time.Time) {
	date := dateOf(now)
	if v.Today.Date == date {
		return
	}

	yesterday := dateOf(now.AddDate(0, 0, -1))
	if v.Today.Date == yesterday {
		v.Yesterday = v.Today
	} else {
		v.Yesterday = Day{Date: yesterday}
	}

	v.Today = Day{Date: date}
}

// tick adds the printing time since the last tick, feeds the pet while
// something prints, and returns what changed that is worth a notice.
func (v *Vitals) tick(now time.Time, printing int) []Notice {
	v.rollDay(now)

	elapsed := now.Sub(v.Ticked)
	v.Ticked = now

	if elapsed > 0 && printing > 0 {
		seconds := min(elapsed, maxAccrual).Seconds() * float64(printing)
		v.Seconds += seconds
		v.Today.Seconds += seconds
	}

	var out []Notice

	if printing > 0 {
		out = append(out, v.feed(now)...)
	}

	out = append(out, v.grow(now)...)

	if level := v.hunger(now); hungerRank(level) > hungerRank(v.Announced) {
		v.Announced = level
		if level != hungerPeckish {
			hours := int(now.Sub(v.Fed).Hours())
			out = append(out, petNotice(now, level, severityWarning, strconv.Itoa(hours)))
		}
	}

	return out
}

// feed records that something printed at now. A pet that was hungry says
// thank you.
func (v *Vitals) feed(now time.Time) []Notice {
	v.Fed = now

	was := v.Announced
	v.Announced = hungerFull

	if hungerRank(was) >= hungerRank(hungerHungry) {
		return []Notice{petNotice(now, "fed", severityInfo, "")}
	}

	return nil
}

// grow moves the pet up to the stage its printing hours reached.
func (v *Vitals) grow(now time.Time) []Notice {
	var out []Notice

	for i := stageIndex(v.Stage) + 1; i < len(growth); i++ {
		if v.hours() < growth[i].Hours {
			break
		}

		v.Stage = growth[i].Stage
		out = append(out, petNotice(now, "evolved", severityInfo, v.Stage))
	}

	return out
}

// record counts one notice of the fleet into the day and the pet's life.
func (v *Vitals) record(n Notice) []Notice {
	v.rollDay(n.At)

	switch n.Kind {
	case "finished":
		v.Finished++
		v.Today.Finished++

		return v.feed(n.At)
	case "failed":
		v.Failed++
		v.Today.Failed++
	case "stopped":
		v.Today.Stopped++
	case "attention":
		v.Today.Attention++
	case "issue":
		v.Today.Issues++
	}

	return nil
}

// roastDue says whether the morning roast of yesterday is still to be
// written at now.
func (v *Vitals) roastDue(now time.Time, hour int) bool {
	if hour < 0 || now.Local().Hour() < hour {
		return false
	}

	v.rollDay(now)

	return v.Roasted != v.Yesterday.Date
}

// view is the body as the page shows it.
func (v *Vitals) view(now time.Time) VitalsView {
	out := VitalsView{
		Stage: v.Stage, Hunger: v.hunger(now), Hours: v.hours(), Fed: v.Fed, Born: v.Born, Roast: v.Roast,
	}

	since := now.Sub(v.Fed)
	out.Fullness = max(0, min(100, int(100-100*since.Seconds()/starvingAfter.Seconds())))

	if i := stageIndex(v.Stage); i+1 < len(growth) {
		from, to := growth[i].Hours, growth[i+1].Hours
		out.Next = growth[i+1].Stage
		out.Growth = max(0, min(100, int(100*(v.hours()-from)/(to-from))))
	} else {
		out.Growth = 100
	}

	return out
}

// text is the pet's body in plain English, for the brain to read.
func (v *Vitals) text(now time.Time) string {
	since := now.Sub(v.Fed)

	hunger := map[string]string{
		hungerFull:     "You are well fed: the printers keep printing.",
		hungerPeckish:  "You are a little peckish: nothing has printed for %d hours.",
		hungerHungry:   "You are hungry: nothing has printed for %d hours. Printing feeds you.",
		hungerStarving: "You are starving: nothing has printed for %d hours. Printing feeds you, and you want food badly.",
	}[v.hunger(now)]

	if v.hunger(now) != hungerFull {
		hunger = fmt.Sprintf(hunger, int(since.Hours()))
	}

	age := map[string]string{
		"baby":   "a newly hatched baby pet",
		"kid":    "a young pet, still growing",
		"adult":  "a grown-up pet",
		"legend": "a legendary old pet who has seen it all",
	}[v.Stage]

	return fmt.Sprintf("About you: you are %s (stage %d of %d), grown on %.0f printing hours, with %d prints finished and %d failed since you hatched. %s",
		age, stageIndex(v.Stage)+1, len(growth), v.hours(), v.Finished, v.Failed, hunger)
}

// petNotice is a notice about the pet itself rather than a printer. It names
// no printer, so everybody who sees the pet sees it.
func petNotice(now time.Time, kind, severity, detail string) Notice {
	return Notice{At: now, Kind: kind, Severity: severity, Detail: detail}
}
