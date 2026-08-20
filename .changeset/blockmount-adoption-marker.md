---
gosd: minor
---

#### `disk` and `emmc` no longer adopt a FAT32/exFAT volume on the strength of its label

Formatting a `disk` or `emmc` volume as FAT32 or exFAT wrote the filesystem and
mounted it, with nothing in between forcing those writes to the medium — and
every later boot decided the volume was "already provisioned" by reading its
label back. Neither half is safe. Until a flush happens, an arbitrary subset of
a format's writes may have reached the card, in no particular order; and the
volume label is written near the end of one, so a card that lost power
mid-format could come back with a label that says "ready" over FAT tables that
were never finished. Adopting it handed the app torn cluster chains that
corrupt on first write. The other way round — a label that did *not* land —
left storage that was refused on every boot, forever, despite never having held
anything.

Both are now fixed the way ext4 already worked. A format is followed by a flush
to the medium, and only once that and the mount have succeeded does GoSD write
a reserved, empty `.gosd-established` file into the volume's root. A later boot
adopts the volume only if that marker is there; a volume carrying the app's
label but no marker and no files is crash debris, and is repaired
(reformatted) without needing `destructive`, because nothing was ever written
to it.

**Cards already in the field keep their data.** A FAT32 or exFAT volume
formatted by an earlier release carries no marker, so it is adopted on the
evidence of the files already in it — GoSD's formatters never create a file, so
anything in the root can only have been written by an app that was handed the
mountpoint, which only happens after a format completed. The marker is written
in passing, so the upgrade happens once. The one volume this can reformat is
one with **no files in its root at all**, which by definition has nothing to
lose.

Two smaller behaviour changes come with it, both matching what ext4 has done
since it became the default:

- A FAT32/exFAT volume that matches the app's label and filesystem but cannot
  be mounted is now refused with an error matching both `ErrRefusedFormat` and
  `ErrUnmountable`, rather than a bare mount error — and, with
  `destructive: true`, is reformatted rather than reported.
- `.gosd-established` is reserved. Apps must not delete it; doing so does not
  destroy data, but it costs the volume its proof of a finished format.

Separately, `emmc` can no longer select an eMMC's `boot0`/`boot1`/`rpmb`/`gp0-3`
hardware partitions as a format target. `disk` has excluded them by name since
the day the risk was found; `emmc` merely never encountered one, because the
kernel happens not to label those devices the way its selection looks for. The
exclusion now lives in the code both packages share, so neither can pick one —
formatting an eMMC's boot area leaves a board that no longer boots and cannot
be recovered from its SD card.
