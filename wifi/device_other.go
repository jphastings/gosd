//go:build !gosd

package wifi

// runDir is empty in every binary `gosd build` didn't produce — a plain go
// build, go run, or go test — so Join never touches /run or claims a
// gosd-init is there to hand a request to. See the package doc.
const runDir = ""
