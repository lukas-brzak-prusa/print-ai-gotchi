package main

// The pet's eyes: it follows the fleet on the host's event stream, keeps the
// last record of every printer, turns each change worth a word into a notice,
// and writes the plain-text picture of the fleet a brain reads before it
// answers.

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prusa3d/prusa-connect/sdk/busapi"
	"github.com/prusa3d/prusa-connect/sdk/deviceapi"
	"github.com/prusa3d/prusa-connect/sdk/event"
)

// Notice is one thing the pet tells people about.
type Notice struct {
	ID         string    `json:"id"`
	At         time.Time `json:"at"`
	DeviceID   string    `json:"device_id"`
	DeviceName string    `json:"device_name"`
	// Kind is what happened, and the page translates it: issue, cleared,
	// attention, started, finished, failed, stopped, online, offline, or
	// about the pet itself, with no device: hungry, starving, fed, evolved,
	// roast.
	Kind string `json:"kind"`
	// Severity is info, warning or error.
	Severity string `json:"severity"`
	// Detail is the machine's own words, in English, when there are any.
	Detail string `json:"detail,omitempty"`
	// Quip is the pet's own comment, written by the brain after the fact.
	Quip string `json:"quip,omitempty"`
}

const (
	severityInfo    = "info"
	severityWarning = "warning"
	severityError   = "error"
)

// maxNotices is how many notices the pet remembers.
const maxNotices = 60

// reloadEvery bounds how often a burst of events reloads the fleet.
const reloadEvery = 2 * time.Second

// Devices is the part of the host's fleet door the watcher uses.
type Devices interface {
	List(ctx context.Context, params deviceapi.ListParams) (deviceapi.Page, error)
}

// Subscribe opens the host's event stream.
type Subscribe func(ctx context.Context, params busapi.SubscribeParams, onWindow func(busapi.Window) error) error

// Watcher follows the fleet as the module itself, inside the observe grant.
type Watcher struct {
	devices   Devices
	subscribe Subscribe
	log       func(string, ...any)
	onNotice  func(Notice)

	mu       sync.Mutex
	printers map[string]deviceapi.Device
	notices  []Notice
	listed   bool
	lastJob  map[string]time.Time // device id → when a print last finished
	seq      int
}

func newWatcher(devices Devices, subscribe Subscribe, log func(string, ...any)) *Watcher {
	return &Watcher{
		devices: devices, subscribe: subscribe, log: log,
		printers: map[string]deviceapi.Device{},
		lastJob:  map[string]time.Time{},
	}
}

// Run follows the fleet until ctx ends, reconnecting with a growing wait.
func (w *Watcher) Run(ctx context.Context) {
	params := busapi.SubscribeParams{
		Types: []string{
			event.TypeDeviceTelemetrySampled, event.TypeDeviceIssuesChanged,
			event.TypeDevicePresenceChanged, event.TypeDeviceRecordChanged,
			event.TypeDeviceArrived, event.TypeDeviceDeparted,
			event.TypeDeviceJobStarted, event.TypeDeviceJobSettled,
		},
		IntervalMS: 1000,
	}

	wait := time.Second

	for ctx.Err() == nil {
		err := w.reload(ctx)
		if err == nil {
			wait = time.Second
			last := time.Now()
			err = w.subscribe(ctx, params, func(window busapi.Window) error {
				if window.Cursor != "" {
					params.Resume = window.Cursor
				}

				if !window.Resync && time.Since(last) < reloadEvery {
					return nil
				}

				last = time.Now()

				return w.reload(ctx)
			})
		}

		if ctx.Err() != nil {
			return
		}

		w.log("following the fleet failed, trying again in %s: %v", wait, err)

		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}

		wait = min(wait*2, 30*time.Second)
	}
}

// reload reads the whole fleet and turns the difference into notices.
func (w *Watcher) reload(ctx context.Context) error {
	current := map[string]deviceapi.Device{}

	for offset := 0; ; {
		page, err := w.devices.List(ctx, deviceapi.ListParams{Limit: 200, Offset: offset})
		if err != nil {
			return err
		}

		for _, device := range page.Items {
			current[device.ID] = device
		}

		offset += len(page.Items)
		if len(page.Items) == 0 || offset >= page.Total {
			break
		}
	}

	w.mu.Lock()
	first := !w.listed
	previous := w.printers
	w.printers, w.listed = current, true

	var fresh []Notice
	if !first {
		fresh = diff(previous, current, time.Now())
		for i := range fresh {
			fresh[i].ID = w.nextIDLocked(fresh[i].At)
			if fresh[i].Kind == "finished" {
				w.lastJob[fresh[i].DeviceID] = fresh[i].At
			}
		}

		w.notices = append(w.notices, fresh...)
		if len(w.notices) > maxNotices {
			w.notices = slices.Clone(w.notices[len(w.notices)-maxNotices:])
		}
	}

	onNotice := w.onNotice
	w.mu.Unlock()

	if onNotice != nil {
		for _, notice := range fresh {
			onNotice(notice)
		}
	}

	return nil
}

// nextIDLocked is a fresh notice id. The caller holds mu.
func (w *Watcher) nextIDLocked(at time.Time) string {
	w.seq++

	return strconv.FormatInt(at.UnixMilli(), 36) + "-" + strconv.Itoa(w.seq)
}

// Add keeps a notice about the pet itself and returns it with its id.
func (w *Watcher) Add(n Notice) Notice {
	w.mu.Lock()
	defer w.mu.Unlock()

	n.ID = w.nextIDLocked(n.At)
	w.notices = append(w.notices, n)

	if len(w.notices) > maxNotices {
		w.notices = slices.Clone(w.notices[len(w.notices)-maxNotices:])
	}

	return n
}

// Printing is how many printers of the whole fleet print, and false until
// the fleet was read once.
func (w *Watcher) Printing() (int, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()

	count := 0

	for _, d := range w.printers {
		if stateOf(d) == "PRINTING" {
			count++
		}
	}

	return count, w.listed
}

// Restore puts back the notices kept from an earlier run.
func (w *Watcher) Restore(notices []Notice) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.notices = notices
}

// SetQuip writes the pet's comment onto a notice it already holds.
func (w *Watcher) SetQuip(id, quip string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	for i := range w.notices {
		if w.notices[i].ID == id {
			w.notices[i].Quip = quip
		}
	}
}

// Notices is every notice held, newest first, limited to the given printers
// when visible is not nil.
func (w *Watcher) Notices(visible map[string]bool) []Notice {
	w.mu.Lock()
	defer w.mu.Unlock()

	out := make([]Notice, 0, len(w.notices))

	for i := len(w.notices) - 1; i >= 0; i-- {
		// A notice about the pet itself names no printer, so everybody sees it.
		if visible == nil || w.notices[i].DeviceID == "" || visible[w.notices[i].DeviceID] {
			out = append(out, w.notices[i])
		}
	}

	return out
}

// Kept is the notices to write to the store, oldest first.
func (w *Watcher) Kept() []Notice {
	w.mu.Lock()
	defer w.mu.Unlock()

	return slices.Clone(w.notices)
}

// recentlyFinished says whether a print ended well on one of the printers in
// the last ten minutes, which makes the pet proud.
func (w *Watcher) recentlyFinished(visible map[string]bool) bool {
	w.mu.Lock()
	defer w.mu.Unlock()

	for id, at := range w.lastJob {
		if (visible == nil || visible[id]) && time.Since(at) < 10*time.Minute {
			return true
		}
	}

	return false
}

// diff is every notice the change from before to after is worth.
func diff(before, after map[string]deviceapi.Device, now time.Time) []Notice {
	var out []Notice

	for _, id := range slices.Sorted(maps.Keys(after)) {
		next := after[id]
		prev, known := before[id]
		notice := func(kind, severity, detail string) {
			out = append(out, Notice{
				At: now, DeviceID: id, DeviceName: displayName(next),
				Kind: kind, Severity: severity, Detail: detail,
			})
		}

		if !known {
			if next.Connected {
				notice("online", severityInfo, "")
			}

			continue
		}

		if prev.Connected != next.Connected {
			if next.Connected {
				notice("online", severityInfo, "")
			} else {
				notice("offline", severityWarning, "")
			}
		}

		was, is := stateOf(prev), stateOf(next)
		if was != is {
			switch is {
			case "PRINTING":
				if was != "PAUSED" && was != "ATTENTION" {
					notice("started", severityInfo, jobName(next))
				}
			case "FINISHED":
				notice("finished", severityInfo, jobName(prev))
			case "STOPPED":
				notice("stopped", severityWarning, jobName(prev))
			case "ERROR":
				notice("failed", severityError, jobName(prev))
			}
		}

		if next.Dialog != nil && (prev.Dialog == nil || string(prev.Dialog.ID) != string(next.Dialog.ID)) {
			notice("attention", severityWarning, strings.TrimSpace(next.Dialog.Title+". "+next.Dialog.Text))
		}

		had := map[string]bool{}
		for _, issue := range prev.Issues {
			had[issue.Reason+"|"+issue.Detail] = true
		}

		has := map[string]bool{}
		for _, issue := range next.Issues {
			has[issue.Reason+"|"+issue.Detail] = true

			if !had[issue.Reason+"|"+issue.Detail] && issue.Reason != "disconnected" && issue.Reason != "awaiting_operator" {
				notice("issue", severityOf(issue.Severity), issueText(issue))
			}
		}

		for _, issue := range prev.Issues {
			if !has[issue.Reason+"|"+issue.Detail] && issue.Reason != "disconnected" && issue.Reason != "awaiting_operator" {
				notice("cleared", severityInfo, issueText(issue))
			}
		}
	}

	for _, id := range slices.Sorted(maps.Keys(before)) {
		if _, still := after[id]; !still && before[id].Connected {
			out = append(out, Notice{At: now, DeviceID: id, DeviceName: displayName(before[id]), Kind: "offline", Severity: severityWarning})
		}
	}

	return out
}

func severityOf(s string) string {
	switch s {
	case severityError, "critical":
		return severityError
	case severityWarning:
		return severityWarning
	}

	return severityInfo
}

func issueText(issue deviceapi.Issue) string {
	if issue.Detail != "" {
		return issue.Detail
	}

	return strings.ReplaceAll(issue.Reason, "_", " ")
}

func displayName(d deviceapi.Device) string {
	if d.Name != "" {
		return d.Name
	}

	return d.ID
}

// The helpers below read one value of a device's state tree.

func stateString(d deviceapi.Device, path string) string {
	var out string
	_ = json.Unmarshal(d.State[path], &out)

	return out
}

func stateNumber(d deviceapi.Device, path string) (float64, bool) {
	raw, ok := d.State[path]
	if !ok {
		return 0, false
	}

	var out float64
	if json.Unmarshal(raw, &out) != nil {
		return 0, false
	}

	return out, true
}

func stateOf(d deviceapi.Device) string { return stateString(d, "/state") }

func jobName(d deviceapi.Device) string {
	if name := stateString(d, "/job/display"); name != "" {
		return name
	}

	path := stateString(d, "/job/path")
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}

	return path
}

// Mood is the face the pet shows, worked out from the fleet without a brain.
type Mood struct {
	// Face is alarmed, worried, busy, proud, hungry, sleepy or bored.
	Face string `json:"face"`
	// Printers, Printing, Problems and Online count the fleet the caller sees.
	Printers int `json:"printers"`
	Online   int `json:"online"`
	Printing int `json:"printing"`
	Problems int `json:"problems"`
}

// Mood reads the face off the printers the caller may see.
func (w *Watcher) Mood(visible map[string]bool) Mood {
	w.mu.Lock()
	printers := slices.Collect(maps.Values(w.printers))
	w.mu.Unlock()

	var mood Mood

	errorsSeen, warnings := 0, 0

	for _, d := range printers {
		if visible != nil && !visible[d.ID] {
			continue
		}

		mood.Printers++

		if d.Connected {
			mood.Online++
		}

		switch stateOf(d) {
		case "PRINTING":
			mood.Printing++
		case "ERROR":
			errorsSeen++
		}

		if d.Dialog != nil {
			warnings++
		}

		for _, issue := range d.Issues {
			switch severityOf(issue.Severity) {
			case severityError:
				errorsSeen++
			case severityWarning:
				warnings++
			}
		}
	}

	mood.Problems = errorsSeen + warnings

	switch {
	case errorsSeen > 0:
		mood.Face = "alarmed"
	case warnings > 0:
		mood.Face = "worried"
	case w.recentlyFinished(visible):
		mood.Face = "proud"
	case mood.Printing > 0:
		mood.Face = "busy"
	case mood.Online == 0:
		mood.Face = "sleepy"
	default:
		mood.Face = "bored"
	}

	return mood
}

// maxDetailed is how many printers the picture describes one by one. A
// small model has a small context, so the rest are counted.
const maxDetailed = 24

// Picture is the plain-English description of the fleet a brain reads.
func Picture(printers []deviceapi.Device, notices []Notice, now time.Time) string {
	var b strings.Builder

	fmt.Fprintf(&b, "Time now: %s\n", now.Format("Mon 2 Jan 15:04"))

	// Printers that need somebody first, then printing, then the rest.
	rank := func(d deviceapi.Device) int {
		switch {
		case len(d.Issues) > 0 || d.Dialog != nil || stateOf(d) == "ERROR":
			return 0
		case stateOf(d) == "PRINTING" || stateOf(d) == "PAUSED":
			return 1
		case d.Connected:
			return 2
		}

		return 3
	}

	slices.SortStableFunc(printers, func(a, b deviceapi.Device) int {
		if ra, rb := rank(a), rank(b); ra != rb {
			return ra - rb
		}

		return strings.Compare(displayName(a), displayName(b))
	})

	fmt.Fprintf(&b, "Printers (%d):\n", len(printers))

	for i, d := range printers {
		if i == maxDetailed {
			counts := map[string]int{}
			for _, rest := range printers[i:] {
				counts[strings.ToLower(stateOrOffline(rest))]++
			}

			fmt.Fprintf(&b, "- and %d more: %s\n", len(printers)-i, countsText(counts))

			break
		}

		b.WriteString("- ")
		b.WriteString(describe(d))
		b.WriteString("\n")
	}

	if len(notices) > 0 {
		b.WriteString("Recent events, newest first:\n")

		for i, n := range notices {
			if i == 12 {
				break
			}

			fmt.Fprintf(&b, "- %s %s: %s", n.At.Format("15:04"), n.DeviceName, n.Kind)

			if n.Detail != "" {
				fmt.Fprintf(&b, " (%s)", n.Detail)
			}

			b.WriteString("\n")
		}
	}

	return b.String()
}

func stateOrOffline(d deviceapi.Device) string {
	if !d.Connected {
		return "offline"
	}

	if s := stateOf(d); s != "" {
		return s
	}

	return "unknown"
}

func countsText(counts map[string]int) string {
	parts := make([]string, 0, len(counts))
	for _, key := range slices.Sorted(maps.Keys(counts)) {
		parts = append(parts, fmt.Sprintf("%d %s", counts[key], key))
	}

	return strings.Join(parts, ", ")
}

// describe is one printer in one line.
func describe(d deviceapi.Device) string {
	var b strings.Builder

	fmt.Fprintf(&b, "%q (%s): %s", displayName(d), d.Model, stateOrOffline(d))

	if s := stateOf(d); s == "PRINTING" || s == "PAUSED" || s == "ATTENTION" {
		if job := jobName(d); job != "" {
			fmt.Fprintf(&b, ", file %q", job)
		}

		if p, ok := stateNumber(d, "/job/progress"); ok {
			fmt.Fprintf(&b, ", %.0f%% done", p)
		}

		if left, ok := stateNumber(d, "/job/time_remaining"); ok && left > 0 {
			fmt.Fprintf(&b, ", %s left", (time.Duration(left) * time.Second).Round(time.Minute))
		}
	}

	if nozzle, ok := stateNumber(d, "/tools/0/nozzle_temp"); ok {
		fmt.Fprintf(&b, ", nozzle %.0f°C", nozzle)

		if target, ok := stateNumber(d, "/tools/0/nozzle_target_temp"); ok && target > 0 {
			fmt.Fprintf(&b, " of %.0f°C", target)
		}
	}

	if bed, ok := stateNumber(d, "/temperature/bed"); ok {
		fmt.Fprintf(&b, ", bed %.0f°C", bed)

		if target, ok := stateNumber(d, "/temperature/bed_target"); ok && target > 0 {
			fmt.Fprintf(&b, " of %.0f°C", target)
		}
	}

	if material := loadedMaterial(d); material != "" {
		fmt.Fprintf(&b, ", loaded %s", material)
	}

	if d.Dialog != nil {
		fmt.Fprintf(&b, ". WAITING FOR A PERSON: %q %q, buttons %s", d.Dialog.Title, d.Dialog.Text, strings.Join(d.Dialog.Buttons, "/"))
	}

	for _, issue := range d.Issues {
		if issue.Reason == "awaiting_operator" && d.Dialog != nil {
			continue
		}

		fmt.Fprintf(&b, ". Problem (%s): %s", issue.Severity, issueText(issue))
	}

	return b.String()
}

// loadedMaterial is the material of the active tool, or of the active slot of
// its feeder.
func loadedMaterial(d deviceapi.Device) string {
	const suffix = "/material_package_instance/package/material/abbreviation"

	for _, path := range slices.Sorted(maps.Keys(d.State)) {
		tool, ok := strings.CutSuffix(path, suffix)
		if !ok {
			continue
		}

		var active bool
		_ = json.Unmarshal(d.State[tool+"/active"], &active)

		if material := stateString(d, path); active && material != "" {
			return material
		}
	}

	return ""
}
