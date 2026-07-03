package image

import "errors"

var errNotImplemented = errors.New("image assembly not implemented")

// ErrRawWriteOverlap is returned (wrapped) by Write when a RawWrite would
// clobber the MBR or the boot partition instead of landing in the
// unpartitioned gap between them.
var ErrRawWriteOverlap = errors.New("raw write overlaps the MBR or the boot partition")
