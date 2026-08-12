---
# gosd-uj36
title: Runtime module cmd/gosd-init/internal/cloudflared (unwired, fake-tested)
status: completed
type: task
priority: normal
created_at: 2026-08-07T12:52:10Z
updated_at: 2026-08-07T17:29:26Z
parent: gosd-virc
blocked_by:
    - gosd-7upw
---

Ingress epic gosd-virc bean 3 (needs gosd-7upw's types; reviewable in
isolation — NOT yet wired into main.go, that is bean 4).

## Locked decisions

- Feature-module shape, NO build tags (exec works on macOS; mdnsresponder
  precedent). Files: cloudflared.go (Deps/Options/Run), mode.go (config
  resolution + validation + token decode + config.yml render), backoff.go,
  logwriter.go (line-splitting prefix writer — none exists in tree),
  interfaces.go (Clock), platform.go (real StartProcess via os/exec), fakes.
- Deps: `StartProcess(path, args, env, stdout, stderr) (pid, error)` (boot's
  AppStarter takes no argv — package-local seam); `Wait` = the PID-1 reaper's
  Wait, NEVER exec.Cmd.Wait (races the central wait4(-1) loop); NetworkUp /
  TimeSynced marker polls with paths injected (no netup/timesync imports —
  timesync precedent); file funcs; Clock; Log (every line "cloudflared: ...").
- Run(): resolve mode (pure; ALL misconfig → one actionable log line + return —
  gosd.toml is only re-read at boot, nothing self-heals) → wait network-up
  (2s poll, parks forever) → wait time-synced ≤2min then proceed with warning
  (clock floor = build timestamp keeps TLS mostly valid; backoff absorbs the
  rest) → decode token, write /run/gosd/cloudflared/credentials.json +
  config.yml (0600, dir 0700) → supervise loop: re-gate network-up before each
  start; log pid+argv (env NEVER — token lives there); Wait; backoff sleep.
- argv: `tunnel --no-autoupdate --loglevel warn --config
  /run/gosd/cloudflared/config.yml run`; env: HOME=/run/gosd/cloudflared.
- config.yml string-formatted (no YAML lib), injection-safe via validation:
  strict FQDN regex, port 1..65535. `tunnel: <t>`, `credentials-file:`, rule
  hostname → http://localhost:<port>, mandatory catch-all `http_status:404`
  (cloudflared refuses a config without one).
- Token decode: base64 (accept std/url, padded/raw) JSON with a/s/t; tolerate
  unknown fields; missing field → actionable error naming `cloudflared tunnel
  token <name>` / the dashboard, hinting a gosd update if the format changed.
- Restart policy: base 1s, cap 30s (auxiliary — ≤2 lines/min when permanently
  broken), reset after 30s stable, NO jitter (single process; cloudflared has
  internal edge-reconnect jitter). NO restart on network change — cloudflared
  holds 4 redundant edge connections and reconnects itself; if it exits, the
  pre-start network re-gate parks the loop instead of burning backoff.
- Failure modes (each ONE actionable line + return, never boot failure):
  unconfigured+baked → one quiet line; configured+not-baked → points at
  --ingress; bad token; missing hostname/port → names exact keys; token-only →
  "remote mode not supported yet". Token value never in any output — tests
  scan all captured log for it.
- logwriter: two independent instances (stdout/stderr), buffer to \n, 4KiB cap
  with truncation note, prefix "cloudflared: ".

## Todos

[x] mode.go + tests (decode, validation, config.yml golden)
[x] logwriter + tests (split/partial/overflow/independence)
[x] Run loop + fakes (gates, backoff sequence 1,2,4..30, stable reset, Stop)
[x] token-never-logged scan test



## Summary of Changes

Implemented `cmd/gosd-init/internal/cloudflared/` exactly per the locked
decisions, unwired (no changes to main.go/boot/any other package):

- `cloudflared.go`: `Deps`/`Options`/`Run`. `Run` resolves the mode (pure),
  gates on network-up (2s poll, parks forever), gates on time-synced (up to 2 minutes,
  then proceeds with a warning), writes the runtime files, then supervises
  cloudflared: re-gating network-up before every single start (parking, not
  backing off, if the network is down — cloudflared holds its own redundant
  edge connections), logging pid+argv only (never env), and resetting backoff
  after a StableAfter-length run. Fixed argv/env per the bean
  (`tunnel --no-autoupdate --loglevel warn --config .../config.yml run`,
  `HOME=/run/gosd/cloudflared`). `Deps.Wait`'s doc comment documents that
  production must wire `boot.Platform.Reaper.Wait`, never `exec.Cmd.Wait`,
  which would race the reaper's central wait4(-1) loop.
- `mode.go`: `resolveMode` (pure — no I/O) covers every locked failure mode
  (unconfigured/baked cross-product, missing-key(s) naming exact keys,
  token-only -> "remote mode not supported yet", bad token, invalid hostname,
  out-of-range port) each producing at most one actionable log line; token
  decode tries all four base64 alphabets and tolerates unknown JSON fields;
  `credentialsJSON`/`configYAML` render the two runtime files by hand
  (no YAML lib), safe against injection because hostname/port are validated
  (strict FQDN regex, 1-65535) before a run=true result is ever produced.
- `backoff.go`: 1s/30s/no-jitter backoff, same doubling-with-cap shape as
  `boot.Backoff`, duplicated per this package's no-cross-imports convention.
- `logwriter.go`: mutex-guarded line-splitting prefix writer ("cloudflared: "),
  4KiB soft cap with a truncation note, `Close` flushes a trailing partial
  line. The mutex exists because `StartProcess` never calls `cmd.Wait`
  (by design), so os/exec's own stdout/stderr-copying goroutine can still be
  writing into a lineWriter after `Deps.Wait` returns and `runOnce` calls
  `Close` — caught by `go test -race`, not merely theoretical.
- `interfaces.go`: `Clock` (Now/After), duplicated per precedent.
- `platform.go`: real `StartProcess` via `os/exec`, no build tags (starting a
  process needs no Linux-only syscall, so it compiles and genuinely runs on
  macOS too, exercised for real in `platform_test.go`).
- Full fake-driven test suite (`*_test.go`): mode resolution table covering
  every failure row plus the happy path; logwriter split/partial/overflow/
  independence/Close; backoff sequence 1,2,4,8,16,30,30 and reset; Run's
  network-up and time-synced gates (including Stop-closes-while-waiting);
  supervise's exact argv/env, escalating-backoff sequence, stable-reset, and
  network re-gate-before-restart; a real end-to-end write of credentials.json/
  config.yml through fake file funcs, byte-for-byte matched against the
  golden content; a token-never-appears-in-any-log-output scan across every
  failure mode plus a full runOnce with relayed stdout/stderr; and a real
  StartProcess integration test (env isolation from the parent process,
  missing-binary error).

Gates: `go test ./cmd/gosd-init/internal/cloudflared/...` (including
`-race`), `go test ./...`, `go vet ./...`, `gofmt -l .`, and
`golangci-lint run ./...` both native and `GOOS=linux` are all clean.
An initial `go test ./...` run hit `no space left on device` in two
unrelated packages (`internal/build`, `internal/diskfmt`) from concurrent
sibling agents on the shared machine; a re-run once disk freed up passed
every package.
