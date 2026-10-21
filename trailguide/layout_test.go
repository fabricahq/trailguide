// Check where a library's files live in its repository, and the group and rule IDs of rule paths.

package trailguide_test

import (
	"errors"
	"testing"

	"github.com/fabricahq/trailguide/trailguide"
)

// TestRepositoryPath_AndLibraryPathAreInverses places each kind of library file and maps it back.
func TestRepositoryPath_AndLibraryPathAreInverses(t *testing.T) {
	for library, repository := range map[string]string{
		trailguide.Manifest:                 ".trailguide/config.yaml",
		"techs/go/handle-errors.md":         ".trailguide/rules/techs/go/handle-errors.md",
		"practices/testing/_group.yaml":     ".trailguide/rules/practices/testing/_group.yaml",
		"assets/diagram.svg":                ".trailguide/assets/diagram.svg",
		"changes/techs/go/handle-errors.md": ".trailguide/changes/techs/go/handle-errors.md",
		"LICENSE.md":                        "LICENSE.md",
	} {
		if got := trailguide.RepositoryPath(library); got != repository {
			t.Errorf("RepositoryPath(%q) = %q; want %q", library, got, repository)
		}
		if got, ok := trailguide.LibraryPath(repository); !ok || got != library {
			t.Errorf("LibraryPath(%q) = %q, %v; want %q", repository, got, ok, library)
		}
	}
	for _, repository := range []string{".trailguide/vendor/team/x.md", ".trailguide/rules/README.md", "config.yaml", "assets/logo.svg"} {
		if got, ok := trailguide.LibraryPath(repository); ok {
			t.Errorf("LibraryPath(%q) = %q; want no library file", repository, got)
		}
	}
}

// TestGroupFromPath_ReturnsTheRulesGroup and refuses a path that can't be a rule file with a *ValidationError.
func TestGroupFromPath_ReturnsTheRulesGroup(t *testing.T) {
	if group, err := trailguide.GroupFromPath("techs/go/errors/wrap.md", "rule"); err != nil || group != "techs/go" {
		t.Fatalf("got %q, %v; want techs/go", group, err)
	}
	for _, path := range []string{"techs/go.md", "techs/go/assets/x.md", "tools/go/x.md", "techs/go/x.txt", "../techs/go/x.md"} {
		_, err := trailguide.GroupFromPath(path, "rule")
		var validation *trailguide.ValidationError
		if !errors.As(err, &validation) || validation.Location != "rule" {
			t.Errorf("GroupFromPath(%q) = %v; want a validation error at rule", path, err)
		}
	}
}

// TestValidateRuleID accepts a rule file's library path without .md, and nothing else.
func TestValidateRuleID(t *testing.T) {
	if err := trailguide.ValidateRuleID("practices/testing/verify-retry-limits", "id"); err != nil {
		t.Fatal(err)
	}
	var validation *trailguide.ValidationError
	if err := trailguide.ValidateRuleID("practices/testing", "id"); !errors.As(err, &validation) || validation.Location != "id" {
		t.Fatalf("got %v; want a validation error at id", err)
	}
}
