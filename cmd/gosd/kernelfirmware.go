package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/jphastings/gosd/internal/fetch"
	"github.com/jphastings/gosd/internal/kernelconfig"
)

// kernelFirmwareCacheDir is where gosd-kernel.toml's [[firmware]] entries
// are cached across builds, kept separate from board artifact caches
// (artifactCacheDir) since these files are developer-declared rather than
// board-declared.
func kernelFirmwareCacheDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("locating a user cache directory for kernel firmware downloads failed: %w", err)
	}
	return filepath.Join(base, "gosd", "kernel-firmware"), nil
}

// loadKernelConfigFirmware loads gosd-kernel.toml (--kernel-config, or the
// default gosd-kernel.toml in the working directory, per the same discovery
// rule build-kernel's --config uses) and resolves its [[firmware]] entries
// to local cached paths, keyed by dest. No config file found at all - the
// common case - yields a nil map and no error: most gosd build invocations
// never touch gosd-kernel.toml or the network for it.
func loadKernelConfigFirmware(ctx context.Context, explicitConfigPath string) (map[string]string, error) {
	cfg, _, err := loadKernelConfig(explicitConfigPath)
	if err != nil {
		return nil, err
	}
	if len(cfg.Firmware) == 0 {
		return nil, nil
	}

	cacheDir, err := kernelFirmwareCacheDir()
	if err != nil {
		return nil, err
	}
	return resolveKernelFirmware(ctx, cfg.Firmware, cacheDir)
}

// resolveKernelFirmware fetches and sha256-verifies every entry in firmware
// via internal/fetch - the same URL+sha256 machinery board artifacts use, so
// these files are never re-hosted either - and returns each entry's local
// cached path keyed by its dest (the path it lands at under /lib/firmware in
// the initramfs). Caching by "<sha256>-<base name of dest>" rather than dest
// alone means two different gosd-kernel.toml files that happen to choose the
// same dest, but pin different content, never collide in the shared cache.
func resolveKernelFirmware(ctx context.Context, firmware []kernelconfig.FirmwareFile, cacheDir string) (map[string]string, error) {
	paths := make(map[string]string, len(firmware))
	for _, f := range firmware {
		name := f.SHA256 + "-" + filepath.Base(f.Dest)
		path, err := fetch.ToDir(ctx, nil, fetch.File{URL: f.URL, SHA256: f.SHA256}, cacheDir, name)
		if err != nil {
			return nil, fmt.Errorf("fetching gosd-kernel.toml [[firmware]] entry for dest %q failed: %w", f.Dest, err)
		}
		paths[f.Dest] = path
	}
	return paths, nil
}

// openKernelFirmware opens a fresh reader for each of paths (dest -> local
// cached file path), so the one resolveKernelFirmware fetch shared across
// every selected board can still be embedded independently in each board's
// own initramfs - pipeline.Assemble closes every reader it's handed once
// that board's build is done, so each board needs its own *os.File.
//
// If any file fails to open, every reader already opened in this call is
// closed before returning the error, so a partial failure never leaks file
// descriptors.
func openKernelFirmware(paths map[string]string) (map[string]io.Reader, error) {
	files := make(map[string]io.Reader, len(paths))
	for dest, path := range paths {
		f, err := os.Open(path)
		if err != nil {
			for _, opened := range files {
				if c, ok := opened.(io.Closer); ok {
					_ = c.Close()
				}
			}
			return nil, fmt.Errorf("opening cached firmware file for dest %q at %s: %w", dest, path, err)
		}
		files[dest] = f
	}
	return files, nil
}
