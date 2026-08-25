// Command versionedfixture is a fixture app with an exported string var a
// real `gosd build --ldflags` can target with `-X main.version=...`, letting
// TestBuildAppliesLDFlags confirm the stamped string lands in the built
// image's /app binary.
package main

var version = "dev"

func main() {
	println("versionedfixture: " + version)
}
