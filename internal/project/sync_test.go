// Exercise complete sync against real Git sources and disposable project trees.

package project

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/errs"
	"github.com/fabricahq/trailguide/internal/filetxn"
	"github.com/fabricahq/trailguide/internal/imports"
	"github.com/fabricahq/trailguide/internal/test/gitfixture"
)

// firstRelease is the release record of the sync fixture's first library release.
const firstRelease = "formatVersion: 1\nrelease: 1\nrules:\n  techs/go/errors: 1.0.0\nchanges:\n  techs/go/errors: {change: new, summaries: [Add the rule.]}\n"

// syncProject initializes a project and a library, with exact binary and license content, whose first library
// release publishes techs/go/errors at 1.0.0. The project selects techs/go.
func syncProject(t *testing.T) (*gitfixture.Fixture, Options, imports.Options) {
	t.Helper()
	f, err := gitfixture.New(context.Background(), gitfixture.Library(map[string][]byte{
		"config.yaml":                     []byte(`{"schemaVersion":2,"sources":{},"library":{"license":{"file":"LICENSE","notices":[]}}}`),
		"LICENSE":                         []byte("Original terms\r\n"),
		"techs/go/_group.yaml":            []byte(projectMetadata),
		"techs/go/errors.md":              []byte(projectRule),
		"techs/go/assets/errors/data.bin": {0, 255, 128},
	}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := f.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := f.Release(context.Background(), 1, firstRelease); err != nil {
		t.Fatal(err)
	}
	root := openTestProject(t)
	if _, err := Initialize(context.Background(), Options{Directory: filepath.Dir(root.Name())}); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{"schemaVersion": 2, "sources": map[string]any{"team": map[string]any{"repository": f.Repository, "ref": "release/1", "rules": map[string]any{"include": []string{"techs/go/*"}}}}})
	writeFixture(t, root, "config.yaml", string(raw))
	return f, Options{Directory: filepath.Dir(root.Name()), ToolVersion: "1.2.3"}, imports.Options{GitPath: f.GitPath, Environment: f.Environment}
}

// TestSyncRoundTripAndRetirement preserves original bytes, produces clean offline output, and removes retired sources.
func TestSyncRoundTripAndRetirement(t *testing.T) {
	_, options, git := syncProject(t)
	ctx := context.Background()
	first, err := Sync(ctx, options, git)
	if err != nil || len(first.Added) == 0 {
		t.Fatalf("initial sync: %+v %v", first, err)
	}
	root, err := openProject(ctx, options, false)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	state, err := readProject(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	snapshots, err := decodeSnapshots(state.config, state.vendor.Files)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(snapshots["team"].Files["LICENSE"], []byte("Original terms\r\n")) || !bytes.Equal(snapshots["team"].Files["techs/go/assets/errors/data.bin"], []byte{0, 255, 128}) {
		t.Fatal("original snapshot bytes changed")
	}
	before, _ := filetxn.ReadTree(ctx, root, ".")
	again, err := Sync(ctx, options, git)
	if err != nil || len(again.Added)+len(again.Changed)+len(again.Removed) != 0 {
		t.Fatalf("repeat: %+v %v", again, err)
	}
	// This offline check cannot use Git or any language runtime through PATH.
	t.Setenv("PATH", t.TempDir())
	checked, err := Check(ctx, options)
	if err != nil || !checked.Current() {
		t.Fatalf("offline check: %+v %v", checked, err)
	}
	after, _ := filetxn.ReadTree(ctx, root, ".")
	if before.Digest() != after.Digest() {
		t.Fatal("repeat/check changed bytes")
	}
	writeFixture(t, root, configurationFile, `{"schemaVersion":2,"sources":{}}`)
	retired, err := Sync(ctx, options, imports.Options{GitPath: "/missing/git"})
	if err != nil || len(retired.Removed) == 0 {
		t.Fatalf("retire: %+v %v", retired, err)
	}
	vendor, err := filetxn.ReadTree(ctx, root, "vendor")
	if err != nil || len(vendor.Files) != 0 {
		t.Fatal("retired files retained", err)
	}
	index, err := root.ReadFile("generated/RULES.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(index), "No active rules are selected for this project.") || strings.Contains(string(index), "Open the relevant group indexes below") {
		t.Fatalf("retired source left misleading reading instructions: %s", index)
	}
}

// TestSyncFailurePreservesManagedTrees covers import failures, invalid local rules, and cancellation after a valid sync.
func TestSyncFailurePreservesManagedTrees(t *testing.T) {
	for _, scenario := range []string{"missing-version", "invalid-local", "cancelled"} {
		t.Run(scenario, func(t *testing.T) {
			_, options, git := syncProject(t)
			ctx := context.Background()
			if _, err := Sync(ctx, options, git); err != nil {
				t.Fatal(err)
			}
			root, err := openProject(ctx, options, false)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			if scenario == "missing-version" {
				raw, err := root.ReadFile(configurationFile)
				if err != nil {
					t.Fatal(err)
				}
				raw = bytes.Replace(raw, []byte(`"include"`), []byte(`"pins":[{"id":"techs/go/errors","version":"9.0.0","reason":"Typo."}],"include"`), 1)
				writeFixture(t, root, configurationFile, string(raw))
			} else if scenario == "invalid-local" {
				writeFixture(t, root, "rules/techs/go/bad.md", "invalid")
			}
			before, err := filetxn.ReadTree(ctx, root, ".")
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "cancelled" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			changes, err := Sync(ctx, options, git)
			if err == nil || !reflect.DeepEqual(changes, FileChanges{}) {
				t.Fatal("failure returned result", err)
			}
			if scenario == "cancelled" && !errors.Is(err, context.Canceled) {
				t.Fatal("lost cancellation", err)
			}
			var validation errs.ValidationError
			if scenario == "invalid-local" && !errors.As(err, &validation) {
				t.Fatal("lost validation error", err)
			}
			after, err := filetxn.ReadTree(context.Background(), root, ".")
			if err != nil || before.Digest() != after.Digest() {
				t.Fatal("failed sync changed project", err)
			}
		})
	}
}

// TestSyncAndUpdate_SayNoFilesWereWrittenWhenTheyFail: a cancelled sync or update says it was cancelled and wrote
// nothing, still reporting the cancellation, and a failed one adds that it wrote nothing to its reason, without
// naming internal steps.
func TestSyncAndUpdate_SayNoFilesWereWrittenWhenTheyFail(t *testing.T) {
	_, options, git := syncProject(t)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	_, syncErr := Sync(cancelled, options, git)
	_, planErr := PlanUpdate(cancelled, options, git, nil, nil)
	for name, err := range map[string]error{"sync": syncErr, "update": planErr} {
		if err == nil || err.Error() != "cancelled; no files were written" || !errors.Is(err, context.Canceled) {
			t.Errorf("cancelled %s: %v", name, err)
		}
	}
	git.GitPath = "/nonexistent/git"
	_, err := Sync(context.Background(), options, git)
	if err == nil || !strings.HasSuffix(err.Error(), " No files were written.") || strings.Contains(err.Error(), "sync project") || strings.Contains(err.Error(), "no libraries were returned") {
		t.Fatalf("failed sync: %v", err)
	}
}

// TestSync_SaysItRecoveredAnInterruptedCommandBeforeFailing, rather than that it wrote no files: sync first cleans
// up what an interrupted earlier command left, then fails on invalid configuration.
func TestSync_SaysItRecoveredAnInterruptedCommandBeforeFailing(t *testing.T) {
	_, options, git := syncProject(t)
	root, err := openProject(context.Background(), options, false)
	if err != nil {
		t.Fatal(err)
	}
	writeFixture(t, root, ".trailguide-transaction/staged", "left by an interrupted command")
	writeFixture(t, root, configurationFile, `{"schemaVersion":2,"sources":{},"unknown":true}`)
	root.Close()
	_, err = Sync(context.Background(), options, git)
	if err == nil || strings.Contains(err.Error(), "No files were written") || !strings.Contains(err.Error(), "recovered an interrupted earlier command") {
		t.Fatalf("got %v", err)
	}
}
