package main

import (
	"os"
	"path/filepath"
	"testing"
)

// The Close-masks-error fix (only defer srv.Close() after a successful
// srv.Up) can't be unit-tested without a real tsnet.Server: tsnet has no
// interface seam to fake, and constructing one that fails Up predictably
// needs real network/tailnet state. It's covered by the code comment at the
// call site in main.go (bean gosd-6cf2) and by the on-bench verification
// recorded in the bean instead.

func TestPrepareStateDir_CreatesDirAndSetsLogsDir(t *testing.T) {
	t.Setenv("TS_LOGS_DIR", "")
	statedir := filepath.Join(t.TempDir(), "nested", "tailscale-state")

	if err := prepareStateDir(statedir); err != nil {
		t.Fatalf("prepareStateDir: %v", err)
	}

	info, err := os.Stat(statedir)
	if err != nil {
		t.Fatalf("statedir was not created: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("statedir is not a directory")
	}

	if got := os.Getenv("TS_LOGS_DIR"); got != statedir {
		t.Errorf("TS_LOGS_DIR = %q, want %q", got, statedir)
	}
}

func TestPrepareStateDir_StateFileHealing(t *testing.T) {
	tests := []struct {
		name    string
		content []byte // nil means the file does not exist at all
		kept    bool
	}{
		{"absent file stays absent", nil, false},
		{"empty file is removed", []byte(""), false},
		{"truncated non-JSON file is removed", []byte(`{"Priv Prefe`), false},
		{"garbage file is removed", []byte("not json at all"), false},
		{"valid JSON file is preserved untouched", []byte(`{"Priv":"key material"}`), true},
	}

	for _, filename := range tsnetStateFiles {
		for _, tt := range tests {
			t.Run(filename+"/"+tt.name, func(t *testing.T) {
				statedir := t.TempDir()
				path := filepath.Join(statedir, filename)
				if tt.content != nil {
					if err := os.WriteFile(path, tt.content, 0o600); err != nil {
						t.Fatalf("seeding %q: %v", path, err)
					}
				}

				if err := prepareStateDir(statedir); err != nil {
					t.Fatalf("prepareStateDir: %v", err)
				}

				got, err := os.ReadFile(path)
				exists := err == nil
				if exists != tt.kept {
					t.Fatalf("file exists = %v, want %v", exists, tt.kept)
				}
				if tt.kept && string(got) != string(tt.content) {
					// This is the identity-survival guarantee: a valid
					// tailscaled.state must never be rewritten or touched,
					// since it holds the node's private key and tailnet
					// membership (losing it means a new node identity and
					// a new public URL).
					t.Errorf("preserved file content changed: got %q, want %q", got, tt.content)
				}
			})
		}
	}
}

func TestPrepareStateDir_LeavesOtherFilesAlone(t *testing.T) {
	statedir := t.TempDir()
	other := filepath.Join(statedir, "not-a-tsnet-file.txt")
	if err := os.WriteFile(other, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := prepareStateDir(statedir); err != nil {
		t.Fatalf("prepareStateDir: %v", err)
	}

	if _, err := os.Stat(other); err != nil {
		t.Errorf("unrelated file was removed: %v", err)
	}
}
