#!/usr/bin/env bash
# Reports what actually differs between two published artifacts releases, by
# comparing the per-file sha256 digests in their manifests (bean gosd-odx3).
#
# This is the evidence a version-pin bump needs: "which boards does moving
# from A to B actually change?" — answered from the released bytes rather
# than from what the changelog claims. A release that was meant to touch one
# board and turns out to have moved five is exactly what a reviewer needs to
# see before merging the pin.
#
# Usage:
#   pin-diff.sh <from-version> <to-version>
#
# Versions may be given as "v0.10.0", "0.10.0" or "artifacts/v0.10.0".
# Requires gh (authenticated) and jq. Prints a markdown summary on stdout.
set -euo pipefail

if [ "$#" -ne 2 ]; then
	echo "usage: pin-diff.sh <from-version> <to-version>" >&2
	exit 2
fi

normalise() {
	local v="${1#artifacts/}"
	printf 'v%s' "${v#v}"
}

from="$(normalise "$1")"
to="$(normalise "$2")"

workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT

for v in "$from" "$to"; do
	if ! gh release download "artifacts/$v" -p manifest.json -D "$workdir/$v" >/dev/null 2>&1; then
		echo "pin-diff.sh: could not download artifacts/$v's manifest.json" >&2
		exit 1
	fi
done

jq -n \
	--slurpfile a "$workdir/$from/manifest.json" \
	--slurpfile b "$workdir/$to/manifest.json" \
	--arg from "$from" --arg to "$to" '
	def files(m): m.boards | to_entries | map({board: .key, files: (.value.files | map({(.name): .sha256}) | add)});
	(files($a[0])) as $old | (files($b[0])) as $new |
	[ $new[] as $n
	  | ($old[] | select(.board == $n.board)) // {board: $n.board, files: {}} as $o
	  | { board: $n.board,
	      changed: [ $n.files | to_entries[] | select($o.files[.key] != .value) | .key ],
	      removed: [ $o.files | to_entries[] | select($n.files[.key] == null) | .key ] }
	] as $rows |
	"### What moves between \($from) and \($to)\n",
	( [ $rows[] | select((.changed | length) > 0 or (.removed | length) > 0) ] as $touched
	  | if ($touched | length) == 0 then
	      "Every board is byte-identical. A pin bump with no artifact changes is suspicious — check the release was built from the intended tree."
	    else
	      ( $touched[]
	        | "- **\(.board)**"
	          + (if (.changed | length) > 0 then "\n  - changed/new: " + (.changed | sort | join(", ")) else "" end)
	          + (if (.removed | length) > 0 then "\n  - **removed**: " + (.removed | sort | join(", ")) else "" end) )
	    end ),
	"",
	( [ $rows[] | select((.changed | length) == 0 and (.removed | length) == 0) | .board ] as $same
	  | if ($same | length) > 0 then "Unchanged: " + ($same | sort | join(", ")) + "." else empty end )
	' --raw-output
