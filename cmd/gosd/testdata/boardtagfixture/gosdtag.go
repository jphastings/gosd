//go:build gosd

package main

// This file and its !gosd counterpart are gosd-cm4b's half of the fixture:
// exactly one of them is compiled in, so which marker the built binary
// carries proves whether the bare `gosd` tag was set. Kept separate from the
// per-board main() variants so the two tags are asserted independently.

func init() {
	println("boardtagfixture-marker:gosd-tag-set")
}
