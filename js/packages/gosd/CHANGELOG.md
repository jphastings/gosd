## 0.3.2 (2026-08-20)

### Fixes

#### The injection manifest is read with a size cap, and warns when it is unpinned over plain HTTP

`fetchManifest` now gives up on a manifest past 1 MiB rather than buffering the
whole response before looking at it: it refuses an oversized `Content-Length`
outright, and counts bytes as they arrive regardless, since a host willing to
send an endless body is not one whose headers mean anything. `manifestSha256`
makes a tampered manifest *detectable*, but only once its bytes are already in
memory, so on its own it was no defence against that same untrusted host
exhausting the tab before there was anything to hash. A real manifest is a few
KiB of JSON, so a response past the cap is a wrong URL or a hostile one either
way. The image fetch was already bounded this way by its manifest's declared
size.

Fetching an unpinned manifest over plain `http://` now logs a warning
(loopback excepted, so a local dev server stays quiet). The image itself is
perfectly safe over http — every byte is hash-verified whatever the transport —
but the manifest is where those hashes come from, so it is the one fetch whose
integrity nothing downstream can re-derive.

The README now also says outright that escaping the content you pass in is
yours to do: placeholder and setting values are written to the card verbatim,
so a quickstart-style `renderConfigYaml(userInput)` is where user input has to
be escaped for the format it is going into.

## 0.3.1 (2026-08-14)

### Fixes

#### Versioning and release notes now come from the repo's release automation

No functional changes: this release validates the new change-file → release-PR → tag → publish pipeline end to end.
