package main

// The models the local brain can download, each pinned to a commit of the
// publisher's own repository and to the SHA-256 of the file.

import (
	"cmp"
	"slices"
)

// pinned is one file this module downloads, by its address and the digest it
// must have.
type pinned struct {
	URL    string `json:"-"`
	SHA256 string `json:"-"`
	Size   int64  `json:"size"`
}

// brain is a model the local backend can load.
type brain struct {
	Key   string `json:"key"`
	Title string `json:"title"`
	// Maker is who trained it.
	Maker string `json:"maker"`
	// Params is the size of the model, in billions of parameters.
	Params float64 `json:"params"`
	// Multilingual says the maker trained it for many languages, Czech among them.
	Multilingual bool `json:"multilingual"`
	// For says what hardware it suits: pi4, pi5, pi5-8gb or pc-8gb.
	For string `json:"for"`
	// License names the licence, and LicenseURL is where it is.
	License    string `json:"license"`
	LicenseURL string `json:"license_url"`
	// Memory is roughly how many bytes it holds while it answers.
	Memory int64 `json:"memory"`
	// Measured says Memory was measured, not estimated from the file size.
	Measured bool   `json:"measured"`
	File     string `json:"-"`
	// Args are the llama-server flags this model needs beside the common ones.
	Args []string `json:"-"`
	pinned
}

const (
	apache     = "Apache-2.0"
	apacheURL  = "https://www.apache.org/licenses/LICENSE-2.0"
	gemma      = "Gemma Terms of Use"
	gemmaURL   = "https://ai.google.dev/gemma/terms"
	gigabyte   = 1_000_000_000
	headroomMB = 600_000_000
)

// Qwen3 thinks aloud before it answers unless told not to, which a pet on a
// small machine cannot afford.
//
//nolint:gochecknoglobals // a closed table.
var noThinking = []string{"--reasoning", "off"}

func hf(repo, commit, file string) string {
	return "https://huggingface.co/" + repo + "/resolve/" + commit + "/" + file
}

// brains is every model the local backend offers.
//
//nolint:gochecknoglobals // a closed table of pins.
var brains = map[string]brain{
	"qwen2.5-0.5b": {
		Title: "Qwen2.5 0.5B", Maker: "Qwen", Params: 0.5, For: "pi4",
		License: apache, LicenseURL: apacheURL, File: "qwen2.5-0.5b-instruct-q4_k_m.gguf",
		pinned: pinned{
			URL:    hf("Qwen/Qwen2.5-0.5B-Instruct-GGUF", "9217f5db79a29953eb74d5343926648285ec7e67", "qwen2.5-0.5b-instruct-q4_k_m.gguf"),
			SHA256: "74a4da8c9fdbcd15bd1f6d01d621410d31c6fc00986f5eb687824e7b93d7a9db", Size: 491400032,
		},
	},
	"gemma3-1b": {
		Title: "Gemma 3 1B", Maker: "Google", Params: 1, Multilingual: true, For: "pi4",
		License: gemma, LicenseURL: gemmaURL, File: "gemma-3-1b-it-Q4_K_M.gguf",
		pinned: pinned{
			URL:    hf("ggml-org/gemma-3-1b-it-GGUF", "f9c28bcd85737ffc5aef028638d3341d49869c27", "gemma-3-1b-it-Q4_K_M.gguf"),
			SHA256: "8ccc5cd1f1b3602548715ae25a66ed73fd5dc68a210412eea643eb20eb75a135", Size: 806058240,
		},
	},
	"qwen2.5-1.5b": {
		Title: "Qwen2.5 1.5B", Maker: "Qwen", Params: 1.5, For: "pi5",
		License: apache, LicenseURL: apacheURL, File: "qwen2.5-1.5b-instruct-q4_k_m.gguf",
		Memory: 1_420_000_000, Measured: true,
		pinned: pinned{
			URL:    hf("Qwen/Qwen2.5-1.5B-Instruct-GGUF", "91cad51170dc346986eccefdc2dd33a9da36ead9", "qwen2.5-1.5b-instruct-q4_k_m.gguf"),
			SHA256: "6a1a2eb6d15622bf3c96857206351ba97e1af16c30d7a74ee38970e434e9407e", Size: 1117320736,
		},
	},
	"qwen3-1.7b": {
		Title: "Qwen3 1.7B", Maker: "Qwen", Params: 1.7, Multilingual: true, For: "pi5",
		License: apache, LicenseURL: apacheURL, File: "qwen3-1.7b-q8_0.gguf", Args: noThinking,
		Memory: 2_470_000_000, Measured: true,
		pinned: pinned{
			URL:    hf("Qwen/Qwen3-1.7B-GGUF", "90862c4b9d2787eaed51d12237eafdfe7c5f6077", "Qwen3-1.7B-Q8_0.gguf"),
			SHA256: "061b54daade076b5d3362dac252678d17da8c68f07560be70818cace6590cb1a", Size: 1834426016,
		},
	},
	"gemma3-4b": {
		Title: "Gemma 3 4B", Maker: "Google", Params: 4, Multilingual: true, For: "pi5-8gb",
		License: gemma, LicenseURL: gemmaURL, File: "gemma-3-4b-it-Q4_K_M.gguf",
		Memory: 3_030_000_000, Measured: true,
		pinned: pinned{
			URL:    hf("ggml-org/gemma-3-4b-it-GGUF", "d0976223747697cb51e056d85c532013931fe52e", "gemma-3-4b-it-Q4_K_M.gguf"),
			SHA256: "882e8d2db44dc554fb0ea5077cb7e4bc49e7342a1f0da57901c0802ea21a0863", Size: 2489757856,
		},
	},
	"qwen3-4b": {
		Title: "Qwen3 4B", Maker: "Qwen", Params: 4, Multilingual: true, For: "pi5-8gb",
		License: apache, LicenseURL: apacheURL, File: "Qwen3-4B-Q4_K_M.gguf", Args: noThinking,
		pinned: pinned{
			URL:    hf("Qwen/Qwen3-4B-GGUF", "bc640142c66e1fdd12af0bd68f40445458f3869b", "Qwen3-4B-Q4_K_M.gguf"),
			SHA256: "7485fe6f11af29433bc51cab58009521f205840f5b4ae3a32fa7f92e8534fdf5", Size: 2497280256,
		},
	},
	"qwen3-8b": {
		Title: "Qwen3 8B", Maker: "Qwen", Params: 8, Multilingual: true, For: "pc-8gb",
		License: apache, LicenseURL: apacheURL, File: "Qwen3-8B-Q4_K_M.gguf", Args: noThinking,
		pinned: pinned{
			URL:    hf("Qwen/Qwen3-8B-GGUF", "7c41481f57cb95916b40956ab2f0b139b296d974", "Qwen3-8B-Q4_K_M.gguf"),
			SHA256: "d98cdcbd03e17ce47681435b5150e34c1417f50b5c0019dd560e4882c5745785", Size: 5027783488,
		},
	},
}

//nolint:gochecknoinits // fills the derived fields of a closed table once.
func init() {
	for key, b := range brains {
		b.Key = key
		if b.Memory == 0 {
			// Not measured: the weights plus room for the context and the server.
			b.Memory = b.Size + headroomMB
		}

		brains[key] = b
	}
}

// brainList is the catalogue, smallest first.
func brainList() []brain {
	out := make([]brain, 0, len(brains))
	for _, b := range brains {
		out = append(out, b)
	}

	slices.SortFunc(out, func(a, b brain) int { return cmp.Compare(a.Size, b.Size) })

	return out
}

func brainKeys() []string {
	keys := make([]string, 0, len(brains))
	for _, b := range brainList() {
		keys = append(keys, b.Key)
	}

	return keys
}
