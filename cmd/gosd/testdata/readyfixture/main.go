// Command readyfixture is a build fixture for gosd-42vb's config.json
// integration test: it imports github.com/jphastings/gosd/ready
// unconditionally, so a real `gosd build` of this package must set
// config.json's appSignalsReady.
package main

import "github.com/jphastings/gosd/ready"

func main() {
	_ = ready.Signal()
}
