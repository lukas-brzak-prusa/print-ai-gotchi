package main

// The local brain: a pinned llama.cpp server and the pinned models of
// models.go, downloaded into the module's data directory when a person asks,
// checked against their digests, and run as a child of this process on the
// loopback address only.
//
// Nothing is downloaded until a person asks for it on the page, because a
// model is a large file and the install must not cost anybody a gigabyte they
// did not choose.

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prusa3d/prusa-connect/sdk/wire"
)

// llamaTag is the llama.cpp release the runtime is pinned to.
const llamaTag = "b12329"

// runtimes is the pinned llama.cpp CPU build for each platform this module
// ships for. The Linux builds need glibc 2.38 or newer: Raspberry Pi OS on
// Debian 13 has it, an older system sets llama_server to its own build.
//
//nolint:gochecknoglobals // a closed table of pins.
var runtimes = map[string]pinned{
	"darwin_arm64": {URL: llamaURL("macos-arm64"), SHA256: "b4d0176d274c576d1f912d6eede6ff67176b4e205af52fa6400b30f55d8f5446", Size: 16910932},
	"darwin_amd64": {URL: llamaURL("macos-x64"), SHA256: "94cb0471d03b8fff96ef8a045573b4a024f719ee77ff4c7d5458bf63cd104171", Size: 12659278},
	"linux_arm64":  {URL: llamaURL("ubuntu-arm64"), SHA256: "bc7aad93c5032d44fd01be48b42145d1fa217e084aedc6651a3601b820c4593b", Size: 18963709},
	"linux_amd64":  {URL: llamaURL("ubuntu-x64"), SHA256: "e77061f0485d685e57d73ebf2f744066566aaa711a671441bb7bf5d31e1cc21b", Size: 23329647},
}

func llamaURL(platform string) string {
	return "https://github.com/ggml-org/llama.cpp/releases/download/" + llamaTag +
		"/llama-" + llamaTag + "-bin-" + platform + ".tar.gz"
}

func platform() string { return runtime.GOOS + "_" + runtime.GOARCH }

// The states of the local server.
const (
	brainAbsent   = "absent"   // the chosen model is not on disk
	brainStarting = "starting" // the server is loading the model
	brainReady    = "ready"    // answering
	brainFailed   = "failed"   // see Error
	brainOff      = "off"      // another provider is chosen
	brainNoData   = "no_data"  // the operator did not grant a data directory
)

// Download is the one download the brain is doing, or did last.
type Download struct {
	Model string `json:"model"`
	// Step is runtime or model.
	Step  string `json:"step"`
	Done  int64  `json:"done"`
	Total int64  `json:"total"`
	Error string `json:"error,omitempty"`
	// Running is false once it ended, well or not.
	Running bool `json:"running"`
}

// LocalStatus is what the local brain is doing.
type LocalStatus struct {
	State string `json:"state"`
	Model string `json:"model"`
	Error string `json:"error,omitempty"`
	// Installed is every model on disk.
	Installed []string  `json:"installed"`
	Download  *Download `json:"download,omitempty"`
	// Platform is GOOS_GOARCH, and Supported says a runtime exists for it.
	Platform  string `json:"platform"`
	Supported bool   `json:"supported"`
	// Budget is the memory the operator gave the module, 0 when not known.
	Budget int64 `json:"budget"`
}

// Brain owns the local llama-server process and everything it needs.
type Brain struct {
	mu         sync.Mutex
	settings   Settings
	dir        string // the module's data directory, empty when not granted
	model      string // the model the server should run, empty when off
	state      string
	err        string
	socket     string             // the unix socket the running server answers on
	sockets    string             // the private directory the sockets are made in
	stop       context.CancelFunc // stops the server's supervisor
	download   *Download
	cancelDL   context.CancelFunc
	local      *http.Client // dials the running server's socket, nil while none runs
	downloads  *http.Client // fetches the runtime and the models from the internet
	log        func(format string, args ...any)
	generation int
}

func newBrain(log func(string, ...any)) *Brain {
	// A download has no timeout of its own: a model is gigabytes on a slow
	// line, and Remove and Close cancel it through its context.
	return &Brain{log: log, state: brainNoData, downloads: &http.Client{}}
}

// Configure takes the operator's settings and the data directory.
func (b *Brain) Configure(s Settings, dir string) {
	b.mu.Lock()
	restart := b.settings.Threads != s.Threads || b.settings.LlamaServer != s.LlamaServer || b.dir != dir
	b.settings, b.dir = s, dir
	model := b.model
	b.mu.Unlock()

	if restart {
		b.run(model, true)
	}
}

// Use makes the server run model, or stops it when model is empty.
func (b *Brain) Use(model string) { b.run(model, false) }

func (b *Brain) run(model string, force bool) {
	b.mu.Lock()
	if model == b.model && !force && b.state != brainFailed {
		b.mu.Unlock()

		return
	}

	b.model = model
	b.mu.Unlock()

	b.halt()

	b.mu.Lock()
	defer b.mu.Unlock()

	switch {
	case model == "":
		b.state, b.err = brainOff, ""
	case b.dir == "":
		b.state, b.err = brainNoData, ""
	case !b.presentLocked(model):
		b.state, b.err = brainAbsent, ""
	default:
		b.state, b.err = brainStarting, ""
		b.launchLocked()
	}
}

// Status is a copy of what the local brain is doing.
func (b *Brain) Status() LocalStatus {
	b.mu.Lock()
	defer b.mu.Unlock()

	out := LocalStatus{State: b.state, Model: b.model, Error: b.err, Platform: platform()}
	_, out.Supported = runtimes[out.Platform]
	out.Supported = out.Supported || b.settings.LlamaServer != ""

	if budget, ok := wire.MemoryBudget(); ok {
		out.Budget = budget
	}

	if b.download != nil {
		d := *b.download
		out.Download = &d
	}

	if b.dir != "" {
		for _, key := range brainKeys() {
			if fileThere(b.modelPathLocked(key)) {
				out.Installed = append(out.Installed, key)
			}
		}
	}

	return out
}

// localBase is the base address of the local server. The host part is a
// label only: the client dials the server's unix socket, so no request to it
// ever touches the network.
const localBase = "http://llama-server/v1"

// Endpoint is the base address of the running server and the client that
// reaches it, and false while it cannot answer.
func (b *Brain) Endpoint() (string, *http.Client, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.state != brainReady {
		return "", nil, false
	}

	return localBase, b.local, true
}

// socketClient is an HTTP client that reaches the server on its unix socket.
func socketClient(socket string) *http.Client {
	var dialer net.Dialer

	return &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, "unix", socket)
		},
	}}
}

// Install downloads the runtime if it is missing and the model, in the
// background. When the model is the one chosen, the server starts after.
func (b *Brain) Install(model string) error {
	if _, ok := brains[model]; !ok {
		return fmt.Errorf("%w: there is no local model %q", errBadChoice, model)
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	switch {
	case b.dir == "":
		return errNoDataDir
	case b.download != nil && b.download.Running:
		return errDownloadBusy
	}

	if _, ok := runtimes[platform()]; !ok && b.settings.LlamaServer == "" {
		return errUnsupportedPlatform
	}

	ctx, cancel := context.WithCancel(context.Background())
	b.cancelDL = cancel
	b.download = &Download{Model: model, Running: true}

	go func() {
		defer cancel()

		err := b.fetch(ctx, model)

		b.mu.Lock()
		b.download.Running = false
		if err != nil {
			b.download.Error = err.Error()
		}
		chosen := b.model == model
		b.mu.Unlock()

		if err != nil {
			b.log("the model %s could not be installed: %v", model, err)

			return
		}

		if chosen {
			b.run(model, true)
		}
	}()

	return nil
}

// Remove deletes a model from disk. The running one is stopped first.
func (b *Brain) Remove(model string) error {
	if _, ok := brains[model]; !ok {
		return fmt.Errorf("%w: there is no local model %q", errBadChoice, model)
	}

	b.mu.Lock()
	if b.download != nil && b.download.Running && b.download.Model == model {
		b.mu.Unlock()

		return errDownloadBusy
	}

	path, running := b.modelPathLocked(model), b.model == model
	b.mu.Unlock()

	if running {
		b.halt()

		b.mu.Lock()
		b.state = brainAbsent
		b.mu.Unlock()
	}

	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	return nil
}

// Close stops a download and the server. The module calls it on shutdown.
func (b *Brain) Close() {
	b.mu.Lock()
	if b.cancelDL != nil {
		b.cancelDL()
	}
	b.mu.Unlock()

	b.halt()

	b.mu.Lock()
	sockets := b.sockets
	b.sockets = ""
	b.mu.Unlock()

	if sockets != "" {
		_ = os.RemoveAll(sockets)
	}
}

// The paths of what the brain keeps. The caller holds mu.

func (b *Brain) runtimeDirLocked() string {
	return filepath.Join(b.dir, "brain", "llama-"+llamaTag+"-"+platform())
}

func (b *Brain) modelPathLocked(model string) string {
	return filepath.Join(b.dir, "brain", "models", brains[model].File)
}

func (b *Brain) serverPathLocked() string {
	if b.settings.LlamaServer != "" {
		return b.settings.LlamaServer
	}

	return filepath.Join(b.runtimeDirLocked(), "llama-server")
}

func (b *Brain) presentLocked(model string) bool {
	return fileThere(b.serverPathLocked()) && fileThere(b.modelPathLocked(model))
}

func fileThere(path string) bool {
	info, err := os.Stat(path)

	return err == nil && !info.IsDir()
}

// fetch downloads and unpacks whatever is missing.
func (b *Brain) fetch(ctx context.Context, model string) error {
	b.mu.Lock()
	server, runtimeDir, modelPath := b.serverPathLocked(), b.runtimeDirLocked(), b.modelPathLocked(model)
	custom := b.settings.LlamaServer != ""
	b.mu.Unlock()

	if !fileThere(server) {
		if custom {
			return fmt.Errorf("llama_server names %s, which cannot be read", server)
		}

		archive := runtimeDir + ".tar.gz"
		if err := b.fetchFile(ctx, "runtime", runtimes[platform()], archive); err != nil {
			return err
		}

		err := extract(archive, runtimeDir)
		_ = os.Remove(archive)

		if err != nil {
			return fmt.Errorf("unpack the llama.cpp runtime: %w", err)
		}
	}

	if fileThere(modelPath) {
		return nil
	}

	return b.fetchFile(ctx, "model", brains[model].pinned, modelPath)
}

// fetchFile writes one pinned file to dest, checking its size and digest
// before it takes the final name.
func (b *Brain) fetchFile(ctx context.Context, step string, file pinned, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o750); err != nil {
		return err
	}

	b.progress(step, 0, file.Size)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, file.URL, nil)
	if err != nil {
		return err
	}

	req.Header.Set("User-Agent", name+"/"+string(release))

	resp, err := b.downloads.Do(req)
	if err != nil {
		return fmt.Errorf("download the %s: %w", step, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download the %s: %s answered %s", step, req.URL.Host, resp.Status)
	}

	part := dest + ".part"

	out, err := os.OpenFile(part, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o640)
	if err != nil {
		return err
	}

	hash := sha256.New()
	counted := &counter{brain: b, step: step, total: file.Size}

	_, err = io.Copy(io.MultiWriter(out, hash, counted), io.LimitReader(resp.Body, file.Size+1))
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}

	if err == nil && counted.done != file.Size {
		err = fmt.Errorf("the %s is %d bytes, and the pin says %d", step, counted.done, file.Size)
	}

	if got := hex.EncodeToString(hash.Sum(nil)); err == nil && got != file.SHA256 {
		err = fmt.Errorf("the %s has the digest %s, and the pin says %s", step, got, file.SHA256)
	}

	if err != nil {
		_ = os.Remove(part)

		return err
	}

	return os.Rename(part, dest)
}

func (b *Brain) progress(step string, done, total int64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.download != nil {
		b.download.Step, b.download.Done, b.download.Total = step, done, total
	}
}

// counter counts the bytes of one download into the status.
type counter struct {
	brain     *Brain
	step      string
	done      int64
	total     int64
	published time.Time
}

func (c *counter) Write(chunk []byte) (int, error) {
	c.done += int64(len(chunk))

	if now := time.Now(); now.Sub(c.published) > 250*time.Millisecond || c.done == c.total {
		c.published = now
		c.brain.progress(c.step, c.done, c.total)
	}

	return len(chunk), nil
}

// extract unpacks the llama.cpp archive into dir. Every entry lands directly
// in dir, under its base name, since the archive keeps everything in one
// folder. A link may only point at a sibling, so nothing leaves dir.
func extract(archive, dir string) error {
	file, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer file.Close()

	gz, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gz.Close()

	staging := dir + ".staging"
	_ = os.RemoveAll(staging)

	if err := os.MkdirAll(staging, 0o750); err != nil {
		return err
	}

	reader := tar.NewReader(gz)

	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return err
		}

		base := filepath.Base(header.Name)
		if base == "." || base == "/" || base == ".." || strings.ContainsAny(base, `/\`) {
			continue
		}

		target := filepath.Join(staging, base)

		switch header.Typeflag {
		case tar.TypeReg:
			if err := writeFile(target, reader, header.Size, os.FileMode(header.Mode)&0o755); err != nil {
				return err
			}
		case tar.TypeSymlink:
			if strings.ContainsAny(header.Linkname, `/\`) || header.Linkname == ".." {
				return fmt.Errorf("the archive links %s outside its folder", header.Name)
			}

			if err := os.Symlink(header.Linkname, target); err != nil {
				return err
			}
		}
	}

	if _, err := os.Stat(filepath.Join(staging, "llama-server")); err != nil {
		return errors.New("the archive holds no llama-server")
	}

	_ = os.RemoveAll(dir)

	return os.Rename(staging, dir)
}

func writeFile(path string, from io.Reader, size int64, mode os.FileMode) error {
	out, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode|0o600)
	if err != nil {
		return err
	}

	if _, err = io.CopyN(out, from, size); err != nil {
		_ = out.Close()

		return err
	}

	return out.Close()
}

// launchLocked starts the server under a supervisor that restarts it a few
// times when it dies. The caller holds mu.
func (b *Brain) launchLocked() {
	ctx, cancel := context.WithCancel(context.Background())
	b.stop = cancel
	b.generation++

	go b.supervise(ctx, b.generation)
}

const maxRestarts = 3

func (b *Brain) supervise(ctx context.Context, generation int) {
	for attempt := 0; ; attempt++ {
		err := b.runOnce(ctx, generation)
		if ctx.Err() != nil {
			return
		}

		b.log("the local brain stopped: %v", err)

		b.mu.Lock()
		if attempt >= maxRestarts {
			b.state, b.err = brainFailed, err.Error()
			b.mu.Unlock()

			return
		}

		b.state, b.err = brainStarting, err.Error()
		b.mu.Unlock()

		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Duration(attempt+1) * 2 * time.Second):
		}
	}
}

// runOnce runs llama-server until it exits or ctx ends.
func (b *Brain) runOnce(ctx context.Context, generation int) error {
	socket, err := b.socketPath(generation)
	if err != nil {
		return err
	}

	b.mu.Lock()
	server, model, threads := b.serverPathLocked(), b.modelPathLocked(b.model), b.settings.Threads
	extra := brains[b.model].Args
	b.mu.Unlock()

	dir := filepath.Dir(server)
	args := []string{
		"--model", model,
		// A path ending in .sock makes llama-server listen on a unix socket
		// instead of a port, so nothing else on the machine or the network
		// can reach it.
		"--host", socket,
		"--ctx-size", "4096",
		"--parallel", "1",
	}

	if threads > 0 {
		args = append(args, "--threads", strconv.Itoa(threads))
	}

	args = append(args, extra...)

	//nolint:gosec // the path is the pinned runtime or the operator's own setting.
	cmd := exec.CommandContext(ctx, server, args...)
	cmd.Env = serverEnv(dir)
	cmd.Dir = dir

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", server, err)
	}

	client := socketClient(socket)

	b.mu.Lock()
	b.socket, b.local = socket, client
	b.mu.Unlock()

	tail := &lastLines{max: 8}

	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			tail.add(scanner.Text())
		}
	}()

	go b.waitHealthy(ctx, client, socket, generation)

	err = cmd.Wait()

	if ctx.Err() != nil {
		return nil
	}

	return fmt.Errorf("llama-server exited (%v): %s", err, tail.String())
}

// serverEnv is the whole environment llama-server gets: where to find its
// own libraries and the few variables a program expects. Nothing else of the
// module's environment reaches it.
func serverEnv(libraries string) []string {
	env := []string{"LD_LIBRARY_PATH=" + libraries, "DYLD_LIBRARY_PATH=" + libraries}

	for _, key := range []string{"PATH", "HOME", "TMPDIR"} {
		if value, ok := os.LookupEnv(key); ok {
			env = append(env, key+"="+value)
		}
	}

	return env
}

// waitHealthy polls /health until the model is loaded.
func (b *Brain) waitHealthy(ctx context.Context, client *http.Client, socket string, generation int) {
	url := strings.TrimSuffix(localBase, "/v1") + "/health"

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(500 * time.Millisecond):
		}

		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)

		resp, err := client.Do(req)
		if err != nil {
			continue
		}

		_ = resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			b.mu.Lock()
			if b.generation == generation && b.socket == socket {
				b.state, b.err = brainReady, ""
			}
			model := b.model
			b.mu.Unlock()
			b.log("the local brain is ready with %s", model)

			return
		}
	}
}

// halt stops the server.
func (b *Brain) halt() {
	b.mu.Lock()
	stop := b.stop
	b.stop, b.socket, b.local = nil, "", nil
	b.generation++
	b.mu.Unlock()

	if stop != nil {
		stop()
	}
}

// socketPath is a fresh socket for one run of the server, in a directory
// only this process may enter. It is under the system's temporary directory
// rather than the data directory, because a unix socket path is limited to
// about a hundred bytes and the data directory can be deeper than that.
func (b *Brain) socketPath(generation int) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.sockets == "" {
		dir, err := os.MkdirTemp("", "print-ai-gotchi-")
		if err != nil {
			return "", fmt.Errorf("make the brain's socket directory: %w", err)
		}

		b.sockets = dir
	}

	socket := filepath.Join(b.sockets, "brain-"+strconv.Itoa(generation)+".sock")
	_ = os.Remove(socket)

	return socket, nil
}

// lastLines keeps the end of what a child wrote, for the error a person reads.
type lastLines struct {
	mu    sync.Mutex
	max   int
	lines []string
}

func (l *lastLines) add(line string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.lines = append(l.lines, line)
	if len(l.lines) > l.max {
		l.lines = l.lines[len(l.lines)-l.max:]
	}
}

func (l *lastLines) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()

	return strings.Join(l.lines, " | ")
}
