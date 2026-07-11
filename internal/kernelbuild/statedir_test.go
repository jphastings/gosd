package kernelbuild

import (
	"path/filepath"
	"strings"
	"testing"
)

func noEnv(string) string { return "" }

// The build root must never resolve into an OS-evictable cache location
// (macOS purges ~/Library/Caches under storage pressure, killing live
// container bind mounts mid-build - gosd-l4y9) and must stay under the
// user's home so Docker Desktop / podman machine share it with their VMs.
func TestBuildRootFor(t *testing.T) {
	t.Run("darwin uses Application Support", func(t *testing.T) {
		root, err := buildRootFor("darwin", noEnv)
		if err != nil {
			t.Fatal(err)
		}
		want := filepath.Join("Library", "Application Support", "gosd", "kernel-build")
		if !strings.HasSuffix(root, want) {
			t.Errorf("darwin root = %s, want suffix %s", root, want)
		}
		if strings.Contains(root, "Caches") {
			t.Errorf("darwin root %s is inside an evictable Caches directory", root)
		}
	})

	t.Run("linux honors XDG_STATE_HOME", func(t *testing.T) {
		getenv := func(k string) string {
			if k == "XDG_STATE_HOME" {
				return "/home/u/.state"
			}
			return ""
		}
		root, err := buildRootFor("linux", getenv)
		if err != nil {
			t.Fatal(err)
		}
		if root != filepath.Join("/home/u/.state", "gosd", "kernel-build") {
			t.Errorf("linux root = %s, want under $XDG_STATE_HOME", root)
		}
	})

	t.Run("linux defaults to ~/.local/state", func(t *testing.T) {
		root, err := buildRootFor("linux", noEnv)
		if err != nil {
			t.Fatal(err)
		}
		want := filepath.Join(".local", "state", "gosd", "kernel-build")
		if !strings.HasSuffix(root, want) {
			t.Errorf("linux root = %s, want suffix %s", root, want)
		}
	})
}
