// Package ready lets an app tell gosd-init it has finished its own startup
// checks — WiFi joined, an API reachable, a sensor initialized, whatever
// "genuinely all okay" means for it — and is ready to be shown as running.
//
// # Importing this package is the declaration
//
// gosd-init flips the status LED from "booting" to "running" the instant
// your app's process starts, before main has run a single line (see
// docs/status-led.md) — there is no flag, and normally nothing in your app
// needs to change. Importing github.com/jphastings/gosd/ready is what opts
// an app out of that default: gosd build inspects your app's own dependency
// graph (the same one `go build` would compile from, under the same build
// tags), and whenever it includes this package, it bakes a bit into
// config.json telling gosd-init to hold the LED on "booting" until [Signal]
// is actually called, rather than at process start.
//
// There is no separate build flag for this because there is nothing a flag
// would add: the whole point of a readiness signal is that your app decides
// when it's ready, and a call it can choose to make (or not) already is
// that decision. One consequence is worth knowing, because gosd-init has no
// other way to warn you about it: importing this package and never calling
// [Signal] holds the LED on "booting" forever. gosd-init logs that
// explicitly, once, at the first app start — see docs/status-led.md.
//
// # Calling Signal
//
// Call [Signal] once your app has done whatever it needs to consider itself
// genuinely ready, not before. Calling it more than once is harmless, and
// calling it at all when gosd-init isn't holding for one — i.e. this
// package was imported but never reached, or this isn't a GoSD device — is
// also harmless: Signal is a no-op off a device (see [Signal]'s doc).
//
// There is no timeout and no fourth LED state: an app that imports this
// package and never calls Signal leaves the LED honestly showing "still
// booting" rather than lying that it's running. An app that wants a solid
// LED plus a written explanation instead calls
// [github.com/jphastings/gosd/fault.Fatal].
package ready

import "github.com/jphastings/gosd/internal/readymark"

// Signal tells gosd-init this app is ready: if gosd-init is holding the
// status LED on "booting" for this call (see the package doc for when that
// is), it flips to "running" once this is observed. Calling it more than
// once is harmless.
//
// Off a GoSD device — your Mac, go test, any binary gosd build didn't
// produce — Signal is a silent no-op returning nil: there is no gosd-init
// to tell, and nothing here touches the filesystem.
func Signal() error {
	if runDir == "" {
		return nil
	}
	return readymark.Mark(runDir)
}
