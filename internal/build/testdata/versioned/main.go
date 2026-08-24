// Command versioned is a build fixture with an exported string var a build
// can target with `-ldflags "-X main.version=..."` - unlike testdata/hello,
// which has nothing to stamp. See TestCrossCompileAppliesLDFlags.
package main

var version = "dev"

func main() {
	println("versioned: " + version)
}
