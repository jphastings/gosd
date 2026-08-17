#!/usr/bin/env bash
#
# Claude Code guard hook for this repository, wired up in .claude/settings.json.
#
#   PreToolUse / Bash          refuses a handful of deterministic CLI mistakes
#                              CLAUDE.md documents, each of which has cost real
#                              time, and says what to run instead.
#   PostToolUse / Write|Edit   reports gofmt drift on a Go file just written,
#                              turning one of the mandated gates into instant
#                              feedback.
#
# Exit 2 is the contract: it blocks the tool call and feeds stderr back to the
# agent. Any other exit code lets the call through.
#
# Two properties matter more than coverage, because a block is a hard stop the
# agent cannot override:
#
#   * it fails OPEN — anything it cannot parse with confidence is allowed;
#   * it matches shell TOKENS, not substrings, so a flag name quoted inside a
#     bean body or a commit message is never mistaken for the flag itself.
#
# Hooks inherit a bare PATH, so nothing here may assume jq, go or git are on it.

set -uo pipefail

PATH="$PATH:/opt/homebrew/bin:/usr/local/bin:/usr/local/go/bin:${HOME:-}/go/bin"

SEGMENT_BREAK=$'\036'

payload=$(cat)
[ -n "$payload" ] || exit 0

# Prints the string value of the first `"<key>":` whose opening quote is not
# backslash-escaped — i.e. a real object key, never text inside another value —
# with JSON escapes resolved. Prints nothing if the key is absent or non-string.
json_string() {
  printf '%s' "$payload" | awk -v key="$1" '
    { buf = buf $0 "\n" }
    END {
      n = length(buf)
      target = "\"" key "\""
      start = 0
      pos = 1
      while ((i = index(substr(buf, pos), target)) > 0) {
        at = pos + i - 1
        if (at == 1 || substr(buf, at - 1, 1) != "\\") { start = at; break }
        pos = at + 1
      }
      if (start == 0) exit
      j = start + length(target)
      while (j <= n && substr(buf, j, 1) ~ /[ \t\r\n]/) j++
      if (substr(buf, j, 1) != ":") exit
      j++
      while (j <= n && substr(buf, j, 1) ~ /[ \t\r\n]/) j++
      if (substr(buf, j, 1) != "\"") exit
      j++
      out = ""
      while (j <= n) {
        c = substr(buf, j, 1)
        if (c == "\\") {
          e = substr(buf, j + 1, 1)
          if (e == "n") out = out "\n"
          else if (e == "t") out = out "\t"
          else if (e == "r") out = out "\r"
          else if (e == "u") { out = out " "; j += 4 }
          else if (e == "b" || e == "f") out = out " "
          else out = out e
          j += 2
          continue
        }
        if (c == "\"") break
        out = out c
        j++
      }
      printf "%s", out
    }
  '
}

# Splits a command line into shell words, one per line, with a lone $SEGMENT_BREAK
# between pipeline/list segments. Quoted spans collapse into a single word, which
# is what keeps prose that merely mentions a flag from ever matching it.
tokenize() {
  awk -v SEP="$SEGMENT_BREAK" '
    function flush() {
      if (has) { gsub(/\n/, " ", tok); print tok }
      tok = ""; has = 0
    }
    BEGIN { SQ = sprintf("%c", 39); DQ = sprintf("%c", 34); BS = sprintf("%c", 92) }
    { buf = buf $0 "\n" }
    END {
      n = length(buf)
      tok = ""; has = 0; q = ""
      for (i = 1; i <= n; i++) {
        c = substr(buf, i, 1)
        if (q == SQ) {
          if (c == SQ) q = ""; else { tok = tok c; has = 1 }
          continue
        }
        if (q == DQ) {
          if (c == BS && i < n) { i++; tok = tok substr(buf, i, 1); has = 1; continue }
          if (c == DQ) { q = ""; continue }
          tok = tok c; has = 1
          continue
        }
        if (c == BS && i < n) { i++; tok = tok substr(buf, i, 1); has = 1; continue }
        if (c == SQ || c == DQ) { q = c; has = 1; continue }
        if (c == " " || c == "\t") { flush(); continue }
        if (c == ";" || c == "&" || c == "|" || c == "\n" || c == "(" || c == ")") {
          flush(); print SEP; continue
        }
        tok = tok c; has = 1
      }
      flush()
    }
  '
}

block() {
  printf '%s\n' "$1" >&2
  exit 2
}

# ---------------------------------------------------------------------------
# PostToolUse: gofmt drift on a Go file that was just written
# ---------------------------------------------------------------------------

check_gofmt() {
  local file gofmt
  file=$(json_string file_path)
  case "$file" in
    *.go) ;;
    *) exit 0 ;;
  esac
  [ -f "$file" ] || exit 0
  gofmt=$(command -v gofmt) || exit 0
  [ -n "$("$gofmt" -l "$file" 2>/dev/null)" ] || exit 0
  block "gofmt would reformat $file — \`gofmt -l .\` is one of the mandated gates
and will fail as it stands.

  Fix it now:  gofmt -w $file"
}

# ---------------------------------------------------------------------------
# PreToolUse: the blocked Bash forms
# ---------------------------------------------------------------------------

# Owner of a repo reference in any of gh's accepted spellings:
# OWNER/NAME, https://github.com/OWNER/NAME, git@github.com:OWNER/NAME.git
owner_of() {
  local v=$1
  v=${v#https://}
  v=${v#http://}
  v=${v#ssh://}
  v=${v#git@}
  v=${v#*github.com/}
  v=${v#*github.com:}
  # GitHub account names are case-insensitive; fold so a capitalised spelling of
  # JP's own account is not read as somebody else's.
  case "$v" in
    */*) printf '%s' "${v%%/*}" | tr '[:upper:]' '[:lower:]' ;;
  esac
}

origin_owner() {
  local url
  command -v git >/dev/null 2>&1 || return 0
  url=$(git -C "${hook_cwd:-$PWD}" remote get-url origin 2>/dev/null) || return 0
  [ -n "$url" ] && owner_of "$url"
}

check_beans_create() {
  local t
  for t in "${seg[@]}"; do
    case "$t" in
      --json) beans_create_json=1 ;;
      --title | --title=*)
        block "Blocked: \`beans create\` has no --title flag — the title is a POSITIONAL
argument, and cobra will reject this with an unhelpful error.

  Instead:  beans create \"Your Title\" -t bug"
        ;;
    esac
  done
}

check_beans_update() {
  local t pairs=0
  for t in "${seg[@]}"; do
    case "$t" in
      --body-replace-old | --body-replace-old=*) pairs=$((pairs + 1)) ;;
    esac
  done
  [ "$pairs" -ge 2 ] || return 0
  block "Blocked: \`beans update\` applies only the LAST --body-replace-old/--body-replace-new
pair per invocation (its GraphQL path differs from the local one), so the $((pairs - 1))
earlier replacement(s) in this command would be dropped with no error.

  Instead:  one replacement per \`beans update\` call — run it $pairs times.
            Check off todos one at a time for the same reason."
}

check_jq_id() {
  local t
  [ "$beans_create_json" -eq 1 ] || return 0
  for t in "${seg[@]}"; do
    case "$t" in
      .id | ".id "*)
        block "Blocked: \`beans create --json\` returns the new id at .bean.id, NOT .id.
\`jq -r .id\` silently yields the string \"null\", which cascades later into a
confusing \"parent bean not found: null\".

  Instead:  beans create \"Title\" -t task --json | jq -r .bean.id"
        ;;
    esac
  done
}

check_gh_merge() {
  block "Blocked: JP reviews and merges every PR himself — never self-merge, even on
green CI.

  Instead:  push, wait for CI in the foreground (gh pr checks <n> --watch --interval 30),
            \`gh pr ready <n>\` if it is still a draft, and leave the merge to JP."
}

check_gh_outside_account() {
  local t owner="" i=3 verb=$1 positional=$2
  while [ "$i" -lt "${#seg[@]}" ]; do
    t=${seg[$i]}
    case "$t" in
      --repo | -R)
        i=$((i + 1))
        [ "$i" -lt "${#seg[@]}" ] && owner=$(owner_of "${seg[$i]}")
        ;;
      --repo=*) owner=$(owner_of "${t#--repo=}") ;;
      -*) ;;
      # Only `gh repo fork` names its target positionally, and only in a shape
      # this narrow — a quoted PR title happens to contain slashes too.
      *) if [ "$positional" = yes ] && [ -z "$owner" ]; then
        case "$t" in
          https://github.com/*/* | http://github.com/*/* | git@github.com:*/* | github.com/*/*)
            owner=$(owner_of "$t")
            ;;
          *[!A-Za-z0-9._/-]* | */*/*) ;;
          */*) owner=$(owner_of "$t") ;;
        esac
      fi ;;
    esac
    i=$((i + 1))
  done
  [ -n "$owner" ] || owner=$(origin_owner)
  [ -n "$owner" ] || return 0
  [ "$owner" = "jphastings" ] && return 0
  block "Blocked: \`$verb\` targets the \"$owner\" account, outside jphastings.
Opening a PR against — or forking — a repository JP does not own needs his
explicit permission, upstream dependencies included.

  Instead:  prepare the patch in a local clone, record it and the rationale in the
            relevant bean, and let JP decide whether to send it. If he has already
            agreed to this one, ask him to run the command himself."
}

check_segment() {
  [ "${#seg[@]}" -gt 0 ] || return 0
  case "${seg[0]}" in
    beans)
      case "${seg[1]:-}" in
        create) check_beans_create ;;
        update) check_beans_update ;;
      esac
      ;;
    jq) check_jq_id ;;
    gh)
      case "${seg[1]:-}/${seg[2]:-}" in
        pr/merge) check_gh_merge ;;
        pr/create) check_gh_outside_account "gh pr create" no ;;
        repo/fork) check_gh_outside_account "gh repo fork" yes ;;
      esac
      ;;
  esac
}

check_bash() {
  local line
  local command_text
  command_text=$(json_string command)
  [ -n "$command_text" ] || exit 0
  hook_cwd=$(json_string cwd)
  beans_create_json=0
  seg=()
  while IFS= read -r line; do
    if [ "$line" = "$SEGMENT_BREAK" ]; then
      check_segment
      seg=()
    else
      seg+=("$line")
    fi
  done < <(printf '%s' "$command_text" | tokenize)
  check_segment
}

case "$(json_string tool_name)" in
  Bash) check_bash ;;
  Write | Edit) check_gofmt ;;
esac

exit 0
