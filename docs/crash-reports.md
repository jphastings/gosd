# Crash reports: telling your users what went wrong

**Status: partly built.** The report format, the renderer every producer
shares, and the writing of `LAST_FATAL_ERROR.md` are shipped: gosd-init
records its own fatal errors this way, and deletes the file once your app has
proven it recovered. What is not built yet is everything your *app* touches —
the `fault` package described below is **not importable**, and gosd-init does
not yet keep your app's console output, so a panic in your code still only
reaches the serial console. Two smaller gaps: nothing yet supplies the
renderer with secrets to redact, and the `device:` line's device-tree read has
not yet been confirmed on real hardware (it falls back to the board name baked
into the image whenever it can't be read). Tracked by bean `gosd-47z3`.

## Why this exists

A GoSD device is unattended and its owner is usually not a developer. When
something goes wrong there is no screen, no shell, no SSH, and — for anyone
who hasn't soldered on a serial adapter — no console. The device is simply
dead, and the owner has no way to tell a broken app from a broken card from
a mistyped WiFi password.

So the card itself is the channel. On a fatal error the device writes
`LAST_FATAL_ERROR.md` to the root of its boot partition, which is FAT32 and
therefore readable on any Mac, Windows or Linux machine. The owner pulls the
card, opens one file, and either fixes it themselves or forwards it to you.

Nothing is ever sent anywhere. There is no telemetry in GoSD and this is not
it — the report is written locally and read locally, by whoever is holding
the card.

## What the owner sees

A Markdown file that renders as prose in any text editor, with a machine-
readable header:

```markdown
---
error_code: NO-API-KEY
timestamp: unknown
clock: unsynced — timestamp is not trustworthy
uptime: 4m12s
boot: 37
device: Raspberry Pi 3 Model B Plus Rev 1.3 (pi-3b)
image: myapp 0.1.0 #a1b2c3d4
---

# myapp crash report

Your myapp device stopped while fetching today's forecast.
...
```

The device name comes from the hardware's own device tree, not from
anything baked into the image, so it names the board that actually booted —
distinguishing hardware GoSD's board IDs deliberately conflate, like a Pi 3B
from a 3B+.

**The clock is often wrong, and the file says so rather than guessing.** The
Pi family has no real-time clock at all, and the boards that do have one need
a coin cell fitted, so a device commonly starts up believing it is 1970 and
stays there until it reaches a time server — and a crash before networking is
up is exactly the kind a report exists for. `uptime` and `boot` are true
regardless, and are what actually answer "did it die instantly or after four
days?".

`boot` is counted on the data partition rather than the boot one, because
counting on the boot partition would mean a write to it on every single boot —
see "How often it writes" below. An image with no writable `/data` reports
`boot: unknown` instead.

## What gosd-init reports for itself

These are gosd-init's own failures. The codes are stable, so a support page
can list them:

| Code | What happened | What the device does |
| --- | --- | --- |
| `GOSD-DATA-CORRUPT` | The data partition is established but no longer holds a filesystem the device recognises | **Halts**, so whatever is still there can be salvaged |
| `GOSD-BOOT-MOUNT` | The device couldn't read the SD card it booted from | Reboots after 5s |
| `GOSD-EARLY-MOUNT` | The in-memory filesystems everything else needs couldn't be set up | Reboots after 5s |

Halting is reserved for a state no retry can improve; anything that might
succeed on a second attempt reboots instead, since a device that fixes itself
is better than one waiting for a visit.

**Two of those three can never reach the card.** `GOSD-EARLY-MOUNT` and
`GOSD-BOOT-MOUNT` both happen before the boot partition is mounted — and the
boot partition is where the report would go — so they exist only on the serial
console, and the console line says as much. Everything after that mount is
recordable.

## What you get for free

_Not built yet — see the status note at the top._ Every GoSD app will get a
report when it crashes, with no code changes at all: gosd-init keeps the tail
of your app's console output and writes it into the report's technical section
when the app dies unexpectedly — including panics, segfaults and OOM kills,
which your code never gets a chance to report for itself.

The supervisor still restarts your app with backoff after one of these,
because a crash nobody classified might well be transient.

## How often it writes

Writing the report means briefly remounting the boot partition read-write,
which is the one moment in a GoSD device's life when a power cut can damage
it. So the writes are deliberately rare, and two rules keep them that way:

- **One report per stable run.** The first failure is written; further
  failures in the same crash loop only reach the console. Once your app has
  run for 30 seconds, the next failure is written again.
- **A recovered device stops looking broken.** Once your app has run that
  long, any report left on the card is deleted — but the device checks
  whether there's a file to delete before remounting anything, so a device
  that has never crashed never remounts at all.

## When to raise one yourself

_Not built yet — the `fault` package described here is not importable. See the
status note at the top._

Use `fault.Fatal` for a condition **no restart can improve** and a human can
act on. An invalid API key, a config naming a sensor this build doesn't
support, a required environment variable that isn't set. These fail
identically forever; restarting just burns the card and buries the report
under a crash loop.

**`fault.Fatal` halts the device.** It does not return, and the board stays
down until someone power-cycles it. That is the point — but it means anything
that might succeed on a retry should be an ordinary returned error instead,
left to the supervisor's backoff.

```go
if cfg.APIKey == "" {
    fault.Fatal(fault.Report{
        Code:    "NO-API-KEY",
        Doing:   "fetching today's forecast",
        Problem: "no API key is set, so the weather service refuses us",
        Fix:     "add WEATHER_API_KEY to gosd.toml on this card",
        Detail:  err,
    })
}
```

Write `Doing`, `Problem` and `Fix` for the device's owner, not for yourself.
`Detail` is the only field aimed at a developer. If you can't name a `Fix`,
leave it empty and the report points the reader at your support URL instead.

Off-device — on your Mac, or under `go test` — `fault.Fatal` renders the same
Markdown to stderr rather than looking for a boot partition, so you can see
exactly what your user will see without flashing anything.

## Secrets

_Partly built: the renderer scrubs what it's given, but nothing yet collects
your environment or your registrations to give it. See the status note at the
top._

A report invites its reader to forward the whole file to you, so the renderer
scrubs it first. Every value in your app's environment is replaced with
`{$ITS_NAME}`, and anything you register explicitly becomes
`{secret: your-label}`:

```go
fault.RegisterSecretString(token, "session-token")
```

Two limits worth knowing. Very short values are left alone deliberately — a
`DEBUG=1` would otherwise blank every `1` in a stack trace — so a short
secret is not protected. And the serial console is never redacted; it's a
channel for someone already holding the board, not a file that travels.

## Building for it

```
gosd build --support-url https://example.com/support --app-version 0.1.0 ./cmd/myapp
```

`--support-url` is where the report sends a reader when you couldn't name a
fix, and should be a page that can answer questions about error codes.
`--app-version` is free-form and never interpreted; without it the report
identifies the build by content hash alone. Your app's name and the board's
are baked automatically.

Both are developer settings baked into the image, deliberately not
overridable through [the operator-facing card config](gosd.toml.md) or the
environment. Neither affects the image's identity, so changing them cannot
disturb [the data partition's upgrade path](runtime.md).

## What this is not

It is not a log. Only the latest fatal issue is kept, because that's the one
whoever collects the device needs, and a report is deleted once the app has
proven it recovered — a device that came back shouldn't look broken.

It is not durability, either. Whether your data survives a power cut is
governed by [the fsync sequence in the runtime contract](runtime.md),
regardless of any of this.
