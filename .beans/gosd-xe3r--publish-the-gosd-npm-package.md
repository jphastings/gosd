---
# gosd-xe3r
title: Publish the gosd npm package
status: todo
type: task
created_at: 2026-08-04T10:10:22Z
updated_at: 2026-08-04T10:10:22Z
---

Follow-up to gosd-hcyn: publish js/packages/gosd to npm.

The publishing PIPELINE landed in PR #181 (.github/workflows/publish-npm.yml + js/PUBLISHING.md): staged and tokenless — `npm/<package>/vX.Y.Z` tag → verify job (all js gates at the tagged commit, tag-vs-manifest version check, commit-must-be-on-main check, tarball listing) → human approval via the `npm-publish` GitHub environment → OIDC trusted publishing with `--provenance`, always to the `next` dist-tag → registry smoke install + `npm audit signatures`. CI never touches `latest`; promotion is a human 2FA action (`npm dist-tag add gosd@X.Y.Z latest`).

## Remaining (JP console actions — can't be done from the repo)

- [ ] Create the `npm-publish` GitHub environment with yourself as required reviewer (repo Settings → Environments)
- [ ] Bootstrap publish of gosd@0.1.0 from your machine (`cd js && npm ci && cd packages/gosd && npm publish --tag next`) — npm can only attach a trusted publisher to an EXISTING package (name verified unregistered 2026-08-04; fallback @jphastings/gosd is a one-line name change)
- [ ] On npmjs.com → gosd → Access: add the trusted publisher (jphastings / gosd / publish-npm.yml / environment npm-publish) and set publishing to require 2FA-or-trusted-publisher (disables token publishing)
- [ ] Promote when happy: `npm dist-tag add gosd@0.1.0 latest`

Full procedure: js/PUBLISHING.md.
