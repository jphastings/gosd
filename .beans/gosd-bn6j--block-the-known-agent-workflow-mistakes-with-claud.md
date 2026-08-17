---
# gosd-bn6j
title: Block the known agent-workflow mistakes with Claude Code hooks
status: in-progress
type: task
priority: normal
created_at: 2026-08-17T16:57:40Z
updated_at: 2026-08-17T17:20:17Z
parent: gosd-8pgg
---

Part of epic gosd-8pgg. Independent of the other children — branch from main.

CLAUDE.md's Workflow section documents a set of CLI gotchas that have each cost
real time, and every one is a *deterministic string pattern*. Documentation
only helps an agent that re-reads the right line at the right moment; a hook
fires as the command is typed. `.claude/settings.json` is checked in (it
currently holds only a permissions allowlist) and has no hooks.

This is the fastest feedback layer in the epic — before a test run, before CI.

## Locked decisions

- One `PreToolUse` hook on `Bash`, delegating to a single reviewable script
  `scripts/claude-guard.sh`, rather than a pile of inline shell in
  settings.json. The script reads the hook JSON on stdin, inspects
  `tool_input.command`, and **exits 2** to block, with the explanation on
  stderr (exit 2 is what feeds stderr back to the agent).
- **Confirm the hook JSON schema against current docs before writing it** — the
  `update-config` skill covers exactly this. Do not guess the shape.
- Rules to encode, all verbatim from CLAUDE.md:

  | Pattern | Message |
  |---|---|
  | `beans create` with `--title` | no such flag; the title is positional |
  | `beans create --json` piped to `jq -r .id` | the id is at `.bean.id`; `.id` yields `null`, which cascades into "parent bean not found: null" |
  | `beans update` with 2+ `--body-replace-old` pairs | only the last pair applies; one replacement per call |
  | `gh pr merge` | JP reviews and merges every PR — never self-merge, even on green CI |
  | `gh pr create` / `gh repo fork` targeting a repo outside `jphastings` | prepare the patch locally and record it in the bean; JP decides whether to send it |

- Also add a `PostToolUse` hook on `Edit`/`Write` for `*.go` running `gofmt -l`
  on the edited file — it turns one of the four mandated gates into instant
  feedback for near-zero cost.
- **False positives are worse than misses here.** A hook that blocks a
  legitimate command is a hard stop with no override. Prefer a narrow pattern
  that misses an exotic phrasing over a broad one that fires on valid work.
- The messages are the whole product. Each must say what to do instead, not
  just what is wrong.

## Todo

- [x] Confirm the hooks schema (`update-config` skill / current docs)
- [x] Write `scripts/claude-guard.sh` with the five PreToolUse rules
- [x] Wire it into `.claude/settings.json` without disturbing the existing permissions block
- [x] Add the `gofmt -l` PostToolUse hook
- [x] Test each blocked form is refused with the intended message
- [x] Test the legitimate forms still pass: `beans create "Title" -t bug`, `beans create --json ... | jq -r .bean.id`, a single `--body-replace-old` pair, `gh pr create` inside `jphastings`
- [x] Reduce the CLAUDE.md Workflow bullets these now enforce to short pointers — the rule shouldn't sit in two places
- [x] Quality gates (go test / go vet / gofmt / golangci-lint x2)

## Notes

`scripts/` already holds bash helpers, so a shell script here is consistent —
the pure-Go rule governs what ships in an image, not dev tooling.

No changeset — internal only, use the `no release notes` label.

## Summary of Changes

`.claude/settings.json` gained a `PreToolUse`/`Bash` hook and a
`PostToolUse`/`Write|Edit` hook, both invoking
`"${CLAUDE_PROJECT_DIR:-.}/scripts/claude-guard.sh"`; the existing
permissions block is untouched. Schema confirmed against the `update-config`
skill's live settings schema before writing — the assumed shape
(`matcher` + `hooks[].{type,command}`, hook JSON on stdin, exit 2 blocks
with stderr fed back to the agent) is correct as the bean described it.

The script dispatches on `tool_name` and enforces:

| Segment shape | Refuses |
|---|---|
| `beans create` | a `--title` token (the title is positional) |
| `beans create --json` … `jq` | a `.id` filter (the id is at `.bean.id`) |
| `beans update` | 2+ `--body-replace-old` tokens (only the last pair applies) |
| `gh pr merge` | always — JP merges every PR |
| `gh pr create` / `gh repo fork` | a target owner other than `jphastings`, taken from `--repo`/`-R`, a `gh repo fork` positional, or failing those the `origin` remote |

`PostToolUse` runs `gofmt -l` on a written `.go` file and blocks on drift,
turning a mandated gate into instant feedback.

Two implementation decisions are load-bearing and shouldn't be undone:

- **It matches shell TOKENS, not substrings.** An awk tokenizer honours quoting
  and splits on unquoted `;`/`&`/`|`/newline/parens, so a flag name written
  inside a bean body, a commit message or a PR title collapses into one opaque
  token and can never be mistaken for the flag. Substring matching would have
  blocked this very bean's Summary of Changes.
- **It fails OPEN.** No jq, no python, no assumption about PATH (hooks inherit a
  bare one, so the script extends PATH itself and locates `gofmt`/`git`
  defensively); unparseable input, a missing tool or an undeterminable repo owner
  all allow the command. A block is a hard stop with no override, so a miss is
  always preferable to a false positive.

Verified against the real hook stdin shape: 9 blocked forms refused with their
message and 18 legitimate forms allowed — including `beans create "Title" -t bug`,
`beans create --json | jq -r .bean.id`, a single `--body-replace-old` pair,
`gh pr create` inside `jphastings` (both via `--repo` and via the origin
remote), `gh pr create --title "Fix foo/bar …"`, and a PR body that names all
three beans traps in prose. Both hook-command expansions checked
(`CLAUDE_PROJECT_DIR` set and unset), degenerate payloads fail open, and the
script is `shellcheck`-clean and bash 3.2-compatible (macOS `/bin/bash`).

CLAUDE.md's three `beans` Workflow bullets collapse to one pointer at the hook:
the reasoning now lives in the block messages, where a blocked agent actually
reads it.
