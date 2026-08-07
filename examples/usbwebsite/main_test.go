package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsAffirmative(t *testing.T) {
	yes := []string{"1", "true", "TRUE", "yes", "Yes", "on", " on "}
	for _, v := range yes {
		if !isAffirmative(v) {
			t.Errorf("isAffirmative(%q) = false, want true", v)
		}
	}

	no := []string{"", "0", "false", "no", "off", "banana"}
	for _, v := range no {
		if isAffirmative(v) {
			t.Errorf("isAffirmative(%q) = true, want false", v)
		}
	}
}

func TestDataPartitionFromMounts(t *testing.T) {
	tests := []struct {
		name   string
		mounts string
		want   dataPartition
		wantOK bool
	}{
		{
			name: "mounted by gosd-init",
			mounts: "/dev/mmcblk0p1 /boot vfat rw,nosuid,nodev 0 0\n" +
				"/dev/mmcblk0p2 /data vfat rw,nosuid,nodev,flush 0 0\n",
			want:   dataPartition{device: "/dev/mmcblk0p2", mounted: true},
			wantOK: true,
		},
		{
			name: "not mounted, derived from the boot partition's disk",
			mounts: "/dev/mmcblk0p1 /boot vfat rw,nosuid,nodev 0 0\n" +
				"tmpfs /data tmpfs ro,nosuid,nodev 0 0\n",
			want:   dataPartition{device: "/dev/mmcblk0p2"},
			wantOK: true,
		},
		{
			name:   "qemu-virt virtio disk",
			mounts: "/dev/vda1 /boot vfat rw,nosuid,nodev 0 0\n",
			want:   dataPartition{device: "/dev/vda2"},
			wantOK: true,
		},
		{
			name: "restacked vfat over the read-only fallback wins",
			mounts: "/dev/mmcblk0p1 /boot vfat rw 0 0\n" +
				"tmpfs /data tmpfs ro 0 0\n" +
				"/dev/mmcblk0p2 /data vfat rw,flush 0 0\n",
			want:   dataPartition{device: "/dev/mmcblk0p2", mounted: true},
			wantOK: true,
		},
		{
			name:   "no boot or data mounts at all",
			mounts: "proc /proc proc rw 0 0\ntmpfs /run tmpfs rw 0 0\n",
			wantOK: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := dataPartitionFromMounts(tt.mounts)
			if ok != tt.wantOK || got != tt.want {
				t.Errorf("dataPartitionFromMounts() = %+v, %v; want %+v, %v", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestSecondPartition(t *testing.T) {
	tests := map[string]string{
		"/dev/mmcblk0p1": "/dev/mmcblk0p2",
		"/dev/mmcblk1p1": "/dev/mmcblk1p2",
		"/dev/vda1":      "/dev/vda2",
		"/dev/mmcblk0p3": "", // not a first partition
		"":               "",
	}
	for dev, want := range tests {
		if got := secondPartition(dev); got != want {
			t.Errorf("secondPartition(%q) = %q, want %q", dev, got, want)
		}
	}
}

func TestIsTruncatedStarter(t *testing.T) {
	truncated := []string{
		"",
		starterPage[:1],
		starterPage[:len(starterPage)/2],
		starterPage[:len(starterPage)-1],
	}
	for _, v := range truncated {
		if !isTruncatedStarter(v) {
			t.Errorf("isTruncatedStarter(%q) = false, want true", v)
		}
	}

	notTruncated := []string{
		starterPage,                             // complete, matches exactly
		starterPage + "\n<p>extra</p>",          // complete, with user additions
		"<!doctype html><title>My site</title>", // unrelated user content
	}
	for _, v := range notTruncated {
		if isTruncatedStarter(v) {
			t.Errorf("isTruncatedStarter(%q) = true, want false", v)
		}
	}
}

func TestEnsureStarterPage(t *testing.T) {
	t.Run("writes the starter page when none exists", func(t *testing.T) {
		dir := t.TempDir()
		ensureStarterPage(dir)
		assertIndexContent(t, dir, starterPage)
		assertNoTmpFileLeftBehind(t, dir)
	})

	t.Run("repairs an empty index.html left by an interrupted write", func(t *testing.T) {
		dir := t.TempDir()
		writeIndex(t, dir, "")
		ensureStarterPage(dir)
		assertIndexContent(t, dir, starterPage)
	})

	t.Run("repairs a truncated index.html left by an interrupted write", func(t *testing.T) {
		dir := t.TempDir()
		writeIndex(t, dir, starterPage[:len(starterPage)/2])
		ensureStarterPage(dir)
		assertIndexContent(t, dir, starterPage)
	})

	t.Run("leaves the user's own content alone", func(t *testing.T) {
		dir := t.TempDir()
		const userContent = "<!doctype html><title>My real site</title>"
		writeIndex(t, dir, userContent)
		ensureStarterPage(dir)
		assertIndexContent(t, dir, userContent)
	})
}

func writeIndex(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(content), 0o644); err != nil {
		t.Fatalf("writing index.html fixture: %v", err)
	}
}

func assertIndexContent(t *testing.T, dir, want string) {
	t.Helper()
	got, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatalf("reading index.html: %v", err)
	}
	if string(got) != want {
		t.Errorf("index.html = %q, want %q", got, want)
	}
}

func assertNoTmpFileLeftBehind(t *testing.T, dir string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(dir, "index.html.tmp")); !os.IsNotExist(err) {
		t.Errorf("index.html.tmp should not survive a successful write, stat err = %v", err)
	}
}
