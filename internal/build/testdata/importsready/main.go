// Command importsready is a build fixture for gosd-42vb's ImportsPackage
// test: it imports github.com/jphastings/gosd/ready unconditionally, so its
// dependency graph must always be found to include it, regardless of tags.
package main

import "github.com/jphastings/gosd/ready"

func main() {
	_ = ready.Signal()
}
