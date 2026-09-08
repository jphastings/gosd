//go:build !withready

// Command importsreadytagged is a build fixture proving ImportsPackage's
// -tags argument actually reaches `go list -deps`: this default file
// doesn't import github.com/jphastings/gosd/ready at all, while
// main_withready.go (gated on the withready tag) does — so the same
// package path must report false without the tag and true with it.
package main

func main() {
	println("importsreadytagged: default")
}
