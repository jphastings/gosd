// Command qemuboot boots an already-built gosd --board=qemu-virt image
// under qemu-system-aarch64, using internal/qemurun for both the boot-file
// extraction and the qemu invocation itself - the same package `gosd run`
// (cmd/gosd) uses to build and boot an image in one step. It exists so
// scripts/qemu-run.sh (and, through it, CI's qemu-boot job) share that
// exact invocation rather than keeping their own copy of the qemu flags.
//
// Usage: go run ./internal/cmd/qemuboot <image.img>
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/jphastings/gosd/internal/qemurun"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "qemuboot: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: qemuboot <image.img>")
	}
	imgPath := args[0]

	if _, err := os.Stat(imgPath); err != nil {
		return fmt.Errorf("no such image file: %s", imgPath)
	}

	if err := qemurun.CheckAvailable(); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "qemuboot: booting %s (Ctrl-A X to quit qemu, Ctrl-C to force-kill).\n", imgPath)
	fmt.Fprintf(os.Stderr, "qemuboot: your app will be reachable at http://localhost:%d once gosd-init starts it and networking comes up.\n", qemurun.DefaultPort)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return qemurun.Run(ctx, qemurun.Options{
		ImagePath: imgPath,
		// QEMU_DISPLAY=1 opts into a host display window (scripts/
		// qemu-run.sh documents it); default stays headless so CI's
		// qemu-boot job is unaffected.
		Display: os.Getenv("QEMU_DISPLAY") == "1",
		// QEMU_EXTRA_ARGS carries extra qemu-system-aarch64 arguments,
		// one per line (documented in scripts/qemu-run.sh) — the
		// already-built-image counterpart of `gosd run --qemu-arg`.
		ExtraArgs: qemurun.ParseExtraArgsEnv(os.Getenv("QEMU_EXTRA_ARGS")),
		Stdin:     os.Stdin,
		Stdout:    os.Stdout,
		Stderr:    os.Stderr,
	})
}
