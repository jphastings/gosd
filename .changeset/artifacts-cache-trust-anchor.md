---
gosd: patch
---

#### The artifact cache can no longer be used to plant a backdoor

`gosd build` downloads each board's kernel and bootloader from a GoSD
artifact release and verifies every file against the digests in that
release's `manifest.json`. The manifest itself was the exception: once
cached, it was trusted because it was there, and it was the only thing
vouching for the files beside it.

That made the cache directory a place to install a backdoor. Anything running
as you — a compromised editor extension, an npm or pip postinstall — could
drop a modified kernel into `~/.cache/gosd/` next to a `manifest.json` listing
that kernel's digest. The pair agreed with each other, so every later build
took the offline cache-hit path, made no request that might have noticed, and
baked the result into every image you flashed, outliving removal of whatever
planted it.

The digest of the pinned release's `manifest.json` is now compiled into
`gosd` and re-checked every time the manifest is read, from the network and
from the cache alike — the same way the CA bundle and every third-party blob
have always been pinned. A tampered cache now costs a re-download rather than
a compromised image, and offline it fails loudly instead of quietly using it.

Builds that supply their own artifacts with `--artifacts-dir` are unaffected.
Two smaller hardening fixes ride along: a board tarball that keeps
decompressing is abandoned rather than allowed to fill the disk, and downloads
now give up on an upstream that accepts the connection and then goes silent,
instead of hanging forever.
