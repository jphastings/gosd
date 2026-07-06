// Command imgextract copies every file at the root of a gosd .img's
// GOSD-BOOT FAT partition out to a destination directory, without root and
// without mtools. It's a thin CLI wrapper around
// github.com/jphastings/gosd/internal/qemurun.ExtractBootFiles, which does
// the actual work (opening the image read-only via go-diskfs and reading
// the FAT32 filesystem directly) and is also used directly by `gosd run`
// and internal/cmd/qemuboot.
//
// Usage: go run ./internal/cmd/imgextract <image.img> <dest-dir>
package main

import (
	"fmt"
	"os"

	"github.com/jphastings/gosd/internal/qemurun"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "imgextract: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("usage: imgextract <image.img> <dest-dir>")
	}
	return qemurun.ExtractBootFiles(args[0], args[1])
}
