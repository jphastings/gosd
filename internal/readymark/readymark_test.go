package readymark

import (
	"path/filepath"
	"testing"
)

func TestMarkedIsFalseBeforeMark(t *testing.T) {
	dir := t.TempDir()
	if Marked(dir) {
		t.Error("Marked() = true before Mark was ever called, want false")
	}
}

func TestMarkThenMarked(t *testing.T) {
	dir := t.TempDir()
	if err := Mark(dir); err != nil {
		t.Fatalf("Mark: %v", err)
	}
	if !Marked(dir) {
		t.Error("Marked() = false after Mark, want true")
	}
}

func TestMarkIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	if err := Mark(dir); err != nil {
		t.Fatalf("Mark (1st): %v", err)
	}
	if err := Mark(dir); err != nil {
		t.Fatalf("Mark (2nd): %v", err)
	}
	if !Marked(dir) {
		t.Error("Marked() = false after two Mark calls, want true")
	}
}

func TestMarkCreatesItsDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "gosd")
	if err := Mark(dir); err != nil {
		t.Fatalf("Mark: %v", err)
	}
	if !Marked(dir) {
		t.Error("Marked() = false after Mark created a fresh directory, want true")
	}
}
