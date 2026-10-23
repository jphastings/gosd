---
gosd: patch
---

#### Build with Go 1.27.3 for the crypto/x509 CA removal

gosd now requires Go 1.27.3, the security release that makes `crypto/x509`
reject certificate chains through eight distrusted intermediates. gosd-init
and the `--ingress` shims are compiled with it, so rebuilding your image
with this release gets them the fix. Your app's own HTTPS gets the fix only
when your app is compiled with Go 1.27.3 or later. Any app that imports a
gosd package now requires that version, and `GOTOOLCHAIN=auto` (Go's
default) downloads it for you if your installed `go` is older. Every
dependency is also on its latest release, including tailscale.com v1.104.2.
