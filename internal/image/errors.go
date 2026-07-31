package image

import "errors"

// ErrRawWriteOverlap is returned (wrapped) by Write when a RawWrite would
// clobber the MBR, a partition, or another RawWrite, instead of landing
// cleanly in the unpartitioned gap between the MBR and the boot partition.
var ErrRawWriteOverlap = errors.New("raw write overlaps the MBR, a partition, or another raw write")
