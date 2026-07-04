package gosdtoml

import "fmt"

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

// hostnameCommentedOut is shown when no hostname was baked in at build
// time — an example for the user to uncomment and edit, not a value that
// currently does anything.
const hostnameCommentedOut = `
# The name this device uses on your network. To set it, remove the "#"
# below and change the name between the quotes. Use only letters, numbers
# and hyphens (-) — no spaces.
# hostname = "my-device"
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

// Render produces the gosd.toml file the builder writes onto every image:
// the plain-language header, followed by the hostname and WiFi settings —
// filled in with the build-time values when set, or left as commented-out
// examples when they're not, so a hand-edited card always shows the user
// exactly what to type and where.
func Render(hostname, wifiSSID, wifiPassphrase string) []byte {
	out := header

	if hostname == "" {
		out += hostnameCommentedOut
	} else {
		out += fmt.Sprintf(hostnameTemplate, hostname)
	}

	if wifiSSID == "" {
		out += wifiCommentedOut
	} else {
		out += fmt.Sprintf(wifiTemplate, wifiSSID, wifiPassphrase)
	}

	return []byte(out)
}
