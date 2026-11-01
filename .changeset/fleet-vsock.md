---
artifacts: minor
---

#### vsock for hosting VMs in every arm64 kernel

Every arm64 board's stock kernel now has AF_VSOCK and vhost-vsock (`/dev/vhost-vsock`), so an app can run QEMU/KVM guests that talk to it over vsock. The Pi 3B, Zero 2 W and CM4 kernels already had it; it's now asserted so a future kernel bump can't drop it. The Pi Zero W is excluded: it has no KVM. The qemu-virt kernel also gets the guest transport.
