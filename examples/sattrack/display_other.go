//go:build !linux

package main

import (
	"errors"
	"image"
)

// display exists on non-linux hosts only so the package compiles (and its
// pure rendering logic tests run) everywhere; GoSD boards are all Linux.
type display struct{}

func openDisplay() (*display, error) {
	return nil, errors.New("DRM/KMS output requires Linux")
}

func (d *display) Size() image.Point { return image.Point{} }

func (d *display) Flush(*image.RGBA, []image.Rectangle) error { return nil }

func (d *display) Close() error { return nil }
