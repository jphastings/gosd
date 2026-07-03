package initcfg

import "testing"

func TestParseCmdline(t *testing.T) {
	tests := []struct {
		name    string
		cmdline string
		want    CmdlineArgs
	}{
		{
			name:    "no gosd params",
			cmdline: "console=ttyAMA0 root=/dev/mmcblk0p2 rw",
			want:    CmdlineArgs{},
		},
		{
			name:    "board and bare debug flag",
			cmdline: "console=ttyAMA0 gosd.board=pi-zero-2w gosd.debug root=/dev/mmcblk0p2",
			want:    CmdlineArgs{Board: "pi-zero-2w", Debug: true},
		},
		{
			name:    "debug explicitly enabled",
			cmdline: "gosd.debug=1",
			want:    CmdlineArgs{Debug: true},
		},
		{
			name:    "debug explicitly disabled",
			cmdline: "gosd.debug=0",
			want:    CmdlineArgs{Debug: false},
		},
		{
			name:    "debug=false is falsy",
			cmdline: "gosd.debug=false",
			want:    CmdlineArgs{Debug: false},
		},
		{
			name:    "board without value is ignored",
			cmdline: "gosd.board",
			want:    CmdlineArgs{},
		},
		{
			name:    "empty cmdline",
			cmdline: "",
			want:    CmdlineArgs{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseCmdline(tt.cmdline)
			if got != tt.want {
				t.Fatalf("ParseCmdline(%q) = %+v, want %+v", tt.cmdline, got, tt.want)
			}
		})
	}
}
