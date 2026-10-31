// Check library plan/commit parity when another writer changes authoring targets.

package library

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/librarytree"
	"github.com/fabricahq/trailguide/internal/rules"
	"github.com/fabricahq/trailguide/trailguide"
)

func TestAuthoringPlansRevalidateLiveState(t *testing.T) {
	ctx := context.Background()
	options := Options{Directory: filepath.Join(t.TempDir(), "team's library")}
	if _, err := Initialize(ctx, withToolVersion(options, "1.2.3"), "", nil); err != nil {
		t.Fatal(err)
	}
	metadata := trailguide.GroupMetadata{Name: "Go", Description: "Go guidance.", WhenToRead: "When editing Go."}
	group, err := PlanGroup(ctx, "techs/go", options)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AddGroup(ctx, "techs/go", metadata, options); err != nil {
		t.Fatal(err)
	}
	_, commitErr := group.Commit(ctx, metadata)
	_, planErr := PlanGroup(ctx, "techs/go", options)
	if commitErr == nil || planErr == nil || commitErr.Error() != planErr.Error() {
		t.Fatal(commitErr, planErr)
	}
	plan, err := PlanRule(ctx, "techs/go/errors", options)
	if err != nil {
		t.Fatal(err)
	}
	rule := rules.RuleMetadata{Title: "Return errors", Impact: "HIGH", ImpactDescription: "Preserve failures.", WhenToRead: "When calling functions."}
	if err := os.Remove(filepath.Join(options.Directory, librarytree.RepositoryPath("techs/go/_group.yaml"))); err != nil {
		t.Fatal(err)
	}
	_, commitErr = plan.Commit(ctx, rule, nil)
	_, planErr = PlanRule(ctx, "techs/go/errors", options)
	if commitErr == nil || planErr == nil || commitErr.Error() != planErr.Error() {
		t.Fatal(commitErr, planErr)
	}
	quoted := "--directory='" + strings.ReplaceAll(options.Directory, "'", "'\"'\"'") + "'"
	if !strings.Contains(commitErr.Error(), quoted) {
		t.Fatal("repair lost library location", commitErr)
	}
	if _, err := os.Stat(filepath.Join(options.Directory, librarytree.RepositoryPath("techs/go/errors.md"))); !os.IsNotExist(err) {
		t.Fatal("wrote rule after group disappeared", err)
	}
}

// TestPlanRule_FindsWhetherTheNewRuleNeedsAChangeNote from the library's release tags, failing in a shallow clone.
func TestPlanRule_FindsWhetherTheNewRuleNeedsAChangeNote(t *testing.T) {
	ctx := context.Background()
	fixture, released := authorClone(t, libraryFiles(), releaseOne)
	_, unreleased := authorClone(t, libraryFiles())
	for options, want := range map[*Options]bool{&released: true, &unreleased: false} {
		plan, err := PlanRule(ctx, "practices/testing/c", *options)
		if err != nil || plan.NeedsChangeNote() != want {
			t.Fatalf("%s: NeedsChangeNote %v, want %v: %v", options.Directory, plan.NeedsChangeNote(), want, err)
		}
	}
	shallow := filepath.Join(t.TempDir(), "shallow")
	if _, err := fixture.CommandIn(ctx, filepath.Dir(shallow), "clone", "--quiet", "--depth=1", "--template=", fixture.Repository, shallow); err != nil {
		t.Fatal(err)
	}
	released.Directory = shallow
	if _, err := PlanRule(ctx, "practices/testing/c", released); errorCode(err) != "shallow-clone" {
		t.Fatal(err)
	}
}
