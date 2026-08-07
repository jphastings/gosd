---
# gosd-uj36
title: Runtime module cmd/gosd-init/internal/cloudflared (unwired, fake-tested)
status: todo
type: task
priority: normal
created_at: 2026-08-07T12:52:10Z
updated_at: 2026-08-07T12:53:22Z
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

[ ] mode.go + tests (decode, validation, config.yml golden)
[ ] logwriter + tests (split/partial/overflow/independence)
[ ] Run loop + fakes (gates, backoff sequence 1,2,4..30, stable reset, Stop)
[ ] token-never-logged scan test
