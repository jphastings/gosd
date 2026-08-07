# Ingress: exposing an app to the internet (`gosd build --ingress cloudflared`)

`gosd build --ingress cloudflared` bakes a [Cloudflare
Tunnel](https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/)
client into the image. `gosd-init` supervises it for the life of the device
and it carries traffic for one public hostname to one local port on your
app — no port forwarding, no public IP address, and no app code at all. v1
is deliberately narrow: one locally-managed tunnel, declared entirely
through `gosd.toml`, with the ingress rule fixed at build-time-independent
runtime resolution (there's no `--ingress-token` build flag — unlike
`--wifi-ssid`/`--wifi-pass`, nothing here is baked into the image itself).

Remote/dashboard-managed tunnels and quick tunnels are out of scope for v1
(see "Why a CLI-created tunnel" below).

## Runbook

These steps run on your own machine, with the
[`cloudflared`](https://github.com/cloudflare/cloudflared) CLI installed —
they have nothing to do with the device itself, which only ever sees the
resulting token.

1. **Build the image with the binary baked in:**

   ```sh
   gosd build . --board pi-zero-2w --ingress cloudflared -o my-app.img
   ```

2. **Log in and create the tunnel** (once per tunnel):

   ```sh
   cloudflared tunnel login
   cloudflared tunnel create my-device
   ```

3. **Print the tunnel's token:**

   ```sh
   cloudflared tunnel token my-device
   ```

   This prints one long base64 string — copy it. (`cloudflared tunnel
   create` also drops a credentials JSON file in `~/.cloudflared/`; GoSD
   never uses that file. Only the token string matters — see "No
   credentials file" below.)

4. **Paste the token, hostname, and port into `gosd.toml`** on the
   `GOSD-BOOT` partition (or hand-edit the commented-out example the image
   already ships, per `docs/provisioning-formats.md`):

   ```toml
   [ingress.cloudflared]
   token = "eyJhIjoiPGFjY291bnQ+IiwidCI6IjxpZD4iLCJzIjoiPHNlY3JldD4ifQ=="
   hostname = "my-device.example.com"
   port = 8080
   ```

   All three keys are required. Restart the device (or power it on for the
   first time) for the change to take effect.

5. **Point DNS at the tunnel:**

   ```sh
   cloudflared tunnel route dns my-device my-device.example.com
   ```

Once the device boots with a network connection, `gosd-init` reads the
token, hostname, and port; decodes the token; writes cloudflared's runtime
config; and starts the tunnel. `my-device.example.com` starts serving
whatever your app answers on `:8080`.

### Why a CLI-created tunnel

Only tunnels created with the `cloudflared` CLI (steps 2-3 above) are
supported. A tunnel created from the Cloudflare Zero Trust dashboard
instead is **remote-managed**: its ingress rules live at Cloudflare's edge,
and cloudflared fetches them from there rather than reading a local config
file — which would make the `[ingress.cloudflared]` hostname/port GoSD
writes into `config.yml` on the device pointless, or worse, silently
ignored in favor of whatever the dashboard says. GoSD v1 only ever writes a
local, locally-managed `config.yml` (see "What gets written on the device"
below), so a dashboard-created tunnel's actual behavior against it hasn't
been bench-verified end-to-end yet — that characterization is pending bench
bean `gosd-igk0`. Stick to CLI-created tunnels until that's recorded.

A token that decodes fine but has no `hostname`/`port` filled in on the
device (the shape a dashboard-created tunnel's token would arrive in) is
refused with an actionable log line rather than silently attempted — see
"Troubleshooting" below.

## What gets written on the device

Nothing about the tunnel lives on `GOSD-BOOT` except what you typed into
`gosd.toml`. At boot, once the network is up, `gosd-init` decodes the token
and synthesizes cloudflared's own two config files fresh, in RAM, under
`/run/gosd/cloudflared/` (mode `0700` directory, `0600` files):

- `credentials.json` — the token's three decoded fields (`AccountTag`,
  `TunnelSecret`, `TunnelID`), under the same field names
  `cloudflared tunnel token --cred-file` would write.
- `config.yml`:

  ```yaml
  tunnel: <tunnel-id>
  credentials-file: /run/gosd/cloudflared/credentials.json
  ingress:
    - hostname: my-device.example.com
      service: http://localhost:8080
    - service: http_status:404
  ```

Both files are rebuilt from `gosd.toml` on every boot — never read back,
never persisted outside `/run`'s tmpfs. `cloudflared` runs as
`tunnel --no-autoupdate --loglevel warn --config /run/gosd/cloudflared/config.yml run`,
with `HOME=/run/gosd/cloudflared` so its own `~/.cloudflared` probing
resolves somewhere writable instead of a nonexistent home directory.

### No credentials file on `GOSD-BOOT`

A Cloudflare Tunnel is normally authorized by a credentials JSON file
(`{"AccountTag", "TunnelSecret", "TunnelID"}`) sitting next to
`config.yml`. GoSD never ships one: the tunnel token you pasted into
`gosd.toml` **is** that same triple, base64-encoded — `gosd-init` decodes
it itself at boot and writes the credentials file into RAM, not onto the
card. There is nothing to distribute, back up, or leak from `GOSD-BOOT`
beyond the token already sitting in `gosd.toml`.

## Secrets on a FAT partition

The tunnel token sits in `gosd.toml` in plain text on `GOSD-BOOT`, a FAT32
partition readable by anyone with the card in a computer — the same trust
level the WiFi passphrase already has there today. Treat it accordingly:
anyone who can read the card can read the token (and, with it, take over
the tunnel), the same way anyone who can read the card today can read your
WiFi password. This is a deliberate consequence of the "no credentials
file, hand-editable `gosd.toml`" design (epic `gosd-virc`, decision 3), not
an oversight.

## Clock and TLS

No GoSD board has a battery-backed real-time clock (see
[`docs/runtime.md`'s "Clock" section](runtime.md#clock-starts-at-1970-until-sntp-syncs)) —
the clock starts at the Unix epoch on every boot and only becomes correct
once SNTP sync completes, which happens after networking comes up.
cloudflared's QUIC connection to the Cloudflare edge is TLS, and a wildly
wrong clock breaks certificate validity checks.

`gosd-init` doesn't block ingress on a perfect clock:

- It waits for the network-up marker first (parking indefinitely if the
  network never comes up — there's no point starting cloudflared before
  there's a network for it to use).
- It then waits up to **2 minutes** for the time-synced marker
  (`/run/gosd/time-synced`), and starts cloudflared anyway if that window
  elapses without a sync. The build's baked timestamp (a clock floor
  `timesync` refuses to step backward past) keeps TLS validity mostly
  correct even before NTP has actually landed, and cloudflared's own
  reconnect logic — plus this package's own restart backoff — absorbs
  whatever's left once the clock does settle.

A board with a real RTC would shrink this window further by giving the
clock a roughly-correct starting point before the network even comes up;
that's tracked separately as epic `gosd-achn` and isn't required for
ingress to work today.

## Pinned version and updates

The `cloudflared` binary is pinned by upstream release tag and SHA-256 in
`internal/cloudflaredpin` — never re-hosted, matching GoSD's policy for
every other third-party binary blob (Pi firmware, Rockchip `rkbin`). It
runs with `--no-autoupdate` always: cloudflared never checks for or
installs its own updates on the device.

The only way to move to a newer `cloudflared` is:

1. Wait for (or ask JP for) a new `gosd` CLI release with a bumped
   `cloudflaredpin.Version`.
2. Rebuild your image with that CLI (`gosd build`).
3. Reflash the device.

There's no in-place update path for the binary itself — this mirrors how
every other baked artifact (kernel, U-Boot) updates in GoSD today.

## The metrics listener

cloudflared opens its own Prometheus-style metrics HTTP endpoint by
default, bound to `localhost` only (its own `--metrics` flag, which
`gosd-init` doesn't override, so cloudflared's own default applies). This
is entirely cloudflared's doing, not something `gosd-init` configures,
documents the port of, or is responsible for keeping running — it exists
on the device's loopback interface only, alongside `gosd-init`'s own
policy of adding no listeners itself. It is not reachable from the tunnel
or from the network; the ingress rule in `config.yml` forwards only to the
one `http://localhost:<port>` your app declared.

## Surviving a reflash

Reflashing rewrites the whole of `GOSD-BOOT`, `gosd.toml` included. Like
the hostname, WiFi network, and `[env]` values, a hand-edited
`[ingress.cloudflared]` section is protected by the [provisioning
snapshot](runtime.md#the-provisioning-snapshot-surviving-a-reflash): once
it settles on a successful boot, `gosd-init` snapshots it to `/data`, and
restores it into the freshly flashed card's `gosd.toml` on the first boot
after a reflash, provided the new card doesn't already declare its own
ingress section (which always wins, as fresh operator intent).

Unlike hostname/WiFi/`[env]`, which are restored field-by-field or
key-by-key, `[ingress.cloudflared]` is restored as a whole section — there
being no baked default to compare individual fields against (config.json
only ever records *whether* the binary is baked, never a real token), any
snapshotted section at all counts as the operator's own intent. Because
there's no credentials file to lose, this works exactly like the WiFi
passphrase already does: the token round-trips through `/data` and the
tunnel reconnects on the next boot without you touching the card at all.
This needs `--data-size=expand` (or any non-zero `--data-size`) — a card
with no data partition has nothing to snapshot to, same as every other
provisioning value.

## Troubleshooting

`gosd-init` validates `[ingress.cloudflared]` once, at boot, and logs
exactly one line (prefixed `cloudflared: `) if something's wrong — it
never re-checks or self-heals a bad value later in the same boot. These
are the actual messages, verbatim, from
`cmd/gosd-init/internal/cloudflared/mode.go`:

| Situation | Log line |
|---|---|
| Binary baked, nothing configured | `cloudflared: binary is baked into this image, but [ingress.cloudflared] isn't configured in gosd.toml; nothing to do` |
| Configured, binary not baked | `cloudflared: [ingress.cloudflared] is configured in gosd.toml, but this image wasn't built with --ingress cloudflared; rebuild with that flag to bake the binary in` |
| Token present, hostname/port missing (a dashboard-style token) | `cloudflared: [ingress.cloudflared] has a token but no hostname/port; remote mode not supported yet — add hostname and port to gosd.toml to run it locally-managed` |
| Any other missing key(s) | `cloudflared: [ingress.cloudflared] is missing required key(s): <list>` |
| Token doesn't decode | `` cloudflared: [ingress.cloudflared] token is not a valid Cloudflare Tunnel token (<error>); generate a fresh one with `cloudflared tunnel token <name>` or copy it from the Cloudflare dashboard — if this token used to work, a gosd update may be needed to support a new token format `` |
| Hostname isn't a valid FQDN | `cloudflared: [ingress.cloudflared] hostname "<hostname>" is not a valid fully-qualified hostname` |
| Port out of range | `cloudflared: [ingress.cloudflared] port <port> is out of range (must be 1-65535)` |

Nothing here is fatal to boot — your app still starts normally either way.
Check the serial console (115200 baud unless `--console-baud` says
otherwise) for these lines if the tunnel doesn't come up.

## What's not supported yet

- **Remote/dashboard-managed tunnels and quick tunnels** — v1 is
  locally-managed only (see "Why a CLI-created tunnel" above).
- **More than one hostname or port per device** — `config.yml` gets exactly
  one ingress rule plus the mandatory catch-all; route multiple services
  through your own app instead (a reverse proxy in front of them) if you
  need more than one public path.
- **Non-arm64 boards.** `pi-zero-w` cannot run `--ingress cloudflared` at
  all: cloudflared's official `arm` release is built for `GOARM=7`, and
  `pi-zero-w`'s BCM2835 is `armv6` — the binary faults with "illegal
  instruction" the moment it runs, and the GOARM level can't be recovered
  from the ELF header alone to catch this any earlier than a hard,
  per-board `--ingress cloudflared` build refusal. See
  `COMPATIBILITY.md`.
