// Package l4t builds the signed boot-chain files a Jetson Nano needs in USB
// recovery mode (two RCM messages, a BCT, CBoot and a device tree) from
// NVIDIA's L4T R32.7.6 kit.
//
// No NVIDIA tool runs: the formats were worked out by comparing the kit's raw
// files with what NVIDIA's tools write, and the module's production fuses mean
// everything is signed with an all-zero AES-CMAC key. The kit itself is never
// re-hosted. [Kit.Download] fetches it from NVIDIA at a pinned hash and
// streams it, keeping only the four members needed, so it also fits on a
// device with a tiny root filesystem.
package l4t
