//go:build !gosd_pi_zero_2w && !gosd_nanopi_zero2

// Command boardtagfixture is a fixture app demonstrating the build tags
// `gosd build` passes to an app compile: this file is the per-board
// fallback default, gated out whenever either board-specific file beside it
// is selected, and gosdtag.go/nogosdtag.go cover the bare `gosd` tag.
// Each variant prints (and so embeds in its compiled binary's rodata) a
// marker string unique to itself, letting TestBuildAppliesGosdBuildTags
// assert which variants a real `gosd build` compiled for each board,
// without needing to run the resulting binary.
package main

func main() {
	println("boardtagfixture-marker:default")
}
