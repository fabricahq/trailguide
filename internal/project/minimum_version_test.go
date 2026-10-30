// Verify that init records minTrailguideVersion, update raises it, and every command refuses an older Trail Guide.

package project

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/filetxn"
	"github.com/fabricahq/trailguide/internal/imports"
)

// TestInitialize_RecordsTheReleaseVersion writes a release build's version into a new configuration, and nothing
// for a development build.
func TestInitialize_RecordsTheReleaseVersion(t *testing.T) {
	for version, want := range map[string]string{
		"1.2.3":             "schemaVersion: 2\nminTrailguideVersion: 1.2.3\nskillTargets: [claude, agents]\nsources: {}\n",
		"0.0.0-development": "schemaVersion: 2\nskillTargets: [claude, agents]\nsources: {}\n",
	} {
		t.Run(version, func(t *testing.T) {
			options := Options{Directory: filepath.Dir(openTestProject(t).Name()), ToolVersion: version}
			if _, err := Initialize(context.Background(), options); err != nil {
				t.Fatal(err)
			}
			if got := readRepositoryFile(t, options, ".trailguide/config.yaml"); got != want {
				t.Fatalf("config.yaml:\n%s\nwant:\n%s", got, want)
			}
		})
	}
}

// TestUpdate_RaisesTheMinimumVersion raises a missing or older minTrailguideVersion to the running release, keeps
// one that names it, and leaves the field alone for a development build, and its preview says which.
func TestUpdate_RaisesTheMinimumVersion(t *testing.T) {
	for name, test := range map[string]struct {
		version, minimum, want, sets string
	}{
		"missing":     {"1.2.3", "", "1.2.3", "1.2.3"},
		"older":       {"1.2.3", "1.0.9", "1.2.3", "1.2.3"},
		"current":     {"1.2.3", "1.2.3", "1.2.3", ""},
		"development": {"0.0.0-development", "1.0.9", "1.0.9", ""},
	} {
		t.Run(name, func(t *testing.T) {
			options, git, _ := syncedProject(t)
			options.ToolVersion = test.version
			root, err := openProject(context.Background(), options, false)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			if test.minimum != "" {
				config := readRepositoryFile(t, options, ".trailguide/config.yaml")
				writeFixture(t, root, configurationFile, strings.Replace(config, `{"schemaVersion":2,`, `{"schemaVersion":2,"minTrailguideVersion":"`+test.minimum+`",`, 1))
			}
			plan, err := PlanUpdate(context.Background(), options, git, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			preview, err := plan.Preview(context.Background(), nil)
			if err != nil || preview.SetsMinimumVersion != test.sets {
				t.Fatalf("preview sets minTrailguideVersion to %q, want %q: %v", preview.SetsMinimumVersion, test.sets, err)
			}
			applied, err := plan.Apply(context.Background(), nil)
			if err != nil || applied.SetsMinimumVersion != test.sets {
				t.Fatalf("update set minTrailguideVersion to %q, want %q: %v", applied.SetsMinimumVersion, test.sets, err)
			}
			_, config, err := configuration(context.Background(), root)
			if err != nil || config.MinimumVersion == nil || config.MinimumVersion.String() != test.want {
				t.Fatalf("minTrailguideVersion %v, want %s: %v", config.MinimumVersion, test.want, err)
			}
			requireCurrent(t, options)
		})
	}
}

// TestCommands_RefuseAnOlderTrailguide fails every command with upgrade-required, writing nothing, when the
// configuration requires a newer release, even when it has fields and values this version doesn't know. A
// development build skips the check and reports the unreadable value instead.
func TestCommands_RefuseAnOlderTrailguide(t *testing.T) {
	ctx := context.Background()
	root, options := localProject(t)
	// A date is a value this version can't read, so the configuration fails ParseConfigurationYAML before its fields do.
	writeFixture(t, root, configurationFile, "schemaVersion: 2\nminTrailguideVersion: 1.10.0\nfutureSetting: 2026-01-01\nsources: {}\n")
	before := projectTree(t, Options{Directory: options.Directory})
	commands := map[string]func(Options) error{
		"init":       func(o Options) error { _, err := Initialize(ctx, o); return err },
		"build":      func(o Options) error { _, err := Build(ctx, o); return err },
		"check":      func(o Options) error { _, err := Check(ctx, o); return err },
		"sync":       func(o Options) error { _, err := Sync(ctx, o, imports.Options{}); return err },
		"update":     func(o Options) error { _, err := PlanUpdate(ctx, o, imports.Options{}, nil, nil); return err },
		"list rules": func(o Options) error { _, err := ListRules(ctx, o); return err },
		"add group":  func(o Options) error { _, err := PlanLocalGroup(ctx, "practices/testing", o); return err },
	}
	for name, run := range commands {
		t.Run(name, func(t *testing.T) {
			projectCode(t, run(options), "upgrade-required")
			development := options
			development.ToolVersion = "0.0.0-development"
			if err := run(development); err == nil || strings.Contains(err.Error(), "upgrade") || !strings.Contains(err.Error(), "futureSetting") {
				t.Fatalf("development build: %v", err)
			}
		})
	}
	if after := projectTree(t, Options{Directory: options.Directory}); after.Digest() != before.Digest() {
		t.Fatal("a refused command wrote files")
	}
	writeFixture(t, root, configurationFile, "schemaVersion: 2\nminTrailguideVersion: 1.2.3\nsources: {}\n")
	if _, err := Build(ctx, options); err != nil {
		t.Fatalf("a build of the minimum version: %v", err)
	}
}

// TestBuild_ChecksTheConfigurationThatRecoveryRestores refuses a build whose writer rolls back an interrupted command
// to a configuration that requires a newer release, though the configuration it found first didn't, and generates
// nothing.
func TestBuild_ChecksTheConfigurationThatRecoveryRestores(t *testing.T) {
	ctx := context.Background()
	root, options := localProject(t)
	writeFixture(t, root, configurationFile, "schemaVersion: 2\nminTrailguideVersion: 1.10.0\nsources: {}\n")
	writeFixture(t, root, string(filetxn.Lock), `{"lockVersion": 1, "sources": {}}`)
	// The interrupted command replaced config.yaml with one that requires no release, then stopped before lock.json.
	blocker := filepath.Join(".trailguide-transaction", "old-"+string(filetxn.Lock))
	output := map[filetxn.Target]map[string][]byte{
		filetxn.Config: {configurationFile: []byte("schemaVersion: 2\nsources: {}\n")},
		filetxn.Lock:   {string(filetxn.Lock): []byte(`{"lockVersion": 1, "notice": "", "sources": {}}`)},
	}
	err := filetxn.WithWriter(ctx, root, func(w *filetxn.Writer) error {
		return w.Apply(output, func() error { return root.MkdirAll(filepath.Join(blocker, "blocker"), 0700) })
	})
	projectCode(t, err, "recovery-required")
	if err := root.RemoveAll(blocker); err != nil {
		t.Fatal(err)
	}
	if config := readRepositoryFile(t, options, ".trailguide/config.yaml"); strings.Contains(config, "minTrailguideVersion") {
		t.Fatalf("the interrupted command didn't replace config.yaml:\n%s", config)
	}
	_, err = Build(ctx, options)
	projectCode(t, err, "upgrade-required")
	if exists(t, options, ".trailguide/_managed/generated/RULES.md") {
		t.Fatal("a refused build generated output")
	}
}

// TestUpdate_AMinimumRaiseAloneIsAnUpdate previews and applies raising minTrailguideVersion in a project with no
// sources, which has nothing else to move, so a terminal asks before other people's older releases refuse it.
func TestUpdate_AMinimumRaiseAloneIsAnUpdate(t *testing.T) {
	ctx := context.Background()
	root, options := localProject(t)
	writeFixture(t, root, configurationFile, "schemaVersion: 2\nminTrailguideVersion: 1.0.0\nsources: {}\n")
	plan, err := PlanUpdate(ctx, options, imports.Options{}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := plan.Preview(ctx, nil)
	if err != nil || !preview.Moves() || preview.SetsMinimumVersion != "1.2.3" {
		t.Fatalf("preview %+v: %v", preview, err)
	}
	if config := readRepositoryFile(t, options, ".trailguide/config.yaml"); !strings.Contains(config, "minTrailguideVersion: 1.0.0") {
		t.Fatalf("the preview wrote config.yaml:\n%s", config)
	}
	applied, err := plan.Apply(ctx, nil)
	if err != nil || !applied.Applied || applied.SetsMinimumVersion != "1.2.3" || !slices.Contains(applied.Changed, configurationFile) {
		t.Fatalf("applied %+v: %v", applied, err)
	}
	if config := readRepositoryFile(t, options, ".trailguide/config.yaml"); !strings.Contains(config, "minTrailguideVersion: 1.2.3") {
		t.Fatalf("config.yaml after the update:\n%s", config)
	}
}

// TestUpdate_WarnsWhenTheCheckWorkflowPinsAnOlderRelease, which a library's raised minimum then refuses, and not
// when the workflow pins the raised release or installs the configuration's minimum.
func TestUpdate_WarnsWhenTheCheckWorkflowPinsAnOlderRelease(t *testing.T) {
	ctx := context.Background()
	for workflow, warns := range map[string]bool{
		"run: |\n          version=1.0.0\n": true,
		"run: |\n          version=1.2.3\n": false,
		"name: Custom\n":                    false,
	} {
		root, options := localProject(t)
		writeFixture(t, root, configurationFile, "schemaVersion: 2\nminTrailguideVersion: 1.0.0\nsources: {}\n")
		path := filepath.Join(options.Directory, ".github", "workflows", "trailguide.yml")
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(workflow), 0600); err != nil {
			t.Fatal(err)
		}
		plan, err := PlanUpdate(ctx, options, imports.Options{}, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		want := ".github/workflows/trailguide.yml installs trailguide 1.0.0, which refuses this repository once minTrailguideVersion is 1.2.3, so its next check fails; change its version=1.0.0 line to version=1.2.3."
		preview, err := plan.Preview(ctx, nil)
		if err != nil || slices.Contains(preview.Warnings, want) != warns {
			t.Fatalf("%q: preview warnings %q: %v", workflow, preview.Warnings, err)
		}
		applied, err := plan.Apply(ctx, nil)
		if err != nil || slices.Contains(applied.Warnings, want) != warns {
			t.Fatalf("%q: applied warnings %q: %v", workflow, applied.Warnings, err)
		}
	}
}
