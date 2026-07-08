//go:build linux

// Command linuxonly is a build fixture gated to GOOS=linux, mirroring a
// main package that depends on a Linux-only library (e.g. examples/gpioinfo
// via github.com/warthog618/go-gpiocdev's chardev uapi). It exists so tests
// can assert that CrossCompile still recognizes it as package main when run
// from a non-Linux host, rather than being fooled by the host's own GOOS.
package main

func main() {
	println("hello from a linux-only gosd build testdata package")
}
