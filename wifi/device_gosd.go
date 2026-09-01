//go:build gosd

package wifi

import "github.com/jphastings/gosd/internal/wifictl"

// runDir is where Join drops a request for gosd-init and reads its status
// back from: /run/gosd/wifi, tmpfs gosd-init mounts before it ever starts
// an app.
//
// The gate is the `gosd` build tag rather than a probe of the filesystem,
// for the same reason fault/device_gosd.go uses it: the tag is set on the
// app compile `gosd build` performs and on nothing else (see
// internal/boards.BuildTags), while /run is writable on any ordinary Linux
// machine running as root. A probe would make a CI container look exactly
// like a board.
const runDir = wifictl.Dir
