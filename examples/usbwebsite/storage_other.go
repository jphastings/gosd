//go:build !linux

package main

import "errors"

// errUnsupportedPlatform stands in for the Linux mount syscalls off-Linux;
// usbwebsite only runs meaningfully on a GoSD board. These stubs exist so
// the example builds and its logic tests run on a developer's machine.
var errUnsupportedPlatform = errors.New("mounting the data partition is only supported on Linux boards")

func mountVFAT(string, string) error { return errUnsupportedPlatform }

func unmountVFAT(string) error { return errUnsupportedPlatform }
