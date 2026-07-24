package main

import (
	"errors"
	"testing"
)

func TestIsAffirmative(t *testing.T) {
	yes := []string{"1", "true", "TRUE", "yes", "Yes", "on", " on "}
	for _, v := range yes {
		if !isAffirmative(v) {
			t.Errorf("isAffirmative(%q) = false, want true", v)
		}
	}

	no := []string{"", "0", "false", "no", "off", "banana"}
	for _, v := range no {
		if isAffirmative(v) {
			t.Errorf("isAffirmative(%q) = true, want false", v)
		}
	}
}

func TestNeedsWipeConsent(t *testing.T) {
	consentErr := errors.New(`the eMMC at /dev/mmcblk0 already holds a FAT volume labelled "OTHER"; refusing to reformat it as "WEBSITE" without permission — pass destructive=true to wipe it`)
	if !needsWipeConsent(consentErr) {
		t.Errorf("needsWipeConsent(%v) = false, want true", consentErr)
	}

	otherErr := errors.New("mounting the eMMC at /dev/mmcblk0 onto /storage failed: permission denied")
	if needsWipeConsent(otherErr) {
		t.Errorf("needsWipeConsent(%v) = true, want false", otherErr)
	}
}
