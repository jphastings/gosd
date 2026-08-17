package repocheck

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// examplesCIWorkflowPath is relative to this package directory.
const examplesCIWorkflowPath = "../../.github/workflows/ci.yml"

// examplesCIWorkflow is the minimal slice of ci.yml this test needs: the
// smoke-build job's steps and their shell commands. Unknown keys are
// ignored, so unrelated workflow growth costs nothing here.
type examplesCIWorkflow struct {
	Jobs map[string]struct {
		Steps []struct {
			Run string `yaml:"run"`
		} `yaml:"steps"`
	} `yaml:"jobs"`
}

// crossCompileArches identifies each leg of the smoke build by the env
// assignment that pins its architecture. These are the two board
// architectures CLAUDE.md requires every example to build for.
var crossCompileArches = map[string]string{
	"arm64": "GOARCH=arm64",
	"armv6": "GOARCH=arm GOARM=6",
}

// CLAUDE.md states that examples "must cross-compile for every board arch
// (arm64 AND GOARCH=arm GOARM=6)". ci.yml's smoke-build job is the only
// thing enforcing that, and while it hand-maintained a package list per
// leg it silently drifted away from examples/ (bean gosd-asdg):
// examples/sattrack appeared in neither leg and examples/usbserial was
// missing from armv6, so three of twenty example/arch pairs were never
// compiled by CI at all. A ./examples/... wildcard cannot drift; naming
// examples individually again reopens the hole, which is what this test
// exists to catch.
func TestSmokeBuildCrossCompilesEveryExample(t *testing.T) {
	steps := smokeBuildRunCommands(t)

	for arch, marker := range crossCompileArches {
		run, ok := smokeBuildRunContaining(steps, marker)
		if !ok {
			t.Errorf("ci.yml's smoke-build job has no step cross-compiling for %s (looked for %q)", arch, marker)
			continue
		}
		if !strings.Contains(run, "./examples/...") {
			t.Errorf("the %s cross-compile step does not build ./examples/..., so a new example would go uncompiled for that arch:\n\t%s", arch, run)
		}
		for _, token := range strings.Fields(run) {
			if strings.HasPrefix(token, "./examples/") && token != "./examples/..." {
				t.Errorf("the %s cross-compile step names %s individually; use the ./examples/... wildcard so the list cannot drift from the examples/ directory", arch, token)
			}
		}
	}
}

// smokeBuildRunCommands returns every `run:` command in ci.yml's
// smoke-build job. Parsing the YAML (rather than grepping the file) keeps
// the job's explanatory comments — which mention example paths — out of
// what is matched.
func smokeBuildRunCommands(t *testing.T) []string {
	t.Helper()

	b, err := os.ReadFile(examplesCIWorkflowPath)
	if err != nil {
		t.Fatalf("reading %s: %v", examplesCIWorkflowPath, err)
	}
	var wf examplesCIWorkflow
	if err := yaml.Unmarshal(b, &wf); err != nil {
		t.Fatalf("parsing %s: %v", examplesCIWorkflowPath, err)
	}

	job, ok := wf.Jobs["smoke-build"]
	if !ok {
		t.Fatalf("%s has no smoke-build job; it is what enforces the examples cross-compile rule", examplesCIWorkflowPath)
	}

	runs := make([]string, 0, len(job.Steps))
	for _, step := range job.Steps {
		if step.Run != "" {
			runs = append(runs, step.Run)
		}
	}
	return runs
}

func smokeBuildRunContaining(runs []string, marker string) (string, bool) {
	for _, run := range runs {
		if strings.Contains(run, marker) {
			return run, true
		}
	}
	return "", false
}
