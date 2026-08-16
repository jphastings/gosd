---
# gosd-3id7
title: U-Boot once failed to read /Image from our FAT32 boot partition ("Invalid FAT entry") on a first boot after flashing
status: todo
type: bug
priority: normal
created_at: 2026-08-16T19:26:01Z
updated_at: 2026-08-16T19:36:13Z
---

Seen once on the Cubie A5E bench (bean gosd-6pfn), on the **first boot after a
flash**. U-Boot found extlinux.conf, then failed reading the kernel off the
FAT32 boot partition:

```
Scanning mmc 0:1...
Found /extlinux/extlinux.conf
Retrieving file: /extlinux/extlinux.conf
1:	gosd
Retrieving file: /Image
Invalid FAT entry
Skipping gosd for failure retrieving kernel
EXTLINUX FAILED: continuing...
```

It then fell through to PXE/DHCP boot and sat there. **A plain power cycle, with
no reflash and no change to the card, booted the identical image cleanly** and
every subsequent boot of that card worked, so the bytes on the card are fine and
this is not a corrupt image.

Not reproduced since — recorded so the next person who sees it has the prior.

## Why it's worth a bean rather than a shrug

The user-visible symptom is "I flashed the card and the board didn't boot",
which is indistinguishable from a bad flash and invites a reflash (which would
"fix" it and hide the cause).

Two candidate directions, neither confirmed:

- **Our FAT32 layout vs U-Boot's FAT driver.** `internal/diskfmt` already
  carries mitigations for go-diskfs under-sizing FATs (bean gosd-e3e3), and
  "Invalid FAT entry" is U-Boot's complaint about a cluster-chain entry it
  cannot make sense of. A layout that Linux, macOS and U-Boot-on-second-read
  all accept, but U-Boot rejects once, would fit a marginal/edge-case chain
  more than a random bit flip.
- **The card itself right after a large write.** The failure was on the first
  read after ~2.3GiB had just been written through the sdwire mux; SD FTLs can
  be slow to settle. This would make it a bench artefact, not a gosd bug.

## How to tell them apart

- Reflash and boot repeatedly (10+), counting first-boot failures, on this
  board and on a second board with the same card, then on a different card.
- If it recurs, dump the boot partition's FAT chain for `/Image` from the image
  file and compare against what U-Boot's driver expects — the image is the
  cheap thing to inspect, since the same bytes read fine later.

## Todos

- [ ] Attempt a repeat-flash reproduction and record the rate
- [ ] If reproduced, compare the FAT cluster chain against U-Boot's parser



## Update from the same session: our formatter is probably not at fault

A fresh `diskfmt` FAT32 image (512MiB, same code path that builds a data
partition) was checked under Linux in a container:

- `fsck.vfat -n -v` straight after formatting: structurally clean. The only
  remark was `Free cluster summary uninitialized (should be 130811)` — the
  FSInfo free count, which Linux recomputes; benign.
- 200 rounds of create/rewrite/delete against it, mounted: **no `FAT-fs`
  kernel messages at all**, and `fsck.vfat` clean afterwards (203 files,
  1768/130812 clusters).

That doesn't clear the boot partition's layout specifically (different size and
contents), but it does mean there is no easily-triggered defect in what
`diskfmt` writes. The card-settling-after-a-2.3GiB-write hypothesis is now the
more likely of the two.
