---
gosd: minor
---

#### New `gosd init` command

`gosd init` scaffolds a `gosd-build.toml` in the current directory, prefilling it with auto-detected values: the app's main package (discovered by scanning for `package main`), a git-tagged version when available, and a derived label prefix. The rest of the configuration remains for you to fill in by hand — partition sizes, ingress tunnels, and other app-specific build options — so every new project can skip the manual template boilerplate and start with a working configuration file.
