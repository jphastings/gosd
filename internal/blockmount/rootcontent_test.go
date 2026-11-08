package blockmount

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jphastings/gosd/internal/diskfmt"
)

func TestRootHasOtherContent(t *testing.T) {
	for name, tc := range map[string]struct {
		files []string // relative paths to create; a trailing / makes a directory
		want  bool
	}{
		"a fresh format":               {files: []string{"lost+found/", EstablishedMarker}, want: false},
		"an app's file":                {files: []string{"lost+found/", "state.json"}, want: true},
		"files fsck moved aside":       {files: []string{"lost+found/", "lost+found/#12"}, want: true},
		"an empty root, no lost+found": {want: false},
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			for _, f := range tc.files {
				path := filepath.Join(root, f)
				var err error
				if f[len(f)-1] == '/' {
					err = os.Mkdir(path, 0o755)
				} else {
					err = os.WriteFile(path, nil, 0o644)
				}
				if err != nil {
					t.Fatal(err)
				}
			}

			got, err := RootHasOtherContent(root, diskfmt.EXT4)
			if err != nil || got != tc.want {
				t.Errorf("RootHasOtherContent() = %v, %v; want %v", got, err, tc.want)
			}
		})
	}
}
