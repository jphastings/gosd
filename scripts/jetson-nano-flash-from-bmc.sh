#!/bin/sh
# Flash a Jetson Nano eMMC module in a Turing Pi 2 node, from the BMC, by
# running NVIDIA's own massflash scripts out of an offline-generated
# mfi_jetson-nano-devkit-emmc blob (docs/jetson-nano-flashing.md).
#
# NVIDIA's tools are 32-bit x86 programs, so each one the scripts call is
# wrapped to run under a 32-bit ARM qemu-i386-static kept beside this script.
#
#   usage: jetson-nano-flash-from-bmc.sh <node> <extracted mfi directory>
set -e
usage="usage: $0 <node> <extracted mfi directory>"
node=${1:?$usage}
mfi=$(cd "${2:?$usage}" && pwd)
qemu=$(cd "$(dirname "$0")" && pwd)/qemu-i386-static
[ -x "$qemu" ] || { echo "no executable qemu-i386-static beside $0" >&2; exit 1; }

for tool in tegrabct tegrarcm tegradevflash; do
	[ -f "$mfi/$tool.x86" ] || mv "$mfi/$tool" "$mfi/$tool.x86"
	printf '#!/bin/sh\nexec %s "$0.x86" "$@"\n' "$qemu" > "$mfi/$tool"
	chmod +x "$mfi/$tool"
done

# flash mode holds the module's recovery strap and routes its recovery USB
# port to the BMC (not to the USB_OTG socket).
tpi power -n "$node" off
tpi usb flash -n "$node" --bmc
tpi power -n "$node" on
i=0
until grep -qs 7f21 /sys/bus/usb/devices/*/idProduct; do
	i=$((i + 1))
	[ $i -gt 30 ] && { echo "the Nano never appeared in recovery mode (USB 0955:7f21)" >&2; exit 1; }
	sleep 1
done

cd "$mfi"
rm -rf mfilogs
status=0
./nvmflash.sh || status=$?
cat mfilogs/*.log
# Release the recovery strap so the next power-on boots the eMMC.
tpi usb device -n "$node" --bmc
exit $status
