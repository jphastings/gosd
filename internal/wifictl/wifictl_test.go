package wifictl

import (
	"os"
	"path/filepath"
	"testing"
)

func TestARequestSurvivesTheRoundTrip(t *testing.T) {
	dir := t.TempDir()
	want := Request{ID: "abc123", SSID: "home-network", Passphrase: "correct-horse", Persist: true}

	if err := WriteRequest(dir, want); err != nil {
		t.Fatal(err)
	}
	got, ok, err := ReadRequest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("ReadRequest reported nothing written, though WriteRequest just wrote one")
	}
	if got != want {
		t.Errorf("ReadRequest() = %+v, want %+v", got, want)
	}
}

func TestAStatusSurvivesTheRoundTrip(t *testing.T) {
	dir := t.TempDir()
	want := Status{ID: "abc123", State: Failed, Error: "4-way handshake timed out"}

	if err := WriteStatus(dir, want); err != nil {
		t.Fatal(err)
	}
	got, ok, err := ReadStatus(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || got != want {
		t.Errorf("ReadStatus() = %+v, %v, want %+v, true", got, ok, want)
	}
}

func TestANeverWrittenFileIsAbsentNotAnError(t *testing.T) {
	dir := t.TempDir()

	if _, ok, err := ReadRequest(dir); ok || err != nil {
		t.Errorf("ReadRequest() on an empty dir = ok=%v err=%v, want ok=false err=nil", ok, err)
	}
	if _, ok, err := ReadStatus(dir); ok || err != nil {
		t.Errorf("ReadStatus() on an empty dir = ok=%v err=%v, want ok=false err=nil", ok, err)
	}
}

func TestAnUnparseableFileIsAnErrorNotSilentlyDropped(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, RequestFile), []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, ok, err := ReadRequest(dir); err == nil {
		t.Errorf("ReadRequest() on garbage = ok=%v err=%v, want a non-nil error so the caller can decide policy", ok, err)
	}
}

func TestWritingReplacesWhatWasThereAtomically(t *testing.T) {
	dir := t.TempDir()
	if err := WriteRequest(dir, Request{ID: "first", SSID: "old-network"}); err != nil {
		t.Fatal(err)
	}
	if err := WriteRequest(dir, Request{ID: "second", SSID: "new-network"}); err != nil {
		t.Fatal(err)
	}

	got, ok, err := ReadRequest(dir)
	if err != nil || !ok {
		t.Fatalf("ReadRequest() = %+v, %v, %v", got, ok, err)
	}
	if got.ID != "second" {
		t.Errorf("ReadRequest() = %+v, want the most recently written request (last-write-wins)", got)
	}
	if _, err := os.Stat(filepath.Join(dir, RequestFile+".tmp")); !os.IsNotExist(err) {
		t.Error("a .tmp file was left behind; a reader must only ever see the finished file")
	}
}

func TestFilePermissionsProtectAPlaintextPassphrase(t *testing.T) {
	// A dir writeJSON must create itself, not one the test harness already
	// made — t.TempDir()'s own permissions aren't this package's to assert
	// on.
	dir := filepath.Join(t.TempDir(), "wifi")
	if err := WriteRequest(dir, Request{ID: "abc", SSID: "home", Passphrase: "hunter2"}); err != nil {
		t.Fatal(err)
	}

	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if perm := dirInfo.Mode().Perm(); perm != 0o700 {
		t.Errorf("%s is mode %v, want 0700", dir, perm)
	}

	fileInfo, err := os.Stat(filepath.Join(dir, RequestFile))
	if err != nil {
		t.Fatal(err)
	}
	if perm := fileInfo.Mode().Perm(); perm != 0o600 {
		t.Errorf("%s is mode %v, want 0600", RequestFile, perm)
	}
}

func TestTerminalStates(t *testing.T) {
	cases := map[State]bool{Joining: false, Joined: true, Failed: true}
	for state, want := range cases {
		if got := state.Terminal(); got != want {
			t.Errorf("%s.Terminal() = %v, want %v", state, got, want)
		}
	}
}
