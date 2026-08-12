//go:build !gosd

package fault

// runDir is empty in every binary `gosd build` didn't produce — a plain go
// build, go run, or go test — so nothing here looks for a boot partition,
// touches /run, or claims a board is about to halt. Fatal prints the report
// it would have written and exits instead; see the package doc.
const runDir = ""
