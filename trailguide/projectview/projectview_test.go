// Check the project view and its updates through the public API, from a configuration and lockfile as files.

package projectview_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/trailguide"
	"github.com/fabricahq/trailguide/trailguide/projectview"
)

// Commits of the fixture library's releases.
var (
	commitOne   = strings.Repeat("1", 40)
	commitTwo   = strings.Repeat("2", 40)
	commitTools = strings.Repeat("3", 40)
)

// fixtureConfig takes go rules from fabrica, pinning one, excluding one, and replacing one, and skills from tools.
const fixtureConfig = `schemaVersion: 2
sources:
  fabrica:
    repository: https://github.com/fabricahq/public-library.git
    ref: release/2
    rules:
      include:
        - techs/go/*
      pins:
        - id: techs/go/naming
          version: "1.0.0"
          reason: Waiting on the rename.
      exclude:
        - id: techs/go/logging
          reason: We log through our platform.
        - id: techs/go/errors
          reason: Our services wrap errors.
          replacedBy: techs/go/wrap-errors
          basedOn: "1.0.0"
  tools:
    repository: https://github.com/acme/tools.git
    ref: main
    skills:
      include:
        - "*"
`

// fixtureLock returns the lockfile sync would write for fixtureConfig: fabrica on release 2, whose rules come from
// releases 1 and 2, with one rule retired and one with unreleased changes.
func fixtureLock(t *testing.T) []byte {
	t.Helper()
	one, two := 1, 2
	v1, v11, v2 := trailguide.RuleVersion{Major: 1}, trailguide.RuleVersion{Major: 1, Minor: 1}, trailguide.RuleVersion{Major: 2}
	lock := trailguide.Lock{Sources: map[string]trailguide.LockedSource{
		"fabrica": {
			Kind: trailguide.KindLibrary, Repository: "https://github.com/fabricahq/public-library.git", Ref: "release/2", Release: 2, Commit: commitTwo,
			ReleaseCommits: map[int]string{1: commitOne, 2: commitTwo},
			Selectors:      trailguide.LockSelectors{Groups: json.RawMessage(`["techs/go"]`), Rules: []string{}, Skills: []string{}},
			Groups:         []string{"techs/go"},
			Rules: map[string]trailguide.LockedRule{
				"techs/go/errors":  {Version: &v11, Release: &two},
				"techs/go/logging": {Version: &v1, Release: &one},
				"techs/go/naming":  {Version: &v1, Release: &one},
				"techs/go/tests":   {Version: &v2, Release: &two},
				"techs/go/draft":   {},
			},
			RetiredRules:    []string{"techs/go/naming"},
			Skills:          map[string]trailguide.LockedSkill{},
			AvailableSkills: map[string]trailguide.LockedAvailableSkill{},
			Files:           map[string]string{"config.yaml": strings.Repeat("a", 64)},
			Executable:      []string{},
			Curated:         map[string]trailguide.LockedCurated{},
		},
		"tools": {
			Kind: trailguide.KindNonLibrary, Repository: "https://github.com/acme/tools.git", Ref: "main", Commit: commitTools,
			Selectors:       trailguide.LockSelectors{Skills: []string{"*"}},
			Skills:          map[string]trailguide.LockedSkill{"tools/plan": {Included: true, UsedBy: []string{}}},
			AvailableSkills: map[string]trailguide.LockedAvailableSkill{"tools/plan": {}},
			Files:           map[string]string{"tools/plan/SKILL.md": strings.Repeat("d", 64)},
			Executable:      []string{},
		},
	}}
	data, err := lock.Encode()
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func version(text string) *trailguide.RuleVersion {
	v, err := trailguide.ParseRuleVersion(text, "version")
	if err != nil {
		panic(err)
	}
	return &v
}

func release(n int) *int { return &n }

func TestRead(t *testing.T) {
	view, err := projectview.Read([]byte(fixtureConfig), fixtureLock(t))
	if err != nil {
		t.Fatal(err)
	}
	wantSources := []projectview.Source{
		{Name: "fabrica", Kind: trailguide.KindLibrary, Repository: "https://github.com/fabricahq/public-library.git", Ref: "release/2", Commit: commitTwo, Release: 2, Groups: []string{"techs/go"}},
		{Name: "tools", Kind: trailguide.KindNonLibrary, Repository: "https://github.com/acme/tools.git", Ref: "main", Commit: commitTools, Groups: []string{}},
	}
	if !reflect.DeepEqual(view.Sources, wantSources) {
		t.Errorf("sources = %+v\nwant %+v", view.Sources, wantSources)
	}
	wantRules := []projectview.Rule{
		{ID: "fabrica:techs/go/draft", Source: "fabrica", Path: "techs/go/draft", Commit: commitTwo, Active: true},
		{ID: "fabrica:techs/go/errors", Source: "fabrica", Path: "techs/go/errors", Version: version("1.1.0"), Release: release(2), Commit: commitTwo,
			Exclusion: &projectview.Exclusion{Reason: "Our services wrap errors."}, ReplacedBy: "local:techs/go/wrap-errors"},
		{ID: "fabrica:techs/go/logging", Source: "fabrica", Path: "techs/go/logging", Version: version("1.0.0"), Release: release(1), Commit: commitOne,
			Exclusion: &projectview.Exclusion{Reason: "We log through our platform."}},
		{ID: "fabrica:techs/go/naming", Source: "fabrica", Path: "techs/go/naming", Version: version("1.0.0"), Release: release(1), Commit: commitOne, Active: true,
			Pin: &projectview.Pin{Version: *version("1.0.0"), Reason: "Waiting on the rename."}, Retired: true},
		{ID: "fabrica:techs/go/tests", Source: "fabrica", Path: "techs/go/tests", Version: version("2.0.0"), Release: release(2), Commit: commitTwo, Active: true},
		{ID: "local:techs/go/wrap-errors", Path: "techs/go/wrap-errors", Active: true,
			Replaces: &projectview.Replacement{ID: "fabrica:techs/go/errors", BasedOn: version("1.0.0")}},
	}
	if !reflect.DeepEqual(view.Rules, wantRules) {
		got, _ := json.MarshalIndent(view.Rules, "", "  ")
		t.Errorf("rules =\n%s", got)
	}
}

// TestReadSelectorExclusion checks that an exclusion by selector excludes every rule it matches.
func TestReadSelectorExclusion(t *testing.T) {
	config := strings.Replace(fixtureConfig, "        - id: techs/go/logging\n", "        - id: techs/go/*\n", 1)
	config = strings.Replace(config, "      pins:\n        - id: techs/go/naming\n          version: \"1.0.0\"\n          reason: Waiting on the rename.\n", "", 1)
	view, err := projectview.Read([]byte(config), fixtureLock(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, rule := range view.Rules {
		if rule.Source == "fabrica" && (rule.Active || rule.Exclusion == nil) {
			t.Errorf("%s isn't excluded: %+v", rule.ID, rule)
		}
	}
}

func TestReadRefusesInvalidInput(t *testing.T) {
	lock := fixtureLock(t)
	tests := map[string]struct {
		config, lock []byte
		location     string
	}{
		"an invalid configuration":    {[]byte("schemaVersion: 2\nsources: []\n"), lock, "sources"},
		"a source the lockfile lacks": {[]byte(strings.Replace(fixtureConfig, "  tools:", "  other:\n    repository: https://github.com/acme/other.git\n    ref: main\n    skills:\n      include:\n        - \"*\"\n  tools:", 1)), lock, "lock.json.sources.other"},
		"an edited lockfile":          {[]byte(fixtureConfig), []byte(strings.Replace(string(lock), `"release/2"`, `"release/3"`, 1)), "lock.json.sources.fabrica"},
		"a changed ref":               {[]byte(strings.Replace(fixtureConfig, "ref: release/2", "ref: release/3", 1)), lock, "lock.json.sources.fabrica"},
		"a changed repository":        {[]byte(strings.Replace(fixtureConfig, "public-library.git", "other-library.git", 1)), lock, "lock.json.sources.fabrica"},
		"a changed rule selector":     {[]byte(strings.Replace(fixtureConfig, "        - techs/go/*\n", "        - techs/*\n", 1)), lock, "lock.json.sources.fabrica"},
		"a changed skill selector":    {[]byte(strings.Replace(fixtureConfig, `        - "*"`, "        - tools/plan", 1)), lock, "lock.json.sources.tools"},
		"a changed pin":               {[]byte(strings.Replace(fixtureConfig, `version: "1.0.0"`, `version: "1.1.0"`, 1)), lock, "lock.json.sources.fabrica"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := projectview.Read(test.config, test.lock)
			var validation *trailguide.ValidationError
			if !errors.As(err, &validation) || !strings.HasPrefix(validation.Location, test.location) {
				t.Fatalf("got %v, want a validation error at %s", err, test.location)
			}
		})
	}
	newer := strings.Replace(string(lock), `"lockVersion": 1`, `"lockVersion": 2`, 1)
	var unsupported *trailguide.UnsupportedLockError
	if _, err := projectview.Read([]byte(fixtureConfig), []byte(newer)); !errors.As(err, &unsupported) {
		t.Fatalf("got %v, want an unsupported lock error", err)
	}
}

func TestUpdates(t *testing.T) {
	view, err := projectview.Read([]byte(fixtureConfig), fixtureLock(t))
	if err != nil {
		t.Fatal(err)
	}
	current := map[string]trailguide.RuleVersion{
		"techs/go/draft":   *version("1.0.0"),
		"techs/go/errors":  *version("1.1.0"),
		"techs/go/logging": *version("1.2.0"),
	}
	// The project still holds naming, which a pin keeps at its last version, and tests, both retired.
	got := view.Updates("fabrica", current, map[string]bool{"techs/go/naming": true, "techs/go/tests": true})
	want := projectview.Updates{
		Active: []projectview.RuleUpdate{
			{ID: "fabrica:techs/go/draft", Path: "techs/go/draft", Current: version("1.0.0")},
			{ID: "fabrica:techs/go/naming", Path: "techs/go/naming", Held: version("1.0.0"), Pinned: true},
			{ID: "fabrica:techs/go/tests", Path: "techs/go/tests", Held: version("2.0.0")},
		},
		// The replacement is based on 1.0.0, so 1.1.0 is news to it even though the project holds 1.1.0.
		ReplacementBasis: []projectview.RuleUpdate{{ID: "fabrica:techs/go/errors", Path: "techs/go/errors", Held: version("1.0.0"), Current: version("1.1.0")}},
		Excluded:         []projectview.RuleUpdate{{ID: "fabrica:techs/go/logging", Path: "techs/go/logging", Held: version("1.0.0"), Current: version("1.2.0")}},
	}
	if !reflect.DeepEqual(got, want) {
		data, _ := json.MarshalIndent(got, "", "  ")
		t.Errorf("updates =\n%s", data)
	}
	if none := view.Updates("tools", nil, nil); len(none.Active)+len(none.ReplacementBasis)+len(none.Excluded) != 0 {
		t.Errorf("a source with no rules has updates: %+v", none)
	}
}

// TestUpdatesListsARetirementTheLockfileRecords checks that a rule the lockfile records as retired, but the project
// still holds, is a pending update even when the caller doesn't pass the library's retirements: trailguide project
// update would remove it.
func TestUpdatesListsARetirementTheLockfileRecords(t *testing.T) {
	config := strings.Replace(fixtureConfig, "      pins:\n        - id: techs/go/naming\n          version: \"1.0.0\"\n          reason: Waiting on the rename.\n", "", 1)
	view, err := projectview.Read([]byte(config), fixtureLock(t))
	if err != nil {
		t.Fatal(err)
	}
	got := view.Updates("fabrica", map[string]trailguide.RuleVersion{"techs/go/tests": *version("2.0.0")}, nil)
	want := []projectview.RuleUpdate{{ID: "fabrica:techs/go/naming", Path: "techs/go/naming", Held: version("1.0.0")}}
	if !reflect.DeepEqual(got.Active, want) {
		data, _ := json.MarshalIndent(got.Active, "", "  ")
		t.Errorf("active updates =\n%s", data)
	}
}

// TestReadKeepsAReplacementOfARemovedRule checks that an authored rule replacing a rule the source no longer holds,
// such as one an update removed after its library retired it, stays in the view with what it replaces.
func TestReadKeepsAReplacementOfARemovedRule(t *testing.T) {
	lock, err := trailguide.ParseLock(fixtureLock(t))
	if err != nil {
		t.Fatal(err)
	}
	entry := lock.Sources["fabrica"]
	delete(entry.Rules, "techs/go/errors")
	entry.RetiredRules = []string{"techs/go/errors", "techs/go/naming"}
	lock.Sources["fabrica"] = entry
	data, err := lock.Encode()
	if err != nil {
		t.Fatal(err)
	}
	view, err := projectview.Read([]byte(fixtureConfig), data)
	if err != nil {
		t.Fatal(err)
	}
	want := projectview.Rule{ID: "local:techs/go/wrap-errors", Path: "techs/go/wrap-errors", Active: true,
		Replaces: &projectview.Replacement{ID: "fabrica:techs/go/errors", BasedOn: version("1.0.0")}}
	if i := slices.IndexFunc(view.Rules, func(rule projectview.Rule) bool { return rule.ID == want.ID }); i < 0 || !reflect.DeepEqual(view.Rules[i], want) {
		t.Errorf("rules = %+v, want %+v among them", view.Rules, want)
	}
	if slices.ContainsFunc(view.Rules, func(rule projectview.Rule) bool { return rule.ID == "fabrica:techs/go/errors" }) {
		t.Error("the view lists a rule the source no longer holds")
	}
}
