//go:build !linux

package sound

import "errors"

// errUnsupportedPlatform is what Open reports off Linux. GoSD boards are all
// Linux; this stub exists so an app importing this package still builds — and
// its own tests still run — on the developer's macOS or Windows host, faking
// Device where they need playback.
var errUnsupportedPlatform = errors.New("sound: audio playback is only supported on Linux boards")

func open(Options) (Device, error) { return nil, errUnsupportedPlatform }
