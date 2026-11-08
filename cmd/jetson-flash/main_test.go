package main

import (
	"io"
	"strings"
	"testing"
)

func TestSpikeRefusesADirectoryWithoutNVIDIAsSignedFiles(t *testing.T) {
	root := newRootCmd()
	root.SetOut(io.Discard)
	root.SetArgs([]string{"spike", t.TempDir()})

	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "rcm_0_encrypt.rcm") {
		t.Fatalf("want an error naming the missing signed file, got %v", err)
	}
}
