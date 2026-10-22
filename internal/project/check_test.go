// Verify complete project status through its caller-facing operation, without CLI orchestration.

package project_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/fabricahq/trailguide/internal/filetxn"
	"github.com/fabricahq/trailguide/internal/project"
)

func TestCheckOwnsGuideAndGeneratedFreshness(t *testing.T) {
	for _, state := range []string{"current", "missing-guide", "edited-guide", "both-stale"} {
		t.Run(state, func(t *testing.T) {
			ctx := context.Background()
			options := project.Options{Directory: t.TempDir()}
			if _, err := project.Initialize(ctx, options); err != nil {
				t.Fatal(err)
			}
			if _, err := project.Build(ctx, options); err != nil {
				t.Fatal(err)
			}
			root, err := os.OpenRoot(filepath.Join(options.Directory, ".trailguide"))
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			switch state {
			case "missing-guide":
				err = root.Remove("README.md")
			case "edited-guide", "both-stale":
				err = root.WriteFile("README.md", []byte("User notes.\n"), 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			if state == "both-stale" {
				if err := root.WriteFile("_managed/generated/RULES.md", []byte("Stale output.\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			before, err := filetxn.ReadTree(ctx, root, ".")
			if err != nil {
				t.Fatal(err)
			}
			report, err := project.Check(ctx, options)
			if err != nil {
				t.Fatal(err)
			}
			want := []project.Problem{}
			if state == "both-stale" {
				want = append(want, project.Problem{Kind: project.StaleContents, Path: "_managed/generated/RULES.md", Repair: project.Rebuild})
			}
			if state != "current" {
				want = append(want, project.Problem{Kind: project.OutdatedGuide, Path: "README.md", Repair: project.RefreshGuide})
			}
			if !reflect.DeepEqual(report.Problems, want) || report.Current() != (state == "current") {
				t.Fatalf("got %+v; want %+v", report, want)
			}
			after, err := filetxn.ReadTree(ctx, root, ".")
			if err != nil || before.Digest() != after.Digest() {
				t.Fatal("check modified project state", err)
			}
		})
	}
}
