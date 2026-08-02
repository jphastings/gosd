package blockmount

import "testing"

func TestVfatMountOption(t *testing.T) {
	tests := []struct {
		name  string
		value string
		set   bool
		want  string
	}{
		{name: "GOSD_DATA_FLUSH=1 enables flush", value: "1", set: true, want: "flush"},
		{name: "GOSD_DATA_FLUSH=0 leaves it unset", value: "0", set: true, want: ""},
		{name: "unset defaults to unset", set: false, want: ""},
		{name: "a garbled value defaults to unset", value: "yes", set: true, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getenv := func(key string) string {
				if key != dataFlushEnvVar {
					t.Errorf("vfatMountOption read env var %q, want %q", key, dataFlushEnvVar)
				}
				if !tt.set {
					return ""
				}
				return tt.value
			}
			if got := vfatMountOption(getenv); got != tt.want {
				t.Errorf("vfatMountOption() = %q, want %q", got, tt.want)
			}
		})
	}
}
