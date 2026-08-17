// Package repocheck is the home for repo-wide invariant tests that no
// production package owns.
//
// GoSD's conventions live in CLAUDE.md, and the deterministic ones - "a
// kernelspec entry needs a registered board", "every example cross-compiles
// for every board arch", "a change file's package keys are unquoted" - only
// hold because somebody remembers them at review time. Several have drifted.
// A convention that a test can state as a fact about the repository belongs
// here instead: the check outlives the reviewer, and the prose it replaces
// comes out of CLAUDE.md so a rule never sits in two places at once.
//
// The package deliberately exports nothing, and holds no production code:
// its content is _test.go files. They read the repository as data, rooted at
// "../.." (this package sits two levels below the repo root), so a check
// asserts about the checkout it ships in rather than about anything compiled
// into a binary. A check that needs to enumerate boards uses
// internal/boardset rather than a list of its own - that duplication is the
// drift these tests exist to catch.
package repocheck
