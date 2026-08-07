// Package cacerts pins the Mozilla CA bundle GoSD bakes into every image's
// initramfs (bean gosd-kzgq), so an app's outbound HTTPS calls find a
// working system trust store with zero setup - see docs/runtime.md's HTTPS
// section.
//
// Bump procedure: pick the newest dated snapshot listed at
// https://curl.se/ca/ (a URL of the form cacert-YYYY-MM-DD.pem - never the
// rolling cacert.pem, which isn't pinned to a fixed sha256 and can change
// under a cached copy), verify its sha256 against curl.se's published
// <name>.sha256 file for that same snapshot, and update Pin's URL and
// SHA256 together.
package cacerts

import "github.com/jphastings/gosd/internal/fetch"

// ArtifactName is the bundle's file name: both the well-known
// --artifacts-dir override name gosd build/run checks first, and the
// basename used when caching the fetched file.
const ArtifactName = "ca-certificates.crt"

// InitramfsPath is where the bundle lands inside every image's initramfs -
// Go's default root-certificate search path on Linux (see crypto/x509's
// certFiles), so an app's crypto/x509 dials find it with no import or
// build-time step of the app's own.
const InitramfsPath = "/etc/ssl/certs/" + ArtifactName

// Pin is the dated Mozilla CA bundle snapshot gosd downloads and verifies
// via internal/fetch. See the package doc comment for the bump procedure.
var Pin = fetch.File{
	URL:    "https://curl.se/ca/cacert-2026-07-16.pem",
	SHA256: "3ff344e30b9b1ed2971044eabb438a08f2e2245ddb5f8ab1a3ad8b63ab4eaf91",
}
