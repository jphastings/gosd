#!/usr/bin/env bash
# Rewrites internal/artifacts.Version onto a published artifacts release and
# splices that release's notes into the constant's doc comment (bean
# gosd-odx3). This is the "bump" half of the tag-first/bump-second procedure
# in docs/artifacts.md: knope tags and publishes the release, CI builds and
# attaches its assets, and only then may gosd point at it.
#
# Kept as a standalone script (rather than inline workflow YAML) so it can be
# exercised locally against a real checkout — the same reasoning as its
# sibling package.sh.
#
# Usage:
#   pin-bump.sh <version>
#
# version is the release to pin, with or without the tag prefix: "v0.10.2",
# "0.10.2" and "artifacts/v0.10.2" are all accepted.
#
# Edits internal/artifacts/artifacts.go in place, writes a change file so the
# bump actually ships in a CLI release, and prints what it did. It does NOT
# commit, push, or open anything: the caller decides what to do with the
# working tree, which is what makes it testable. Exit status 0 means the
# file now pins <version>; exit status 3 means it already did, and nothing
# was changed (so a caller can skip opening a duplicate pull request).
set -euo pipefail

if [ "$#" -ne 1 ]; then
	echo "usage: pin-bump.sh <version>" >&2
	exit 2
fi

raw="$1"
version="${raw#artifacts/}"
version="v${version#v}"
if ! printf '%s' "$version" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+$'; then
	echo "pin-bump.sh: $raw is not an artifacts version (want vX.Y.Z)" >&2
	exit 2
fi

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
source_file="$repo_root/internal/artifacts/artifacts.go"
changelog="$repo_root/docs/releases/artifacts.md"

current="$(grep -oE 'const Version = "v[0-9]+\.[0-9]+\.[0-9]+"' "$source_file" | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+')"
if [ "$current" = "$version" ]; then
	echo "pin-bump.sh: already pinned to $version; nothing to do"
	exit 3
fi

export PIN_VERSION="$version" PIN_CURRENT="$current"
changefile="$(python3 - "$source_file" "$changelog" "$repo_root/.changeset" <<'PYEOF'
import os, re, sys, textwrap

source_file, changelog_path, changeset_dir = sys.argv[1:4]
current = os.environ["PIN_CURRENT"]
target = os.environ["PIN_VERSION"]

def key(v):
    return tuple(int(p) for p in v.lstrip("v").split("."))

# Every release BETWEEN the old pin and the new one is news, not just the
# newest: bumping v0.10.0 -> v0.10.2 delivers v0.10.1's changes too, and
# those are often the reason the bump matters (v0.10.1 is what made the
# Cubie A5E boot at all).
sections, version = {}, None
for line in open(changelog_path):
    m = re.match(r"^## (\d+\.\d+\.\d+)\b", line)
    if m:
        version = "v" + m.group(1)
        sections[version] = []
        continue
    if version:
        sections[version].append(line.rstrip("\n"))

wanted = sorted((v for v in sections if key(current) < key(v) <= key(target)), key=key)
if not wanted and target in sections:
    wanted = [target]  # a rollback, or a re-pin: describe the target itself
if not wanted:
    sys.exit(f"pin-bump.sh: no release notes between {current} and {target} in {changelog_path}")

titles = {v: [l.strip()[5:].strip() for l in sections[v] if l.strip().startswith("#### ")] for v in wanted}

src = open(source_file).read()
m = re.search(r'^const Version = "v\d+\.\d+\.\d+"$', src, re.M)
if not m:
    sys.exit("pin-bump.sh: could not find the Version constant")

# Releases the comment already describes are not described twice: this can
# legitimately run again over a tree a previous run already annotated.
already = {ln.split(":")[0].removeprefix("//   - ").strip()
           for ln in src.splitlines() if ln.startswith("//   - v")}

comment = []
for v in wanted:
    if v in already:
        continue
    body = ("; ".join(titles[v]) + ".") if titles[v] else "rebuilt artifacts, no user-facing changes."
    wrapped = textwrap.wrap(f"{v}: {body}", width=66)
    comment.append(f"//   - {wrapped[0]}")
    comment.extend(f"//     {l}" for l in wrapped[1:])

new_comment = ("\n".join(comment) + "\n") if comment else ""
open(source_file, "w").write(
    src[: m.start()] + new_comment + f'const Version = "{target}"' + src[m.end() :]
)

# Without a change file, knope cuts no CLI release and the bump reaches
# nobody who installs gosd rather than building it from source.
os.makedirs(changeset_dir, exist_ok=True)
path = os.path.join(changeset_dir, f"artifacts-pin-{target.replace('.', '-')}.md")
lines = [
    "---", "gosd: patch", "---", "",
    f"#### Board images are now built from artifacts {target}", "",
    "`gosd build` downloads the board kernels and bootloaders published as",
    f"{target}" + (f", up from {current}," if len(wanted) > 1 else ",") + " which brings:",
    "",
]
for v in wanted:
    lines.extend(f"- {t}" for t in titles[v])
    if not titles[v]:
        lines.append(f"- {v}: rebuilt artifacts with no user-facing changes")
open(path, "w").write("\n".join(lines) + "\n")
print(os.path.basename(path))
PYEOF
)"

gofmt -w "$source_file"
echo "pin-bump.sh: pinned $current -> $version"
echo "pin-bump.sh: wrote .changeset/$changefile so the bump ships in a release"
