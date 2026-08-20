package diskfmt

// A FAT32 volume is self-consistent only when its file allocation table has an
// entry for every cluster the volume advertises: with 32-bit entries in
// 512-byte sectors, sectorsPerFAT*128 >= clusterCount+2 (entries 0 and 1 are
// reserved markers rather than clusters).
//
// go-diskfs v1.9.3 solves for sectorsPerFAT with the numerator
// 4*(totalSectors-32), but that constraint solves to
// 4*(totalSectors-32) + 8*sectorsPerCluster (derivation on
// fat32SelfConsistentSectorLimit below), so its ceiling under-rounds whenever
// the division lands within a cluster of exact. The volume then advertises one
// or two more clusters than either FAT can index. It goes unnoticed on Linux,
// which clamps silently, but macOS First Aid and Windows chkdsk both call such
// a volume damaged ("FAT size too small, N entries won't fit") and offer to
// rewrite its BPB — so an app author hears that GoSD broke their card.
//
// Only the top sectorsPerCluster+1 sectors of each sectorsPerFAT band are
// defective, which is ~0.8% of sizes overall but 11% of whole-GiB sizes (16,
// 32, 64, 128 and 256 GiB among them, plus MaxFAT32Bytes itself) — round sizes
// land near a band's top far more often than chance.
//
// The dependency is not patched here (see bean gosd-e3e3 for the one-line
// upstream fix). Instead every GoSD FAT32 format hands go-diskfs a size it
// lays out correctly, trimmed by at most two clusters — 32.5 KiB at the
// largest cluster size, which no caller can notice. fat32limit.go's mirror of
// go-diskfs's arithmetic stays exactly as it is: this file derives from that
// mirror rather than correcting it, because the MaxFAT32Bytes ceiling guard
// depends on it modelling what go-diskfs actually does.
//
// This whole file, and fat32limit.go, are a bet that go-diskfs v1.9.3's
// internal arithmetic stays fixed (bean gosd-qvjs): nothing here would fail
// to compile or fail fast if a future go-diskfs bump changed
// sectorsPerFAT's formula or its cluster-size table. Before bumping the
// go.mod pin, re-run TestFAT32MirroredArithmeticMatchesGoDiskfsRealOutput
// (fat32limit_test.go) — it formats real go-diskfs FAT32 volumes and checks
// the on-disk BPB against fat32SectorsPerFAT/fat32SectorsPerCluster's
// predictions, so a changed formula fails there first, before it can ship
// as a silently-wrong LargestSelfConsistentFAT32Bytes/MaxFAT32Bytes. A
// passing re-run does not by itself mean this file is still correct for
// the new version — it only proves the specific sizes that test covers;
// re-derive the arithmetic in this file's and fat32limit.go's doc comments
// against the new version's source before trusting it further.

// LargestSelfConsistentFAT32Bytes returns the largest size no greater than
// sizeBytes for which go-diskfs's own sectors-per-FAT formula yields a FAT big
// enough to index every cluster the resulting volume would advertise. It is
// sizeBytes itself for all but ~0.8% of sizes, and never more than two
// clusters below it.
//
// Callers that choose a FAT32 volume's size before handing it to go-diskfs —
// FormatFAT32 for a whole device or partition, internal/image for the
// data partition it lays out — pass it through here first.
func LargestSelfConsistentFAT32Bytes(sizeBytes int64) int64 {
	candidate := sizeBytes
	for {
		totalSectors := candidate / sectorSizeBytes
		if totalSectors <= fat32ReservedSectors {
			// Too small for go-diskfs to lay out at all; let it say so in its
			// own words rather than trimming a size that has no answer.
			return sizeBytes
		}
		limit := fat32SelfConsistentSectorLimit(candidate)
		if totalSectors-fat32ReservedSectors <= limit {
			return candidate
		}
		// Trimming can drop the volume into a smaller cluster-size class,
		// which moves the limit, so re-derive against the new size. Each pass
		// strictly shrinks the candidate, so this settles (in practice after
		// at most one extra pass per class boundary crossed).
		candidate = (limit + fat32ReservedSectors) * sectorSizeBytes
	}
}

// fat32SelfConsistentSectorLimit is the most non-reserved sectors a volume of
// sizeBytes may span before go-diskfs's sectors-per-FAT ceiling under-rounds.
//
// Writing N for the non-reserved sectors, F for go-diskfs's sectors per FAT
// and S for its sectors per cluster, the volume's data area is N-2F sectors
// and holds floor((N-2F)/S) clusters. Requiring the FAT to index them all —
// 128F >= floor((N-2F)/S) + 2 — and solving for N over the integers gives
// N <= F*(128S+2) - (S+1), which is what this returns. Solving the same
// inequality for F instead gives F >= (4N + 8S) / (512S+8): go-diskfs's
// numerator, missing its 8S term.
//
// Because 512S+8 is exactly 4*(128S+2), go-diskfs's F is ceil(N/(128S+2)) —
// so F*(128S+2) is the top of F's band, and only its last S+1 sectors are
// defective.
func fat32SelfConsistentSectorLimit(sizeBytes int64) int64 {
	sectorsPerCluster := fat32SectorsPerCluster(sizeBytes)
	bandTop := fat32SectorsPerFAT(sizeBytes) * (fat32PerFATDenominator(sizeBytes) / 4)
	return bandTop - (sectorsPerCluster + 1)
}
