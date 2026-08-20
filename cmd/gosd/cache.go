package main

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/jphastings/gosd/internal/extbuild"
	"github.com/jphastings/gosd/internal/kernelbuild"
)

// cacheLocation names one directory gosd caches something in. Expensive
// locations are the durable build-kernel/build-external state dirs (bean
// gosd-9o73): 20-75 minutes to rebuild an entry, so `gosd cache clean`
// leaves them alone unless the developer explicitly opts in with --builds.
// Every other location is a pinned, sha256-verified download that gosd
// re-fetches transparently on the next build/run - cheap to lose, and
// already auto-pruned to the current pin after every successful
// build/run (bean gosd-gdro).
type cacheLocation struct {
	name      string
	dir       func() (string, error)
	expensive bool
}

func cacheLocations() []cacheLocation {
	return []cacheLocation{
		{name: "board artifacts", dir: artifactCacheDir},
		{name: "CA certificate bundle", dir: caCertsCacheDir},
		{name: "ingress binaries (cloudflared)", dir: ingressCacheDir},
		{name: "kernel firmware (gosd-kernel.toml [[firmware]])", dir: kernelFirmwareCacheDir},
		{name: "build-kernel state", dir: kernelbuild.BuildRoot, expensive: true},
		{name: "build-external state", dir: extbuild.BuildRoot, expensive: true},
	}
}

func newCacheCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cache",
		Short: "Inspect or clear gosd's on-disk caches",
		Long: `Inspect or clear the directories gosd caches downloaded artifacts and
(optionally) built kernels/externals in.

Everyday growth is already bounded automatically: gosd prunes its pinned
download caches (board artifacts, the CA bundle, ingress binaries, kernel
firmware) to the current pin after every successful build/run, and the
build-kernel/build-external state directory keeps only its most recently
used entries. This command is for manual visibility and control, not a fix
you need to run routinely.`,
	}
	cmd.AddCommand(newCacheDirCmd())
	cmd.AddCommand(newCacheSizeCmd())
	cmd.AddCommand(newCacheCleanCmd())
	return cmd
}

func newCacheDirCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "dir",
		Short: "Print the paths gosd caches things in",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			for _, loc := range cacheLocations() {
				dir, err := loc.dir()
				if err != nil {
					cmd.PrintErrf("%s: unavailable (%v)\n", loc.name, err)
					continue
				}
				cmd.Printf("%s: %s\n", loc.name, dir)
			}
			return nil
		},
	}
}

func newCacheSizeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "size",
		Short: "Report how much disk space gosd's caches use",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var total int64
			var errs []error
			for _, loc := range cacheLocations() {
				dir, err := loc.dir()
				if err != nil {
					cmd.PrintErrf("%s: unavailable (%v)\n", loc.name, err)
					errs = append(errs, err)
					continue
				}
				size, err := dirSizeBytes(dir)
				if err != nil {
					cmd.PrintErrf("%s: could not be measured (%v)\n", loc.name, err)
					errs = append(errs, err)
					continue
				}
				total += size
				cmd.Printf("%-8s %s (%s)\n", humanizeBinaryBytes(size), loc.name, dir)
			}
			cmd.Printf("%-8s total\n", humanizeBinaryBytes(total))
			return errors.Join(errs...)
		},
	}
}

var cacheCleanBuilds bool

func newCacheCleanCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "clean",
		Short: "Delete gosd's cached downloads (and, with --builds, its build-kernel/build-external state)",
		Long: `Delete gosd's cached downloads: board artifacts, the CA certificate bundle,
ingress binaries and kernel firmware. Every one of these is a pinned,
sha256-verified download - the next build/run that needs one simply
re-fetches it, so this is always safe.

--builds additionally deletes the durable build-kernel/build-external
state directory. That state is NOT a download cache: it holds every kernel
and external binary gosd build-kernel/gosd build-external has compiled,
each of which costs 20-75 minutes of container build time to reproduce.
Plain "gosd cache clean" never touches it for exactly that reason - pass
--builds only when you mean to pay that cost back.`,
		Args: cobra.NoArgs,
		RunE: runCacheClean,
	}
	cmd.Flags().BoolVar(&cacheCleanBuilds, "builds", false,
		"also delete the build-kernel/build-external cache (each entry costs 20-75 minutes to rebuild)")
	return cmd
}

func runCacheClean(cmd *cobra.Command, _ []string) error {
	var errs []error
	var freed int64
	for _, loc := range cacheLocations() {
		if loc.expensive && !cacheCleanBuilds {
			continue
		}
		dir, err := loc.dir()
		if err != nil {
			cmd.PrintErrf("%s: unavailable (%v)\n", loc.name, err)
			errs = append(errs, err)
			continue
		}
		size, err := dirSizeBytes(dir)
		if err != nil && !os.IsNotExist(err) {
			cmd.PrintErrf("%s: could not be measured (%v)\n", loc.name, err)
		}
		if err := os.RemoveAll(dir); err != nil {
			cmd.PrintErrf("%s: removing %s failed (%v)\n", loc.name, dir, err)
			errs = append(errs, err)
			continue
		}
		freed += size
		cmd.Printf("removed %s (freed %s): %s\n", loc.name, humanizeBinaryBytes(size), dir)
	}
	if cacheCleanBuilds {
		cmd.PrintErrf("gosd cache clean --builds: every cached kernel/external build was removed; the next gosd build-kernel/build-external for each will take 20-75 minutes again\n")
	}
	cmd.Printf("gosd cache clean: freed %s\n", humanizeBinaryBytes(freed))
	return errors.Join(errs...)
}

// dirSizeBytes sums the size of every regular file under dir. A missing dir
// is 0 bytes, not an error - nothing has been cached there yet.
func dirSizeBytes(dir string) (int64, error) {
	var total int64
	err := filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		total += info.Size()
		return nil
	})
	if os.IsNotExist(err) {
		return 0, nil
	}
	return total, err
}
