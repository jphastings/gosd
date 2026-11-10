// Package docview shows the documentation compiled into gosd.
package docview

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/glamour/styles"
	"github.com/charmbracelet/glow/v2/ui"
	"github.com/charmbracelet/log"
	"golang.org/x/term"

	"github.com/jphastings/gosd/docs"
)

// Names returns the sorted doc names, without their .md suffix.
func Names() []string {
	files, _ := fs.Glob(docs.FS, "*.md")
	for i, f := range files {
		files[i] = strings.TrimSuffix(f, ".md")
	}
	return files
}

// Show opens the named doc in a terminal reader when out is a terminal, and
// otherwise writes its raw Markdown to out.
func Show(name string, out io.Writer) error {
	body, err := docs.FS.ReadFile(name + ".md")
	if err != nil {
		return fmt.Errorf("no doc named %q; run \"gosd docs\" to list them", name)
	}
	if f, ok := out.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		return readInTUI(name, body)
	}
	_, err = out.Write(body)
	return err
}

func readInTUI(name string, body []byte) error {
	// glow re-reads Config.Path from disk even when given content, so the doc
	// must exist as a real file; a relative path keeps its status bar tidy.
	dir, err := os.MkdirTemp("", "gosd-docs-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	file := name + ".md"
	if err := os.WriteFile(filepath.Join(dir, file), body, 0o600); err != nil {
		return err
	}
	if err := os.Chdir(dir); err != nil {
		return err
	}
	// glow logs to stderr, which would draw over its own screen.
	log.SetOutput(io.Discard)
	_, err = ui.NewProgram(ui.Config{
		Path:                 file,
		GlamourEnabled:       true,
		GlamourStyle:         styles.AutoStyle,
		GlamourMaxWidth:      120,
		HighPerformancePager: true,
	}, "").Run()
	return err
}
