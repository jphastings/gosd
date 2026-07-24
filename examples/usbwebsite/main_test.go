package main

import "testing"

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
