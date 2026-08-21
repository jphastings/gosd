---
gosd: minor
---

#### `gosd build --kernel-param` adds your own kernel command-line parameters

Some things can only be turned on from the kernel command line, and until now
a GoSD app had no way to put anything there. `gosd build --kernel-param
snd_bcm2835.enable_hdmi=1` (repeatable) now bakes a parameter into the image.

You write it once and GoSD delivers it wherever the board's family actually
reads its command line from — `cmdline.txt` on the Raspberry Pi boards, the
`append` line of `extlinux.conf` on the Rockchip and Allwinner boards — so a
bare `gosd build`, which builds every board, carries your parameters onto all
of them. A parameter one board's kernel doesn't recognise is inert there, just
as an unrecognised kernel parameter always is, so cross-board builds don't
need per-board flags.

Values are checked for shape and never for vocabulary: whitespace, newlines
and other characters that would corrupt the boot config are refused with an
error naming the offending value, but GoSD keeps no list of "known" kernel
parameters to fall foul of — which matters, since a `gosd build-kernel` kernel
can introduce parameters no such list would ever have had. Parameters render
in the order you pass them, after GoSD's own, so builds stay byte-identical.

`gosd run` mirrors the flag, extending qemu's kernel command line, so a
parameter can be tried under qemu before a card is ever flashed.
