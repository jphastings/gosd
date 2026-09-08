//go:build !gosd

package ready

// runDir is empty in every binary `gosd build` didn't produce — a plain go
// build, go run, or go test — so Signal never touches /run or claims a
// gosd-init is there to hand a marker to. See the package doc.
const runDir = ""
