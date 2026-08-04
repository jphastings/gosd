---
# gosd-xe3r
title: Publish the gosd npm package
status: todo
type: task
priority: normal
created_at: 2026-08-04T10:10:22Z
updated_at: 2026-08-04T12:30:27Z
---

Follow-up to gosd-hcyn: publish js/packages/gosd to npm.

The publishing PIPELINE landed in PR #181 (.github/workflows/publish-npm.yml + js/PUBLISHING.md): staged and tokenless — `npm/<dir>/vX.Y.Z` tag, keyed on the package's directory under `js/packages/` rather than its npm name (gosd-g7n4) → verify job (all js gates at the tagged commit, tag-vs-manifest version check, commit-must-be-on-main check, tarball listing) → human approval via the `npm-publish` GitHub environment → OIDC trusted publishing with `--provenance --access public`, always to the `next` dist-tag → registry smoke install + `npm audit signatures`. CI never touches `latest`; promotion is a human 2FA action (`npm dist-tag add @jphastings/gosd@X.Y.Z latest`).

The package publishes as `@jphastings/gosd` — the bare name `gosd` was 403'd by npm's typosquat-similarity guard (gosd-g7n4). That removes the name-claiming urgency the earlier draft of this bean had: a scoped name under an account only JP controls can't be squatted out from under him, so there's no race to bootstrap-publish before someone else grabs it.

## Remaining (JP console actions — can't be done from the repo)

- [ ] Create the `npm-publish` GitHub environment with yourself as required reviewer (repo Settings → Environments)
- [ ] Bootstrap publish of @jphastings/gosd@0.1.0 from your machine (`cd js && pnpm install --frozen-lockfile && cd packages/gosd && npm publish --access public --tag next`) — npm can only attach a trusted publisher to an EXISTING package
- [ ] On npmjs.com → @jphastings/gosd → Access: add the trusted publisher (jphastings / gosd / publish-npm.yml / environment npm-publish) and set publishing to require 2FA-or-trusted-publisher (disables token publishing)
- [ ] Promote when happy: `npm dist-tag add @jphastings/gosd@0.1.0 latest`

Full procedure: js/PUBLISHING.md.
