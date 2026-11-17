# Changelog

What an operator sees change when they upgrade this module, newest release
first. `version:` in `module.yaml` is the newest heading below.

Write the entry in the same change as the code. One bullet per behaviour a
person outside this repository can observe: a route, a setting, a screen, a
refusal. Name the route and the setting key, never a function or a file.

## 0.2.0 - 2026-11-17

### Added

- The pet gets hungry when nothing prints: peckish after 12 hours, hungry after 24, starving after
  48. It says so, shows a hungry face, and thanks whoever feeds it with a print. `GET /pet` carries
  `vitals` with its hunger and how full it is.
- The pet grows with the printing hours of the whole fleet: hatchling, youngster, grown-up, legend.
  It is drawn smaller in its shell at first and wears a crown as a legend.
- A morning roast: every morning from `roast_hour` (8 by default, -1 turns it off) the pet sums up
  the day before in its own character. It counts only, and names no printer.
- The stage shows a hunger meter and a growth meter.
- A model download can be cancelled on the Brain tab, or with `DELETE /brain/download`. Nothing of
  it stays on disk.

### Changed

- Downloading a model no longer switches the pet to it at once. The pet keeps its brain while the
  model downloads and moves to it once it is on disk, unless somebody chose another brain meanwhile.
  A cancelled or failed download leaves the pet able to talk.
- Installing, removing and cancelling a model are refused when `page_setup` is off, like choosing a
  brain already was.

## 0.1.2 - 2026-11-17

### Fixed

- Installing a model on the Brain tab works again. In 0.1.1 the first download stopped the module,
  and a download while the local brain ran failed.

### Changed

- `llama-server` gets only the environment it needs: its library path, `PATH`, `HOME` and `TMPDIR`.

## 0.1.1 - 2026-11-17

### Changed

- The local brain answers on a unix socket in a private directory instead of a port on 127.0.0.1,
  so nothing else on the machine can reach it, and the module listens on no port.

## 0.1.0 - 2026-11-17

### Added

- PrintAIgotchi: a pet with a page of its own and a card on the dashboard. It watches the printers,
  reports what needs a person, and comments on it in its own character.
- Chat with the pet about the printers a person may see, with suggested questions.
- Brains to choose from on the page: a local model run by llama.cpp on the server itself
  (downloaded and verified on request), Ollama, LM Studio, Claude, OpenAI, Gemini, OpenRouter, Groq,
  Mistral or any OpenAI-compatible server.
- Character: name, personality, species and language of the pet.
