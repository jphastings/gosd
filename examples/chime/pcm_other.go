//go:build !linux

package main

import "errors"

// openSink exists on non-Linux hosts only so the package compiles (and the
// tone synthesis and device-selection tests run) everywhere; GoSD boards are
// all Linux.
func openSink(string) (sink, error) {
	return nil, errors.New("ALSA PCM playback requires Linux")
}
