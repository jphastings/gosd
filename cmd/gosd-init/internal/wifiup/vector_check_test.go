package wifiup

import (
	"encoding/hex"
	"testing"
)

func TestDerivePSKAgainstIEEEVector(t *testing.T) {
	got, err := DerivePSK("password", "IEEE")
	if err != nil {
		t.Fatal(err)
	}
	want := "f42c6fc52df0ebef9ebb4b90b38a5f902e83fe1b135a70e23aed762e9710a12e"
	if hex.EncodeToString(got[:]) != want {
		t.Errorf("DerivePSK IEEE vector mismatch:\n got %s\nwant %s", hex.EncodeToString(got[:]), want)
	}
}
