// The codes this module refuses a caller with.
//
// A refusal a person or a client can see is a registered code, never a bare
// English sentence. The code is what a browser half switches on to choose a
// translation, and the English sentence travels beside it so a log line and an
// untranslated screen still read.
//
// Registering a code takes its documentation with it: what went wrong, what the
// caller should do, whether sending the same request again can work, and the
// parameters the sentence names. The sdk refuses a registration without them.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/prusa3d/prusa-connect/sdk/fault"
)

// faultDomain is the first half of every code below. It is this module's
// namespace, which the host derived from the Go module path github.com/lukas-brzak-prusa/print-ai-gotchi, so no
// other author can register a code under it. The sdk refuses the core
// namespace, which is what keeps this module's codes apart from the server's.
//
//nolint:gochecknoglobals // a domain built once at start-up; a const cannot hold one.
var faultDomain = mustDomain("com.github.aux.lukas-brzak-prusa.print-ai-gotchi")

// The codes this module raises. Add a code here, and nowhere else, so the list
// a client branches on is one file.
//
//nolint:gochecknoglobals // definitions built once at start-up.
var (
	// RouteNotFound is a path this module does not serve.
	RouteNotFound = fault.Register(faultDomain, "ROUTE_NOT_FOUND", http.StatusNotFound, fault.Document(
		"This module does not serve that path",
		"Read openapi.json. Every path in it is relative to the mount the operator gave this module.",
		fault.RetryNever,
		fault.ShowDetail,
		"path",
	))

	// InvalidBody is a request body this module cannot read.
	InvalidBody = fault.Register(faultDomain, "INVALID_BODY", http.StatusBadRequest, fault.Document(
		"The request body is not what this route takes",
		"Read openapi.json for the shape of the body, and send it again.",
		fault.RetryNever,
		fault.ShowDetail,
	))

	// FleetUnavailable is the host's fleet not answering.
	FleetUnavailable = fault.Register(faultDomain, "FLEET_UNAVAILABLE", http.StatusBadGateway, fault.Document(
		"The printers could not be read",
		"Try again in a moment. If it stays, read the server's log.",
		fault.RetryAllowed,
		fault.ShowDetail,
	))

	// StoreUnavailable is the host's record store refusing a write.
	StoreUnavailable = fault.Register(faultDomain, "STORE_UNAVAILABLE", http.StatusServiceUnavailable, fault.Document(
		"The pet could not keep what you changed",
		"Try again in a moment. The server needs a record store under resources.db.",
		fault.RetryAllowed,
		fault.ShowDetail,
	))

	// ReplyNotFound is an answer id the module does not hold.
	ReplyNotFound = fault.Register(faultDomain, "REPLY_NOT_FOUND", http.StatusNotFound, fault.Document(
		"There is no answer with that id",
		"An answer is kept for 15 minutes and until the module restarts. Ask again.",
		fault.RetryNever,
		fault.ShowDetail,
		"id",
	))

	// BrainAsleep is a question asked before the brain can answer.
	BrainAsleep = fault.Register(faultDomain, "BRAIN_ASLEEP", http.StatusConflict, fault.Document(
		"The pet's brain is not running",
		"Choose a brain on the pet's page: download a local model, or enter a provider and its key.",
		fault.RetryAllowed,
		fault.ShowDetail,
	))

	// BrainLocked is a change of brain the operator does not allow on the page.
	BrainLocked = fault.Register(faultDomain, "BRAIN_LOCKED", http.StatusForbidden, fault.Document(
		"Only the operator changes the brain here",
		"Ask the operator, who sets page_setup or the brain settings of this module.",
		fault.RetryNever,
		fault.ShowDetail,
	))

	// BrainBusy is a model download asked for while another one runs.
	BrainBusy = fault.Register(faultDomain, "BRAIN_BUSY", http.StatusConflict, fault.Document(
		"A model is downloading",
		"Wait until the download ends, then try again.",
		fault.RetryAllowed,
		fault.ShowDetail,
	))

	// BrainNotInstallable is an install the settings rule out.
	BrainNotInstallable = fault.Register(faultDomain, "BRAIN_NOT_INSTALLABLE", http.StatusConflict, fault.Document(
		"The local brain cannot be installed here",
		"Read the detail: the module needs a data directory, and a platform with a pinned runtime or llama_server.",
		fault.RetryNever,
		fault.ShowDetail,
	))
)

// The errors the pet's parts return, each answered with one code above.
var (
	errBrainAsleep         = errors.New("the brain is not running")
	errDownloadBusy        = errors.New("another model is downloading")
	errNoDataDir           = errors.New("the operator did not grant this module a data directory")
	errUnsupportedPlatform = errors.New("there is no pinned llama.cpp build for this platform; set llama_server")
)

// faultOf is the refusal an error of the pet's parts reads as.
func faultOf(err error) *fault.Fault {
	switch {
	case errors.Is(err, errBrainAsleep):
		return BrainAsleep.New(err.Error())
	case errors.Is(err, errNoDataDir), errors.Is(err, errUnsupportedPlatform):
		return BrainNotInstallable.New(err.Error())
	case errors.Is(err, errDownloadBusy):
		return BrainBusy.New(err.Error())
	case errors.Is(err, errBadChoice):
		return InvalidBody.New(err.Error())
	}

	return FleetUnavailable.New(err.Error())
}

// mustDomain builds this module's fault domain. A name the sdk refuses is a
// mistake in this file, so it stops the process at start-up rather than
// answering a caller with an empty code.
func mustDomain(name string) fault.Domain {
	domain, err := fault.NewDomain(name)
	if err != nil {
		panic(fmt.Sprintf("print-ai-gotchi: %s is not a fault domain: %v", name, err))
	}

	return domain
}

// refuse writes one refusal as RFC 9457 frames it, which is the shape the
// generated client reads and the shape describeFault reads in the browser half:
// the code, the English sentence, the values that sentence names, and where the
// code is documented.
func refuse(w http.ResponseWriter, r *http.Request, raised *fault.Fault) {
	problem := raised.ProblemJSON(r.URL.Path)

	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(problem.Status)

	_ = json.NewEncoder(w).Encode(problem)
}
