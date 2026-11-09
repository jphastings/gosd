package jetsonflash

import (
	"bytes"
	"slices"
	"strings"
	"testing"
)

func TestProbeSpecs(t *testing.T) {
	got, err := parseProbeSpecs([]string{"0x02-0x04", "11:41505000", "0x0b"})
	if err != nil {
		t.Fatal(err)
	}
	want := []probeCmd{{2, nil}, {3, nil}, {4, nil}, {11, []byte("APP\x00")}, {11, nil}}
	if !slices.EqualFunc(got, want, func(a, b probeCmd) bool { return a.cmd == b.cmd && bytes.Equal(a.args, b.args) }) {
		t.Errorf("got %v, want %v", got, want)
	}
	for _, bad := range []string{"x", "0x5-0x2", "0x1-0x3:00", "1:zz", "1:0", ""} {
		if _, err := parseProbeSpecs([]string{bad}); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
}

func TestProbeRefusesOEMCommandsButType1(t *testing.T) {
	for _, spec := range []string{"0x05:09000000", "0x05:0f000000", "0x05", "0x05:01", "0x04-0x06"} {
		if _, err := parseProbeSpecs([]string{spec}); err == nil || !strings.Contains(err.Error(), "RPMB") {
			t.Errorf("%q: got %v, want a refusal naming the RPMB risk", spec, err)
		}
	}
	if _, err := parseProbeSpecs([]string{"0x05:0100000001000000"}); err != nil {
		t.Errorf("the storage query was refused: %v", err)
	}
}

func TestProbeCarriesOnPastAFailingCommand(t *testing.T) {
	failed := devicePacket(packetCmd, 5, append(le32s(cmdStatus, 0x12), make([]byte, 32)...))
	second := &fakeDevice{in: slices.Concat(versionReply(), [][]byte{devicePacket(packetAck, 1, nil)})}
	first := &fakeDevice{in: slices.Concat(versionReply(), [][]byte{devicePacket(packetAck, 1, nil), failed})}
	devs := []*fakeDevice{first, second}
	open := func() (conn, error) {
		d := devs[0]
		devs = devs[1:]
		return d, nil
	}
	var log strings.Builder

	err := probe(open, []probeCmd{{0x20, nil}, {0x21, nil}}, nil, quietWait, nil, &log)

	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(bytes.Join(second.out, nil), bytes.Join(frame(packetCmd, 1, le32s(0x21)), nil)) {
		t.Error("the second command was never sent")
	}
	if !strings.Contains(log.String(), "command 0x20 failed") || !strings.Contains(log.String(), "0x12") {
		t.Errorf("the log doesn't name the first command's failure:\n%s", log.String())
	}
}

func TestProbeOutCapturesRepliesButNotTheVersionQuery(t *testing.T) {
	reply := devicePacket(packetData, 1, []byte("payload"))
	dev := &fakeDevice{in: slices.Concat(versionReply(), [][]byte{devicePacket(packetAck, 1, nil), reply})}
	var out bytes.Buffer

	err := probe(func() (conn, error) { return dev, nil }, []probeCmd{{0x20, nil}}, nil, quietWait, &out, &strings.Builder{})

	if err != nil {
		t.Fatal(err)
	}
	if out.String() != "payload" {
		t.Errorf("captured %q, want only the command's reply", out.String())
	}
}

func TestStreamSendsTheWholeFileInChunks(t *testing.T) {
	file := bytes.Repeat([]byte{7}, dataChunk+1)
	dev := &fakeDevice{in: [][]byte{devicePacket(packetAck, 0, nil), devicePacket(packetAck, 1, nil)}}
	s := &session{c: dev, log: &strings.Builder{}}

	if err := s.stream(bytes.NewReader(file)); err != nil {
		t.Fatal(err)
	}

	want := slices.Concat(frame(packetData, 0, file[:dataChunk]), frame(packetData, 1, file[dataChunk:]))
	if !slices.EqualFunc(dev.out, want, bytes.Equal) {
		t.Errorf("sent %d transfers, want the file as two DATA packets", len(dev.out))
	}
}
