---
gosd: major
---

#### Build options can live in a checked-in `gosd-build.toml` (and three flags are renamed)

A repository can now record its canonical build in a `gosd-build.toml` next
to its code: every `gosd build` flag has a key of the same name, an `[app]
main` key stands in for the package-path argument, and so a bare `gosd
build` in a fresh checkout reproduces the repo's intended image(s). Flags
passed on the command line always win, per option, so nothing recorded in
the file is ever locked in. Relative paths in the file resolve against the
file's own directory, unknown keys are errors naming the key, and `gosd
run` honours the subset of keys whose flags it mirrors. The details, and a
full example file, are in the new build-config documentation page.

The file's keys map onto flags structurally — `--boot-size` is `size` under
`[boot]` — and to make the real groups fit, three flags are **renamed**
(breaking, with no alias):

- `--support-url` is now `--app-support-url`
- `--config-dir` is now `--boot-config-dir` (on `gosd build` and `gosd run`)
- `--catalog` is now `--publish-catalog`

Invocations using the old spellings fail with cobra's unknown-flag error;
update scripts and Makefiles to the new names — or move the values into a
`gosd-build.toml` and drop the flags altogether.
