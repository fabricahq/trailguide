// Fork published rule versions from a real library into rules/, and check what the fork writes and refuses.

package project

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/errs"
	"github.com/fabricahq/trailguide/internal/filetxn"
	"github.com/fabricahq/trailguide/internal/imports"
	"github.com/fabricahq/trailguide/internal/test/gitfixture"
	"github.com/fabricahq/trailguide/trailguide"
)

// forkedRule is a rule document whose body is text.
func forkedRule(text string) string {
	return "---\ntitle: Return errors\nimpact: HIGH\nimpactDescription: Preserve failures.\nwhenToRead: When calling functions.\n---\n" + text + "\n"
}

// forkFixture is a project synced from a library whose release/1 publishes techs/go/errors, which links to shared
// assets directly and through its own asset, techs/go/licensed, which links to the library's license, and
// practices/testing/verify, which the project doesn't import. Its release/2 moves errors to 1.1.0 and adds
// techs/go/added.
type forkFixture struct {
	fixture *gitfixture.Fixture
	options Options
	git     imports.Options
}

// newForkFixture publishes both library releases after syncing the project at release/1. The project's source is
// team, selecting techs/go from repository, or from the fixture's own address when repository is empty. The
// fixture's transport serves every SSH address.
func newForkFixture(t *testing.T, repository string) forkFixture {
	t.Helper()
	ctx := context.Background()
	f, err := gitfixture.New(ctx, gitfixture.Library(map[string][]byte{
		"config.yaml":                     []byte(`{"schemaVersion":2,"sources":{},"library":{"license":{"file":"LICENSE","notices":[]}}}`),
		"LICENSE":                         []byte("Terms\n"),
		"techs/go/_group.yaml":            []byte("# Go metadata.\n" + projectMetadata + "\n"),
		"techs/go/errors.md":              []byte(forkedRule("Read [the guide](../../../assets/guide.md), [notes](assets/errors/notes.md), and [data](assets/errors/data.bin#top).")),
		"techs/go/assets/errors/notes.md": []byte("![Flow](../../../../../assets/diagrams/flow.svg) [Data](data.bin)\n"),
		"techs/go/assets/errors/data.bin": {0, 255},
		"techs/go/licensed.md":            []byte(forkedRule("See [the terms](../../../../LICENSE).")),
		"practices/testing/_group.yaml":   []byte(`{"name":"Testing","description":"Tests.","whenToRead":"When testing."}`),
		"practices/testing/verify.md":     []byte(forkedRule("Verify.")),
		"assets/guide.md":                 []byte("[More](more.md)\n"),
		"assets/more.md":                  []byte("More.\n"),
		"assets/diagrams/flow.svg":        []byte("<svg/>"),
	}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	record := "formatVersion: 1\nrelease: 1\nrules:\n  practices/testing/verify: 1.0.0\n  techs/go/errors: 1.0.0\n  techs/go/licensed: 1.0.0\nchanges:\n" +
		"  practices/testing/verify: {change: new, summaries: [Add the rule.]}\n  techs/go/errors: {change: new, summaries: [Add the rule.]}\n  techs/go/licensed: {change: new, summaries: [Add the rule.]}\n"
	if err := f.Release(ctx, 1, record); err != nil {
		t.Fatal(err)
	}
	if repository == "" {
		repository = f.Repository
	}
	root := openTestProject(t)
	options := Options{Directory: filepath.Dir(root.Name()), ToolVersion: "1.2.3"}
	if _, err := Initialize(ctx, options); err != nil {
		t.Fatal(err)
	}
	git := imports.Options{GitPath: f.GitPath, Environment: f.Environment}
	writeFixture(t, root, configurationFile, withNewestRef(t, "# Team rules.\nschemaVersion: 2\nsources:\n  team:\n    repository: "+repository+"\n    rules:\n      include:\n        - techs/go/*\n", repository, f.Repository, git))
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Commit(ctx, f.Worktree(), "Second release", gitfixture.Library(map[string][]byte{"techs/go/errors.md": []byte(forkedRule("Wrap errors.")), "techs/go/added.md": []byte(forkedRule("Added."))})); err != nil {
		t.Fatal(err)
	}
	if err := f.Release(ctx, 2, "formatVersion: 1\nrelease: 2\nrules:\n  practices/testing/verify: 1.0.0\n  techs/go/added: 1.0.0\n  techs/go/errors: 1.1.0\n  techs/go/licensed: 1.0.0\nchanges:\n  techs/go/added: {change: new, summaries: [Add the rule.]}\n  techs/go/errors: {change: minor, from: 1.0.0, summaries: [Add wrapping.]}\n"); err != nil {
		t.Fatal(err)
	}
	return forkFixture{fixture: f, options: options, git: git}
}

// fork plans and commits a fork of id from library@version with reason.
func (f forkFixture) fork(t *testing.T, id, from, reason string) (AuthoringResult, error) {
	t.Helper()
	source, err := ParseForkSource(from)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := PlanFork(context.Background(), id, source, f.options, f.git)
	if err != nil {
		return AuthoringResult{}, err
	}
	return plan.Commit(context.Background(), reason)
}

// files returns every file of the project's Trail Guide directory.
func (f forkFixture) files(t *testing.T) map[string][]byte {
	t.Helper()
	return projectTree(t, f.options).Files
}

// TestFork_ReplacesAnImportedRuleWithAnOlderVersion forks 1.0.0 after release/2 published 1.1.0: the fork holds
// the older text, its assets, and the shared assets it links to with rewritten links, and the exclusion that makes
// it replace the imported rule when the project builds. The imported library supplies the group, so the fork
// creates no local group metadata.
func TestFork_ReplacesAnImportedRuleWithAnOlderVersion(t *testing.T) {
	f := newForkFixture(t, "")
	before := f.files(t)
	result, err := f.fork(t, "techs/go/errors", "team@1.0.0", "Our services need the original wording.")
	if err != nil {
		t.Fatal(err)
	}
	after := f.files(t)
	added := []string{}
	for file := range after {
		if _, existed := before[file]; !existed {
			added = append(added, file)
		}
	}
	slices.Sort(added)
	want := []string{"rules/techs/go/assets/errors/data.bin", "rules/techs/go/assets/errors/diagrams/flow.svg", "rules/techs/go/assets/errors/guide.md", "rules/techs/go/assets/errors/more.md", "rules/techs/go/assets/errors/notes.md", "rules/techs/go/errors.md"}
	if !reflect.DeepEqual(added, want) || len(result.Added) != len(want) || len(result.Changed) != 1 || filepath.Base(result.Changed[0]) != "config.yaml" {
		t.Fatalf("added %v, reported %v created and %v changed; want %v created and config.yaml changed", added, result.Added, result.Changed, want)
	}
	if got := string(after["rules/techs/go/errors.md"]); got != forkedRule("Read [the guide](assets/errors/guide.md), [notes](assets/errors/notes.md), and [data](assets/errors/data.bin#top).") {
		t.Fatalf("forked rule %q", got)
	}
	if got := string(after["rules/techs/go/assets/errors/notes.md"]); got != "![Flow](diagrams/flow.svg) [Data](data.bin)\n" {
		t.Fatalf("forked asset %q", got)
	}
	if got := string(after["rules/techs/go/assets/errors/guide.md"]); got != "[More](more.md)\n" {
		t.Fatalf("copied shared asset %q", got)
	}
	config := string(after["config.yaml"])
	if !strings.HasPrefix(config, "# Team rules.\n") || !strings.Contains(config, "      exclude:\n        - id: techs/go/errors\n          reason: Our services need the original wording.\n          replacedBy: techs/go/errors\n          basedOn: \"1.0.0\"\n") {
		t.Fatalf("configuration %q", config)
	}
	if _, err := Build(context.Background(), f.options); err != nil {
		t.Fatal(err)
	}
	generated := f.files(t)
	if _, ok := generated["generated/rules/local/techs/go/errors.md"]; !ok {
		t.Fatal("the build has no fork")
	}
	// Provenance names the version the fork is based on, beside the imported rule it replaces.
	var provenance struct {
		Rules []struct {
			ID       string
			BasedOn  *string `json:"basedOn"`
			Upstream struct{ Version string }
		}
	}
	if err := json.Unmarshal(generated["generated/provenance.json"], &provenance); err != nil {
		t.Fatal(err)
	}
	index := slices.IndexFunc(provenance.Rules, func(rule struct {
		ID       string
		BasedOn  *string `json:"basedOn"`
		Upstream struct{ Version string }
	}) bool {
		return rule.ID == "local:techs/go/errors"
	})
	if index < 0 || provenance.Rules[index].BasedOn == nil || *provenance.Rules[index].BasedOn != "1.0.0" || provenance.Rules[index].Upstream.Version != "1.0.0" {
		t.Fatalf("provenance:\n%s", generated["generated/provenance.json"])
	}
	if _, ok := generated["generated/rules/team/techs/go/errors.md"]; ok {
		t.Fatal("the build still has the imported rule")
	}
}

// TestFork_OfAPinnedRuleRemovesThePin forks the version a pin holds, 1.0.0: the same configuration write replaces
// the pin with the exclusion, based on 1.0.0, with a warning, and the next update lists the rule as replaced, with
// the library's changes since 1.0.0.
func TestFork_OfAPinnedRuleRemovesThePin(t *testing.T) {
	f := newForkFixture(t, "")
	ctx := context.Background()
	root, err := openProject(ctx, f.options, false)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	writeFixture(t, root, configurationFile, withNewestRef(t, "# Team rules.\nschemaVersion: 2\nsources:\n  team:\n    repository: "+f.fixture.Repository+"\n    rules:\n      include:\n        - techs/go/*\n      pins:\n        - id: techs/go/errors\n          version: \"1.0.0\"\n          reason: Not ready.\n", f.fixture.Repository, f.fixture.Repository, f.git))
	if _, err := updateProject(ctx, f.options, f.git); err != nil {
		t.Fatal(err)
	}
	result, err := f.fork(t, "techs/go/errors", "team@1.0.0", "Ours.")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Warnings) != 1 || !strings.HasPrefix(result.Warnings[0], "Removed sources.team.rules.pins.techs/go/errors, which kept the rule at 1.0.0, because the fork replaces the imported rule.") {
		t.Fatalf("warnings %q", result.Warnings)
	}
	config := string(f.files(t)["config.yaml"])
	if strings.Contains(config, "pins") || !strings.Contains(config, "      exclude:\n        - id: techs/go/errors\n          reason: Ours.\n          replacedBy: techs/go/errors\n          basedOn: \"1.0.0\"\n") {
		t.Fatalf("configuration:\n%s", config)
	}
	if _, err := Build(ctx, f.options); err != nil {
		t.Fatal(err)
	}
	plan, err := PlanUpdate(ctx, f.options, f.git, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := plan.Preview(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if rows := preview.Sources[0].Rules; !slices.ContainsFunc(rows, func(row imports.RuleUpdate) bool {
		return row.ID == "techs/go/errors" && row.Change == imports.UpdateReplaced && row.LocalRule == "rules/techs/go/errors.md" && row.BasedOn != nil && row.BasedOn.String() == "1.0.0" && slices.Equal(row.Summaries, []string{"Add wrapping."})
	}) {
		t.Fatalf("rows %+v, want errors replaced by the fork", rows)
	}
}

// TestFork_OfARuleTheProjectDoesNotImportWritesNoExclusion forks a rule of a group the source doesn't select, and
// a rule through a repository address that no source uses: neither changes the configuration.
func TestFork_OfARuleTheProjectDoesNotImportWritesNoExclusion(t *testing.T) {
	f := newForkFixture(t, "")
	for _, test := range []struct{ id, from string }{
		{"practices/testing/verify", "team@1.0.0"},
		{"techs/go/errors", "ssh://git@fixture.invalid/rules@1.1.0"},
	} {
		before := f.files(t)
		source, err := ParseForkSource(test.from)
		if err != nil {
			t.Fatal(err)
		}
		plan, err := PlanFork(context.Background(), test.id, source, f.options, f.git)
		if err != nil {
			t.Fatal(err)
		}
		if plan.Replaces() != "" {
			t.Fatalf("%s: the fork replaces source %s", test.id, plan.Replaces())
		}
		if _, err := plan.Commit(context.Background(), "No reason is recorded."); err == nil {
			t.Fatalf("%s: a reason with nothing to replace was accepted", test.id)
		}
		if _, err := plan.Commit(context.Background(), ""); err != nil {
			t.Fatal(err)
		}
		after := f.files(t)
		if string(after["config.yaml"]) != string(before["config.yaml"]) {
			t.Fatalf("%s: configuration changed to %q", test.id, after["config.yaml"])
		}
	}
	files := f.files(t)
	if string(files["rules/techs/go/errors.md"]) != forkedRule("Wrap errors.") || !strings.Contains(string(files["rules/practices/testing/_group.yaml"]), "Testing") {
		t.Fatalf("forked files %v", slices.Sorted(maps.Keys(files)))
	}
	if _, err := Build(context.Background(), f.options); err != nil {
		t.Fatal(err)
	}
}

// TestFork_OfAnImportedRuleByRepositoryAddressExcludesIt matches an address to the source that uses it.
func TestFork_OfAnImportedRuleByRepositoryAddressExcludesIt(t *testing.T) {
	f := newForkFixture(t, "")
	source, err := ParseForkSource(f.fixture.Repository + "@1.1.0")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := PlanFork(context.Background(), "techs/go/errors", source, f.options, f.git)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Replaces() != "team" || plan.Release() != 2 {
		t.Fatalf("replaces %q from release %d", plan.Replaces(), plan.Release())
	}
	if _, err := plan.Commit(context.Background(), " "); err == nil {
		t.Fatal("a blank reason was accepted")
	}
	if _, err := plan.Commit(context.Background(), "Ours."); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(f.files(t)["config.yaml"]), "replacedBy: techs/go/errors") {
		t.Fatal("no exclusion")
	}
}

// TestFork_AttributesGitHubRulesAtTheReleaseCommit links the fork to the rule's file at the commit of the library
// release that published the version.
func TestFork_AttributesGitHubRulesAtTheReleaseCommit(t *testing.T) {
	f := newForkFixture(t, "git@github.com:acme/rules.git")
	commit, err := f.fixture.Command(context.Background(), "rev-parse", "release/1^{commit}")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.fork(t, "techs/go/errors", "team@1.0.0", "Ours."); err != nil {
		t.Fatal(err)
	}
	rule, err := trailguide.ParseRule(string(f.files(t)["rules/techs/go/errors.md"]), "techs/go/errors.md", "local")
	want := []trailguide.Attribution{{URL: "https://github.com/acme/rules/blob/" + commit + "/.trailguide/rules/techs/go/errors.md", Description: "Forked from version 1.0.0 of techs/go/errors, published in library release 1 at commit " + commit + "."}}
	if err != nil || !reflect.DeepEqual(rule.Attribution, want) {
		t.Fatalf("attribution %+v, %v; want %+v", rule.Attribution, err, want)
	}
}

// TestForkAttribution_DependsOnTheHost links GitHub.com and GitLab.com rules at the commit, where the commit's
// layout keeps them, cites other hosts' HTTPS addresses, and gives other addresses no entry.
func TestForkAttribution_DependsOnTheHost(t *testing.T) {
	published := imports.PublishedRule{Release: 4, Commit: "0123456789abcdef0123456789abcdef01234567"}
	version, _ := trailguide.ParseRuleVersion("1.3.0", "version")
	for _, test := range []struct {
		repository, url string
		legacy          bool
	}{
		{"https://github.com/acme/rules.git", "https://github.com/acme/rules/blob/" + published.Commit + "/.trailguide/rules/techs/go/errors.md", false},
		{"https://github.com/acme/rules.git", "https://github.com/acme/rules/blob/" + published.Commit + "/techs/go/errors.md", true},
		{"git@gitlab.com:acme/eng/rules.git", "https://gitlab.com/acme/eng/rules/-/blob/" + published.Commit + "/.trailguide/rules/techs/go/errors.md", false},
		{"https://git.example.org/srv/rules.git", "https://git.example.org/srv/rules.git", false},
		{"git@git.example.org:srv/rules.git", "", false},
	} {
		published.LegacyLayout = test.legacy
		got, err := forkAttribution(test.repository, "techs/go/errors", version, published)
		if err != nil || (got == nil) != (test.url == "") || got != nil && (got.URL != test.url || got.Description != "Forked from version 1.3.0 of techs/go/errors, published in library release 4 at commit "+published.Commit+".") {
			t.Errorf("%s: got %+v, %v; want %q", test.repository, got, err, test.url)
		}
	}
}

// TestFork_RefusesWithoutWriting refuses unknown versions, existing local rules and exclusions, a missing reason,
// links a fork can't copy, and a configuration changed after planning, leaving the project unchanged.
func TestFork_RefusesWithoutWriting(t *testing.T) {
	f := newForkFixture(t, "")
	root, err := openProject(context.Background(), f.options, false)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	writeFixture(t, root, "rules/techs/go/existing.md", forkedRule("Ours."))
	before := f.files(t)
	unreachable := f.git
	unreachable.GitPath = "/nonexistent/git"
	for _, test := range []struct {
		name, id, from, reason string
		git                    imports.Options
		edit                   string
		check                  func(error) bool
	}{
		{"unknown version", "techs/go/errors", "team@1.2.0", "Ours.", f.git, "", code("version-not-found")},
		{"existing local rule, before reading the library", "techs/go/existing", "team@1.0.0", "Ours.", unreachable, "", code("already-exists")},
		{"unknown source", "techs/go/errors", "other@1.0.0", "Ours.", f.git, "", location("--from")},
		{"license link", "techs/go/licensed", "team@1.0.0", "Ours.", f.git, "", message("links to LICENSE")},
		{"missing reason", "techs/go/errors", "team@1.0.0", "", f.git, "", location("--reason")},
		{"configuration changed after planning", "techs/go/errors", "team@1.0.0", "Ours.", f.git, "# Edited.\n", code("concurrent-change")},
	} {
		t.Run(test.name, func(t *testing.T) {
			source, err := ParseForkSource(test.from)
			if err != nil {
				t.Fatal(err)
			}
			plan, err := PlanFork(context.Background(), test.id, source, f.options, test.git)
			if err == nil {
				if test.edit != "" {
					writeFixture(t, root, configurationFile, test.edit+string(before["config.yaml"]))
					defer writeFixture(t, root, configurationFile, string(before["config.yaml"]))
				}
				_, err = plan.Commit(context.Background(), test.reason)
			}
			if err == nil || !test.check(err) {
				t.Fatalf("got %v", err)
			}
			after := f.files(t)
			delete(after, "config.yaml")
			wanted := maps.Clone(before)
			delete(wanted, "config.yaml")
			if !reflect.DeepEqual(slices.Sorted(maps.Keys(after)), slices.Sorted(maps.Keys(wanted))) {
				t.Fatalf("files changed: %v", slices.Sorted(maps.Keys(after)))
			}
		})
	}
}

// TestFork_NeverReplacesAnExistingExclusion refuses before reading the library when the source already excludes
// the rule, by its ID or by a selector that matches it.
func TestFork_NeverReplacesAnExistingExclusion(t *testing.T) {
	for _, entry := range []string{"techs/go/errors", "techs/go/*", "techs/*", `"*"`} {
		t.Run(entry, func(t *testing.T) {
			f := newForkFixture(t, "")
			root, err := openProject(context.Background(), f.options, false)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			config := string(f.files(t)["config.yaml"]) + "      exclude:\n        - id: " + entry + "\n          reason: Not for us.\n"
			writeFixture(t, root, configurationFile, config)
			source, _ := ParseForkSource("team@1.0.0")
			_, err = PlanFork(context.Background(), "techs/go/errors", source, f.options, imports.Options{GitPath: "/nonexistent/git"})
			var invalid errs.ValidationError
			if !errors.As(err, &invalid) || invalid.ValidationLocation() != "sources.team.rules.exclude" {
				t.Fatalf("got %v", err)
			}
			if got := string(f.files(t)["config.yaml"]); got != config {
				t.Fatalf("configuration changed to %q", got)
			}
		})
	}
}

// code matches a failure with a stable code.
func code(want string) func(error) bool {
	return func(err error) bool {
		var domain *filetxn.Error
		var git *imports.Error
		return errors.As(err, &domain) && domain.Code == want || errors.As(err, &git) && git.Code == want
	}
}

// location matches a validation error at a location.
func location(want string) func(error) bool {
	return func(err error) bool {
		var invalid errs.ValidationError
		return errors.As(err, &invalid) && invalid.ValidationLocation() == want
	}
}

// message matches an error whose text contains want.
func message(want string) func(error) bool {
	return func(err error) bool { return strings.Contains(err.Error(), want) }
}

// TestFork_OfARuleARefDoesNotImportWritesNoExclusion forks a rule that release/2 added while the source imports
// release/1 with ref: the source selects the rule's group but doesn't import it, so the fork needs no reason and
// writes no exclusion, and sync and build still succeed.
func TestFork_OfARuleARefDoesNotImportWritesNoExclusion(t *testing.T) {
	f := newForkFixture(t, "")
	root, err := openProject(context.Background(), f.options, false)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	config := "schemaVersion: 2\nsources:\n  team:\n    repository: " + f.fixture.Repository + "\n    ref: release/1\n    rules:\n      include:\n        - techs/go/*\n"
	writeFixture(t, root, configurationFile, config)
	if _, err := Sync(context.Background(), f.options, f.git); err != nil {
		t.Fatal(err)
	}
	source, _ := ParseForkSource("team@1.0.0")
	plan, err := PlanFork(context.Background(), "techs/go/added", source, f.options, f.git)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Replaces() != "" {
		t.Fatalf("the fork replaces source %s's rule", plan.Replaces())
	}
	if _, err := plan.Commit(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	if got := string(f.files(t)["config.yaml"]); got != config {
		t.Fatalf("configuration changed to %q", got)
	}
	if _, err := Sync(context.Background(), f.options, f.git); err != nil {
		t.Fatal(err)
	}
	if _, err := Build(context.Background(), f.options); err != nil {
		t.Fatal(err)
	}
}

// TestFork_KeepsTheLibrarysGroupDescription forks the only rule a source imports from its group, individually:
// the library still supplies the group, so the fork writes no local metadata, and the generated group page keeps
// the library's description.
func TestFork_KeepsTheLibrarysGroupDescription(t *testing.T) {
	f := newForkFixture(t, "")
	root, err := openProject(context.Background(), f.options, false)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	writeFixture(t, root, configurationFile, withNewestRef(t, "schemaVersion: 2\nsources:\n  team:\n    repository: "+f.fixture.Repository+"\n    rules:\n      include:\n        - practices/testing/verify\n", f.fixture.Repository, f.fixture.Repository, f.git))
	if _, err := updateProject(context.Background(), f.options, f.git); err != nil {
		t.Fatal(err)
	}
	if _, err := f.fork(t, "practices/testing/verify", "team@1.0.0", "Ours."); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.files(t)["rules/practices/testing/_group.yaml"]; ok {
		t.Fatal("the fork wrote local group metadata")
	}
	if _, err := Build(context.Background(), f.options); err != nil {
		t.Fatal(err)
	}
	if page := string(f.files(t)["generated/groups/practices/testing.md"]); !strings.Contains(page, "Tests.") || !strings.Contains(page, "rules/practices/testing/verify") {
		t.Fatalf("group page:\n%s", page)
	}
}

// TestUpdate_KeepsTheGroupMetadataAForkOfARetiredRuleNeeds forks an individually selected rule, whose import alone
// supplies the group's metadata, then updates past the rule's retirement: the update applies, writes the group's
// metadata from the library release that now supplies the shared files to rules/ in the same transaction, says so,
// and the fork still builds.
func TestUpdate_KeepsTheGroupMetadataAForkOfARetiredRuleNeeds(t *testing.T) {
	f := newForkFixture(t, "")
	ctx := context.Background()
	root, err := openProject(ctx, f.options, false)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	writeFixture(t, root, configurationFile, withNewestRef(t, "schemaVersion: 2\nsources:\n  team:\n    repository: "+f.fixture.Repository+"\n    rules:\n      include:\n        - techs/go/errors\n", f.fixture.Repository, f.fixture.Repository, f.git))
	if _, err := updateProject(ctx, f.options, f.git); err != nil {
		t.Fatal(err)
	}
	if _, err := f.fork(t, "techs/go/errors", "team@1.1.0", "Ours."); err != nil {
		t.Fatal(err)
	}
	if _, err := Build(ctx, f.options); err != nil {
		t.Fatal(err)
	}
	library := f.fixture
	if _, err := library.Commit(ctx, library.Worktree(), "Retire errors", gitfixture.Library(map[string][]byte{"techs/go/errors.md": nil, "techs/go/assets/errors/notes.md": nil, "techs/go/assets/errors/data.bin": nil, "techs/go/_group.yaml": []byte("# Go metadata, described again.\n" + projectMetadata + "\n")})); err != nil {
		t.Fatal(err)
	}
	if err := library.Release(ctx, 3, "formatVersion: 1\nrelease: 3\nrules:\n  practices/testing/verify: 1.0.0\n  techs/go/added: 1.0.0\n  techs/go/licensed: 1.0.0\nretired:\n  techs/go/errors: {lastVersion: 1.1.0, summaries: [No longer recommended.]}\n"); err != nil {
		t.Fatal(err)
	}
	plan, err := PlanUpdate(ctx, f.options, f.git, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	applied, err := plan.Apply(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	metadata := "rules/techs/go/_group.yaml"
	if !slices.Contains(applied.Added, metadata) || !slices.ContainsFunc(applied.Warnings, func(warning string) bool { return strings.HasPrefix(warning, "Wrote "+metadata) }) {
		t.Fatalf("added %v, warnings %v", applied.Added, applied.Warnings)
	}
	if got := string(f.files(t)[metadata]); got != "# Go metadata, described again.\n"+projectMetadata+"\n" {
		t.Fatalf("metadata %q, want release 3's", got)
	}
	if page := string(f.files(t)["generated/groups/techs/go.md"]); !strings.Contains(page, "Go guidance.") || !strings.Contains(page, "rules/techs/go/errors") {
		t.Fatalf("group page:\n%s", page)
	}
	requireCurrent(t, f.options)
}

// TestFork_OfASelectedRuleNeedsASyncedRecord refuses, before reading the library, to fork a rule the source
// selects when the project hasn't synced the source's current configuration, since whether it imports the rule
// isn't known.
func TestFork_OfASelectedRuleNeedsASyncedRecord(t *testing.T) {
	f := newForkFixture(t, "")
	root, err := openProject(context.Background(), f.options, false)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	writeFixture(t, root, configurationFile, strings.Replace(string(f.files(t)["config.yaml"]), "ref: release/1", "ref: main", 1))
	source, _ := ParseForkSource("team@1.0.0")
	_, err = PlanFork(context.Background(), "techs/go/errors", source, f.options, imports.Options{GitPath: "/nonexistent/git"})
	var invalid errs.ValidationError
	if !errors.As(err, &invalid) || invalid.ValidationLocation() != "vendor/team/_source.json" || !strings.Contains(invalid.ValidationProblem(), "run trailguide project sync") {
		t.Fatalf("got %v", err)
	}
}

// TestForkFiles_RelocatesExplicitSelfLinks rewrites a moved shared asset's root-relative and relative links to
// itself, and keeps its pathless ones.
func TestForkFiles_RelocatesExplicitSelfLinks(t *testing.T) {
	published := imports.PublishedRule{Release: 1, Commit: "0123456789abcdef0123456789abcdef01234567", Files: map[string][]byte{
		"techs/go/errors.md": []byte(forkedRule("See [the guide](../../../assets/guide.md).")),
		"assets/guide.md":    []byte("[Root](/assets/guide.md#top) [Relative](../assets/guide.md) [Here](#top)\n"),
	}}
	files, err := forkFiles("techs/go/errors", "techs/go/errors.md", published, nil)
	if want := "[Root](guide.md#top) [Relative](guide.md) [Here](#top)\n"; err != nil || string(files["techs/go/assets/errors/guide.md"]) != want {
		t.Fatalf("got %q, %v; want %q", files["techs/go/assets/errors/guide.md"], err, want)
	}
}

// TestForkFiles_RefusesRelativeRawHTMLLinks refuses a fork whose Markdown has a relative link in raw HTML, in the
// rule or a shared asset, because generation rejects those in local rules; external HTML links are fine.
func TestForkFiles_RefusesRelativeRawHTMLLinks(t *testing.T) {
	for _, test := range []struct {
		name, rule, guide string
		refused           bool
	}{
		{"rule's own asset", `See <a href="assets/errors/x.md">x</a>.`, "Guide.\n", true},
		{"shared asset", "See [the guide](../../../assets/guide.md).", `<img src="flow.svg">` + "\n", true},
		{"anchor", `See <a href="#top">the top</a>.`, "Guide.\n", true},
		{"external", `See <a href="https://example.com/x">x</a>.`, "Guide.\n", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			published := imports.PublishedRule{Release: 1, Commit: "0123456789abcdef0123456789abcdef01234567", Files: map[string][]byte{
				"techs/go/errors.md":          []byte(forkedRule(test.rule + " [Guide](../../../assets/guide.md)")),
				"techs/go/assets/errors/x.md": []byte("X.\n"),
				"assets/guide.md":             []byte(test.guide),
				"assets/flow.svg":             []byte("<svg/>"),
			}}
			files, err := forkFiles("techs/go/errors", "techs/go/errors.md", published, nil)
			if !test.refused {
				if err != nil || !strings.Contains(string(files["techs/go/errors.md"]), test.rule) {
					t.Fatalf("the fork lost its external link: %v\n%s", err, files["techs/go/errors.md"])
				}
				return
			}
			var validation errs.ValidationError
			if !errors.As(err, &validation) || !strings.Contains(validation.ValidationProblem(), "raw HTML") {
				t.Fatalf("got %v, want a refusal of the raw HTML link", err)
			}
		})
	}
}

// TestForkFiles_RewritesLinksInDocumentOrder rewrites a reference link that precedes an inline link, and an image
// nested in a link, whose destinations the Markdown tree lists out of document order.
func TestForkFiles_RewritesLinksInDocumentOrder(t *testing.T) {
	published := imports.PublishedRule{Release: 1, Commit: "0123456789abcdef0123456789abcdef01234567", Files: map[string][]byte{
		"techs/go/errors.md": []byte(forkedRule("See [the guide][g] and [![Flow](../../../assets/flow.svg)](../../../assets/guide.md).\n\n[g]: ../../../assets/guide.md")),
		"assets/guide.md":    []byte("Guide.\n"),
		"assets/flow.svg":    []byte("<svg/>"),
	}}
	files, err := forkFiles("techs/go/errors", "techs/go/errors.md", published, nil)
	want := forkedRule("See [the guide][g] and [![Flow](assets/errors/flow.svg)](assets/errors/guide.md).\n\n[g]: assets/errors/guide.md")
	if err != nil || string(files["techs/go/errors.md"]) != want {
		t.Fatalf("got %q, %v; want %q", files["techs/go/errors.md"], err, want)
	}
}

// TestUpdate_KeepingAReplacedRuleStillListsItsChangesForTheFork: keeping a fork's replaced rule pins the imported
// copy, yet the next update still lists the changes after basedOn, and replacing the fork with the newest version
// advances basedOn while the pin stays.
func TestUpdate_KeepingAReplacedRuleStillListsItsChangesForTheFork(t *testing.T) {
	f := newForkFixture(t, "")
	ctx := context.Background()
	if _, err := f.fork(t, "techs/go/errors", "team@1.0.0", "Ours."); err != nil {
		t.Fatal(err)
	}
	if _, err := Build(ctx, f.options); err != nil {
		t.Fatal(err)
	}
	preview := func(decisions []UpdateDecision) (*UpdatePlan, *imports.RuleUpdate) {
		t.Helper()
		plan, err := PlanUpdate(ctx, f.options, f.git, nil, decisions)
		if err != nil {
			t.Fatal(err)
		}
		result, err := plan.Preview(context.Background(), decisions)
		if err != nil {
			t.Fatal(err)
		}
		return plan, previewRow(result.Sources, "team", "techs/go/errors")
	}
	keep := []UpdateDecision{{Source: "team", Rule: "techs/go/errors", Kind: DecisionKeep, Reason: "Not yet."}}
	plan, _ := preview(keep)
	if _, err := plan.Apply(ctx, keep); err != nil {
		t.Fatal(err)
	}
	plan, row := preview(updateFork)
	if row == nil || row.Change != imports.UpdateReplaced || row.Pin == nil || row.Newest == nil || row.Newest.String() != "1.1.0" || row.To != nil {
		t.Fatalf("after keeping, the next update lists %+v, want the pinned replaced row", row)
	}
	if _, err := plan.Apply(ctx, updateFork); err != nil {
		t.Fatal(err)
	}
	files := f.files(t)
	config := string(files["config.yaml"])
	if !strings.Contains(config, "        basedOn: \"1.1.0\"\n") || !strings.Contains(config, "pins:") || string(files["rules/techs/go/errors.md"]) != forkedRule("Wrap errors.") {
		t.Fatalf("configuration:\n%s\nfork:\n%s", config, files["rules/techs/go/errors.md"])
	}
	if _, row := preview(nil); row != nil {
		t.Fatalf("the update still lists the replaced fork: %+v", row)
	}
}

// TestEveryRecordReader_RefusesARecordChangedOutsideSync: sync, build, check, update, and a fork all refuse team's
// record once its checksum is gone, and a fork of a rule the edited record left out writes nothing, even with local
// group metadata that would let it proceed without the record.
func TestEveryRecordReader_RefusesARecordChangedOutsideSync(t *testing.T) {
	f := newForkFixture(t, "")
	ctx := context.Background()
	root, err := openProject(ctx, f.options, false)
	if err != nil {
		t.Fatal(err)
	}
	writeFixture(t, root, "rules/techs/go/_group.yaml", projectMetadata)
	root.Close()
	editRecord(t, f.options, func(record map[string]any) { delete(record["rules"].(map[string]any), "techs/go/errors") })
	before := projectTree(t, f.options)
	_, forkErr := f.fork(t, "techs/go/errors", "team@1.0.0", "Ours.")
	_, syncErr := Sync(ctx, f.options, f.git)
	_, updateErr := PlanUpdate(ctx, f.options, f.git, nil, nil)
	for name, err := range map[string]error{"fork": forkErr, "sync": syncErr, "update": updateErr} {
		t.Run(name, func(t *testing.T) { requireRefusedRecord(t, err) })
	}
	if after := projectTree(t, f.options); after.Digest() != before.Digest() {
		t.Fatal("a refusal changed the project")
	}
	requireEditedRecord(t, f.options)
}
