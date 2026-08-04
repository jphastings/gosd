---
# gosd-bs9s
title: 'gosd/downloads: download resuming on the fs-access tier'
status: todo
type: feature
created_at: 2026-08-04T10:10:22Z
updated_at: 2026-08-04T10:10:22Z
---

Follow-up to gosd-hcyn: resume interrupted image downloads on the File System Access tier. Design sketch (from the gosd-hcyn planning): HTTP Range + If-Range (ETag or Last-Modified) against the image URL; persist the FileSystemFileHandle + progress state in IndexedDB; on resume, re-verify the partial file on disk by re-hashing it with pristine placeholder range bytes swapped back in (store those ~KiB-scale pristine bytes during the first attempt), then continue the stream from the verified offset. The SaveSink interface was deliberately kept sequential-only — add an optional seekable capability rather than changing the base contract. The vendored Sha256 already exposes clone() for in-session checkpoints; cross-session resume needs either serializable hash state or the re-hash approach above (prefer re-hash: no serialization format to maintain). A server 200 (ignoring Range) restarts from scratch.
