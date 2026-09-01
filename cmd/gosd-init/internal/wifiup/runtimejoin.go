package wifiup

import (
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/jphastings/gosd/internal/wifictl"
)

// DefaultRequestPollInterval is how often the runtime-join watcher polls
// request.json for a new id — the same 2-3s marker-file cadence the rest of
// gosd-init's polling loops use (see cloudflared.DefaultNetworkUpPollInterval
// and its siblings).
const DefaultRequestPollInterval = 2 * time.Second

// runtimeJoinPassphraseLabel matches wifi.Join's own
// fault.RegisterSecretString label exactly (wifi/wifi.go's
// wifiPassphraseLabel), so a passphrase reaching both channels — the app's
// own registration and this belt-and-suspenders one — renders identically
// wherever a crash report ends up quoting it.
const runtimeJoinPassphraseLabel = "wifi-passphrase"

// credState is the "creds currently in effect" wifiup.Run's association
// loop and its runtime-join watcher share. Boot supplies the initial value,
// if any; each new join request replaces it, interrupting whatever the
// association loop is doing so it restarts with the new network right away.
// Once installed, creds stay current regardless of whether that attempt
// joined (epic gosd-ojbm decision 6 — no revert on failure).
type credState struct {
	mu      sync.Mutex
	creds   Credentials
	ok      bool
	outcome func(joined bool, reason string)
	changed chan struct{}
}

func newCredState() *credState {
	return &credState{changed: make(chan struct{})}
}

// set installs creds as current and wakes anything waiting on the changed
// channel current returned. outcome, if non-nil, is handed to the
// association loop that picks these creds up next, and is called at most
// once, reporting whether its first attempt joined (see
// runAssociationLoop's firstOutcome parameter).
func (s *credState) set(creds Credentials, outcome func(joined bool, reason string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.creds, s.ok, s.outcome = creds, true, outcome
	close(s.changed)
	s.changed = make(chan struct{})
}

// current returns the creds currently in effect, whether any are set at
// all, the outcome callback installed for them (nil for boot creds), and a
// channel that closes the next time set is called.
func (s *credState) current() (creds Credentials, ok bool, outcome func(joined bool, reason string), changed <-chan struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.creds, s.ok, s.outcome, s.changed
}

// mergedStop returns a channel that closes as soon as either a or b does,
// so runAssociationLoop can be interrupted by a genuine shutdown (opts.Stop)
// or by a runtime join request replacing the creds it's working with
// (credState's changed channel) on the same terms. Either input may be nil
// (opts.Stop is nil in production): select on a nil channel simply never
// fires for that case.
func mergedStop(a, b <-chan struct{}) <-chan struct{} {
	out := make(chan struct{})
	go func() {
		defer close(out)
		select {
		case <-a:
		case <-b:
		}
	}()
	return out
}

// watchRequests polls opts.RequestDir for a new wifi.Join request for the
// life of the device (epic decision 5: it runs even when boot found no
// credentials, and even with deps.Wifi nil — decision 8's honest "no WiFi
// interface" failure). Only called by Run when opts.RequestDir is set.
//
// Its poll timer uses real wall-clock time (time.After), not deps.Clock:
// this cadence is independent of the association loop's backoff/reconnect
// timing, which is what deps.Clock exists to drive deterministically in
// tests — coupling the two would make a test that parks the association
// loop on a never-advanced fake clock (to hold a connection "stable" while
// a later request interrupts it) also freeze the watcher. RequestPollInterval
// is directly overridable in tests instead.
func watchRequests(deps Deps, opts Options, state *credState) {
	interval := opts.RequestPollInterval
	if interval <= 0 {
		interval = DefaultRequestPollInterval
	}

	var lastGoodID, lastBadContent string
	for {
		req, ok, err := wifictl.ReadRequest(opts.RequestDir)
		switch {
		case err != nil:
			// Unparseable request: report it once, then ignore it until the
			// bytes actually change (self-healing, the gosd-6cf2 lesson —
			// req is zero-valued here, so there's no id to dedupe on).
			content, readErr := readRawRequest(opts.RequestDir)
			if readErr == nil && content == lastBadContent {
				break
			}
			lastBadContent = content
			deps.Log("runtime WiFi join request could not be read: %v", err)
			_ = wifictl.WriteStatus(opts.RequestDir, wifictl.Status{
				State: wifictl.Failed,
				Error: "request could not be parsed: " + err.Error(),
			})
		case ok && req.ID != lastGoodID:
			lastGoodID = req.ID
			lastBadContent = ""
			handleRequest(deps, opts.RequestDir, state, req)
		}

		select {
		case <-opts.Stop:
			return
		case <-time.After(interval):
		}
	}
}

// readRawRequest reads request.json's raw bytes, used only to tell whether
// an unparseable file has changed since the last poll (wifictl.ReadRequest
// itself reports only that a file failed to parse, not its content).
func readRawRequest(dir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(dir, wifictl.RequestFile))
	return string(data), err
}

// handleRequest reconciles one runtime join request: writes "joining",
// registers the passphrase for redaction, resolves the credentials, and —
// unless the board has no WiFi interface at all or the credentials
// themselves don't resolve — hands them to the association loop for one
// bounded attempt (via credState.set), reporting "joined" or "failed" from
// its outcome callback. Never blocks: the callback fires later, from
// whichever goroutine is running the association loop.
func handleRequest(deps Deps, dir string, state *credState, req wifictl.Request) {
	if req.SSID == "" {
		deps.Log("runtime WiFi join request %s has no SSID", req.ID)
		_ = wifictl.WriteStatus(dir, wifictl.Status{ID: req.ID, State: wifictl.Failed, Error: "SSID is required"})
		return
	}
	if req.Passphrase != "" && deps.RegisterSecret != nil {
		deps.RegisterSecret(req.Passphrase, runtimeJoinPassphraseLabel)
	}

	deps.Log("runtime WiFi join requested: %q", req.SSID)
	_ = wifictl.WriteStatus(dir, wifictl.Status{ID: req.ID, State: wifictl.Joining})

	if deps.Wifi == nil {
		deps.Log("runtime WiFi join to %q failed: no WiFi interface", req.SSID)
		_ = wifictl.WriteStatus(dir, wifictl.Status{ID: req.ID, State: wifictl.Failed, Error: "no WiFi interface"})
		return
	}

	creds, err := resolveCredentials(req.SSID, req.Passphrase)
	if err != nil {
		deps.Log("runtime WiFi join to %q failed: %v", req.SSID, err)
		_ = wifictl.WriteStatus(dir, wifictl.Status{ID: req.ID, State: wifictl.Failed, Error: err.Error()})
		return
	}

	state.set(creds, func(joined bool, reason string) {
		if !joined {
			deps.Log("runtime WiFi join to %q failed: %s", req.SSID, reason)
			_ = wifictl.WriteStatus(dir, wifictl.Status{ID: req.ID, State: wifictl.Failed, Error: reason})
			return
		}

		deps.Log("runtime WiFi join to %q succeeded", req.SSID)
		_ = wifictl.WriteStatus(dir, wifictl.Status{ID: req.ID, State: wifictl.Joined})

		if req.Persist {
			if deps.Persist == nil {
				deps.Log("runtime WiFi join to %q succeeded but this image can't persist it", req.SSID)
			} else if err := deps.Persist(req.SSID, req.Passphrase); err != nil {
				deps.Log("persisting the WiFi network %q failed: %v", req.SSID, err)
			}
		}

		if deps.RestartIngress != nil {
			deps.RestartIngress()
		}
	})
}
