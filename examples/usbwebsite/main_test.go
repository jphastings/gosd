package main

import "testing"

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
