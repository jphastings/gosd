// Package notmain is a build fixture that is deliberately not a main
// package, so tests can assert that gosd rejects it with a clear error.
package notmain

// Add exists only so this package has some content.
func Add(a, b int) int { return a + b }
