//go:build gosd_pi_zero_2w

// This file is the gosd_pi_zero_2w-tagged counterpart to main.go: it's the
// one variant that actually compiles, so CrossCompile("./testdata/boardtag",
// ..., "gosd_pi_zero_2w", ...) succeeding (while the untagged build fails)
// demonstrates the tag reached `go build` before the package path.
package main

func main() {
	println("hello from the gosd_pi_zero_2w-tagged boardtag fixture")
}
