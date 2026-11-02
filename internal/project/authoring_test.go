// Exercise project initialization, local authoring, and imported group selection.

package project

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/rules"
	"github.com/fabricahq/trailguide/trailguide"
)

// TestProjectAuthoringLifecycle covers idempotent init, local authoring, offline build, and config-only source addition.
func TestProjectAuthoringLifecycle(t *testing.T) {
	ctx := context.Background()
	directory := filepath.Join(t.TempDir(), ".trailguide")
	options := Options{Directory: filepath.Dir(directory)}
	result, err := Initialize(ctx, options)
	// config.yaml, rules/README.md, README.md, skills/README.md, and AGENTS.md.
	if err != nil || len(result.Written()) != 5 {
		t.Fatal(result, err)
	}
	if guide, err := os.ReadFile(filepath.Join(directory, "skills", "README.md")); err != nil || !bytes.Equal(guide, []byte(skillsGuide)) {
		t.Fatalf("skills/README.md: %q, %v", guide, err)
	}
	original, err := os.ReadFile(filepath.Join(directory, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	result, err = Initialize(ctx, options)
	if err != nil || len(result.Written()) != 0 {
		t.Fatal(result, err)
	}
	metadata := trailguide.GroupMetadata{Name: " Go ", Description: " Local Go guidance. ", WhenToRead: " When editing Go. "}
	result, err = AddLocalGroup(ctx, "techs/go", metadata, options)
	if err != nil || len(result.Written()) != 2 {
		t.Fatal(result, err)
	}
	stored, err := os.ReadFile(result.Written()[0])
	if err != nil || !bytes.Contains(stored, []byte(`name: Go`)) {
		t.Fatal(string(stored), err)
	}
	body := "# Return errors\n\nReturn failures to the caller.\n"
	ruleMeta := rules.RuleMetadata{Title: "Return errors: always", Impact: "HIGH", ImpactDescription: "Preserve failures.", WhenToRead: "When calling fallible functions."}
	result, err = AddLocalRule(ctx, "techs/go/errors", ruleMeta, RuleOptions{Options: options, Body: &body})
	if err != nil || len(result.Written()) != 1 {
		t.Fatal(result, err)
	}
	if _, err = Build(ctx, options); err != nil {
		t.Fatal(err)
	}
	if _, err = AddLocalRule(ctx, "techs/go/errors", ruleMeta, RuleOptions{Options: options, Body: &body}); err == nil {
		t.Fatal("overwrote existing rule")
	}
	document, err := os.ReadFile(result.Written()[0])
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := trailguide.ParseRule(string(document), "techs/go/errors.md", "local")
	if err != nil || parsed.Title != ruleMeta.Title {
		t.Fatal(parsed, err)
	}
	if rules.HasDraftMarker(document) {
		t.Fatal("complete body-file rule was marked as draft")
	}
	result, err = AddLocalRule(ctx, "techs/go/draft", ruleMeta, RuleOptions{Options: options})
	if err != nil || len(result.Written()) != 1 {
		t.Fatal(result, err)
	}
	draft, _ := os.ReadFile(result.Written()[0])
	if !bytes.Contains(draft, []byte("### Validation")) || !bytes.Contains(draft, []byte("<State one concrete obligation.>")) || bytes.Count(draft, []byte(rules.DraftMarker)) != 1 {
		t.Fatal("canonical draft missing")
	}
	source := SourceInput{Repository: "https://github.com/acme/rules", Ref: "refs/tags/a&<b>", Rules: []string{"techs/*"}}
	result, err = AddSource(ctx, "team", source, options)
	if err != nil || len(result.Written()) != 1 {
		t.Fatal(result, err)
	}
	updated, _ := os.ReadFile(filepath.Join(directory, "config.yaml"))
	if bytes.Equal(updated, original) || bytes.Contains(updated, []byte(`\u003e`)) {
		t.Fatal(string(updated))
	}
	if _, err = rules.ParseConfigurationYAML(updated); err != nil {
		t.Fatal(err)
	}
	if _, err = AddSource(ctx, "team", source, options); err == nil {
		t.Fatal("duplicate alias accepted")
	}
	if _, err = os.Stat(filepath.Join(directory, "_managed/vendor")); !os.IsNotExist(err) {
		t.Fatal("source authoring fetched files", err)
	}
}

// TestAuthoringRefusesUnsafeAndIncompleteInput verifies failure paths preserve existing project bytes.
func TestAuthoringRefusesUnsafeAndIncompleteInput(t *testing.T) {
	for _, scenario := range []string{"invalid-config", "linked-rules", "case-alias", "hardlink", "pending-recovery", "cancelled"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			directory := filepath.Join(t.TempDir(), ".trailguide")
			options := Options{Directory: filepath.Dir(directory)}
			if _, err := Initialize(ctx, options); err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "invalid-config":
				os.WriteFile(filepath.Join(directory, "config.yaml"), []byte(`{`), 0600)
			case "linked-rules":
				os.RemoveAll(filepath.Join(directory, "rules"))
				if err := os.Symlink(t.TempDir(), filepath.Join(directory, "rules")); err != nil {
					t.Fatal(err)
				}
			case "case-alias":
				os.MkdirAll(filepath.Join(directory, "rules", "Techs"), 0700)
			case "hardlink":
				if err := os.Link(filepath.Join(directory, "config.yaml"), filepath.Join(directory, "alias")); err != nil {
					t.Fatal(err)
				}
			case "pending-recovery":
				os.Mkdir(filepath.Join(directory, ".trailguide-authoring-pending"), 0700)
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			before, _ := os.ReadFile(filepath.Join(directory, "config.yaml"))
			result, err := AddLocalGroup(ctx, "techs/go", trailguide.GroupMetadata{Name: "Go", Description: "Go.", WhenToRead: "When editing Go."}, options)
			if err == nil || result.Written() != nil {
				t.Fatal("expected no partial result", result, err)
			}
			after, _ := os.ReadFile(filepath.Join(directory, "config.yaml"))
			if !bytes.Equal(before, after) {
				t.Fatal("changed config on failure")
			}
			if _, err = os.Lstat(filepath.Join(directory, "rules", "techs", "go", "_group.yaml")); !os.IsNotExist(err) {
				t.Fatal("created group on failure", err)
			}
		})
	}
}

// TestRuleRequiresGroup rejects missing metadata without changing an existing document.
func TestRuleRequiresGroup(t *testing.T) {
	ctx := context.Background()
	directory := filepath.Join(t.TempDir(), ".trailguide")
	options := Options{Directory: filepath.Dir(directory)}
	if _, err := Initialize(ctx, options); err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(filepath.Join(directory, "rules/techs/go"), 0700)
	target := filepath.Join(directory, "rules/techs/go/errors.md")
	os.WriteFile(target, []byte("authored content"), 0600)
	body := "Return failures."
	_, err := AddLocalRule(ctx, "techs/go/errors", rules.RuleMetadata{Title: "Errors", Impact: "HIGH", ImpactDescription: "Failures.", WhenToRead: "When calling."}, RuleOptions{Options: options, Body: &body})
	if err == nil {
		t.Fatal("accepted collision")
	}
	if _, err = os.Stat(filepath.Join(directory, "rules/techs/go/_group.yaml")); !os.IsNotExist(err) {
		t.Fatal("published partial group", err)
	}
	data, _ := os.ReadFile(target)
	if string(data) != "authored content" {
		t.Fatal("lost prior bytes")
	}
}

// TestRuleUsesImportedGroup accepts imported metadata without creating a local override.
func TestRuleUsesImportedGroup(t *testing.T) {
	ctx := context.Background()
	directory := filepath.Join(t.TempDir(), ".trailguide")
	options := Options{Directory: filepath.Dir(directory)}
	if _, err := Initialize(ctx, options); err != nil {
		t.Fatal(err)
	}
	source := SourceInput{Repository: "https://github.com/acme/rules", Ref: "v1.0.0", Rules: []string{"techs/go/*"}}
	if _, err := AddSource(ctx, "team", source, options); err != nil {
		t.Fatal(err)
	}
	if _, err := PlanLocalRule(ctx, "techs/go/errors", options); err == nil {
		t.Fatal("planned rule before imported group was fetched")
	}
	data, err := os.ReadFile(filepath.Join(directory, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	config, err := rules.ParseConfigurationYAML(data)
	if err != nil {
		t.Fatal(err)
	}
	vendor, lock, err := encodeSnapshots(config, map[string]snapshot{"team": {Repository: config.Sources[0].Repository, Ref: gitRef(t, "v1.0.0"), Commit: strings.Repeat("a", 40), Groups: []string{"techs/go"}, Selection: config.Sources[0].Groups, Files: map[string][]byte{"config.yaml": []byte(`{"schemaVersion":2,"sources":{},"library":{"schemaVersion":2,"name":"library"}}`), "techs/go/_group.yaml": []byte(`{"name":"Go","description":"Imported guidance.","whenToRead":"When editing Go."}`)}}}, sourceRecords{})
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range vendor {
		target := filepath.Join(directory, "_managed/vendor", name)
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(directory, lockFile), lock, 0600); err != nil {
		t.Fatal(err)
	}
	body := "Return errors."
	result, err := AddLocalRule(ctx, "techs/go/errors", rules.RuleMetadata{Title: "Errors", Impact: "HIGH", ImpactDescription: "Failures.", WhenToRead: "When calling."}, RuleOptions{Options: options, Body: &body})
	if err != nil || len(result.Written()) != 1 {
		t.Fatal(result, err)
	}
	if _, err := os.Stat(filepath.Join(directory, "rules/techs/go/_group.yaml")); !os.IsNotExist(err) {
		t.Fatal("rule creation created local override", err)
	}
}
