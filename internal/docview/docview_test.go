package docview

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	"github.com/jphastings/gosd/docs"
)

func TestNamesIncludesBuildConfig(t *testing.T) {
	if !slices.Contains(Names(), "build-config") {
		t.Fatalf("Names() = %v, want build-config", Names())
	}
}

func TestShowWritesRawMarkdownToNonTerminal(t *testing.T) {
	want, err := docs.FS.ReadFile("build-config.md")
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := Show("build-config", &buf); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(buf.Bytes(), want) {
		t.Error("Show did not write the doc's raw markdown")
	}
}

func TestShowUnknownNameIsActionable(t *testing.T) {
	err := Show("nope", &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "gosd docs") {
		t.Fatalf("err = %v, want one pointing at gosd docs", err)
	}
}
