//go:build withready

package main

import "github.com/jphastings/gosd/ready"

func main() {
	_ = ready.Signal()
}
