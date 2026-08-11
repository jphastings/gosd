//go:build gosd

package fault

import "github.com/jphastings/gosd/internal/faultdrop"

// runDir is where this binary hands reports and secret registrations to
// gosd-init: /run, a tmpfs gosd-init mounts before it ever starts an app.
//
// The gate is the `gosd` build tag rather than a probe of the filesystem,
// because the tag is set on the app compile `gosd build` performs and on
// nothing else (see internal/boards.BuildTags), while /run is writable on
// any ordinary Linux machine running as root. A probe would make a CI
// container look exactly like a board: it would leave a drop file nothing
// will ever read, and tell a developer their device is about to halt when
// there is no device.
const runDir = faultdrop.Dir
