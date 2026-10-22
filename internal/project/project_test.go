// Verify complete offline project behavior against real disposable project trees.

package project

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/errs"
	"github.com/fabricahq/trailguide/internal/filetxn"
	"github.com/fabricahq/trailguide/internal/imports"
	"github.com/fabricahq/trailguide/internal/library"
	"github.com/fabricahq/trailguide/internal/rules"
)

const projectRule = "---\ntitle: Return errors\nimpact: HIGH\nimpactDescription: Preserve failures.\nwhenToRead: When calling functions.\n---\n# Return errors\n\nReturn errors to the caller.\n"
const projectMetadata = `{"name":"Go","description":"Go guidance.","whenToRead":"When editing Go."}`

// localProject supplies a project with local-only authored Go guidance.
func localProject(t *testing.T) (*os.Root, Options) {
	t.Helper()
	root := openTestProject(t)
	if _, err := Initialize(context.Background(), Options{Directory: filepath.Dir(root.Name())}); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, root, "config.yaml", `{"schemaVersion":2,"sources":{}}`)
	writeFixture(t, root, "rules/techs/go/_group.yaml", projectMetadata)
	writeFixture(t, root, "rules/techs/go/errors.md", projectRule)
	return root, Options{Directory: filepath.Dir(root.Name()), ToolVersion: "1.2.3"}
}

// importedProject seeds byte-verified native snapshots without invoking Git or a network process.
func importedProject(t *testing.T) (*os.Root, Options) {
	t.Helper()
	root, options := localProject(t)
	if err := root.RemoveAll("rules"); err != nil {
		t.Fatal(err)
	}
	configJSON := []byte(`{"schemaVersion":2,"sources":{"team":{"repository":"https://github.com/acme/rules","ref":"v1.0.0","rules":{"include":["techs/go/*"]}}}}`)
	writeFixture(t, root, "config.yaml", string(configJSON))
	config, err := rules.ParseConfiguration(configJSON)
	if err != nil {
		t.Fatal(err)
	}
	commit := strings.Repeat("a", 40)
	imported := snapshot{Repository: config.Sources[0].Repository, Ref: gitRef(t, "v1.0.0"), Commit: commit, Groups: []string{"techs/go"}, Selection: config.Sources[0].Groups, RuleSelection: []string{},
		Rules: map[string]library.ImportedRule{"techs/go/errors": {Commit: commit}},
		Files: map[string][]byte{"config.yaml": []byte(`{"schemaVersion":2,"sources":{},"library":{"schemaVersion":1}}`), "techs/go/_group.yaml": []byte(projectMetadata), "techs/go/errors.md": []byte(projectRule)}}
	vendor, lock, err := encodeSnapshots(config, map[string]snapshot{"team": imported}, sourceRecords{})
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range vendor {
		writeFixture(t, root, filepath.Join("vendor", name), string(data))
	}
	writeFixture(t, root, lockFile, string(lock))
	return root, options
}

// TestOfflineBuildCheckAndRepeat covers generated changes and no writes from both stale and clean checks.
func TestOfflineBuildCheckAndRepeat(t *testing.T) {
	for _, kind := range []string{"local", "imported"} {
		t.Run(kind, func(t *testing.T) {
			var root *os.Root
			var options Options
			if kind == "local" {
				root, options = localProject(t)
			} else {
				root, options = importedProject(t)
			}
			// An empty PATH proves no runtime dependency on Git, Node, Bun, or shell utilities.
			t.Setenv("PATH", t.TempDir())
			before, err := filetxn.ReadTree(context.Background(), root, ".")
			if err != nil {
				t.Fatal(err)
			}
			stale, err := Check(context.Background(), options)
			if err != nil || stale.Current() {
				t.Fatalf("stale check: %+v %v", stale, err)
			}
			after, err := filetxn.ReadTree(context.Background(), root, ".")
			if err != nil || before.Digest() != after.Digest() {
				t.Fatal("check wrote files")
			}
			changes, err := Build(context.Background(), options)
			if err != nil {
				t.Fatal(err)
			}
			// Init wrote AGENTS.md's section without a version, so check reports it last, and build updates it.
			last := len(stale.Problems) - 1
			if len(changes.Added) != last || stale.Problems[last] != (Problem{Kind: OutdatedAgents, Path: "../AGENTS.md", Repair: Rebuild}) || changes.Agents == nil || *changes.Agents != (AgentsChange{Path: "AGENTS.md", Action: "updated"}) {
				t.Fatalf("build/check differ: %+v %+v", changes, stale)
			}
			for i, problem := range stale.Problems[:last] {
				if problem.Kind != MissingFile || problem.Path != "generated/"+changes.Added[i] || problem.Repair != Rebuild {
					t.Fatalf("unexpected initial problem: %+v", problem)
				}
			}
			tree, err := filetxn.ReadTree(context.Background(), root, "generated")
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(tree.Files["groups/techs/go.md"]), "Return errors to the caller.") {
				t.Fatal("short group not delivered in full")
			}
			again, err := Build(context.Background(), options)
			if err != nil || len(again.Added)+len(again.Changed)+len(again.Removed) != 0 || again.Agents != nil {
				t.Fatalf("not idempotent: %+v %v", again, err)
			}
			clean, err := Check(context.Background(), options)
			if err != nil || !clean.Current() {
				t.Fatalf("clean check: %+v %v", clean, err)
			}
			writeFixture(t, root, "generated/RULES.md", "stale")
			writeFixture(t, root, "generated/extra.md", "extra")
			if err := root.Remove("generated/groups/techs/go.md"); err != nil {
				t.Fatal(err)
			}
			before, err = filetxn.ReadTree(context.Background(), root, ".")
			if err != nil {
				t.Fatal(err)
			}
			differences, err := Check(context.Background(), options)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(differences.Problems, []Problem{{Kind: MissingFile, Path: "generated/groups/techs/go.md", Repair: Rebuild}, {Kind: StaleContents, Path: "generated/RULES.md", Repair: Rebuild}, {Kind: UnexpectedFile, Path: "generated/extra.md", Repair: Rebuild}}) {
				t.Fatalf("diffs: %+v", differences)
			}
			after, err = filetxn.ReadTree(context.Background(), root, ".")
			if err != nil || before.Digest() != after.Digest() {
				t.Fatal("stale check changed tree")
			}
		})
	}
}

// TestOfflineFailuresPreserveOutput checks invalid rules/vendor content, pending recovery, and cancellation.
func TestOfflineFailuresPreserveOutput(t *testing.T) {
	for _, kind := range []string{"local", "vendor", "recovery", "cancel"} {
		t.Run(kind, func(t *testing.T) {
			root, options := importedProject(t)
			writeFixture(t, root, "generated/keep", "existing output")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch kind {
			case "local":
				writeFixture(t, root, "rules/techs/go/bad.md", "invalid rule")
			case "vendor":
				writeFixture(t, root, "vendor/team/rules/techs/go/errors.md", "modified imported rule")
			case "recovery":
				writeFixture(t, root, ".trailguide-transaction"+"/journal.json", `{"formatVersion":99}`)
			case "cancel":
				cancel()
			}
			before, err := filetxn.ReadTree(context.Background(), root, ".")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := Check(ctx, options); err == nil {
				t.Fatal("check accepted invalid project")
			}
			after, err := filetxn.ReadTree(context.Background(), root, ".")
			if err != nil || before.Digest() != after.Digest() {
				t.Fatal("failed check wrote files")
			}
			if _, err := Build(ctx, options); err == nil {
				t.Fatal("build accepted invalid project")
			} else if kind == "cancel" && !errors.Is(err, context.Canceled) {
				t.Fatalf("lost cancellation: %v", err)
			}
			if b, _ := root.ReadFile("generated/keep"); string(b) != "existing output" {
				t.Fatal("lost existing output")
			}
		})
	}
}

// TestOfflineRejectsSemanticCorruptionBeyondDigests validates catalog content even with self-consistent hashes.
func TestOfflineRejectsSemanticCorruptionBeyondDigests(t *testing.T) {
	root, options := importedProject(t)
	writeFixture(t, root, "vendor/team/rules/techs/go/errors.md", "invalid but rehashed")
	rewriteEntry(t, root, "team", func(record *sourceRecord) {
		record.Files["techs/go/errors.md"] = digest([]byte("invalid but rehashed"))
	})
	if _, err := Check(context.Background(), options); err == nil {
		t.Fatal("accepted invalid content with valid digest")
	}
}

// TestOfflineRejectsRuleFilesTheRecordDoesntDescribe detects tampering that keeps every digest consistent: a rule
// file with no version record.
func TestOfflineRejectsRuleFilesTheRecordDoesntDescribe(t *testing.T) {
	for name, move := range map[string][2]string{
		"unrecorded rule": {"", "techs/go/extra.md"},
	} {
		t.Run(name, func(t *testing.T) {
			root, options := importedProject(t)
			// The tampering also recomputes the entry's integrity, so only the catalog can catch it.
			rewriteEntry(t, root, "team", func(record *sourceRecord) {
				if move[0] != "" {
					delete(record.Files, move[0])
					if err := root.Remove("vendor/team/rules/" + move[0]); err != nil {
						t.Fatal(err)
					}
				}
				record.Files[move[1]] = digest([]byte(projectRule))
			})
			writeFixture(t, root, "vendor/team/rules/"+move[1], projectRule)
			_, err := Check(context.Background(), options)
			projectCode(t, err, "invalid-snapshot")
		})
	}
}

// TestOfflineRejectsAnExclusionSyncDidntCheck fails build and check for an exclusion naming a rule the snapshot
// doesn't import, until sync checks it.
func TestOfflineRejectsAnExclusionSyncDidntCheck(t *testing.T) {
	root, options := importedProject(t)
	writeFixture(t, root, "config.yaml", `{"schemaVersion":2,"sources":{"team":{"repository":"https://github.com/acme/rules","ref":"v1.0.0","rules":{"include":["techs/go/*"],"exclude":[{"id":"techs/go/erorrs","reason":"Typo."}]}}}}`)
	if _, err := Check(context.Background(), options); err == nil || !strings.Contains(err.Error(), "run trailguide project sync") {
		t.Fatalf("check accepted an unchecked exclusion: %v", err)
	}
	if _, err := Build(context.Background(), options); err == nil {
		t.Fatal("build accepted an unchecked exclusion")
	}
}

// sharedAssetProject supplies a project whose authored rule in rules/ links to a shared asset in assets/.
func sharedAssetProject(t *testing.T) (*os.Root, Options) {
	t.Helper()
	root, options := localProject(t)
	writeFixture(t, root, "rules/techs/go/errors.md", projectRule+"\nSee [the retry lifecycle](../../../assets/retry.svg).\n")
	writeFixture(t, root, "assets/retry.svg", "<svg/>")
	return root, options
}

// TestBuild_LinksAuthoredRulesInRulesToSharedAssetsInAssets links the generated copy of an authored rule in rules/
// to its source file and to the shared asset in assets/ that it links to.
func TestBuild_LinksAuthoredRulesInRulesToSharedAssetsInAssets(t *testing.T) {
	root, options := sharedAssetProject(t)
	if _, err := Build(context.Background(), options); err != nil {
		t.Fatal(err)
	}
	generated, err := root.ReadFile("generated/rules/local/techs/go/errors.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"[the retry lifecycle](../../../../../assets/retry.svg)", "**Rule source:** [Original rule](../../../../../rules/techs/go/errors.md)"} {
		if !strings.Contains(string(generated), want) {
			t.Errorf("missing %q in %s", want, generated)
		}
	}
}

// TestBuild_RefusesMisplacedSharedAssets fails, keeping the generated output, when the shared asset an authored rule
// links to is missing from assets/, or when a file sits in rules/assets/, where shared assets don't belong.
func TestBuild_RefusesMisplacedSharedAssets(t *testing.T) {
	for _, test := range []struct{ name, location, problem string }{
		{"missing asset", "local:techs/go/errors", "missing asset link destination: ../../../assets/retry.svg"},
		{"asset in rules", "rules/assets/retry.svg", "rules/ holds groups under techs/ and practices/, so shared assets belong in .trailguide/assets/; move this file there"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root, options := sharedAssetProject(t)
			if _, err := Build(context.Background(), options); err != nil {
				t.Fatal(err)
			}
			if test.name == "missing asset" {
				if err := root.Remove("assets/retry.svg"); err != nil {
					t.Fatal(err)
				}
			} else {
				writeFixture(t, root, "rules/assets/retry.svg", "<svg/>")
			}
			before, err := filetxn.ReadTree(context.Background(), root, "generated")
			if err != nil {
				t.Fatal(err)
			}
			_, err = Build(context.Background(), options)
			var validation errs.ValidationError
			if !errors.As(err, &validation) || validation.ValidationLocation() != test.location || validation.ValidationProblem() != test.problem {
				t.Fatal(err)
			}
			after, err := filetxn.ReadTree(context.Background(), root, "generated")
			if err != nil || after.Digest() != before.Digest() {
				t.Fatal("changed generated output", err)
			}
		})
	}
}

// TestProjectInputRecheckRejectsLaterEdits verifies the same guard Build supplies to Apply.
func TestProjectInputRecheckRejectsLaterEdits(t *testing.T) {
	root, _ := localProject(t)
	state, err := readProject(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	writeFixture(t, root, "rules/techs/go/errors.md", projectRule+"Later edit.\n")
	projectCode(t, requireUnchanged(context.Background(), root, state), "concurrent-change")
}

// TestProjectInputRecheckRejectsALaterLocalFolder refuses a local/ folder that appears after the project opened,
// which the rechecked trees don't cover.
func TestProjectInputRecheckRejectsALaterLocalFolder(t *testing.T) {
	root, _ := localProject(t)
	state, err := readProject(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	writeFixture(t, root, "local/techs/go/errors.md", projectRule)
	var validation *ValidationError
	if err := requireUnchanged(context.Background(), root, state); !errors.As(err, &validation) || validation.Location != ".trailguide/local/" {
		t.Fatalf("recheck error = %v, want the local/ refusal", err)
	}
}

// TestOfflineRendersOnlyVerifiedSnapshotBytes loads the bytes whose digests were verified, so an edit made after
// verification can't reach generated output, and the final recheck reports it.
func TestOfflineRendersOnlyVerifiedSnapshotBytes(t *testing.T) {
	for _, file := range []string{"techs/go/errors.md", "techs/go/_group.yaml"} {
		t.Run(file, func(t *testing.T) {
			root, options := importedProject(t)
			state, err := readProject(context.Background(), root)
			if err != nil {
				t.Fatal(err)
			}
			changed := strings.ReplaceAll(projectRule, "Return errors to the caller.", "Unverified text.")
			if strings.HasSuffix(file, ".yaml") {
				changed = strings.ReplaceAll(projectMetadata, "Go guidance.", "Unverified text.")
			}
			writeFixture(t, root, "vendor/team/"+file, changed)
			output, err := prepareProject(context.Background(), state, options)
			if err != nil {
				t.Fatal(err)
			}
			for name, data := range output.Files {
				if strings.Contains(string(data), "Unverified text.") {
					t.Fatalf("%s rendered unverified bytes", name)
				}
			}
			projectCode(t, requireUnchanged(context.Background(), root, state), "concurrent-change")
		})
	}
}

// withNewestRef returns config, a configuration in JSON or YAML, with ref: release/<number> for the newest library
// release of repository, as trailguide project add source records it, added after the repository that placeholder
// stands for, unless that source already has a ref or the repository has no library release.
func withNewestRef(t *testing.T, config, placeholder, repository string, git imports.Options) string {
	t.Helper()
	inJSON := `"repository":` + placeholder
	inYAML := "repository: " + placeholder + "\n"
	at := strings.Index(config, inYAML)
	needed := strings.Contains(config, inJSON) && !strings.Contains(sourceObject(config, inJSON), `"ref":`) || at >= 0 && !strings.HasPrefix(strings.TrimLeft(config[at+len(inYAML):], " "), "ref:")
	if !needed {
		return config
	}
	newest, err := imports.NewestRelease(context.Background(), repository, git)
	if err != nil {
		t.Fatal(err)
	}
	if newest == 0 {
		return config
	}
	ref := fmt.Sprintf("release/%d", newest)
	if strings.Contains(config, inJSON) {
		return strings.ReplaceAll(config, inJSON, inJSON+`,"ref":"`+ref+`"`)
	}
	line := strings.LastIndex(config[:at], "\n") + 1
	indent := config[line:at]
	return config[:at+len(inYAML)] + indent + "ref: " + ref + "\n" + config[at+len(inYAML):]
}

// updateProject applies an update of every source with no decisions, as a project whose ref was edited to name a
// newer library release takes it.
func updateProject(ctx context.Context, options Options, git imports.Options) (FileChanges, error) {
	plan, err := PlanUpdate(ctx, options, git, nil, nil)
	if err != nil {
		return FileChanges{}, err
	}
	result, err := plan.Apply(ctx, nil)
	return result.FileChanges, err
}

// sourceObject returns the rest of the JSON object in config that contains key, from key to the brace that closes
// the object.
func sourceObject(config, key string) string {
	start := strings.Index(config, key)
	if start < 0 {
		return ""
	}
	depth := 0
	for i := start; i < len(config); i++ {
		switch config[i] {
		case '{', '[':
			depth++
		case '}', ']':
			if depth == 0 {
				return config[start:i]
			}
			depth--
		}
	}
	return config[start:]
}

// requireRefRequired fails unless err is the project's ref-required failure and mentions text.
func requireRefRequired(t *testing.T, err error, text string) {
	t.Helper()
	var failure *filetxn.Error
	if !errors.As(err, &failure) || failure.Code != "ref-required" || !strings.Contains(err.Error(), text) {
		t.Fatalf("got %v; want ref-required mentioning %q", err, text)
	}
}
