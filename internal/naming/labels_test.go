package naming

import "testing"

func TestLabelPrefix(t *testing.T) {
	cases := map[string]string{
		"hello":      "hello",
		"sattrack":   "sattra",
		"usbwebsite": "usbweb",
		// A hyphen exposed at the truncation point is trimmed, exactly as
		// Sanitize trims one exposed at its own cap.
		"abcde-fgh":   "abcde",
		"a":           "a",
		"My App v2!!": "my-app",
		"":            "app",
		"---":         "app",
	}

	for in, want := range cases {
		if got := LabelPrefix(in); got != want {
			t.Errorf("LabelPrefix(%q) = %q, want %q", in, got, want)
		}
	}
}

// A prefix is only ever derived once, but nothing stops a caller deriving it
// from an already-sanitized name (gosd build sanitizes the app name before
// this ever sees it), so both must land on the same answer.
func TestLabelPrefixIsIdempotent(t *testing.T) {
	for _, in := range []string{"hello", "sattrack", "My App v2!!", "abcde-fgh", "", "---"} {
		once := LabelPrefix(in)
		if got := LabelPrefix(once); got != once {
			t.Errorf("LabelPrefix(LabelPrefix(%q)) = %q, want %q", in, got, once)
		}
		if got := LabelPrefix(Sanitize(in)); got != once {
			t.Errorf("LabelPrefix(Sanitize(%q)) = %q, want %q", in, got, once)
		}
	}
}

func TestLabelsForFitFATsLabelLimit(t *testing.T) {
	const fatMaxLabelLen = 11

	labels := LabelsFor(LabelPrefix("usbwebsite"))
	if labels.Boot != "usbweb-boot" || labels.Data != "usbweb-data" {
		t.Errorf("LabelsFor(LabelPrefix(\"usbwebsite\")) = %+v, want {usbweb-boot usbweb-data}", labels)
	}
	for _, label := range []string{labels.Boot, labels.Data} {
		if len(label) > fatMaxLabelLen {
			t.Errorf("label %q is %d bytes, more than FAT's %d-byte limit", label, len(label), fatMaxLabelLen)
		}
	}
}
