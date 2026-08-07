//go:build linux

package main

import "syscall"

// filesystemSizeBytes reports the filesystem mounted at path's total size:
// block size times block count, converting each field to int64 explicitly
// since syscall.Statfs_t's field widths differ across GOARCH (int32 on
// 32-bit armv6, int64/uint64 on arm64).
func filesystemSizeBytes(path string) (int64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, err
	}
	return int64(st.Bsize) * int64(st.Blocks), nil
}
