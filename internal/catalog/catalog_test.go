package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// --- URL joining edge cases ---

func TestJoinURL(t *testing.T) {
	cases := []struct {
		name     string
		baseURL  string
		filename string
		want     string
	}{
		{"no trailing slash", "https://example.com/downloads", "hello-pi-zero-2w.img", "https://example.com/downloads/hello-pi-zero-2w.img"},
		{"one trailing slash", "https://example.com/downloads/", "hello-pi-zero-2w.img", "https://example.com/downloads/hello-pi-zero-2w.img"},
		{"multiple trailing slashes", "https://example.com/downloads///", "hello-pi-zero-2w.img", "https://example.com/downloads/hello-pi-zero-2w.img"},
		{"bare host", "https://example.com", "hello.img", "https://example.com/hello.img"},
		{"bare host trailing slash", "https://example.com/", "hello.img", "https://example.com/hello.img"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := JoinURL(c.baseURL, c.filename); got != c.want {
				t.Errorf("JoinURL(%q, %q) = %q, want %q", c.baseURL, c.filename, got, c.want)
			}
		})
	}
}

// --- hash/size correctness against real fixture bytes ---

func TestBuildEntryComputesRealHashAndSize(t *testing.T) {
	dir := t.TempDir()
	content := []byte("arbitrary fixture image bytes, not a real .img\n")
	imgPath := filepath.Join(dir, "hello-pi-zero-2w.img")
	if err := os.WriteFile(imgPath, content, 0o644); err != nil {
		t.Fatalf("writing fixture image: %v", err)
	}

	wantSum := sha256.Sum256(content)
	wantHex := hex.EncodeToString(wantSum[:])

	entry, err := BuildEntry(Image{AppName: "hello", BoardID: "pi-zero-2w", Path: imgPath}, Options{
		BaseURL:     "https://example.com/downloads",
		ReleaseDate: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("BuildEntry: %v", err)
	}

	if got, want := entry.ExtractSize, int64(len(content)); got != want {
		t.Errorf("ExtractSize = %d, want %d", got, want)
	}
	if got, want := entry.ImageDownloadSize, int64(len(content)); got != want {
		t.Errorf("ImageDownloadSize = %d, want %d", got, want)
	}
	if entry.ExtractSHA256 != wantHex {
		t.Errorf("ExtractSHA256 = %q, want %q", entry.ExtractSHA256, wantHex)
	}
}

func TestBuildEntryRequiresBaseURL(t *testing.T) {
	dir := t.TempDir()
	imgPath := filepath.Join(dir, "hello-pi-zero-2w.img")
	if err := os.WriteFile(imgPath, []byte("x"), 0o644); err != nil {
		t.Fatalf("writing fixture image: %v", err)
	}

	if _, err := BuildEntry(Image{AppName: "hello", BoardID: "pi-zero-2w", Path: imgPath}, Options{}); err == nil {
		t.Fatal("BuildEntry with no BaseURL succeeded, want an error")
	}
}

func TestBuildEntryUsesHumanFriendlyNameAndKnownDisplayNames(t *testing.T) {
	dir := t.TempDir()
	imgPath := filepath.Join(dir, "hello-radxa-zero-3e.img")
	if err := os.WriteFile(imgPath, []byte("x"), 0o644); err != nil {
		t.Fatalf("writing fixture image: %v", err)
	}

	entry, err := BuildEntry(Image{AppName: "hello", BoardID: "radxa-zero-3e", Path: imgPath}, Options{BaseURL: "https://example.com"})
	if err != nil {
		t.Fatalf("BuildEntry: %v", err)
	}
	if want := "hello (Radxa Zero 3E)"; entry.Name != want {
		t.Errorf("Name = %q, want %q", entry.Name, want)
	}
}

func TestBuildEntryFallsBackToBoardIDForUnknownBoards(t *testing.T) {
	dir := t.TempDir()
	imgPath := filepath.Join(dir, "hello-some-future-board.img")
	if err := os.WriteFile(imgPath, []byte("x"), 0o644); err != nil {
		t.Fatalf("writing fixture image: %v", err)
	}

	entry, err := BuildEntry(Image{AppName: "hello", BoardID: "some-future-board", Path: imgPath}, Options{BaseURL: "https://example.com"})
	if err != nil {
		t.Fatalf("BuildEntry: %v", err)
	}
	if want := "hello (some-future-board)"; entry.Name != want {
		t.Errorf("Name = %q, want %q (fallback to raw board ID)", entry.Name, want)
	}
}

// --- golden JSON for a fake build ---

// fakeBuild writes two fake .img files (standing in for a real gosd build's
// output) into dir and returns the Image values WriteFiles expects.
func fakeBuild(t *testing.T, dir string) []Image {
	t.Helper()

	files := map[string]string{
		"hello-pi-zero-2w.img":    "fake pi-zero-2w image content\n",
		"hello-radxa-zero-3e.img": "fake radxa-zero-3e image content\n",
	}
	images := make([]Image, 0, len(files))
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("writing fake image %s: %v", name, err)
		}
	}
	// Built separately from the map above so the resulting order (and
	// hence WriteFiles' sort-by-BoardID) is deterministic regardless of
	// map iteration order.
	images = append(images,
		Image{AppName: "hello", BoardID: "radxa-zero-3e", Path: filepath.Join(dir, "hello-radxa-zero-3e.img")},
		Image{AppName: "hello", BoardID: "pi-zero-2w", Path: filepath.Join(dir, "hello-pi-zero-2w.img")},
	)
	return images
}

func TestWriteFilesMatchesGoldenOutput(t *testing.T) {
	dir := t.TempDir()
	images := fakeBuild(t, dir)

	opts := Options{
		BaseURL:     "https://example.com/downloads/",
		ReleaseDate: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
	}

	entries, err := WriteFiles(dir, images, opts)
	if err != nil {
		t.Fatalf("WriteFiles: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("WriteFiles returned %d entries, want 2", len(entries))
	}
	// WriteFiles sorts by BoardID: "pi-zero-2w" < "radxa-zero-3e".
	if entries[0].Devices[0] != "pi-zero-2w" || entries[1].Devices[0] != "radxa-zero-3e" {
		t.Errorf("WriteFiles entries not sorted by board ID: %+v", entries)
	}

	golden, err := os.ReadFile(filepath.Join("testdata", "golden_os_list.json"))
	if err != nil {
		t.Fatalf("reading golden fixture: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "os_list.json"))
	if err != nil {
		t.Fatalf("reading generated os_list.json: %v", err)
	}
	if string(got) != string(golden) {
		t.Errorf("combined os_list.json = %s, want %s", got, golden)
	}

	// The per-image fragment for pi-zero-2w must contain exactly its own
	// entry, none of the other board's.
	fragment, err := os.ReadFile(filepath.Join(dir, "hello-pi-zero-2w.os_list.json"))
	if err != nil {
		t.Fatalf("reading pi-zero-2w fragment: %v", err)
	}
	var fragList List
	if err := json.Unmarshal(fragment, &fragList); err != nil {
		t.Fatalf("unmarshaling fragment: %v", err)
	}
	if len(fragList.OSList) != 1 || fragList.OSList[0].Devices[0] != "pi-zero-2w" {
		t.Errorf("hello-pi-zero-2w.os_list.json = %+v, want a single pi-zero-2w entry", fragList.OSList)
	}
}

// --- validate the generated shape against the vendored rpi-imager schema ---
//
// A full JSON-Schema (draft-07) validator is significant machinery (ref
// resolution, format keywords, anyOf/allOf combinators) for what's really
// one flat, well-known object shape. Rather than add a new dependency just
// for this, schemaRequirements below parses only the two things that
// actually gate whether Imager accepts an entry: which keys the "Operating
// system entry" variant requires, and init_format's enum. Reading these out
// of the vendored schema file (instead of hardcoding a duplicate list)
// means the test still catches drift if the vendored schema is ever
// re-pinned to a newer commit with different requirements.

type osListSchema struct {
	Properties struct {
		OSList struct {
			Items struct {
				AnyOf []struct {
					Required   []string `json:"required"`
					Properties map[string]struct {
						Type string   `json:"type"`
						Enum []string `json:"enum"`
					} `json:"properties"`
				} `json:"anyOf"`
			} `json:"items"`
		} `json:"os_list"`
	} `json:"properties"`
}

func loadSchema(t *testing.T) osListSchema {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "os-list-schema.json"))
	if err != nil {
		t.Fatalf("reading vendored schema: %v", err)
	}
	var schema osListSchema
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatalf("parsing vendored schema: %v", err)
	}
	return schema
}

// osEntryRequirements returns the "Operating system entry" anyOf variant
// (identified, as os-list-schema.json's own titles do, by requiring "url" -
// the other two os_list item variants are subitems_url/subitems
// containers, which don't) and its init_format enum.
func osEntryRequirements(t *testing.T, schema osListSchema) (required []string, initFormatEnum []string) {
	t.Helper()
	for _, variant := range schema.Properties.OSList.Items.AnyOf {
		hasURL := false
		for _, r := range variant.Required {
			if r == "url" {
				hasURL = true
			}
		}
		if !hasURL {
			continue
		}
		return variant.Required, variant.Properties["init_format"].Enum
	}
	t.Fatal("vendored schema has no os_list item variant requiring \"url\"; can't find the OS entry shape")
	return nil, nil
}

// TestGeneratedEntriesSatisfySchema checks every field the vendored
// schema's OS-entry variant requires is present (and, for init_format, one
// of its enumerated values) on every entry gosd generates - see the package
// doc comment above for why this is a targeted structural check rather
// than a full JSON-Schema validation.
func TestGeneratedEntriesSatisfySchema(t *testing.T) {
	schema := loadSchema(t)
	required, initFormatEnum := osEntryRequirements(t, schema)

	dir := t.TempDir()
	images := fakeBuild(t, dir)
	entries, err := WriteFiles(dir, images, Options{BaseURL: "https://example.com/downloads"})
	if err != nil {
		t.Fatalf("WriteFiles: %v", err)
	}

	for _, entry := range entries {
		raw, err := json.Marshal(entry)
		if err != nil {
			t.Fatalf("marshaling entry: %v", err)
		}
		var asMap map[string]any
		if err := json.Unmarshal(raw, &asMap); err != nil {
			t.Fatalf("unmarshaling entry: %v", err)
		}

		for _, field := range required {
			if _, ok := asMap[field]; !ok {
				t.Errorf("entry %q is missing schema-required field %q", entry.Name, field)
			}
		}

		if !contains(initFormatEnum, entry.InitFormat) {
			t.Errorf("entry %q has init_format %q, want one of %v", entry.Name, entry.InitFormat, initFormatEnum)
		}
	}

	// The document's own top-level required field.
	combined, err := os.ReadFile(filepath.Join(dir, "os_list.json"))
	if err != nil {
		t.Fatalf("reading os_list.json: %v", err)
	}
	var asMap map[string]any
	if err := json.Unmarshal(combined, &asMap); err != nil {
		t.Fatalf("unmarshaling os_list.json: %v", err)
	}
	if _, ok := asMap["os_list"]; !ok {
		t.Error(`os_list.json is missing the top-level required "os_list" key`)
	}
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
