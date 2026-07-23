package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jphastings/gosd/internal/boards"
)

func TestResolveBoardsDefaultsToAll(t *testing.T) {
	got, err := resolveBoards(nil)
	if err != nil {
		t.Fatalf("resolveBoards(nil): %v", err)
	}
	if !reflect.DeepEqual(got, boards.All()) {
		t.Errorf("resolveBoards(nil) = %v, want all registered boards %v", got, boards.All())
	}
}

func TestResolveBoardsFiltersAndDeduplicates(t *testing.T) {
	got, err := resolveBoards([]string{"pi-zero-2w", "pi-zero-2w"})
	if err != nil {
		t.Fatalf("resolveBoards: %v", err)
	}
	if len(got) != 1 || got[0].Name() != "pi-zero-2w" {
		t.Errorf("resolveBoards([pi-zero-2w, pi-zero-2w]) = %v, want a single pi-zero-2w entry", got)
	}
}

func TestResolveBoardsRejectsUnknownBoard(t *testing.T) {
	if _, err := resolveBoards([]string{"not-a-board"}); err == nil {
		t.Fatal("resolveBoards([not-a-board]) succeeded, want an error")
	}
}

func TestDeriveAppNameFromDotUsesWorkingDirectoryName(t *testing.T) {
	appDir := filepath.Join(t.TempDir(), "widget-3")
	if err := os.Mkdir(appDir, 0o755); err != nil {
		t.Fatalf("creating fixture app directory: %v", err)
	}
	t.Chdir(appDir)

	got, err := deriveAppName(".")
	if err != nil {
		t.Fatalf(`deriveAppName("."): %v`, err)
	}
	if got != "widget-3" {
		t.Errorf(`deriveAppName(".") = %q, want "widget-3"`, got)
	}
}

func TestDeriveAppNameFromRelativePath(t *testing.T) {
	got, err := deriveAppName("./examples/hello")
	if err != nil {
		t.Fatalf("deriveAppName: %v", err)
	}
	if got != "hello" {
		t.Errorf(`deriveAppName("./examples/hello") = %q, want "hello"`, got)
	}
}

func TestParseDataSize(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"0", 0},
		{"1GiB", 1024 * 1024 * 1024},
		{"1gib", 1024 * 1024 * 1024},
		{"512MiB", 512 * 1024 * 1024},
		{"2G", 2 * 1024 * 1024 * 1024},
		{"64K", 64 * 1024},
		{"4096", 4096},
		{" 8 MiB ", 8 * 1024 * 1024},
	}
	for _, c := range cases {
		got, err := parseDataSize(c.in)
		if err != nil {
			t.Errorf("parseDataSize(%q) error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("parseDataSize(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestParseDataSizeRejectsInvalidValues(t *testing.T) {
	for _, in := range []string{"", "-1", "-1GiB", "1GB", "lots", "1.5GiB"} {
		if _, err := parseDataSize(in); err == nil {
			t.Errorf("parseDataSize(%q) succeeded, want an error", in)
		}
	}
}

func mustFindBoard(t *testing.T, id string) boards.Board {
	t.Helper()
	b, ok := boards.Find(id)
	if !ok {
		t.Fatalf("boards.Find(%q) = not found; registered boards: %v", id, boards.IDs())
	}
	return b
}

func TestResolveOutputsSingleBoardUsesOutputAsFile(t *testing.T) {
	selected := []boards.Board{mustFindBoard(t, "pi-zero-2w")}

	got, err := resolveOutputs(selected, "myapp", "/tmp/x.img")
	if err != nil {
		t.Fatalf("resolveOutputs: %v", err)
	}
	if got["pi-zero-2w"] != "/tmp/x.img" {
		t.Errorf("outputs[pi-zero-2w] = %q, want /tmp/x.img", got["pi-zero-2w"])
	}
}

func TestResolveOutputsSingleBoardDefaultsToAppNameBoard(t *testing.T) {
	selected := []boards.Board{mustFindBoard(t, "pi-zero-2w")}

	got, err := resolveOutputs(selected, "myapp", "")
	if err != nil {
		t.Fatalf("resolveOutputs: %v", err)
	}
	if got["pi-zero-2w"] != "myapp-pi-zero-2w.img" {
		t.Errorf("outputs[pi-zero-2w] = %q, want myapp-pi-zero-2w.img", got["pi-zero-2w"])
	}
}

func TestResolveOutputsMultiBoardTreatsOutputAsDirectory(t *testing.T) {
	selected := boards.All()

	got, err := resolveOutputs(selected, "myapp", "/tmp/out")
	if err != nil {
		t.Fatalf("resolveOutputs: %v", err)
	}
	for _, b := range selected {
		want := "/tmp/out/myapp-" + b.Name() + ".img"
		if got[b.Name()] != want {
			t.Errorf("outputs[%s] = %q, want %q", b.Name(), got[b.Name()], want)
		}
	}
}

func TestEnsureOutputDirCreatesMissingMultiBoardDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sub", "dir")

	if err := ensureOutputDir(dir, true); err != nil {
		t.Fatalf("ensureOutputDir(%q, true): %v", dir, err)
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Errorf("ensureOutputDir(%q, true) did not create a directory there", dir)
	}
}

func TestEnsureOutputDirCreatesMissingSingleBoardParent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "dir", "app-board.img")

	if err := ensureOutputDir(path, false); err != nil {
		t.Fatalf("ensureOutputDir(%q, false): %v", path, err)
	}
	if info, err := os.Stat(filepath.Dir(path)); err != nil || !info.IsDir() {
		t.Errorf("ensureOutputDir(%q, false) did not create the parent directory", path)
	}
}

func TestEnsureOutputDirMultiBoardRejectsExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatalf("writing fixture file: %v", err)
	}

	err := ensureOutputDir(path, true)
	if err == nil {
		t.Fatalf("ensureOutputDir(%q, true) succeeded, want an error", path)
	}
	if got, want := err.Error(), "-o must be a directory when building multiple boards"; !strings.Contains(got, want) {
		t.Errorf("ensureOutputDir error = %q, want it to contain %q", got, want)
	}
}

func TestEnsureOutputDirEmptySingleBoardOutputIsNoop(t *testing.T) {
	if err := ensureOutputDir("", false); err != nil {
		t.Errorf("ensureOutputDir(\"\", false) = %v, want nil", err)
	}
}

func TestParseEnvFlagsNil(t *testing.T) {
	got, err := parseEnvFlags(nil)
	if err != nil {
		t.Fatalf("parseEnvFlags(nil): %v", err)
	}
	if got != nil {
		t.Errorf("parseEnvFlags(nil) = %v, want nil", got)
	}
}

func TestParseEnvFlagsValid(t *testing.T) {
	got, err := parseEnvFlags([]string{"API_URL=https://example.com", "LOG_LEVEL=debug"})
	if err != nil {
		t.Fatalf("parseEnvFlags: %v", err)
	}
	want := map[string]string{"API_URL": "https://example.com", "LOG_LEVEL": "debug"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseEnvFlags = %v, want %v", got, want)
	}
}

func TestParseEnvFlagsEmptyValueIsOK(t *testing.T) {
	got, err := parseEnvFlags([]string{"FOO="})
	if err != nil {
		t.Fatalf("parseEnvFlags: %v", err)
	}
	if v, ok := got["FOO"]; !ok || v != "" {
		t.Errorf(`parseEnvFlags(["FOO="]) = %v, want {"FOO": ""}`, got)
	}
}

func TestParseEnvFlagsValueContainingEqualsSplitsOnFirst(t *testing.T) {
	got, err := parseEnvFlags([]string{"CONN=user=admin;pass=secret"})
	if err != nil {
		t.Fatalf("parseEnvFlags: %v", err)
	}
	if want := "user=admin;pass=secret"; got["CONN"] != want {
		t.Errorf("parseEnvFlags CONN = %q, want %q", got["CONN"], want)
	}
}

func TestParseEnvFlagsRejectsMissingEquals(t *testing.T) {
	_, err := parseEnvFlags([]string{"NOEQUALS"})
	if err == nil {
		t.Fatal("parseEnvFlags([NOEQUALS]) succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "--env needs KEY=VALUE") {
		t.Errorf("error = %q, want it to mention KEY=VALUE", err.Error())
	}
}

func TestParseEnvFlagsRejectsInvalidKeyChars(t *testing.T) {
	for _, in := range []string{"1STARTSWITHDIGIT=x", "HAS-HYPHEN=x", "HAS SPACE=x", "=emptykey"} {
		if _, err := parseEnvFlags([]string{in}); err == nil {
			t.Errorf("parseEnvFlags([%q]) succeeded, want an error", in)
		}
	}
}

func TestParseEnvFlagsRejectsReservedGosdNamespace(t *testing.T) {
	_, err := parseEnvFlags([]string{"GOSD_FOO=bar"})
	if err == nil {
		t.Fatal("parseEnvFlags([GOSD_FOO=bar]) succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "GOSD_") {
		t.Errorf("error = %q, want it to mention the GOSD_ namespace", err.Error())
	}
}

func TestParseEnvFlagsRejectsDuplicateKey(t *testing.T) {
	_, err := parseEnvFlags([]string{"FOO=one", "FOO=two"})
	if err == nil {
		t.Fatal("parseEnvFlags with a duplicate key succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "FOO") {
		t.Errorf("error = %q, want it to mention the duplicate key FOO", err.Error())
	}
}

func TestResolveOutputsMultiBoardDefaultsToCurrentDirectory(t *testing.T) {
	selected := boards.All()

	got, err := resolveOutputs(selected, "myapp", "")
	if err != nil {
		t.Fatalf("resolveOutputs: %v", err)
	}
	for _, b := range selected {
		want := "myapp-" + b.Name() + ".img"
		if got[b.Name()] != want {
			t.Errorf("outputs[%s] = %q, want %q", b.Name(), got[b.Name()], want)
		}
	}
}

func TestGosdInitSrcFlagDefaultsToEnv(t *testing.T) {
	t.Setenv("GOSD_INIT_SRC", "/nix/store/example-gosd-src")

	flag := newBuildCmd().Flags().Lookup("gosd-init-src")
	if flag == nil {
		t.Fatal("build command has no --gosd-init-src flag")
	}
	if flag.DefValue != "/nix/store/example-gosd-src" {
		t.Errorf("--gosd-init-src default = %q, want the GOSD_INIT_SRC env value (the package-manager hook)", flag.DefValue)
	}
}

func TestGosdInitSrcFlagDefaultsEmptyWithoutEnv(t *testing.T) {
	t.Setenv("GOSD_INIT_SRC", "")

	flag := newBuildCmd().Flags().Lookup("gosd-init-src")
	if flag == nil {
		t.Fatal("build command has no --gosd-init-src flag")
	}
	if flag.DefValue != "" {
		t.Errorf("--gosd-init-src default = %q, want empty when GOSD_INIT_SRC is unset", flag.DefValue)
	}
}
