package naming

import "strings"

const (
	// LabelPrefixMaxLength is the longest partition-label prefix that still
	// leaves room for both suffixes inside FAT's 11-byte volume-label limit:
	// 11 - len("-boot"). The data suffix is the same length, and the boot
	// partition is always FAT32 whatever `gosd build --data-filesystem`
	// chooses, so this one cap covers both labels.
	LabelPrefixMaxLength = 6

	// BootLabelSuffix and DataLabelSuffix are what LabelsFor appends to a
	// prefix to name each partition.
	BootLabelSuffix = "-boot"
	DataLabelSuffix = "-data"
)

// PartitionLabels is the pair of volume labels one image is built with: the
// name a person sees when they plug the flashed card into a computer.
type PartitionLabels struct {
	Boot, Data string
}

// LabelPrefix derives an app's default partition-label prefix from its name
// (`gosd build`'s deriveAppName): Sanitize's [a-z0-9-] charset, truncated to
// LabelPrefixMaxLength bytes, with any hyphen the truncation exposes at the
// new end trimmed off — the same re-trim Sanitize does at its own cap, and
// for the same reason ("sattra-boot" reads as a name, "abcde--boot" reads as
// a mistake). Every return value is a usable prefix: Sanitize never returns
// an empty string (it falls back to "app") and never returns one starting
// with a hyphen, so neither step here can empty it.
//
// The result is idempotent: feeding a prefix back through LabelPrefix (or
// through Sanitize first) returns it unchanged.
func LabelPrefix(appName string) string {
	prefix := Sanitize(appName)
	if len(prefix) > LabelPrefixMaxLength {
		prefix = strings.TrimRight(prefix[:LabelPrefixMaxLength], "-")
	}
	return prefix
}

// LabelsFor names both partitions from prefix: <prefix>-boot and
// <prefix>-data. It does not validate prefix — an explicit `gosd build
// --label-prefix` is used verbatim, and the CLI is where both resulting
// labels are checked against blockmount.ValidateLabel.
func LabelsFor(prefix string) PartitionLabels {
	return PartitionLabels{
		Boot: prefix + BootLabelSuffix,
		Data: prefix + DataLabelSuffix,
	}
}
