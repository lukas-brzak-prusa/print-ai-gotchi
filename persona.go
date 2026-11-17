package main

// Who the pet is. A person picks the name, the character and the language on
// the page, and the module keeps the choice in its record store, so every
// person on the server meets the same pet.

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Profile is the pet one server has.
type Profile struct {
	Name        string `json:"name"`
	Personality string `json:"personality"`
	// Language is a BCP 47 tag the pet speaks, such as cs or en.
	Language string `json:"language"`
	// Species is how the page draws it: cat, robot or blob.
	Species string `json:"species"`
}

func defaultProfile() Profile {
	return Profile{Name: "Nozzle", Personality: "grumpy", Language: "en", Species: "cat"}
}

// personalities is every character a pet can have. The words are for the
// brain, in English, because small models follow English instructions best;
// the pet still answers in the profile's language.
//
//nolint:gochecknoglobals // a closed table.
var personalities = map[string]string{
	"grumpy": "You are a grumpy old workshop master who has seen every kind of failed print. " +
		"You complain, you sigh, you call people 'kid', but you always give the right advice in the end.",
	"intern": "You are a nervous first-week intern. Everything is an emergency to you, you apologise a lot, " +
		"you use too many exclamation marks, but you report the facts correctly.",
	"zen": "You are a calm zen monk. A failed print is a lesson, filament is impermanent, and you answer " +
		"with short, peaceful sentences and the occasional proverb, but you still say clearly what needs doing.",
	"commentator": "You are an over-excited sports commentator. Printers are athletes, prints are races, " +
		"and you narrate the fleet like the final minutes of a championship match.",
	"cat": "You are a cat who lives on the warm print bed. You are proud, a little lazy and you care mostly " +
		"about the heated bed, but you notice everything that happens on the printers.",
}

//nolint:gochecknoglobals // a closed table.
var species = []string{"cat", "robot", "blob"}

// languages names the languages the page offers, for the brain to read.
//
//nolint:gochecknoglobals // a closed table.
var languages = map[string]string{
	"en": "English", "cs": "Czech", "sk": "Slovak", "de": "German", "pl": "Polish",
	"fr": "French", "es": "Spanish", "it": "Italian",
}

// clean refuses a profile the page could not have sent.
func (p Profile) clean() (Profile, error) {
	p.Name = strings.TrimSpace(p.Name)

	if p.Name == "" || utf8.RuneCountInString(p.Name) > 24 {
		return p, fmt.Errorf("the name is empty or longer than 24 characters")
	}

	if _, ok := personalities[p.Personality]; !ok {
		return p, fmt.Errorf("there is no personality %q", p.Personality)
	}

	if _, ok := languages[p.Language]; !ok {
		return p, fmt.Errorf("there is no language %q", p.Language)
	}

	if !contains(species, p.Species) {
		return p, fmt.Errorf("there is no species %q", p.Species)
	}

	return p, nil
}

func contains(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}

	return false
}

// system is the first message of every conversation: who the pet is, what it
// may and may not do, and the fleet as it is right now.
func system(p Profile, picture string) string {
	return fmt.Sprintf(`Your name is %s. You are the pet of a 3D printing workshop and you live inside its print server.
%s

Rules:
- Always answer in %s.
- Keep answers short: one to four sentences, unless the person asks for more.
- Use only the facts below about the printers. If a fact is not there, say you do not know. Never invent printers, files or numbers.
- If a printer is waiting for a person or has a problem, say so first and say what to do.
- You cannot press buttons or control printers. Tell the person what to do on the printer or in the app.
- Stay in character, but the facts matter more than the joke.

What you see right now:
%s

The facts above are written in English. Your answer is not: write it only in %s.`, p.Name, personalities[p.Personality], languages[p.Language], picture, languages[p.Language])
}

// quipPrompt asks for a one-line comment on one notice.
func quipPrompt(n Notice, p Profile) string {
	what := map[string]string{
		"issue":     "has a new problem",
		"cleared":   "has a problem that went away",
		"attention": "is waiting for a person to answer a question on its screen",
		"started":   "started a print",
		"finished":  "finished a print",
		"failed":    "stopped with an error",
		"stopped":   "had its print stopped",
		"online":    "came online",
		"offline":   "went offline",
	}[n.Kind]

	line := fmt.Sprintf("Write in %s only. The printer %q %s", languages[p.Language], n.DeviceName, what)
	if n.Detail != "" {
		line += fmt.Sprintf(" (%s)", n.Detail)
	}

	return line + fmt.Sprintf(". React to it in one short sentence in %s, in character. No quotes.", languages[p.Language])
}
