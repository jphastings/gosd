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
# Edits internal/artifacts/artifacts.go in place and prints what it did. It
# does NOT commit, push, or open anything: the caller decides what to do with
# the working tree, which is what makes it testable. Exit status 0 means the
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

# The release notes knope wrote for this version become the doc comment's
# entry for it, so the constant explains what moving to it changes without a
# human having to restate the changelog. Prose polish happens in review; the
# point here is that the summary is never missing or invented.
notes="$(awk -v want="## ${version#v} " '
	index($0, want) == 1 { grabbing = 1; next }
	grabbing && /^## / { exit }
	grabbing { print }
' "$changelog")"

if [ -z "$(printf '%s' "$notes" | tr -d '[:space:]')" ]; then
	echo "pin-bump.sh: no ${version#v} section in docs/releases/artifacts.md; has knope released it yet?" >&2
	exit 1
fi

export PIN_VERSION="$version" PIN_NOTES="$notes"
python3 - "$source_file" <<'PYEOF'
import os, re, sys, textwrap

path = sys.argv[1]
version = os.environ["PIN_VERSION"]
notes = os.environ["PIN_NOTES"]

# Only the change TITLES (knope's "#### ..." headings) go in the comment.
# The bodies live in docs/releases/artifacts.md and would swamp a doc
# comment; what a reader of the constant needs is what moving to this
# release changes, in one line each.
titles = [l.strip().lstrip("#").strip() for l in notes.splitlines() if l.strip().startswith("####")]
if not titles:
    sys.exit("pin-bump.sh: no change entries found in the release notes")

# Titles are used verbatim: lowercasing the first word to make one
# sentence would mangle proper nouns like "Cubie".
body = "; ".join(titles) + "."
wrapped = textwrap.wrap(f"{version}: {body}", width=66)
entry = [f"//   - {wrapped[0]}"] + [f"//     {l}" for l in wrapped[1:]]

src = open(path).read()
m = re.search(r'^const Version = "v\d+\.\d+\.\d+"$', src, re.M)
if not m:
    sys.exit("pin-bump.sh: could not find the Version constant")

src = src[:m.start()] + "\n".join(entry) + "\n" + f'const Version = "{version}"' + src[m.end():]
open(path, "w").write(src)
PYEOF

gofmt -w "$source_file"
echo "pin-bump.sh: pinned $current -> $version"
