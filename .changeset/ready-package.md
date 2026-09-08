---
gosd: minor
---

#### New `ready` package: hold the status LED until your app is ready

An app that needs WiFi, an external API, or a sensor initialized before
it's genuinely "all okay" can now hold `gosd-init`'s status LED on
"booting" past `/app`'s process start, and only let it show "running" once
the app says so. Importing `github.com/jphastings/gosd/ready` is the
declaration — `gosd build` inspects the app's own dependency graph, no flag
needed — and calling `ready.Signal()` is what flips the LED. Every app that
doesn't import the package is unaffected: the LED still moves to "running"
the instant `/app` starts, exactly as before.
