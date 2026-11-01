//go:build !gosd

package ingress

// runDir is empty in every binary `gosd build` didn't produce, so URL never
// touches /run or waits for a tunnel that cannot exist. See the package doc.
const runDir = ""
