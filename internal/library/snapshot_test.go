// Check individually selected rules loaded from a snapshot's files.

package library_test

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/library"
	"github.com/fabricahq/trailguide/internal/rules"
)

// ruleDocument is a valid rule whose body identifies it, so tests can tell copies apart.
func ruleDocument(body string) string {
	return strings.Replace(document, "Return the error.", body, 1)
}

// selectionFiles is a library with two groups, each with two rules, one of them with assets.
func selectionFiles() map[string][]byte {
	return map[string][]byte{
		"config.yaml":                        []byte(`{"schemaVersion":2,"sources":{},"library":{"formatVersion":1}}`),
		"techs/go/_group.yaml":               []byte(metadata),
		"techs/go/errors.md":                 []byte(ruleDocument("Errors.")),
		"techs/go/assets/errors/diagram.bin": {0, 1},
		"techs/go/naming.md":                 []byte(ruleDocument("Naming.")),
		"techs/go/assets/naming/example.bin": {2, 3},
		"practices/testing/_group.yaml":      []byte(metadata),
		"practices/testing/limits.md":        []byte(ruleDocument("Limits.")),
	}
}

// loadFiles loads files in memory, as project builds load vendored snapshots.
func loadFiles(t *testing.T, files map[string][]byte, groups rules.GroupSelection, ruleIDs []string) (library.Catalog, error) {
	t.Helper()
	return library.LoadSource(context.Background(), library.Snapshot{Files: files}.Source(), "team", groups, ruleIDs)
}

// TestLoadSource_IndividualRulesBringOnlyThemselvesAndTheirGroupMetadata keeps unselected rules and assets out.
func TestLoadSource_IndividualRulesBringOnlyThemselvesAndTheirGroupMetadata(t *testing.T) {
	catalog, err := loadFiles(t, selectionFiles(), rules.GroupSelection{Groups: []string{"practices/testing"}}, []string{"techs/go/errors"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"config.yaml", "practices/testing/_group.yaml", "practices/testing/limits.md", "techs/go/_group.yaml", "techs/go/assets/errors/diagram.bin", "techs/go/errors.md"}
	if !reflect.DeepEqual(catalog.Paths(), want) {
		t.Fatalf("paths %v, want %v", catalog.Paths(), want)
	}
	if !reflect.DeepEqual(catalog.Rules, []string{"techs/go/errors"}) || len(catalog.Groups) != 2 || catalog.Groups[1].ID != "techs/go" || len(catalog.Groups[1].Rules) != 1 {
		t.Fatalf("unexpected catalog %+v", catalog)
	}
}

// TestLoadSource_IndividualRuleInASelectedGroupChangesNothing loads the whole group once.
func TestLoadSource_IndividualRuleInASelectedGroupChangesNothing(t *testing.T) {
	catalog, err := loadFiles(t, selectionFiles(), rules.GroupSelection{Pattern: "techs/*"}, []string{"techs/go/errors"})
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Groups) != 1 || len(catalog.Groups[0].Rules) != 2 || !reflect.DeepEqual(catalog.Rules, []string{"techs/go/errors"}) {
		t.Fatalf("unexpected catalog %+v", catalog)
	}
}

// TestLoadSource_RejectsAnIndividualRuleTheLibraryLacks names the missing rule, even in a selected group.
func TestLoadSource_RejectsAnIndividualRuleTheLibraryLacks(t *testing.T) {
	for name, groups := range map[string]rules.GroupSelection{"unselected group": {Groups: []string{}}, "selected group": {Groups: []string{"techs/go"}}} {
		t.Run(name, func(t *testing.T) {
			_, err := loadFiles(t, selectionFiles(), groups, []string{"techs/go/missing"})
			if err == nil || !strings.Contains(err.Error(), "techs/go/missing") {
				t.Fatalf("got %v", err)
			}
		})
	}
}

// TestLoadSource_SkipsInvalidUnselectedRules imports a valid rule from a group whose other rules are invalid.
func TestLoadSource_SkipsInvalidUnselectedRules(t *testing.T) {
	files := selectionFiles()
	files["techs/go/naming.md"] = []byte("no frontmatter")
	if _, err := loadFiles(t, files, rules.GroupSelection{Groups: []string{}}, []string{"techs/go/errors"}); err != nil {
		t.Fatal(err)
	}
	if _, err := loadFiles(t, files, rules.GroupSelection{Groups: []string{"techs/go"}}, nil); err == nil {
		t.Fatal("a selected group accepted an invalid rule")
	}
}
