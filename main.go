// Command print-ai-gotchi is a pet that lives in a Prusa Connect Local server. It
// watches the printers, tells people what needs them, and talks: with a small
// language model it runs on the same machine, or with any server that speaks
// the OpenAI chat API.
//
// # The parts
//
//	fleet.go    the eyes: the event stream, the notices, the mood, the picture a brain reads
//	brain.go    the local brain: a pinned llama.cpp server and a pinned model, run as a child
//	llm.go      one streaming client for every brain
//	persona.go  who the pet is, and the words that tell a brain so
//	app.go      the routes the page talks to
//
// # The routes
//
//	GET  /pet            the profile, the mood, the notices and the brain, for the person asking
//	PUT  /profile        the pet's name, character, language and look
//	GET  /brain          what the brain is doing
//	POST /brain/install  download and start the local brain
//	POST /chat           start an answer
//	GET  /chat/{id}      read an answer while the brain writes it
//
// The host decides who may call each one: view_resource for a read and
// operate_resource for a write.
package main

import (
	"context"
	_ "embed"
	"fmt"
	"maps"
	"os"
	"os/signal"
	"sync"

	"github.com/prusa3d/prusa-connect/sdk/busapi"
	"github.com/prusa3d/prusa-connect/sdk/deviceapi"
	"github.com/prusa3d/prusa-connect/sdk/extension"
	"github.com/prusa3d/prusa-connect/sdk/routesapi"
	"github.com/prusa3d/prusa-connect/sdk/stateapi"
	"github.com/prusa3d/prusa-connect/sdk/wire"
	"github.com/prusa3d/prusa-connect/sdk/wirerpc"
)

// What this child reports when the host asks it what it is.
const (
	name    = "print-ai-gotchi"
	release = extension.Version("0.1.2")
)

//go:embed openapi.json
var openAPI []byte

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", name, err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	line, err := wire.Handshake{
		Protocol: wire.ProtocolMajor,
		MinMinor: wire.MinMinor,
		MaxMinor: wire.MaxMinor,
		Address:  wire.StdioAddress,
	}.Line()
	if err != nil {
		return fmt.Errorf("render the handshake: %w", err)
	}

	if _, err = os.Stdout.WriteString(line); err != nil {
		return fmt.Errorf("write the handshake: %w", err)
	}

	// The method set is filled after the session exists, because the pet
	// calls the host's fleet, bus and store on it.
	serves := wirerpc.Methods{}

	conn, err := wirerpc.NewConn(stdio{}, wirerpc.Options{Role: wirerpc.RoleChild, Methods: serves})
	if err != nil {
		return fmt.Errorf("build the session: %w", err)
	}

	devices := deviceapi.NewClient(conn)
	watcher := newWatcher(devices, func(ctx context.Context, p busapi.SubscribeParams, onWindow func(busapi.Window) error) error {
		return busapi.Subscribe(ctx, conn, p, onWindow)
	}, logf)
	brain := newBrain(logf)
	app := newApp(stateapi.NewClient(conn), devices, watcher, brain)

	defer brain.Close()

	maps.Copy(serves, methods(ctx, app, brain))

	fmt.Fprintf(os.Stderr, "%s %s is serving on stdio as pid %d\n", name, release, os.Getpid())

	if err = conn.Serve(ctx); err != nil {
		return fmt.Errorf("serve: %w", err)
	}

	return nil
}

// methods is the lifecycle set every child answers, plus the module's routes.
func methods(ctx context.Context, app *App, brain *Brain) wirerpc.Methods {
	var started sync.Once

	return wirerpc.Methods{
		wirerpc.MethodInfo: func(context.Context, wirerpc.Call) (any, error) {
			return routesapi.Describe(
				wirerpc.Info{
					Name:     name,
					Version:  release,
					Contract: wirerpc.Contract,
					Serves:   []wirerpc.Method{wirerpc.MethodConfigure},
				}, openAPI)
		},
		// The settings and the data directory, sent before the first request.
		// The eyes start on the first one, because they read the store and
		// the fleet on the session, which is serving by then.
		wirerpc.MethodConfigure: wirerpc.ConfigureGranted(func(_ context.Context, sent Settings, grants wirerpc.Grants) error {
			if err := app.Configure(sent, grants.Data); err != nil {
				return err
			}

			started.Do(func() { go app.Start(ctx) })

			return nil
		}),
		wirerpc.MethodShutdown: func(context.Context, wirerpc.Call) (any, error) {
			fmt.Fprintf(os.Stderr, "%s was asked to stop\n", name)
			brain.Close()

			return nil, nil
		},
		routesapi.MethodServe: routesapi.Serve(app.routes()),
	}
}

// stdio is the pair the host handed over.
type stdio struct{}

func (stdio) Read(p []byte) (int, error)  { return os.Stdin.Read(p) }
func (stdio) Write(p []byte) (int, error) { return os.Stdout.Write(p) }
func (stdio) Close() error                { return os.Stdin.Close() }
