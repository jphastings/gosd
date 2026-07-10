//go:build !gosd_pi_zero_2w && !gosd_nanopi_zero2

// Command boardtagfixture is a fixture app demonstrating gosd-1937's
// per-board build tags: this file is the fallback default, gated out
// whenever either board-specific file below is selected. Each variant
// prints (and so embeds in its compiled binary's rodata) a marker string
// unique to itself, letting TestBuildAppliesPerBoardBuildTags below assert
// which variant a real `gosd build` compiled for each board, without
// needing to run the resulting binary.
package main

func main() {
	println("boardtagfixture-marker:default")
}
