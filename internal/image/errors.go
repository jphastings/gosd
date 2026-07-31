package image

import "errors"

// ErrRawWriteOverlap is returned (wrapped) by Write when a RawWrite would
// clobber the MBR, a partition, or another RawWrite, instead of landing
// cleanly in the unpartitioned gap between the MBR and the boot partition.
var ErrRawWriteOverlap = errors.New("raw write overlaps the MBR, a partition, or another raw write")

// ErrBootPartitionFull is returned (wrapped) by Write when the boot partition
// (Spec.BootSizeBytes, or DefaultBootPartitionSizeBytes) ran out of room for
// the boot files it was given.
var ErrBootPartitionFull = errors.New("boot partition ran out of space while writing its files")
