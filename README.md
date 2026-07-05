# GoSD

Turn Go applications into SD card images for the Raspberry Pi Zero 2w and the Radxa Zero 3E. (Planned: FriendlyElec NanoPi Zero2.)

Like GoKrazy, but the result is something _anyone_ can burn and use.

## Features

- Simple CLI tool that can be run locally or in CI
- Extremely fast boot (under 5 seconds, including Wifi)
- Optional USB OTG (run as a USB _device_)
- Connect to the internet via Ethernet (assumes DHCP) or WiFi (credentials added as your SD card is written)
- Run any normal (linux-capable) Go application

## Quickstart

> **This project is pre-release.** The steps below are the intended
> end-to-end workflow, written against what exists on `main` today. Two
> things stop it from being runnable start-to-finish right now:
>
> - No version of `gosd` has been tagged/released yet, so `go install
>   .../gosd@latest` has nothing to install.
> - `gosd build`'s final step — assembling the compiled binaries into a
>   flashable `.img` — isn't wired up yet (tracked as bean `gosd-3zrc`), and
>   the prebuilt kernel/bootloader artifacts it will need aren't published
>   yet either (bean `gosd-wtpa`). Today, `gosd build` cross-compiles your
>   app and `gosd-init` correctly, then fails clearly at the assembly step.
>
> Once those land, the flow below is what you'll run. In the meantime, see
> `internal/build`, `examples/hello`, and `docs/runtime.md` for what's real
> today.

1. Install the CLI:

   ```sh
   go install github.com/jphastings/gosd/cmd/gosd@latest
   ```

2. Write a `main.go`. GoSD runs any normal Go program — no special imports
   or SDK required:

   ```go
   package main

   import (
       "fmt"
       "net/http"
   )

   func main() {
       http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
           fmt.Fprintln(w, "hello from gosd")
       })
       http.ListenAndServe(":80", nil)
   }
   ```

   See `examples/hello` for a slightly fuller worked example (it also
   reports hostname and uptime, and falls back to `:8080` if `:80` is
   unavailable).

3. Build an image for your board:

   ```sh
   gosd build . --board pi-zero-2w -o hello.img
   ```

   Omit `--board` to build every supported board at once; run `gosd build
   --help` for the full set of flags (`--hostname`, `--wifi-ssid` /
   `--wifi-pass`, repeatable `--board`, `-o`/`--output`).

4. Flash `hello.img` to an SD card (e.g. with [Raspberry Pi
   Imager](https://www.raspberrypi.com/software/), "Use custom image"), boot
   the board, and open `http://<hostname>.local/` — or the sanitized name of
   your main package if you didn't pass `--hostname`. A dedicated
   screenshot-driven flashing guide for non-technical users is planned
   (bean `gosd-ufeh`); until then, and until mDNS support lands (bean
   `gosd-r796`), you may need to find the device's address via your
   router instead of `.local`.

For the runtime contract your app runs under once it's booted — supervision,
environment variables, networking timing, storage, logging — see
[`docs/runtime.md`](docs/runtime.md).
