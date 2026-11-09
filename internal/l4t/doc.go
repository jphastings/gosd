// Package l4t builds, from NVIDIA's L4T R32.7.6 kit, the signed boot-chain
// files a Jetson Nano needs in USB recovery mode (two RCM messages, CBoot
// and a device tree), and every partition a first flash of its 16GB eMMC
// module writes, with the BCT that is downloaded in recovery mode too.
//
// No NVIDIA tool runs: the formats were worked out by comparing the kit's raw
// files with what NVIDIA's tools write, and the module's production fuses mean
// everything is signed with an all-zero AES-CMAC key. The kit itself is never
// re-hosted. [Kit.Open] fetches it from NVIDIA and [Kit.Read] streams it,
// checking the pinned hash and keeping only the members needed, so it also
// fits on a device with a tiny root filesystem. [Kit.Cache] keeps those
// members, a few MB, so the next flash needn't fetch the kit again.
package l4t
