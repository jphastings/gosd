Points `internal/artifacts.Version` at [artifacts/{{VERSION}}](https://github.com/jphastings/gosd/releases/tag/artifacts/{{VERSION}}), whose assets are published. Opened automatically by `.github/workflows/pin-artifacts-version.yml` (bean gosd-odx3).

Until this merges, `gosd build` still downloads the previous release — so every board fix in {{VERSION}} reaches nobody, however long ago it was merged.

**Verification is CI's job, not yours.** The `Verify artifacts pin` workflow runs on this pull request and:

- builds **every public board** against a redirected, empty cache, so all artifacts come from a real download of {{VERSION}} (gosd checks each file against the release manifest's sha256 as it unpacks, so a green build is also the digest check);
- confirms the cache really holds {{VERSION}}, catching a build that quietly succeeded against some other release;
- rebuilds with every proxy pointed at a closed port, proving the offline path works from cache alone;
- posts a summary of **which boards the pin actually moves**, compared byte-for-byte against the previous pin.

Read that summary before merging: it is the evidence that this release changes what it was cut to change, and nothing else.

The doc-comment entry is spliced from the release notes — reword it here if it reads awkwardly. Anything hardware-specific (booting a real board) is still a human call, and belongs on the bean that motivated the release.
