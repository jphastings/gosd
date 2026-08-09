# Ingress: exposing an app to the internet (`gosd build --ingress`)

`gosd build --ingress <agent>` bakes a client into the image that exposes an
app on the device to the public internet with zero app code: no port
forwarding, no public IP address, and (depending on the agent) no account on
the device itself beyond a token or key you paste into `gosd.toml`.
`gosd-init` supervises the client for the life of the device — there is
still no shell, no SSH, and no listener gosd-init itself adds; the agent's
own outbound connection is the only new thing on the wire.

## Choosing an ingress

| Agent | `--ingress` value | Board support | Where TLS terminates | Account you need | Public URL | Surviving a reflash |
|---|---|---|---|---|---|---|
| Cloudflare Tunnel | `cloudflared` | arm64 boards only ([^armv6]) | Cloudflare's edge | A Cloudflare account (free tier works) | Your own domain, e.g. `app.example.com` | Token round-trips through `/data`; nothing to re-enter |
| Tailscale Funnel | `tailscale-funnel` | Every board, including `pi-zero-w` | On the device itself ([^tsfunnel-onnode]) | A Tailscale account (free plan works) | `https://<hostname>.<tailnet>.ts.net` | Node identity lives on `/data` — reconnects with **zero re-auth** |

[^armv6]: cloudflared's official `arm` release is built for `GOARM=7`, which
    faults with "illegal instruction" on `pi-zero-w`'s `armv6` CPU — see
    "What's not supported yet" in the Cloudflare Tunnel section below.

[^tsfunnel-onnode]: A Let's Encrypt certificate that Tailscale's control
    plane issues automatically (Tailscale ACME) — Tailscale's own relay
    infrastructure only ever routes the still-encrypted bytes, by TLS SNI,
    and never decrypts them. See the Tailscale Funnel section below.

Every agent shares the same shape, whichever one you pick:

- **What it is.** A binary baked into the image at build time
  (`--ingress <agent>`), supervised by `gosd-init` for the device's whole
  life, that carries traffic for a declared public hostname to a declared
  local port on your app.
- **Where TLS terminates** differs by agent, and matters for who can see
  your traffic in flight (see the table above for which is which):
  cloudflared decrypts at Cloudflare's edge and forwards to your device over
  its own tunnel protocol; Tailscale Funnel terminates TLS on the device
  itself, using a certificate Tailscale's control plane issues automatically
  — its relay infrastructure only ever routes the still-encrypted bytes, by
  TLS SNI, and never decrypts them. Either way, your app itself speaks plain
  HTTP on `localhost` — the agent is what makes that safe to expose.
  Terminating TLS yourself, end-to-end past either agent, isn't something
  either one supports.
- **Whose account you need** is the operator whose infrastructure carries
  your traffic — you authenticate the *tunnel* (or, for Tailscale Funnel,
  the device's tailnet membership), at build/config time, with a token or
  key from that account; the device itself never logs in anywhere
  interactively (it has no interactive surface at all — see
  `docs/runtime.md`).
- **Declared entirely through `gosd.toml`**, under `[ingress.<agent>]` — the
  ingress rule (hostname, local port, and whatever else that agent needs)
  is a runtime setting, never baked into the image itself, so the same image
  works for any tunnel you later create.
- **Survives a reflash** the same way a hand-edited WiFi passphrase does —
  see each agent's own "Surviving a reflash" section for the details.
  Tailscale Funnel goes one step further: its node identity lives on
  `/data` rather than in `gosd.toml` at all, so it survives a reflash with
  no re-authentication whatsoever — see that section for what this means in
  practice.

Per-agent sections follow below: Cloudflare Tunnel first, then Tailscale
Funnel.

## Cloudflare Tunnel (`gosd build --ingress cloudflared`)

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

### Runbook

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
   boot partition (or hand-edit the commented-out example the image
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

#### Why a CLI-created tunnel

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

### What gets written on the device

Nothing about the tunnel lives on the boot partition except what you typed into
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

#### No credentials file on the boot partition

A Cloudflare Tunnel is normally authorized by a credentials JSON file
(`{"AccountTag", "TunnelSecret", "TunnelID"}`) sitting next to
`config.yml`. GoSD never ships one: the tunnel token you pasted into
`gosd.toml` **is** that same triple, base64-encoded — `gosd-init` decodes
it itself at boot and writes the credentials file into RAM, not onto the
card. There is nothing to distribute, back up, or leak from the boot
partition beyond the token already sitting in `gosd.toml`.

### Secrets on a FAT partition

The tunnel token sits in `gosd.toml` in plain text on the boot partition, a FAT32
partition readable by anyone with the card in a computer — the same trust
level the WiFi passphrase already has there today. Treat it accordingly:
anyone who can read the card can read the token (and, with it, take over
the tunnel), the same way anyone who can read the card today can read your
WiFi password. This is a deliberate consequence of the "no credentials
file, hand-editable `gosd.toml`" design (epic `gosd-virc`, decision 3), not
an oversight.

### Clock and TLS

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

### Pinned version and updates

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

### The metrics listener

cloudflared opens its own Prometheus-style metrics HTTP endpoint by
default, bound to `localhost` only (its own `--metrics` flag, which
`gosd-init` doesn't override, so cloudflared's own default applies). This
is entirely cloudflared's doing, not something `gosd-init` configures,
documents the port of, or is responsible for keeping running — it exists
on the device's loopback interface only, alongside `gosd-init`'s own
policy of adding no listeners itself. It is not reachable from the tunnel
or from the network; the ingress rule in `config.yml` forwards only to the
one `http://localhost:<port>` your app declared.

### Surviving a reflash

Reflashing rewrites the whole of the boot partition, `gosd.toml` included. Like
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

### Troubleshooting

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

### What's not supported yet

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

## Tailscale Funnel (`gosd build --ingress tailscale-funnel`)

`gosd build --ingress tailscale-funnel` bakes a tiny
[tsnet](https://tailscale.com/kb/1244/tsnet)-based shim, `gosd-tsfunnel`,
into the image — compiled from GoSD's own source for every board's
architecture (including `pi-zero-w`'s `GOARCH=arm GOARM=6`), never
downloaded as a prebuilt blob. `gosd-init` supervises it for the life of the
device: the shim registers a node on your tailnet, opens a [Tailscale
Funnel](https://tailscale.com/kb/1223/funnel) listener, and reverse-proxies
every request to one local port on your app — no port forwarding, no public
IP address, and no app code at all. Because GoSD compiles the shim itself
rather than relying on an upstream release asset, **every board is
supported**, including `pi-zero-w` — contrast Cloudflare Tunnel's
arm64-only reach ([^armv6] above).

The public address is always `https://<hostname>.<tailnet>.ts.net` — a
MagicDNS name built from the device's hostname and your tailnet's name,
never a domain of your own choosing.

### Prerequisites: three tailnet settings the device can't set for itself

Funnel needs three things enabled on your **tailnet**, all from the
[Tailscale admin console](https://login.tailscale.com/admin/) —
`gosd-tsfunnel` has no way to set any of them itself, and fails with an
actionable error at boot if they're missing (see "Troubleshooting" below):

1. **MagicDNS** — Settings → DNS → enable MagicDNS. Funnel's public hostname
   is a MagicDNS name.
2. **HTTPS Certificates** — Settings → enable HTTPS Certificates. This is
   what lets Tailscale issue the on-node Let's Encrypt certificate Funnel
   terminates TLS with.
3. **The `funnel` node attribute**, granted to this device in your tailnet's
   ACL policy file (Access Controls in the admin console). Targeting a tag
   (rather than one specific device) is what makes this survive a device
   being rebuilt or re-registered — see "Create a tagged, reusable auth
   key" below — with a policy entry like:

   ```jsonc
   {
     // ... your existing ACLs ...
     "tagOwners": {
       "tag:gosd-device": ["autogroup:admin"],
     },
     "nodeAttrs": [
       {
         "target": ["tag:gosd-device"],
         "attr":   ["funnel"],
       },
     ],
   }
   ```

   Swap `tag:gosd-device` for whatever tag you actually use — it just has to
   match the tag on the auth key from the runbook below.

None of this is a GoSD setting; it lives entirely in your Tailscale account
and has nothing to do with which image you build.

### Runbook

1. **Build the image with the shim baked in, and a data partition** — the
   shim's tailnet identity needs somewhere durable to live (see "The data
   partition requirement" below):

   ```sh
   gosd build . --board pi-zero-2w --ingress tailscale-funnel --data-size=expand -o my-app.img
   ```

2. **Set up the tailnet prerequisites** above, once per tailnet — not once
   per device.

3. **Create a tagged, reusable auth key** in the admin console (Settings →
   Keys → Generate auth key):
   - **Tagged**, with the same tag your ACL policy's `nodeAttrs` targets
     (`tag:gosd-device` in the example above). Tagging is what disables this
     node's key expiry: an *untagged* device's key expires after
     Tailscale's default 180 days, which would eventually disconnect a
     device nobody is sitting in front of to re-authenticate. A tag makes
     the identity permanent instead.
   - **Reusable**, so the same key works for more than one device, or to
     re-register this one later if it ever loses its state (see "Surviving
     a reflash" below).
   - The key itself still expires — Tailscale caps auth keys at 90 days —
     but that only matters for this device's *first* registration.
     `gosd-tsfunnel` never looks at the key again once the device is
     registered, so a device built today with a key generated today keeps
     working long after that key has expired.

4. **Paste the auth key and port into `gosd.toml`** on the boot
   partition (or hand-edit the commented-out example the image already
   ships, per `docs/provisioning-formats.md`):

   ```toml
   [ingress.tailscale-funnel]
   authkey = "tskey-auth-xxxxxxxxxxxx-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
   port = 8080
   ```

   `port` is the only required key. Restart the device (or power it on for
   the first time) for the change to take effect.

5. **Delete the auth key from `gosd.toml`, if you like**, once the device
   has appeared in your tailnet's device list — see "Secrets on a FAT
   partition" below for why that's safe.

Once the device boots with a network connection, `gosd-init` starts
`gosd-tsfunnel`, which registers with your tailnet (if it hasn't already),
opens a Funnel listener, and starts proxying to `http://localhost:8080`.
The device is reachable at `https://<hostname>.<tailnet>.ts.net`.

### `gosd.toml` keys

| Key | Required | Default | Notes |
|---|---|---|---|
| `authkey` | Only for first registration | — | A tagged, reusable auth key (see "Runbook" above). Safe to delete once the device has registered. Never logged. |
| `port` | Yes | — | The local port your app listens on. |
| `hostname` | No | The device's own hostname | Becomes the `<hostname>` in `https://<hostname>.<tailnet>.ts.net`. |
| `funnel_port` | No | `443` | Which of Tailscale Funnel's three supported ports to listen on: `443`, `8443`, or `10000`. |

### The data partition requirement

Unlike Cloudflare Tunnel, whose credentials are stateless from GoSD's point
of view, Tailscale Funnel's node identity — its private key, its tailnet
membership, the public hostname it was assigned — lives on disk, at
`/data/.gosd/tailscale`, on the data partition. Losing that directory
means the device shows up as a *brand new* node the next time it starts,
with a new identity and a new public URL.

Because of that, `gosd build --ingress tailscale-funnel` **refuses to
build** without a data partition, with this exact error:

```
--ingress tailscale-funnel failed: tailscale-funnel stores its tailnet identity on the data partition; pass --data-size (e.g. --data-size=64MiB or --data-size=expand)
```

Any non-zero `--data-size` satisfies this; `--data-size=expand` (the usual
choice — see [docs/runtime.md](runtime.md)) is fine. If `/data` is ever
mounted read-only at runtime (a failing card, for instance), `gosd-init`
logs one actionable line and never starts the shim, rather than running it
with nowhere durable to keep its identity — see "Troubleshooting" below.

### What gets written on the device

There's no config file, unlike cloudflared's `config.yml`. Every per-boot
value `gosd-tsfunnel` needs travels as a command-line argument or an
environment variable, built fresh by `gosd-init` on every boot from
`gosd.toml`, and never read back:

- `--statedir /data/.gosd/tailscale` (created mode `0700` before the shim
  ever starts, since it holds private key material)
- `--hostname <hostname>`
- `--backend http://localhost:<port>`
- `--funnel-port <funnel_port>`
- `TS_AUTHKEY=<authkey>` — **only in the environment, never in argv.**
  `gosd-init`'s supervisor logs the pid and argv of every child it starts,
  but never its environment, so the one genuinely secret value here travels
  a way that can never end up in a log line.

Tailscale's own `tsnet` library manages everything else — the node's
private key, its tailnet registration, its ACME certificate — inside
`--statedir`.

### Secrets on a FAT partition

Like the WiFi passphrase and Cloudflare Tunnel's token, the auth key sits in
`gosd.toml` in plain text on the boot partition, a FAT32 partition readable by
anyone with the card in a computer. Treat it accordingly. Unlike those two,
though, this secret is genuinely disposable: once the device has registered
(check your tailnet's device list), delete the `authkey` line from
`gosd.toml` entirely — `gosd-tsfunnel` never needs it again, since tsnet
ignores `TS_AUTHKEY` once state already exists at `--statedir`.

### Clock and TLS, and the restart backoff

Same underlying gates as Cloudflare Tunnel — see
[docs/runtime.md's "Clock" section](runtime.md#clock-starts-at-1970-until-sntp-syncs):
no GoSD board has a battery-backed clock, so it starts every boot at the
Unix epoch until SNTP sync completes. `gosd-init` waits for the network-up
marker first (parking indefinitely if the network never comes up), then
waits up to **2 minutes** for the time-synced marker before starting the
shim anyway — Tailscale's own control-plane and ACME retries, plus the
restart backoff below, absorb whatever clock skew is left once NTP does
land.

If the shim exits, for any reason — an expired auth key, a network blip, a
still-missing `funnel` nodeAttr — `gosd-init` restarts it with a backoff
that starts at 1 second, doubles on each consecutive failure, and caps at
30 seconds; running stably for 30 seconds resets it back to 1 second for
the next failure. If the network goes down between attempts, a restart
waits on the network-up marker again rather than burning through backoff
for no reason.

### Surviving a reflash

Reflashing rewrites the whole of the boot partition, `gosd.toml` included, but
**not** the data partition — and Tailscale Funnel's node identity is what makes
this agent's reflash story strictly better than Cloudflare Tunnel's:

- **The tailnet identity itself never moves.** It lives at
  `/data/.gosd/tailscale`, which a plain Raspberry Pi Imager reflash never
  touches, as long as the new image also keeps `--data-size=expand` (or any
  non-zero `--data-size`). The device comes back up as the *same* tailnet
  node, at the *same* `https://<hostname>.<tailnet>.ts.net` URL, with
  **zero re-authentication** — you don't even need the auth key anymore,
  let alone a fresh one.
- **A hand-edited `[ingress.tailscale-funnel]` section** is separately
  protected by the [provisioning
  snapshot](runtime.md#the-provisioning-snapshot-surviving-a-reflash), the
  same way `[ingress.cloudflared]` is: restored as a whole section on the
  first boot after a reflash, provided the freshly flashed card doesn't
  already declare its own `[ingress.tailscale-funnel]` (which always wins,
  as fresh operator intent). This also needs `--data-size=expand` (or any
  non-zero `--data-size`) on the new image — a card with no data partition
  has nothing to snapshot to or restore from.

**If you ever do lose `/data`** — a card swap that doesn't carry the old
data partition over, a `--data-size=0` rebuild, or a from-scratch reformat
— the device registers as a brand new node on its next boot. Tailscale
won't hand out a name that's already taken: if `<hostname>` is already
registered, the new node is renamed `<hostname>-1`, and its public URL
changes to match. To recover the original name and URL: delete the stale
node from your tailnet's device list, then supply a live tagged auth key in
`gosd.toml` (a fresh one from the same tag works just as well as the
original, even if that one has since expired) so the device can
re-register under the name it used to have.

### Troubleshooting

`gosd-init` validates `[ingress.tailscale-funnel]` once, at boot, and logs
exactly one line (prefixed `tailscale-funnel: `) if something's wrong — it
never re-checks or self-heals a bad value later in the same boot. These are
the actual messages, verbatim, from
`cmd/gosd-init/internal/tsfunnel/mode.go` and
`cmd/gosd-init/internal/tsfunnel/tsfunnel.go`:

| Situation | Log line |
|---|---|
| Binary baked, nothing configured | `tailscale-funnel: binary is baked into this image, but [ingress.tailscale-funnel] isn't configured in gosd.toml; nothing to do` |
| Configured, binary not baked | `tailscale-funnel: [ingress.tailscale-funnel] is configured in gosd.toml, but this image wasn't built with --ingress tailscale-funnel; rebuild with that flag to bake the binary in` |
| `port` missing | `tailscale-funnel: [ingress.tailscale-funnel] is missing required key: port` |
| `port` out of range | `` tailscale-funnel: [ingress.tailscale-funnel] port <port> is out of range (must be 1-65535) `` |
| `funnel_port` isn't 443/8443/10000 | `` tailscale-funnel: [ingress.tailscale-funnel] funnel_port <port> is not one of the supported values (443, 8443, 10000) `` |
| No data partition, or `/data` read-only | `` tailscale-funnel: /data/.gosd/tailscale is not writable (<error>); tailscale-funnel needs a data partition; rebuild with --data-size `` |

Nothing here is fatal to boot — your app still starts normally either way.

Two more failure modes come from the shim itself (`cmd/gosd-tsfunnel`,
`errors.go`), once it's actually running. These arrive as ordinary process
output relayed through the supervisor, so they appear on the console with
**both** prefixes — `tailscale-funnel: ` from the relay, then
`gosd-tsfunnel: ` from the shim's own error reporting:

- **Registration doesn't finish within 5 minutes** (usually an expired or
  wrong `TS_AUTHKEY`, or a clock still far from correct):

  > `tailscale-funnel: gosd-tsfunnel: tsnet node did not finish registering within 5m0s: <error> — check that TS_AUTHKEY hasn't expired (auth keys expire within 90 days and are only needed for first registration; tsnet ignores the key once state already exists) and that the device clock is roughly correct (GoSD boards have no RTC and start every boot at the Unix epoch until SNTP syncs)`

- **The Funnel listener won't open** (one of the three tailnet prerequisites
  above isn't actually set):

  > `tailscale-funnel: gosd-tsfunnel: tsnet could not start a Funnel listener: <error> — check that this tailnet's ACL policy grants this device the "funnel" nodeAttr, that HTTPS certificates are enabled for the tailnet, and that MagicDNS is enabled; see docs/ingress.md`

Either way, the shim exits, `gosd-init` logs its pid and exit status, and
the restart backoff (see "Clock and TLS, and the restart backoff" above)
tries again — useful when
the fix is on the tailnet side (an ACL edit propagates without a reboot),
less useful for a genuinely expired key, which needs a fresh one written
into `gosd.toml` and the device restarted.

### What's not supported yet, and other caveats

- **No custom domains.** The public URL is always
  `https://<hostname>.<tailnet>.ts.net` — Funnel doesn't support bringing
  your own domain, unlike Cloudflare Tunnel.
- **Bandwidth caps.** Tailscale applies non-configurable bandwidth limits to
  Funnel traffic; they aren't published as a specific number.
- **`FunnelOnly` isn't exposed.** The shim always calls tsnet's plain
  `ListenFunnel`, reachable from both the public internet and your tailnet
  directly — useful as a debugging path while you're still fixing a
  misconfigured `funnel` nodeAttr. A tailnet-only-vs-public toggle may
  become a `gosd.toml` key later, but isn't one today.
- **Memory footprint.** The shim is initramfs-resident (RAM, not the SD
  card) for as long as the device runs. Baking both `cloudflared` and
  `tailscale-funnel` into the same image at once costs roughly an extra
  40-60MB of RAM combined — most devices only need one agent, but nothing
  stops building with both.
- **Not yet hardware-verified.** Every piece of this — the build, the
  `gosd.toml` schema, the runtime module, the supervisor wiring — is
  unit-tested and exercised under QEMU, but hasn't yet been run against a
  real tailnet on real hardware. That's epic `gosd-65uy`'s bench bean
  `gosd-79v8`; see `COMPATIBILITY.md` for current per-board status.
