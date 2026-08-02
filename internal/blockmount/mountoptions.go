package blockmount

// dataFlushEnvVar is the reserved env var gosd-init exports with the
// effective vfat "flush" mount-option setting for this device: config.json's
// baked gosd build --data-flush default, overridden per-device by
// gosd.toml's data_flush key (see cmd/gosd-init/internal/boot/sequence.go's
// effectiveDataFlush). emmc and disk mount from the app's own process, which
// has no access to config.json or gosd.toml directly, so this env var is
// the only channel between gosd-init's decision and this package's mount
// call (bean gosd-9m1k).
const dataFlushEnvVar = "GOSD_DATA_FLUSH"

// vfatMountOption returns the vfat mount(2) option string Mount passes for a
// FAT32 device: "flush" when GOSD_DATA_FLUSH=1, pushing a file's data and
// metadata to the card promptly on close(2) at a real write-throughput cost;
// "" (normal Linux writeback, ~30s dirty_expire) for anything else - unset,
// "0", or a garbled value, matching gosd-init's own default-false,
// malformed-falls-back-to-default behavior. getenv is injected (rather than
// calling os.Getenv directly) purely so this mapping stays a plain,
// deterministic function - it's the pure decision half of the platform
// seam; platform_linux.go's mountData is the syscall-adjacent half that
// calls it with the real process environment.
func vfatMountOption(getenv func(string) string) string {
	if getenv(dataFlushEnvVar) == "1" {
		return "flush"
	}
	return ""
}
