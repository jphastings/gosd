Points `internal/artifacts.Version` at [artifacts/{{VERSION}}](https://github.com/jphastings/gosd/releases/tag/artifacts/{{VERSION}}), whose assets are published. Opened automatically by `.github/workflows/pin-artifacts-version.yml` (bean gosd-odx3).

Until this merges, `gosd build` still downloads the previous release — so every board fix in {{VERSION}} reaches nobody, however long ago it was merged.

**Draft until verified.** Per the artifacts documentation, a pin bump is sound only once someone has checked the release it points at:

- [ ] **Clean-machine build** — fresh `HOME`, no `--board`, no `--artifacts-dir`: every public board image builds from a real download
- [ ] **Offline re-run** — same `HOME` with the network blocked: the build succeeds entirely from cache
- [ ] **Content spot-check** — the downloaded tarball really carries the change this release was cut for (e.g. `dtc -I dtb -O dts` showing an enabled node, or bytes absent from a bootloader)

Mark ready for review once those pass, saying what you ran. The doc-comment entry is spliced from the release notes — reword it here if it reads awkwardly.
