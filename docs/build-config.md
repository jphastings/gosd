# Checked-in build options: `gosd-build.toml`

`gosd build` has a lot of flags, and the ones that matter to your app — its
partition sizes, its ingress tunnel, its placeholder files — are facts about
the app, not about whoever happens to be running the build. A
`gosd-build.toml` checked into your repository records them once, so that in
a fresh checkout:

```console
$ gosd build
```

builds the repository's canonical image(s), no flags, no Makefile
incantation. A flag passed on the command line always wins over the file, so
any developer can override any recorded option for one invocation without
editing anything.

## Where gosd looks

`gosd build` and `gosd run` read `gosd-build.toml` from the **working
directory** — the same rule as `gosd-kernel.toml`, and deliberately not a
walk up the directory tree, so a build run in some subdirectory can't
silently inherit options you can't see. No file is no error: everything
stays flag-driven. From elsewhere (a monorepo, a CI job), point at the file
explicitly:

```console
$ gosd build --build-config apps/frobnicator/gosd-build.toml
```

An explicit `--build-config` path that doesn't exist is an error.
`--build-config` is the one build flag with no file key, for the same reason
a map has no "you are here" entry.

## Every key is a flag

The file's vocabulary is exactly the flag surface, mapped structurally: a
flag `--<section>-<rest>` (where `<section>` is one of `app`, `boot`,
`data`, `kernel`, `publish`) appears as `rest` under `[section]`, and every
other flag is a top-level key spelled exactly like the flag. Values take
exactly what the flag takes — repeatable flags become arrays. There is one
key with no flag: `[app] main`, which stands in for the positional
package-path argument, and is what makes a bare `gosd build` work.

Unknown keys are errors naming the key, so a typo can't silently build the
wrong image.

```toml
# gosd-build.toml — checked-in defaults for `gosd build` (and the subset
# `gosd run` shares). Every key is the flag of the same name; a flag passed
# on the command line always wins. Relative paths resolve against this
# file's own directory.

# Boards to build; omit to build every supported board.
board = ["pi-zero-2w", "radxa-zero-3e"]

# Output directory (several boards) or .img path (one board) — same as -o.
output = "dist"

label-prefix = "myapp"

# Bake in an internet tunnel client (same values as --ingress).
ingress = ["tailscale-funnel"]

# Fixed-size placeholder files on the boot partition, for provisioning
# tools that splice bytes into a downloaded image. Paths are *on the
# image*, never resolved against this file.
placeholder = ["provision.yaml=32KiB"]

# A prebuilt static companion binary, <path>[:<dest>] — the path half is
# relative to this file; dest is an absolute path inside the image.
with-external = ["./third_party/mpv:/bin/mpv"]

[app]
main = "./cmd/myapp"       # the app to build, so a bare `gosd build` works
version = "1.4.2"          # --app-version; or "git:v*.*.*" to resolve from
                           # your repo's tags at build time (see below)
support-url = "https://example.com/support"   # --app-support-url

# Changing boot size, data filesystem, or label-prefix in a later release
# changes the app's on-disk layout: an upgrading device's existing data
# partition is erased and re-established.
[boot]
size = "256MiB"            # --boot-size
config-dir = "config"      # --boot-config-dir: on-card setting overlays

[data]
size = "512MiB"            # --data-size
filesystem = "fat32"       # --data-filesystem
flush = false              # --data-flush

[kernel]
param = ["snd_bcm2835.enable_hdmi=1"]   # --kernel-param
config = "gosd-kernel.toml"             # --kernel-config

[publish]
catalog = true             # --publish-catalog: emit rpi-imager os_list.json
base-url = "https://example.com/downloads"    # --publish-base-url

# Also honoured, less commonly checked in (top level):
#   usb-gadget = true
#   console-baud = 115200
#   artifacts-dir = "gosd-artifacts"    # e.g. local `gosd build-kernel` output
#   gosd-init-src = "../gosd/gosd-init" # a flag or $GOSD_INIT_SRC overrides this
#   ldflags = "-X main.version=1.4.2"   # --ldflags, applied to your app's compile only
#   tags = "myfeature"                  # --tags, merged with gosd's own gosd/gosd_<board> tags
#   trimpath = true                     # --trimpath
#   gcflags = "-m"                      # --gcflags
#   asmflags = "-D FOO=1"               # --asmflags
```

## Precedence, exactly

Per key, the flag wins whenever it was passed at all — including
`--publish-catalog=false` beating a file `catalog = true`, and a
command-line `--board` replacing the file's whole `board` array rather than
merging with it. Keys the file doesn't set keep the flag's normal default.
A key written as an empty value is *set to empty*, not unset: `label-prefix
= ""` is refused exactly like `--label-prefix=""`.

One key has a third tier: `gosd-init-src` is resolved flag, then
`$GOSD_INIT_SRC`, then the file — the environment variable is the hook a
package manager's wrapper uses to pin its bundled gosd-init on one machine,
and a checked-in file that travels between machines shouldn't defeat it.

## Relative paths resolve against the file

A relative path in the file — `output = "dist"`, `config-dir = "config"`,
`artifacts-dir`, `gosd-init-src`, `kernel.config`, a `with-external` entry's
local half, and filesystem-relative forms of `main` (`./x`, `../x`, `.`) —
resolves against the directory `gosd-build.toml` itself lives in, the same
rule the kernel and external recipe files follow. That way the file means
the same thing no matter where gosd is invoked from. `placeholder` entries
are paths *on the built image*, so they are never rebased; an import-path
`main` (say `github.com/you/app`) passes through untouched.

## App versions from git tags

A checked-in file can't carry a literal version that changes every release,
so `version` (and the `--app-version` flag) accepts a `git:` source instead:

```toml
[app]
version = "git:v*.*.*"
```

At build time gosd finds the matching tag **nearest the commit being
built** — `git describe` semantics, in pure Go, no git binary needed — so
building a maintenance branch never picks up a newer tag from another
branch. An exactly-tagged checkout yields the tag alone; otherwise the
version reads `<tag>-<commits-since>-g<abbreviated-hash>`, and an unclean
worktree appends `-dirty` (never an error). The part after `git:` is a
shell-style wildcard pattern matched against tag names; its literal prefix
is stripped from the result, so the pattern is also the extraction rule:
`git:v*.*.*` turns tag `v1.4.2` into `1.4.2`, `git:release-*` turns
`release-7` into `7`, and a bare `git:` matches any tag and keeps its full
name. A pattern with no wildcards names one exact tag and keeps it whole.
gosd resolves the source but still never interprets the resulting version.

**Building in CI?** Most CI checkouts are shallow and tagless — GitHub
Actions' checkout defaults to a depth of 1 — which leaves nothing for the
pattern to match. Resolution needs the commit graph and the tags but none
of the historical file contents, and git can fetch exactly that — a
*treeless* fetch:

```yaml
- uses: actions/checkout@v4
  with:
    fetch-depth: 0  # the whole commit graph and every tag...
    filter: tree:0  # ...but no historical file contents
```

or repair an existing shallow clone with
`git fetch --unshallow --tags --filter=tree:0`. A plain `fetch-depth: 0`
also works — it just downloads every version of every file to answer a
question about commits — and a server without partial-clone support
ignores the filter and sends everything, so the treeless form never does
worse. (A build triggered by a tag push can get away with
`fetch-tags: true` alone: the pushed tag points at the very commit being
built.) The build error names these fixes when it detects a shallow
clone.

**`--ldflags` has no `git:` resolution of its own.** Unlike `version`,
`ldflags` (and `--ldflags`) is a literal string gosd passes straight to `go
build -ldflags` — there's no templating that ties it to whatever `version`
resolves to. To stamp the same version into both `config.json` (via
`app.version`/`--app-version`) and the compiled binary (via `go build`'s
own `-X main.version=...`), resolve the version once in your build script
and pass it to both flags explicitly; `gosd-build.toml` alone can't express
"derive ldflags from app.version".

## The keys that are on-disk layout

`boot.size`, `data.filesystem` and `label-prefix` are part of your app's
on-card layout: change any of them in a later release and an upgrading
device's existing data partition is erased and cleanly re-established on
reflash, per [the upgrade path design](design/upgrade-path.md). Checking
them into the repository is exactly the point — the file makes the values
deliberate and reviewable — but treat a diff touching them with the same
care as a schema migration.

## What `gosd run` reads

`gosd run` honours the keys whose flags it mirrors: `app.main`, `boot.size`,
`boot.config-dir`, `data.size`, `data.flush`, `kernel.param`,
`label-prefix`, `ingress`, `artifacts-dir` and `gosd-init-src`. Keys that
only mean something to `gosd build` (`board`, `output`, `data.filesystem`,
`usb-gadget`, the `[publish]` table, …) are ignored under `gosd run`, the
same way run deliberately has no `--data-filesystem` flag — its qemu smoke
test isn't a shipping image. Strict parsing still catches typos in them.

## Not the config tree

`gosd-build.toml` is the **developer's** input: it decides what gets
compiled into the image, at build time, on the developer's machine. It is
not [the config tree every card carries](config.md), which is the **card
owner's** surface — settings edited in a text editor on the flashed card, at
any time, by whoever holds the hardware. A value belongs in exactly one of
the two: if the person holding the card should be able to change it, it's a
config-tree setting; if changing it should require rebuilding and reflashing
an image, it's a build option, and now it can live here.
