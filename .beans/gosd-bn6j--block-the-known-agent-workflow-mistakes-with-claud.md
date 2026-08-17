---
# gosd-bn6j
title: Block the known agent-workflow mistakes with Claude Code hooks
status: todo
type: task
created_at: 2026-08-17T16:57:40Z
updated_at: 2026-08-17T16:57:40Z
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

- [ ] Confirm the hooks schema (`update-config` skill / current docs)
- [ ] Write `scripts/claude-guard.sh` with the five PreToolUse rules
- [ ] Wire it into `.claude/settings.json` without disturbing the existing permissions block
- [ ] Add the `gofmt -l` PostToolUse hook
- [ ] Test each blocked form is refused with the intended message
- [ ] Test the legitimate forms still pass: `beans create "Title" -t bug`, `beans create --json ... | jq -r .bean.id`, a single `--body-replace-old` pair, `gh pr create` inside `jphastings`
- [ ] Reduce the CLAUDE.md Workflow bullets these now enforce to short pointers — the rule shouldn't sit in two places
- [ ] Quality gates (go test / go vet / gofmt / golangci-lint x2)

## Notes

`scripts/` already holds bash helpers, so a shell script here is consistent —
the pure-Go rule governs what ships in an image, not dev tooling.

No changeset — internal only, use the `no release notes` label.
