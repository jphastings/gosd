package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/jphastings/gosd/internal/blockmount"
	"github.com/jphastings/gosd/internal/diskfmt"
	"github.com/jphastings/gosd/internal/naming"
)

// resolveLabels decides the volume labels both of an image's partitions are
// formatted with: <prefix>-boot and <prefix>-data.
//
// Without --label-prefix (flagGiven false), the prefix is derived from the
// app's own name (naming.LabelPrefix: sanitized, then truncated to fit FAT's
// 11-byte label limit alongside the 5-byte suffixes). An explicit
// --label-prefix is used exactly as typed — no lowercasing, no sanitizing,
// no truncating: someone naming their own partitions gets the name they
// asked for, or an error saying why they can't have it. Passing the flag
// with an empty value is that error rather than a silent fall-back to the
// default, since it can only be a mistake.
//
// Both resulting labels are then checked against blockmount.ValidateLabel,
// the single authority on what a volume label may contain, so a prefix that
// would only fail deep inside the FAT formatter — or, worse, silently round
// trip as something else — is refused here instead.
func resolveLabels(flagPrefix string, flagGiven bool, appName string) (naming.PartitionLabels, error) {
	prefix := flagPrefix
	if !flagGiven {
		prefix = naming.LabelPrefix(appName)
	} else if err := checkLabelPrefix(prefix); err != nil {
		return naming.PartitionLabels{}, err
	}

	labels := naming.LabelsFor(prefix)
	for _, label := range []string{labels.Boot, labels.Data} {
		// FAT32's rules for both: the boot partition is always FAT32
		// whatever --data-filesystem chooses, and a label FAT32 accepts is
		// acceptable to every filesystem the data partition can hold.
		if err := blockmount.ValidateLabel("gosd", diskfmt.FAT32, label); err != nil {
			return naming.PartitionLabels{}, fmt.Errorf("the label prefix %q makes the volume label %q, which can't be used: %w; pass a different --label-prefix", prefix, label, err)
		}
	}
	return labels, nil
}

// checkLabelPrefix rejects an explicit --label-prefix that cannot make a
// usable pair of labels, reported against the flag the person actually typed
// (rather than against a label they never saw) and naming a prefix that
// would work wherever there's an obvious one. These are the rules worth
// stating in terms of the prefix; every other label rule — including FAT's
// 8th-byte space hazard — belongs to blockmount.ValidateLabel, which
// resolveLabels applies to both finished labels regardless.
//
// The charset is deliberately narrower than "whatever a FAT32 label can
// hold": a volume label lives in a FAT short-name directory entry, so the
// 8.3-reserved punctuation (/ \ : * ? " < > | and friends) has no business
// being there, and spaces are silently stripped when the label is read back.
// Letters, digits, hyphen and underscore are simple to state, can't collide
// with any filesystem's own rules, and still keep case exactly as typed.
func checkLabelPrefix(prefix string) error {
	if prefix == "" {
		return fmt.Errorf("--label-prefix is empty; pass a prefix of up to %d characters (the partitions become <prefix>%s and <prefix>%s), or omit the flag entirely to name them after the app",
			naming.LabelPrefixMaxLength, naming.BootLabelSuffix, naming.DataLabelSuffix)
	}
	for _, r := range prefix {
		if !labelPrefixRune(r) {
			return fmt.Errorf("--label-prefix %q contains %q; a prefix may only use letters, digits, hyphens and underscores (case is kept exactly as typed) — run the words together, or join them with a hyphen", prefix, r)
		}
	}
	if len(prefix) > naming.LabelPrefixMaxLength {
		return fmt.Errorf("--label-prefix %q is %d characters; it must be at most %d, so both %q and %q fit FAT32's 11-character volume label — try --label-prefix=%s",
			prefix, len(prefix), naming.LabelPrefixMaxLength,
			prefix+naming.BootLabelSuffix, prefix+naming.DataLabelSuffix,
			prefix[:naming.LabelPrefixMaxLength])
	}
	return nil
}

// printPartitionLabels reports the volume labels this build is stamping, so
// whoever flashes the card knows what will appear on their desktop. The data
// half is only mentioned when there will be a data partition to carry it:
// with --data-size=0 the label is still resolved (and still baked into
// config.json, where it costs nothing), but no partition is ever created for
// it, so naming it here would only invite a hunt for a drive that can't
// exist.
func printPartitionLabels(cmd *cobra.Command, command string, labels naming.PartitionLabels, hasDataPartition bool) {
	if !hasDataPartition {
		cmd.PrintErrf("%s: boot partition label: %s\n", command, labels.Boot)
		return
	}
	cmd.PrintErrf("%s: partition labels: %s (boot), %s (data)\n", command, labels.Boot, labels.Data)
}

// labelPrefixRune reports whether r is one of the characters an explicit
// --label-prefix may contain: [A-Za-z0-9_-], verbatim case.
func labelPrefixRune(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return true
	default:
		return r == '-' || r == '_'
	}
}
