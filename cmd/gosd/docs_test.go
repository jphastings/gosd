package main

import (
	"bytes"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/pflag"

	"github.com/jphastings/gosd/internal/docview"
)

func TestDocsListsNames(t *testing.T) {
	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"docs"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "build-config\n") {
		t.Errorf("docs output = %q, want build-config listed", out.String())
	}
}

func TestFlagHelpOnlyPointsAtRealDocs(t *testing.T) {
	ref := regexp.MustCompile(`gosd docs (\S+)`)
	names := docview.Names()
	for _, c := range newRootCmd().Commands() {
		c.Flags().VisitAll(func(f *pflag.Flag) {
			for _, m := range ref.FindAllStringSubmatch(f.Usage, -1) {
				if !slices.Contains(names, m[1]) {
					t.Errorf("%s --%s points at missing doc %q", c.Name(), f.Name, m[1])
				}
			}
		})
	}
}
