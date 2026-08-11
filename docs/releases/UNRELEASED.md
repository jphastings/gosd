# Unreleased

Release-notes-level call-outs — breaking changes above all — accumulate here
between CLI `vX.Y.Z` tags. At each release they fold into the tag's notes
(`gh release create --notes-file`, edited as needed) and this file resets to
this stub. Last folded into: v0.3.0 (2026-08-10).

## Breaking changes

- **`boot-failure.log` is now `LAST_FATAL_ERROR.md`** (bean `gosd-pun9`). The
  file a device writes to the root of its boot partition when it hits a fatal
  error has been renamed and rewritten: it is Markdown with a machine-readable
  header rather than a bare log line, so a device's owner can read it in any
  text editor or preview pane and forward it whole. Anything that watched for
  the old name — a support script, a fleet check — needs updating. Devices
  running this release delete a `boot-failure.log` left behind by an older one
  rather than leaving two files that disagree.

## Other call-outs

- **A fatal error now leaves a readable explanation on the card.** Previously
  only one failure (a corrupt data partition) wrote anything at all; now every
  gosd-init failure that happens after the boot partition is mounted writes a
  report naming what the device was doing, what went wrong, a fix where there
  is one, and your `--support-url` where there isn't. The report carries the
  board's own device-tree name, the image's identity, how long the device had
  been up and how many times it has booted — and says plainly when the clock
  can't be trusted rather than printing a 1970 timestamp. It is deleted again
  once your app has run for thirty seconds, so a device that recovered doesn't
  keep looking broken. Reports for your *app's* own crashes are still to come;
  see [the crash report guide](../crash-reports.md).
