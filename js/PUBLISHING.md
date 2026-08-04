# Publishing js/packages/\* to npm

Every package in this workspace publishes through one staged, tokenless
pipeline: [`.github/workflows/publish-npm.yml`](../.github/workflows/publish-npm.yml).
The properties it guarantees:

- **No long-lived npm token exists anywhere.** Publishing uses
  [npm trusted publishing](https://docs.npmjs.com/trusted-publishers/)
  (OIDC): the workflow mints a short-lived, per-run credential. There is no
  secret to leak, rotate, or steal.
- **Every publish carries a provenance attestation** — the npm page shows
  the verified link back to this repo, the workflow, and the exact commit,
  and `npm audit signatures` can verify it offline.
- **CI only ever publishes to the `next` dist-tag.** What a plain
  `npm install <package>` resolves to (`latest`) changes only when a human
  with a 2FA'd npm login promotes a version. A compromised CI run can at
  worst publish an opt-in preview, never silently replace the default
  install.
- **Releases come from reviewed history**: the workflow refuses a tag whose
  commit isn't on `main`, re-runs every js quality gate at that commit, and
  requires a human approval (the `npm-publish` GitHub environment) between
  verification and publish.

## One-time setup (per repository)

1. In the repo settings, create the environment **npm-publish**
   (Settings → Environments → New environment) and add yourself as a
   **required reviewer**. That approval prompt is the staged-release gate —
   without a reviewer the gate silently approves itself.

## One-time setup (per package)

npm can only attach a trusted publisher to a package that already exists,
so the very first version of a new package is published by hand — after
that no token is ever used again:

1. Bootstrap publish, from your own machine with your 2FA'd npm login,
   also staged to `next` rather than `latest`:

   ```sh
   cd js && npm ci && cd packages/<package>
   npm publish --tag next     # prepack builds; npm prompts for 2FA
   ```

2. On npmjs.com → the package → **Settings/Access → Trusted Publisher**,
   add a GitHub Actions publisher:
   - Organization or user: `jphastings`
   - Repository: `gosd`
   - Workflow filename: `publish-npm.yml`
   - Environment: `npm-publish`

3. (Recommended) In the same package settings, set publishing access to
   **require two-factor authentication or a trusted publisher** — this
   disables token-based publishing entirely, closing the door the
   bootstrap step briefly used.

4. Promote the bootstrap version once you're happy with it:

   ```sh
   npm dist-tag add <package>@<version> latest
   ```

## Cutting a release

1. Land a PR that bumps `version` in `js/packages/<package>/package.json`
   (the workflow refuses a tag whose version disagrees with the manifest).
2. Tag the merged commit on `main` and push the tag:

   ```sh
   git tag npm/<package>/v<version> && git push origin npm/<package>/v<version>
   ```

3. The workflow verifies everything and then pauses; approve the
   **npm-publish** environment run (the `verify` job's log shows the exact
   tarball file list you're approving).
4. After publish + smoke pass, the version is live on the `next` dist-tag.
   Try it out (`npm install <package>@next`), then promote it:

   ```sh
   npm dist-tag add <package>@<version> latest
   ```

## Rolling back

Published versions are immutable — you never unpublish, you re-point:

- A bad version still on `next`: just don't promote it; publish the fix as
  the next version.
- A bad version already promoted: point `latest` back at the previous good
  version, then mark the bad one:

  ```sh
  npm dist-tag add <package>@<previous-good> latest
  npm deprecate <package>@<bad> "broken: <one line why>; use <previous-good>"
  ```

## Adding a future package

Nothing in the workflow is gosd-specific: create `js/packages/<name>` with
`"name": "<name>"` in its package.json (and a `"./package.json"` export,
which the smoke job reads), do the per-package one-time setup above, then
release with `npm/<name>/v<version>` tags exactly as described here.
