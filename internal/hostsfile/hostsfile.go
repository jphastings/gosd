// Package hostsfile renders /etc/hosts for a gosd image: the static
// localhost entries gosd build bakes into the initramfs (see
// internal/pipeline) and the device's own hostname line gosd-init appends
// once it settles (see cmd/gosd-init/internal/boot). Sharing the static
// content between both sides is what lets gosd-init's own rewrite
// regenerate the file byte-for-byte from Render rather than needing to
// read and patch whatever gosd build baked.
//
// # Why this needs to exist at all
//
// GoSD images ship no /etc/hosts by default. With CGO_ENABLED=0 (every
// build, no exceptions — see the repo CLAUDE.md), Go's pure-Go resolver
// reads /etc/hosts directly instead of calling libc's NSS, and a static
// external binary linked against musl behaves the same way. Without this
// file, resolving "localhost" is sent to DNS: it fails outright before
// gosd-init's networking goroutine has brought up an interface, and once
// networking is up it leaks a "localhost" query to whatever resolver the
// LAN handed out via DHCP — which may even answer it, with something that
// isn't 127.0.0.1.
//
// # /etc/nsswitch.conf is deliberately not shipped
//
// Go's resolver only consults /etc/nsswitch.conf's "hosts" line to decide
// the files-vs-DNS lookup order; when that file is absent (as it always is
// on a gosd image) or doesn't mention "hosts" at all, it falls back to
// checking files first and DNS second (hostLookupFilesDNS in the standard
// library's net/conf.go) — exactly the order this package's job depends
// on. Shipping an nsswitch.conf would only add a file to keep in sync for
// no behavioral gain, so this package leaves it out on purpose.
package hostsfile

import (
	"fmt"
	"os"

	"github.com/jphastings/gosd/internal/naming"
)

// Path is /etc/hosts' location: both the path gosd build writes into the
// initramfs archive, and the path gosd-init later rewrites on the
// RAM-backed rootfs the unpacked initramfs already provides (that rootfs
// *is* gosd-init's writable /etc — see netup.WriteResolvConf's doc for the
// same point made about /etc/resolv.conf).
const Path = "/etc/hosts"

// staticLines are baked into the initramfs unchanged (see Static): the
// loopback address for "localhost", plus the IPv6 addresses conventionally
// paired with it. This alone makes "localhost" resolve correctly from PID 1
// onward, with zero runtime code — see the package doc for why it's needed
// at all.
const staticLines = "127.0.0.1 localhost\n::1 localhost ip6-localhost ip6-loopback\n"

// Static returns the build-time-static portion of /etc/hosts that gosd
// build bakes into the initramfs (see internal/pipeline).
func Static() string {
	return staticLines
}

// Render returns the full /etc/hosts content for hostname: the static
// lines Static returns, plus a `127.0.1.1 <hostname>` line (the Debian
// convention for a machine's own hostname, distinct from the 127.0.0.1
// loopback above) so that code resolving os.Hostname() works without a
// network, too. gosd-init calls this once the device's hostname has
// settled for the boot in progress (see cmd/gosd-init/internal/boot) and
// rewrites the whole file from scratch, rather than reading and patching
// whatever gosd build baked — which is what guarantees the static lines
// above always survive untouched, even across a hostname that later turns
// out invalid or unchanged.
//
// A hostname this file could not represent on one line — anything
// naming.ValidHostname refuses, which is anything outside [a-z0-9-] or
// past naming.MaxLength — gets no line at all rather than a mangled or
// forged one. Callers are expected to have refused such a value long
// before this point (gosd-init resolves the card's hostname setting
// through the same gate), but /etc/hosts is the format an injected
// newline actually targets, so this is the component that must not
// depend on being called correctly: one "\n" in a hostname would
// otherwise append an attacker-chosen address for an attacker-chosen
// name, which Go's pure resolver consults ahead of DNS for every lookup
// the app makes (bean gosd-39da).
func Render(hostname string) string {
	if !naming.ValidHostname(hostname) {
		return staticLines
	}
	return staticLines + fmt.Sprintf("127.0.1.1 %s\n", hostname)
}

// Write overwrites path with Render(hostname)'s content, atomically: a
// concurrent reader always sees either the previous complete contents or
// the new complete ones, never a truncated or empty file mid-write. This
// mirrors netup.WriteResolvConf's own temp+rename pattern and its
// rationale, which applies unchanged here: gosd-init's rootfs is the
// initramfs's own permanent, RAM-backed, already-writable mount (not a
// read-only cpio image), so os.WriteFile's O_CREATE lazily creates the
// file if the build ever ships without one, and no fsync is needed since
// there is nothing durable to protect — the whole rootfs is discarded on
// every reboot regardless, and this file is fully regenerated from Render
// on the next boot anyway.
func Write(path, hostname string) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(Render(hostname)), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("renaming %s to %s: %w", tmp, path, err)
	}
	return nil
}
