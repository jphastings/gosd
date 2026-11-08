package jetsonflash

import (
	"bytes"
	"io"
	"slices"
	"testing"
)

// versionReply is what the applet and CBoot send back to a version query.
func versionReply() [][]byte {
	status := devicePacket(packetCmd, 3, le32s(cmdStatus, 0))
	reply := slices.Concat(devicePacket(packetCmd, 1, le32s(0x08, 16)), devicePacket(packetData, 2, make([]byte, 16)), status)
	return [][]byte{devicePacket(packetAck, 0, nil), reply, nil}
}

// download is the device's side of one image download: the empty CMD 0x08
// after the command, the ACK of the DATA, then the STATUS.
func download(seq uint32) [][]byte {
	return [][]byte{
		devicePacket(packetAck, seq, nil), devicePacket(packetCmd, 9, le32s(0x08, 0)), nil,
		devicePacket(packetAck, seq+1, nil), devicePacket(packetCmd, 9, le32s(cmdStatus, 0)), nil,
	}
}

func TestSpikeBootsIntoTheFlashingServerAndQueriesIt(t *testing.T) {
	img := make([]byte, 0x400)
	files := spikeFiles{rcm0: make([]byte, 1032), rcm1: make([]byte, 1032), bct: img, ebt: img, rp1: img}
	bootDev := &fakeDevice{in: slices.Concat(versionReply(), [][]byte{devicePacket(packetAck, 1, nil)})}
	queryDev := &fakeDevice{in: versionReply()}
	devs := []*fakeDevice{
		{in: [][]byte{make([]byte, 16), le32s(rcmVersion), le32s(0)}},
		{in: slices.Concat(versionReply(), download(1))},
		{in: slices.Concat(versionReply(), download(1), download(3))},
		bootDev,
		queryDev,
	}
	open := func() (conn, error) {
		d := devs[0]
		devs = devs[1:]
		return d, nil
	}

	if err := spike(open, files, io.Discard); err != nil {
		t.Fatal(err)
	}

	wantBoot := bytes.Join(frame(packetCmd, 1, le32s(cmdBoot, bootFlashingServer)), nil)
	if !bytes.Contains(bytes.Join(bootDev.out, nil), wantBoot) {
		t.Errorf("the boot session sent % x, want the boot command", bootDev.out)
	}
	if want := frame(packetCmd, 0, le32s(cmdVersion)); !slices.EqualFunc(queryDev.out[:len(want)], want, bytes.Equal) {
		t.Errorf("the last session sent % x, want it to open with the version query", queryDev.out)
	}
}
