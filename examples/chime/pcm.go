package main

import (
	"bufio"
	"fmt"
	"io"
	"path"
	"sort"
	"strconv"
	"strings"
)

// Choosing which audio device to play to is pure text handling, so it lives
// here and its tests run on any host. The kernel ABI it feeds is in
// pcm_linux.go.

const bytesPerSample = 2 // S16_LE

// format is the PCM stream shape this example plays: interleaved signed
// 16-bit little-endian frames.
type format struct {
	rate     int
	channels int
}

func (f format) frameBytes() int { return f.channels * bytesPerSample }

// pcmDevice is one playback PCM the kernel is exposing.
type pcmDevice struct {
	card, device int
	id, name     string
}

func (d pcmDevice) path() string {
	return fmt.Sprintf("/dev/snd/pcmC%dD%dp", d.card, d.device)
}

func (d pcmDevice) String() string {
	if d.name == "" {
		return d.path()
	}
	return fmt.Sprintf("%s (%s)", d.name, d.path())
}

// isHDMI reports whether this PCM looks like an HDMI/SPDIF sink. The kernel
// offers no machine-readable flag for it — drivers just name the device
// descriptively ("bcm2835 HDMI 1", "vc4-hdmi", "bcm2835 IEC958/HDMI") — so
// the name is all there is to go on.
func (d pcmDevice) isHDMI() bool {
	s := strings.ToUpper(d.id + " " + d.name)
	return strings.Contains(s, "HDMI") || strings.Contains(s, "IEC958")
}

// parseProcPCM reads /proc/asound/pcm, whose lines the kernel formats as
// "%02i-%02i: <id> : <name>" followed by " : playback N" and/or
// " : capture N". Only playback-capable devices are returned.
func parseProcPCM(r io.Reader) []pcmDevice {
	var out []pcmDevice
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		fields := strings.Split(strings.TrimSpace(sc.Text()), " : ")
		if len(fields) < 2 {
			continue
		}
		playback := false
		for _, f := range fields[1:] {
			if strings.HasPrefix(f, "playback") {
				playback = true
			}
		}
		if !playback {
			continue
		}
		head := strings.SplitN(fields[0], ": ", 2)
		if len(head) != 2 {
			continue
		}
		nums := strings.SplitN(head[0], "-", 2)
		if len(nums) != 2 {
			continue
		}
		card, err := strconv.Atoi(nums[0])
		if err != nil {
			continue
		}
		device, err := strconv.Atoi(nums[1])
		if err != nil {
			continue
		}
		out = append(out, pcmDevice{card: card, device: device, id: head[1], name: fields[1]})
	}
	return out
}

// parseDevSnd is the fallback when /proc/asound isn't mounted: derive the
// card and device numbers from /dev/snd/pcmC<card>D<device>p names, with no
// idea what any of them is called.
func parseDevSnd(paths []string) []pcmDevice {
	var out []pcmDevice
	for _, p := range paths {
		base := path.Base(p)
		if !strings.HasPrefix(base, "pcmC") || !strings.HasSuffix(base, "p") {
			continue
		}
		card, device, ok := strings.Cut(strings.TrimSuffix(strings.TrimPrefix(base, "pcmC"), "p"), "D")
		if !ok {
			continue
		}
		c, err := strconv.Atoi(card)
		if err != nil {
			continue
		}
		d, err := strconv.Atoi(device)
		if err != nil {
			continue
		}
		out = append(out, pcmDevice{card: c, device: d})
	}
	return out
}

// rank orders candidate devices best-first: HDMI sinks ahead of everything
// else (this example's headline output), then by card and device number so
// the choice is stable across boots.
func rank(devices []pcmDevice) []pcmDevice {
	out := make([]pcmDevice, len(devices))
	copy(out, devices)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].isHDMI() != out[j].isHDMI() {
			return out[i].isHDMI()
		}
		if out[i].card != out[j].card {
			return out[i].card < out[j].card
		}
		return out[i].device < out[j].device
	})
	return out
}
