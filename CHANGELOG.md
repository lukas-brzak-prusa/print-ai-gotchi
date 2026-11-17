# Changelog

What an operator sees change when they upgrade this module, newest release
first. `version:` in `module.yaml` is the newest heading below.

Write the entry in the same change as the code. One bullet per behaviour a
person outside this repository can observe: a route, a setting, a screen, a
refusal. Name the route and the setting key, never a function or a file.

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
