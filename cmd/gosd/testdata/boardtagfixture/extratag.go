//go:build extratagmarker

// This file is gosd-wjjn's addition to the fixture: it's gated on a tag no
// board ever sets, so its marker only appears in a build's compiled /app
// when a caller's own --tags value reached the compile - proving --tags
// merges onto (rather than replaces) the board's own mandatory tags, since
// the per-board markers in main_pi_zero_2w.go/main_nanopi_zero2.go must
// still appear alongside it.
package main

func init() {
	println("boardtagfixture-marker:extratagmarker-set")
}
