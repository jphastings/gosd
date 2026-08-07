package kernelspec_test

import (
	"os"
	"strings"
	"testing"

	"github.com/jphastings/gosd/internal/kernelspec"
	"gopkg.in/yaml.v3"
)

// buildArtifactsWorkflowPath is relative to this package directory, mirroring
// how TestPiRequiredYIsDerivedFromFragment above locates repo-root files.
const buildArtifactsWorkflowPath = "../../.github/workflows/build-artifacts.yml"

// ghWorkflow is the minimal slice of a GitHub Actions workflow file this
// test needs: enough to find every job, its `needs` list, and its
// upload-artifact/download-artifact step names. Unknown YAML keys are
// ignored (no KnownFields), so this stays cheap to maintain as the workflow
// grows unrelated steps.
type ghWorkflow struct {
	Jobs map[string]ghJob `yaml:"jobs"`
}

type ghJob struct {
	Needs stringOrList `yaml:"needs"`
	Steps []ghStep     `yaml:"steps"`
}

type ghStep struct {
	Uses string `yaml:"uses"`
	With struct {
		Name string `yaml:"name"`
	} `yaml:"with"`
}

// stringOrList decodes a YAML `needs:` field, which GitHub Actions accepts
// as either a single job-name scalar or a sequence of them.
type stringOrList []string

func (s *stringOrList) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		var single string
		if err := node.Decode(&single); err != nil {
			return err
		}
		*s = []string{single}
	case yaml.SequenceNode:
		var list []string
		if err := node.Decode(&list); err != nil {
			return err
		}
		*s = list
	default:
		*s = nil
	}
	return nil
}

func (s stringOrList) contains(name string) bool {
	for _, n := range s {
		if n == name {
			return true
		}
	}
	return false
}

// artifactUploadName returns the `with.name` of this job's
// actions/upload-artifact step, or "" if it has none. Matching on the
// `actions/upload-artifact@` prefix (rather than an exact pinned SHA)
// deliberately survives routine version bumps of the action itself.
func (j ghJob) artifactUploadName() string {
	for _, step := range j.Steps {
		if strings.HasPrefix(step.Uses, "actions/upload-artifact@") {
			return step.With.Name
		}
	}
	return ""
}

// downloadsArtifact reports whether this job has an actions/download-artifact
// step for the given artifact name.
func (j ghJob) downloadsArtifact(name string) bool {
	for _, step := range j.Steps {
		if strings.HasPrefix(step.Uses, "actions/download-artifact@") && step.With.Name == name {
			return true
		}
	}
	return false
}

func loadBuildArtifactsWorkflow(t *testing.T) ghWorkflow {
	t.Helper()
	data, err := os.ReadFile(buildArtifactsWorkflowPath)
	if err != nil {
		t.Fatalf("reading %s: %v", buildArtifactsWorkflowPath, err)
	}
	var wf ghWorkflow
	if err := yaml.Unmarshal(data, &wf); err != nil {
		t.Fatalf("parsing %s: %v", buildArtifactsWorkflowPath, err)
	}
	return wf
}

// TestBuildArtifactsWorkflowCoversEveryKernelspecBoard is the drift guard
// bean gosd-mbdc calls for: nothing else ties build-artifacts.yml's
// per-board jobs to kernelspec.BoardIDs(), so a board could gain a
// KernelSpec (and pass every other Go test) without ever getting a CI job
// that actually builds and publishes its kernel - a silent download-404 far
// downstream of a green PR.
//
// It parses the committed workflow YAML directly rather than adding new
// workflow machinery, so it runs as a normal `go test ./...` case in CI (see
// CLAUDE.md's "Board work & artifact releases" section). Per board it
// checks three things, matching the bean's "job names / needs list /
// download steps" wording:
//  1. a "<board>-kernel" job exists,
//  2. package-and-release's `needs:` list includes that job, and
//  3. package-and-release downloads the artifact that job uploads.
//
// It also checks the reverse: every "<X>-kernel" job in the workflow names a
// board kernelspec actually knows about, catching a stale job left behind by
// a removed board.
func TestBuildArtifactsWorkflowCoversEveryKernelspecBoard(t *testing.T) {
	wf := loadBuildArtifactsWorkflow(t)

	const releaseJobName = "package-and-release"
	releaseJob, ok := wf.Jobs[releaseJobName]
	if !ok {
		t.Fatalf("%s has no %q job; this test's assumptions about the workflow's shape are stale", buildArtifactsWorkflowPath, releaseJobName)
	}

	// Discover every per-board kernel job by its "<board>-kernel" naming
	// convention (see the workflow's job list) rather than hard-coding a
	// board list here - that would just recreate the drift this test exists
	// to catch.
	kernelJobBoards := make(map[string]string) // boardID -> job name
	for jobName := range wf.Jobs {
		if board, ok := strings.CutSuffix(jobName, "-kernel"); ok {
			kernelJobBoards[board] = jobName
		}
	}

	specBoards := kernelspec.BoardIDs()
	specBoardSet := make(map[string]bool, len(specBoards))
	for _, board := range specBoards {
		specBoardSet[board] = true
	}

	for _, board := range specBoards {
		jobName, ok := kernelJobBoards[board]
		if !ok {
			t.Errorf("board %q has a kernelspec entry but no %q job in %s; add one (see the other <board>-kernel jobs for the pattern) and wire it into %s's needs/download-artifact steps",
				board, board+"-kernel", buildArtifactsWorkflowPath, releaseJobName)
			continue
		}

		if !releaseJob.Needs.contains(jobName) {
			t.Errorf("job %q builds board %q's kernel but is missing from %s's needs list in %s; add %q to needs",
				jobName, board, releaseJobName, buildArtifactsWorkflowPath, jobName)
		}

		artifactName := wf.Jobs[jobName].artifactUploadName()
		if artifactName == "" {
			t.Errorf("job %q has no actions/upload-artifact step with a name in %s; add one so %s can download its output",
				jobName, buildArtifactsWorkflowPath, releaseJobName)
			continue
		}
		if !releaseJob.downloadsArtifact(artifactName) {
			t.Errorf("%s has no actions/download-artifact step for artifact %q (produced by job %q, board %q) in %s; add one",
				releaseJobName, artifactName, jobName, board, buildArtifactsWorkflowPath)
		}
	}

	for board, jobName := range kernelJobBoards {
		if !specBoardSet[board] {
			t.Errorf("%s has job %q for board %q, but kernelspec.BoardIDs() doesn't know that board; remove the stale job or register the board's KernelSpec",
				buildArtifactsWorkflowPath, jobName, board)
		}
	}
}
