//go:build !gosd_pi_zero_2w

// Command boardtag is a build fixture for gosd-1937's CrossCompile tag test:
// this default (fallback) file only compiles when the gosd_pi_zero_2w tag is
// absent, and deliberately fails to compile (an undefined reference) so the
// test can tell whether CrossCompile's -tags argument actually reached the
// compiler - a failed build here without the tag, and a clean one with it,
// proves the tag was placed before the package path in the argv.
package main

func main() {
	undefinedBoardTagFixtureSymbol()
}
