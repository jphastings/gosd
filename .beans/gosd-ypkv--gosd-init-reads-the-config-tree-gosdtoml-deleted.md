---
# gosd-ypkv
title: gosd-init reads the config tree; gosd.toml deleted
status: todo
type: feature
priority: normal
created_at: 2026-08-13T15:39:37Z
updated_at: 2026-08-13T15:40:23Z
parent: gosd-rw6n
blocked_by:
    - gosd-cn4p
---

The read half of epic gosd-rw6n (which holds all locked decisions): gosd-init
consumes the `config/` tree and gosd.toml is deleted everywhere.

Note for reviewers: this bean also deletes provsnapshot (it re-renders
gosd.toml, which no longer exists). Between this bean and the store bean,
main has NO reflash persistence — acceptable because every release is held
until the epic closes.

## Todos

- [ ] Tree enumeration with the reserved-name/junk filtering from the epic;
      values newline-trimmed; empty = unset, falling back per field to
      config.json's baked values
- [ ] `env/` -> app environment (GOSD_* ignored and logged; redaction rules
      built from the merged values exactly as today); `wifi/ssid` +
      `wifi/passphrase` -> wifiup; `hostname` (config.json's sanitized
      default when unset — preserving gosd-4hz1's wizard-can-win behaviour
      naturally); `data_flush`; `ingress/*` (present only when the feature
      shipped in this image)
- [ ] Cloud-init consumption: read seed -> delete + sync -> write values into
      the tree; a crash loses wizard input, never clobbers later edits
- [ ] Delete gosd.toml: the template and parser in internal/gosdtoml (the
      cloud-init YAML parsing in internal/provision stays), pipeline's
      gosd.toml boot file, provsnapshot, and every reference in gosd-init
- [ ] Crash-report source summaries (describeEnvSources) reworded for the
      tree
- [ ] Fake-driven tests that pass on macOS, per the repo's platform seam
      shape; the qemu boot-to-HTTP CI job must stay green
- [ ] Quality gates: `go test ./...`, `go vet ./...`, `gofmt -l .`,
      `golangci-lint run ./...` and `GOOS=linux golangci-lint run ./...`
