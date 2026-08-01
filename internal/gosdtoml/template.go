package gosdtoml

import (
	"fmt"
	"sort"
)

// header is written in plain language for a non-technical audience: whoever
// opens gosd.toml may never have edited a config file before, so it spells
// out which programs to use, exactly which characters to change, and what
// to do with the file afterwards.
const header = `# These are the settings for this device.
#
# You can change them by opening this file in a plain text editor — for
# example Notepad (Windows), TextEdit (Mac, but see the note below), or
# nano (Linux) — making your changes, and saving the file.
#
# IMPORTANT if you use TextEdit on a Mac: click Format > Make Plain Text
# before you save, or this file will stop working.
#
# Any line starting with a "#" (like this one) is just a note and is
# ignored — you never need to remove the "#" from a line unless you want
# to turn that setting on.
#
# When you're done, put the memory card back in the device and turn it on
# (or restart it). Your changes take effect the next time it starts up.
`

// hostnameCommentedTemplate is shown whenever the hostname line should not
// take effect from gosd.toml itself: either no hostname was baked in at
// build time, or (the common case) --hostname was left at its
// sanitized-package-name default rather than chosen explicitly, so an
// Imager wizard hostname is free to take effect instead (gosd.toml >
// cloud-init > config.json precedence; see bean gosd-4hz1). The value shown
// is still the name gosd-init would fall back to (config.json's baked
// default) if nothing else provides one — an example for the user to
// uncomment and edit, not a value that currently does anything.
const hostnameCommentedTemplate = `
# The name this device uses on your network. To set it, remove the "#"
# below and change the name between the quotes. Use only letters, numbers
# and hyphens (-) — no spaces.
# hostname = %q
`

const hostnameTemplate = `
# The name this device uses on your network. To change it, edit the name
# between the quotes below. Use only letters, numbers and hyphens (-) — no
# spaces.
hostname = %q
`

// wifiCommentedOut is shown when no WiFi network was baked in at build
// time — an example for the user to uncomment and edit.
const wifiCommentedOut = `
# WiFi details, if this device should connect to a wireless network. To
# turn this on, remove the "#" from the start of all three lines below,
# then change the network name and password between the quotes.
# [wifi]
# ssid = "MyHomeNetwork"
# passphrase = "MyWiFiPassword"
`

const wifiTemplate = `
# WiFi details for this device. To change them, edit the network name and
# password between the quotes below.
[wifi]
ssid = %q
passphrase = %q
`

// envCommentedOut is shown when no environment variables were baked in at
// build time — an example for the user to uncomment and edit, not settings
// that currently do anything.
const envCommentedOut = `
# Extra settings your app reads when it starts, sometimes called
# "environment variables" — most apps don't need any. To add one, remove
# the "#" from the two lines below and change NAME and "value"; add more
# lines the same way for further settings. Names are case-sensitive, and
# values always need double quotes.
# [env]
# NAME = "value"
`

// envHeader introduces the [env] table when there's at least one value to
// show, baked-in or otherwise — the per-line settings themselves are
// appended by Render.
const envHeader = `
# Extra settings your app reads when it starts, sometimes called
# "environment variables". To change one, edit the value between the
# quotes below; to add another, add a line like NAME = "value". Names are
# case-sensitive, and values always need double quotes.
[env]
`

// Render produces the gosd.toml file the builder writes onto every image:
// the plain-language header, followed by the hostname, WiFi and [env]
// settings — filled in with the build-time values when set, or left as
// commented-out examples when they're not, so a hand-edited card always
// shows the user exactly what to type and where.
//
// bakeHostname distinguishes an operator-chosen hostname from the sanitized
// -package-name default: only bakeHostname=true renders the hostname line
// uncommented (hostname must also be non-empty). A commented default still
// shows hostname's value as the example, so the card documents what it'll
// be named without that line taking effect — leaving room for an Imager
// wizard hostname (cloud-init) to win instead, per the locked
// gosd.toml > cloud-init > config.json precedence (bean gosd-4hz1). A
// hand-edit that later uncomments the line always wins, same as today.
func Render(hostname string, bakeHostname bool, wifiSSID, wifiPassphrase string, env map[string]string) []byte {
	out := header

	if hostname != "" && bakeHostname {
		out += fmt.Sprintf(hostnameTemplate, hostname)
	} else {
		example := hostname
		if example == "" {
			example = "my-device"
		}
		out += fmt.Sprintf(hostnameCommentedTemplate, example)
	}

	if wifiSSID == "" {
		out += wifiCommentedOut
	} else {
		out += fmt.Sprintf(wifiTemplate, wifiSSID, wifiPassphrase)
	}

	if len(env) == 0 {
		out += envCommentedOut
	} else {
		out += envHeader
		keys := make([]string, 0, len(env))
		for key := range env {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			out += fmt.Sprintf("%s = %q\n", key, env[key])
		}
	}

	return []byte(out)
}
