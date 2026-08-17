---
gosd: patch
---

#### A data partition reformatted from ext4 to FAT32 no longer halts the device

Formatting a volume as FAT32 over a previous ext4 volume left the old ext4
superblock intact, because the FAT32 writer never touches the offset it sits
at. gosd-init then identified the dead filesystem in preference to the live
one and halted the board on its next boot, reporting corruption and the old
volume's label. Establishing a volume now clears any previous filesystem's
signatures first, so changing `--data-filesystem` between releases reformats
cleanly — as documented — rather than stopping a healthy device.
