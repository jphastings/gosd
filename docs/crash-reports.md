# Crash reports: telling your users what went wrong

**Status: partly built.** The report format, the renderer every producer
shares, and the writing of `LAST_FATAL_ERROR.md` are shipped: gosd-init
records its own fatal errors this way, and deletes the file once your app has
proven it recovered. Redaction is wired in: every value in your app's own
environment is scrubbed automatically, and `RegisterSecretString` covers the
secrets no environment variable names. gosd-init keeps the tail of your app's
own console output and writes a report when it crashes — see "What you get
for free" below — with no code changes required, and the `fault` package below
is importable, so a condition your own code understands ("your API key is
wrong") can declare a report of its own and stop the device. All of it is
verified on real hardware, on both a Rockchip board and a Raspberry Pi.

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
image: "myapp 0.1.0 #a1b2c3d4"
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

Every GoSD app gets a report when it crashes, with no code changes at all:
gosd-init keeps the tail of your app's own console output and writes it into
the report's technical section when the app dies unexpectedly — including
panics, segfaults and OOM kills, which your code never gets a chance to
report for itself.

| Code | What happened | What the device does |
| --- | --- | --- |
| `GOSD-APP-CRASH` | Your app exited with a non-zero status, or was killed by a signal | Restarts with backoff, exactly as it would for any other exit |

A signal death is named in terms you don't need a manual to read — an OOM
kill reads as "it ran out of memory", not "signal 9". An unrecovered Go
panic doesn't actually show up as a signal at all: the Go runtime reports one
by exiting with status 2, so that's the case a bare non-zero exit code
covers.

There's no app-supplied context on this path, so the report is honest about
it rather than guessing: it says your app "stopped unexpectedly while
running," not what it was doing when it did.

A deliberate `exit 0` is never treated as a crash and never gets a report,
even though the supervisor restarts an app that exits 0 exactly the same way
it restarts a crash — an app that decided to stop on its own isn't broken.

## How often it writes

Writing the report means briefly remounting the boot partition read-write,
which is the one moment in a GoSD device's life when a power cut can damage
it. So the writes are deliberately rare, and three rules keep them that way:

- **One report per stable run.** The first failure is written; further
  failures in the same crash loop only reach the console. Once your app has
  run for 30 seconds, the next failure is written again.
- **A recovered device stops looking broken.** Once your app has run that
  long, any report left on the card is deleted — but the device checks
  whether there's a file to delete before remounting anything, so a device
  that has never crashed never remounts at all.
- **A boot writes at most 10 reports, ever.** The first two rules alone only
  bound the *rate* of an app that keeps crash-looping, not its total cost
  over a device that stays up indefinitely: an app that reliably dies just
  after the 30-second mark would otherwise cost a delete-then-write pair of
  remounts every cycle, forever. Once a boot hits that ceiling, the device
  stops refreshing the card for any further crash until it next reboots —
  the last report written just stays there — but it still cleans up after
  itself once: if your app then genuinely recovers, the stale report is
  still deleted the next time it proves stable, so it doesn't keep looking
  broken either.

## When to raise one yourself

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
import "github.com/jphastings/gosd/fault"

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

`Fatal` ends the process where it stands, so **deferred functions do not
run**. Flush whatever must be flushed before you call it.

Your app never writes to the boot partition itself — it would race
gosd-init's own remounts and leave the card writable under a live app.
`Fatal` leaves the report in `/run`, a RAM filesystem, and gosd-init writes
it to the card once your app has exited.

Your report and the free one above never fight over the card. A `Fatal` exit
is a non-zero exit, so it looks like a crash too — but the device writes
exactly one report per exit, and yours wins: it knows what your user was
promised and what would fix it, where the console tail only knows what blew
up. The tail is kept as your report's technical detail, which matters when a
`Fatal` on one goroutine and a panic on another genuinely coincide. The write
rules above do still apply across exits: a device already carrying a report
from this stable run keeps it, and your newer one only reaches the console.

**On a device, `Fatal` prints only a short line naming your error code — never
the full report.** gosd-init keeps a tail of your app's own console output for
the free crash report above, and that tail is exactly this process's own
stdout/stderr. Printing the whole report there would hand gosd-init a copy of
your report as its own technical detail, nested inside the very report
gosd-init is about to write — the thinner copy, since your app's own process
can never know the device model, uptime or boot count, so it would sit right
below the real header contradicting it. gosd-init logs the complete report to
the serial console itself once it commits one, which is strictly better than
anything your app could print for itself: someone with a cable attached still
sees the full report, with the answers only gosd-init has.

Off-device — on your Mac, or under `go test` — `fault.Fatal` renders the same
Markdown to stderr and exits non-zero, rather than looking for a boot
partition, so you can see exactly what your user will see without flashing
anything. The line after the report says which of the two happened. The
printed copy carries only what your own process can know: on a device, the
copy on the card also names the hardware, the image, the uptime and the boot
count, and has your app's environment scrubbed out of it. A header field this
process could never honestly answer off a device — `uptime`, `boot`, `device`
— is left out of the preview entirely, rather than printed as `unknown`, so
what you see reads like the real report you're checking the wording of, not a
half-populated one.

The switch is the `gosd` build tag, which `gosd build` sets and nothing else
does — not a probe for `/run`, which any Linux machine running as root would
pass. A binary you built yourself never writes to `/run`, however
device-shaped its filesystem looks.

`examples/hello` has this wired up behind an environment variable: set
`HELLO_FATAL` in `gosd.toml`'s `[env]` table, reboot, and the device writes a
report and stays down — the whole path, on real hardware, without writing any
code.

## Secrets

A report invites its reader to forward the whole file to you, so the renderer
scrubs it first. Every value in your app's own environment — anything baked
into `config.json` or set through [gosd.toml's `[env]` table](gosd.toml.md) —
is replaced with `{$ITS_NAME}` automatically, no code changes required.
gosd-init's own reserved `GOSD_*` variables are never swept this way:
`GOSD_DATA_FLUSH` is `0` or `1`, and redacting it would blank every digit in
the technical detail.

Anything you register explicitly becomes `{secret: your-label}`:

```go
fault.RegisterSecretString(token, "session-token")
```

The second argument is a **label, not a second secret** — it is printed in
the file you're asking someone to forward.

Register as soon as your app holds the value, not when something goes wrong.
The registration is written through to `/run` on that call, and gosd-init
re-reads it at the moment of every report, because the crash that most needs
redacting is the one your app never sees coming: on a panic your code gets no
chance to hand anything over, and the secret still sitting in the console
output is exactly the one nobody registered in time. `/run` is a RAM
filesystem, so the plaintext value never touches the card.

Registering is additive and idempotent — registering the same value twice is
not an error, and the first label given for a value is the one that sticks.

Three limits worth knowing:

- **Very short values are left alone deliberately.** Anything under eight
  bytes is not redacted, because a `DEBUG=1` would otherwise blank every `1`
  in a stack trace. A short secret is therefore not protected, and the
  omission is logged by label wherever the report is produced.
- **At most 64 registered secrets, and 64KiB of them.** A registration past
  either bound is refused, with a line on stderr naming its label, and
  leaves the ones already made working — the whole set has to be readable
  back for any of it to apply, so dropping the newest is what protects the
  rest. Register the handful of long-lived credentials your app holds, not
  one per request.
- **The serial console is never redacted.** It's a channel for someone
  already holding the board, not a file that travels.

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
