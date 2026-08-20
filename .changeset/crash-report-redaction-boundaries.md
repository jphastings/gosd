---
gosd: patch
---

#### Crash reports redact what your app supplied, and leave gosd's own words alone

Three corrections to what `LAST_FATAL_ERROR.md` scrubs.

The **error code** is now redacted like every other field. It is your text —
an app can build one from an upstream failure, a request id or an account
identifier — and it was being written into the report's header untouched
because the header was assumed to hold only values gosd generates itself.

**gosd's own wording is no longer rewritten.** Redaction used to run over the
finished document, boilerplate included, so an ordinary environment value
that happened to be a word in gosd's prose rewrote the sentences your user
reads: an app with `APPNAME=weatherbox` got `# {$APPNAME} crash report`, and
any value of `computer` mangled the line explaining what the file is. Those
strings are compiled into gosd and can never contain your secret, so they are
left exactly as written. Everything the report carries in from your side —
the error code, all four written sections, the technical detail, and the app
name, version and support URL baked into the image — is still scrubbed
wherever it lands.

**Tunnel credentials are now redacted too.** A Cloudflare tunnel token
becomes `{ingress: cloudflared-token}` and a Tailscale auth key
`{ingress: tailscale-funnel-authkey}`, so the secrets gosd-init holds for
itself are covered by the same net as the ones it holds for you.
