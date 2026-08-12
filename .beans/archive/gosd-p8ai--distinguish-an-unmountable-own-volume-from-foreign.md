---
# gosd-p8ai
title: Distinguish an unmountable own-volume from foreign content
status: completed
type: feature
priority: normal
created_at: 2026-08-10T18:47:09Z
updated_at: 2026-08-10T18:54:10Z
---

gosd-psj0 made runEXT4 refuse, rather than silently reformat, a volume that carries the app's own label and filesystem but will not mount. Right fix, but it reports that refusal as the same ErrRefusedFormat already used for foreign content, so a caller cannot tell 'there is someone else's data here' from 'your own volume is sick'.

Those two deserve different consent. Found downstream in atfs (bean ATFS-c2ny): its ATFS_FORMAT_DRIVES_IF_NOT_ATFS flag exists to authorize adopting a drive that isn't atfs's yet, and its documentation promised a wipe never fires against a volume already carrying atfs's label. Since gosd-psj0 that promise is false — the same flag now also consents to destroying atfs's own blobs and its libp2p identity key, whose loss orphans the instance's atproto config record. The realistic path is an operator setting the flag once to adopt a drive, leaving it set, and a corruption event months later taking the identity with no second prompt.

Text-matching the error would work today and is exactly what a sentinel is for.

- [x] internal/blockmount: ErrUnmountable, wrapped alongside ErrRefusedFormat by the mount-failure refusal (multiple %w), so existing errors.Is(err, ErrRefusedFormat) callers are unaffected
- [x] Re-export from emmc and disk beside their existing ErrRefusedFormat/ErrUnsupportedFS
- [x] Tests: the mount-failure refusal satisfies both sentinels; the foreign-content and label-mismatch refusals satisfy ErrRefusedFormat and NOT ErrUnmountable
- [x] Document what it means and what it does not: only fires when the label and filesystem both match, i.e. it is specifically 'this is yours and it is sick', never a generic mount error

## Summary of Changes

- Added `blockmount.ErrUnmountable`, a sentinel narrower than `ErrRefusedFormat`: it fires only when the device's existing volume already matches the caller's label AND filesystem but could not be mounted — "this is yours and it is sick" — never foreign content and never a generic mount error.
- `runEXT4`'s mount-failure refusal now wraps both `ErrUnmountable` and `ErrRefusedFormat` via a single `fmt.Errorf` with two `%w` verbs, so every existing `errors.Is(err, ErrRefusedFormat)` caller is unaffected; the visible error text is unchanged apart from the new sentinel's own clause.
- Re-exported `ErrUnmountable` from both `emmc` and `disk`, beside their existing `ErrRefusedFormat`/`ErrUnsupportedFS` re-exports, with docs spelling out the downstream shape: an app with one flag authorizing adoption of a foreign drive should gate a second, differently-named flag on `ErrUnmountable` before it will destroy its own data.
- Added three tests in `internal/blockmount/blockmount_test.go`: the mount-failure refusal satisfies both sentinels; a different-label refusal and a same-label-different-filesystem refusal each satisfy `ErrRefusedFormat` but NOT `ErrUnmountable`, pinning the separation the two consent questions now have.
- Checked the rest of the repo for other sites constructing a refusal for an unmountable own-volume: `runEXT4` is the only one. `dataexpand.survivorPresent` hits an analogous unmountable-ext4 case but takes a deliberately different path (treats it as debris and self-heals by reformatting, per its own doc's MBR-entry reasoning) rather than returning a refusal error, so it has no error to wrap.

## Message wording (2026-08-09)

The sentinel's message is a sentence fragment — "it could not be mounted" —
in the same shape as ErrRefusedFormat's "refusing to reformat", and it
stands in for the phrase the refusal already used rather than being appended
to it. Wrapping it therefore costs the operator-facing message no extra
words: the rendered text is byte-identical to what shipped in gosd-psj0.
That matters because the serial console is the only user interface these
boards have, and an error that restates itself mid-sentence is noise on it.
