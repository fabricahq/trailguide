// Check release notes and tag messages rendered from release records, with the expected text inline.

package library

import (
	"os"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/trailguide"
)

// version returns a rule version for test records.
func version(major, minor, patch int) *trailguide.RuleVersion {
	return &trailguide.RuleVersion{Major: major, Minor: minor, Patch: patch}
}

// guideRecord is the library release the version rules guide shows as a GitHub Release page.
func guideRecord() trailguide.ReleaseRecord {
	return trailguide.ReleaseRecord{
		Release: 4,
		Rules: map[string]trailguide.RuleVersion{
			"practices/code-design/organize-code-by-feature": *version(1, 1, 0),
			"practices/testing/verify-backoff":               *version(1, 3, 0),
			"practices/testing/verify-retries":               *version(1, 0, 0),
			"practices/testing/verify-retry-limits":          *version(2, 0, 0),
			"techs/react/prefer-server-components":           *version(1, 4, 0),
			"techs/react/test-hooks-in-isolation":            *version(2, 2, 0),
		},
		Changes: map[string]trailguide.RecordedChange{
			"practices/testing/verify-retry-limits": {Change: trailguide.ChangeMajor, From: version(1, 3, 0), Summaries: []string{"Require a test at the limit for every retry policy."}},
			"practices/testing/verify-retries":      {Change: trailguide.ChangeNew, Summaries: []string{"Add a broader rule about testing retries."}},
			"techs/react/test-hooks-in-isolation":   {Change: trailguide.ChangeMinor, From: version(2, 1, 0), Summaries: []string{"Add an example for custom hooks."}},
		},
		Retired: map[string]trailguide.RetiredRule{
			"practices/testing/check-retry-backoff": {LastVersion: *version(1, 2, 0), ReplacedBy: "practices/testing/verify-retries", Summaries: []string{"Covered by the broader rule about testing retries."}},
		},
		LibraryFiles: []string{"practices/testing/_group.yaml"},
	}
}

// guideNotes is the example GitHub Release page in the version rules guide.
const guideNotes = "Library release 4 changes 4 rules: 1 new, 1 major, 1 minor, and 1 retired.\n\n## New rules\n\n- **practices/testing/verify-retries** `1.0.0`\n  - Add a broader rule about testing retries.\n\n## Major changes\n\nCode that complied with the previous rule version could fail the new one, so review these before updating.\n\n- **practices/testing/verify-retry-limits** `1.3.0` → `2.0.0`\n  - Require a test at the limit for every retry policy.\n\n## Minor changes\n\n- **techs/react/test-hooks-in-isolation** `2.1.0` → `2.2.0`\n  - Add an example for custom hooks.\n\n## Retired rules\n\n- **practices/testing/check-retry-backoff**, last version `1.2.0`, replaced by **practices/testing/verify-retries**\n  - Covered by the broader rule about testing retries.\n\nThis library release also updates shared files, such as group descriptions or shared assets.\n\n<details>\n<summary>All rule versions in this library release</summary>\n\n| Rule | Version |\n| --- | --- |\n| practices/code-design/organize-code-by-feature | 1.1.0 |\n| practices/testing/verify-backoff | 1.3.0 |\n| practices/testing/verify-retries | 1.0.0 |\n| practices/testing/verify-retry-limits | 2.0.0 |\n| techs/react/prefer-server-components | 1.4.0 |\n| techs/react/test-hooks-in-isolation | 2.2.0 |\n\n</details>"

// TestRenderReleaseNotes_MatchesTheGuideExample keeps the generated page and the guide's example identical.
func TestRenderReleaseNotes_MatchesTheGuideExample(t *testing.T) {
	if notes := renderReleaseNotes(guideRecord(), skillContext{}); notes != guideNotes {
		t.Fatalf("notes:\n%s\nwant:\n%s", notes, guideNotes)
	}
	guide, err := os.ReadFile("../../docs/src/content/docs/guides/version-rules.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(guide), "```md wrap\n"+guideNotes+"\n```") {
		t.Fatal("the version rules guide's example GitHub Release page differs from the rendered notes; update both together")
	}
}

// guideSkillsRecord is the library release that changes only skills, which the version rules guide shows as a GitHub
// Release page, and guideSkillsContext the levels its manifest gives them.
func guideSkillsRecord() (trailguide.ReleaseRecord, skillContext) {
	record := guideRecord()
	record.Release, record.Changes, record.Retired, record.LibraryFiles = 5, nil, nil, []string{"config.yaml"}
	record.Skills = &trailguide.SkillRecord{
		Versions: map[string]trailguide.RuleVersion{"skills/build-feature": *version(2, 0, 0), "skills/fix-bug": *version(1, 0, 0), "skills/review-tests": *version(1, 1, 0)},
		Changes: map[string]trailguide.RecordedChange{
			"skills/build-feature": {Change: trailguide.ChangeMajor, From: version(1, 3, 0), Summaries: []string{"Require an agreed design before the first slice."}},
			"skills/fix-bug":       {Change: trailguide.ChangeNew, Summaries: []string{"Add a playbook for fixing a reported bug with a regression test."}},
		},
		Retired: map[string]trailguide.RetiredRule{"skills/triage-bug": {LastVersion: *version(1, 2, 0), ReplacedBy: "skills/fix-bug", Summaries: []string{"Covered by the fix-bug playbook, which also writes the regression test."}}},
	}
	context := skillContext{
		levels:         map[string]string{"skills/build-feature": "required", "skills/fix-bug": "recommended", "skills/review-tests": "recommended"},
		previousLevels: map[string]string{"skills/build-feature": "required", "skills/fix-bug": "offered", "skills/review-tests": "offered"},
	}
	return record, context
}

// guideSkillsNotes is the example GitHub Release page of a library release that changes only skills in the version
// rules guide.
const guideSkillsNotes = "Library release 5 changes 3 skills: 1 new, 1 major, and 1 retired.\n\n## Skills\n\nA major skill change can change what the skill produces or requires, so review these before updating.\n\n- **skills/fix-bug** `1.0.0`, new, recommended\n  - Add a playbook for fixing a reported bug with a regression test.\n- **skills/build-feature** `1.3.0` → `2.0.0`, major, required\n  - Require an agreed design before the first slice.\n- **skills/triage-bug**, last version `1.2.0`, retired, replaced by **skills/fix-bug**\n  - Covered by the fix-bug playbook, which also writes the regression test.\n- **skills/review-tests** `1.1.0`, now recommended, was offered\n\nThis library release also updates shared files, such as group descriptions or shared assets.\n\n<details>\n<summary>All rule versions in this library release</summary>\n\n| Rule | Version |\n| --- | --- |\n| practices/code-design/organize-code-by-feature | 1.1.0 |\n| practices/testing/verify-backoff | 1.3.0 |\n| practices/testing/verify-retries | 1.0.0 |\n| practices/testing/verify-retry-limits | 2.0.0 |\n| techs/react/prefer-server-components | 1.4.0 |\n| techs/react/test-hooks-in-isolation | 2.2.0 |\n\n</details>\n\n<details>\n<summary>All skill versions in this library release</summary>\n\n| Skill | Version |\n| --- | --- |\n| skills/build-feature | 2.0.0 |\n| skills/fix-bug | 1.0.0 |\n| skills/review-tests | 1.1.0 |\n\n</details>"

// TestRenderReleaseNotes_MatchesTheGuideSkillsExample keeps the generated page of a library release that changes
// only skills and the guide's example identical, and the record it publishes valid.
func TestRenderReleaseNotes_MatchesTheGuideSkillsExample(t *testing.T) {
	record, context := guideSkillsRecord()
	if notes := renderReleaseNotes(record, context); notes != guideSkillsNotes {
		t.Fatalf("notes:\n%s\nwant:\n%s", notes, guideSkillsNotes)
	}
	if _, err := releaseMessage(guideSkillsNotes, record); err != nil {
		t.Fatal(err)
	}
	guide, err := os.ReadFile("../../docs/src/content/docs/guides/version-rules.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(guide), "```md wrap\n"+guideSkillsNotes+"\n```") {
		t.Fatal("the version rules guide's example GitHub Release page for skills differs from the rendered notes; update both together")
	}
}

// TestRenderReleaseNotes_CountsSkillChangesBesideRuleChanges adds a sentence counting skill changes after the rule
// count, counts the skills the first library release publishes, and says a release with no rule or skill changes
// updates shared files.
func TestRenderReleaseNotes_CountsSkillChangesBesideRuleChanges(t *testing.T) {
	both := guideRecord()
	both.Skills = &trailguide.SkillRecord{Versions: map[string]trailguide.RuleVersion{"skills/fix-bug": *version(1, 1, 0), "skills/new": *version(1, 0, 0)}, Changes: map[string]trailguide.RecordedChange{
		"skills/fix-bug": {Change: trailguide.ChangeMinor, From: version(1, 0, 0), Summaries: []string{"Add a step."}},
		"skills/new":     {Change: trailguide.ChangeNew, Summaries: []string{"Add it."}},
	}}
	first := trailguide.ReleaseRecord{Release: 1, Rules: map[string]trailguide.RuleVersion{}, Skills: &trailguide.SkillRecord{Versions: map[string]trailguide.RuleVersion{"skills/fix-bug": trailguide.FirstRuleVersion}, Changes: map[string]trailguide.RecordedChange{"skills/fix-bug": {Change: trailguide.ChangeNew, Summaries: []string{"Add the skill."}}}}}
	unchanged := trailguide.ReleaseRecord{Release: 6, Rules: map[string]trailguide.RuleVersion{}, LibraryFiles: []string{"assets/a.svg"}, Skills: &trailguide.SkillRecord{Versions: map[string]trailguide.RuleVersion{"skills/fix-bug": trailguide.FirstRuleVersion}}}
	for _, test := range []struct {
		record trailguide.ReleaseRecord
		want   string
	}{
		{both, "Library release 4 changes 4 rules: 1 new, 1 major, 1 minor, and 1 retired. It also changes 2 skills: 1 new and 1 minor.\n\n"},
		{first, "Library release 1 publishes 1 skill.\n\n## Skills\n\n- **skills/fix-bug** `1.0.0`, new, offered\n\n<details>"},
		{unchanged, "Library release 6 changes no rules or skills. It updates shared files, such as group descriptions or shared assets.\n\n<details>\n<summary>All skill versions"},
	} {
		context := skillContext{levels: map[string]string{"skills/fix-bug": "offered", "skills/new": "offered"}}
		if notes := renderReleaseNotes(test.record, context); !strings.HasPrefix(notes, test.want) {
			t.Fatalf("notes:\n%s\nwant them to start with:\n%s", notes, test.want)
		}
	}
}

// TestRenderReleaseNotes_CountsAndSectionsFollowTheChanges covers singular counts, two kinds, several entries
// and summaries per section, patch changes, the first library release, and the shared-files sentence: a closing
// sentence when rules and library-wide files both change, none for rules alone, and the opening line for
// library-wide files alone.
func TestRenderReleaseNotes_CountsAndSectionsFollowTheChanges(t *testing.T) {
	for _, test := range []struct {
		name   string
		record trailguide.ReleaseRecord
		want   string
	}{
		{
			name: "rules and library-wide files",
			record: trailguide.ReleaseRecord{
				Release:      7,
				Rules:        map[string]trailguide.RuleVersion{"practices/testing/a": *version(1, 1, 0)},
				Changes:      map[string]trailguide.RecordedChange{"practices/testing/a": {Change: trailguide.ChangeMinor, From: version(1, 0, 0), Summaries: []string{"Add an example."}}},
				LibraryFiles: []string{"practices/testing/_group.yaml"},
			},
			want: "Library release 7 changes 1 rule: 1 minor.\n\n## Minor changes\n\n- **practices/testing/a** `1.0.0` → `1.1.0`\n  - Add an example.\n\nThis library release also updates shared files, such as group descriptions or shared assets.\n\n<details>\n<summary>All rule versions in this library release</summary>\n\n| Rule | Version |\n| --- | --- |\n| practices/testing/a | 1.1.0 |\n\n</details>",
		},
		{
			name: "rules without library-wide files: one patch with two notes",
			record: trailguide.ReleaseRecord{
				Release: 3,
				Rules:   map[string]trailguide.RuleVersion{"practices/testing/a": *version(1, 0, 1)},
				Changes: map[string]trailguide.RecordedChange{"practices/testing/a": {Change: trailguide.ChangePatch, From: version(1, 0, 0), Summaries: []string{"Fix a typo.", "Clarify an example."}}},
			},
			want: "Library release 3 changes 1 rule: 1 patch.\n\n## Patch changes\n\n- **practices/testing/a** `1.0.0` → `1.0.1`\n  - Fix a typo.\n  - Clarify an example.\n\n<details>\n<summary>All rule versions in this library release</summary>\n\n| Rule | Version |\n| --- | --- |\n| practices/testing/a | 1.0.1 |\n\n</details>",
		},
		{
			name: "two kinds and a retirement without a replacement",
			record: trailguide.ReleaseRecord{
				Release: 5,
				Rules:   map[string]trailguide.RuleVersion{"practices/testing/a": *version(1, 10, 0), "practices/testing/b": *version(1, 5, 0)},
				Changes: map[string]trailguide.RecordedChange{
					"practices/testing/b": {Change: trailguide.ChangeMinor, From: version(1, 4, 0), Summaries: []string{"Add a Go example."}},
					"practices/testing/a": {Change: trailguide.ChangeMinor, From: version(1, 9, 0), Summaries: []string{"Add a Python example."}},
				},
				Retired: map[string]trailguide.RetiredRule{"practices/testing/c": {LastVersion: *version(3, 1, 4), Summaries: []string{"Agents shouldn't add these comments."}}},
			},
			want: "Library release 5 changes 3 rules: 2 minor and 1 retired.\n\n## Minor changes\n\n- **practices/testing/a** `1.9.0` → `1.10.0`\n  - Add a Python example.\n- **practices/testing/b** `1.4.0` → `1.5.0`\n  - Add a Go example.\n\n## Retired rules\n\n- **practices/testing/c**, last version `3.1.4`\n  - Agents shouldn't add these comments.\n\n<details>\n<summary>All rule versions in this library release</summary>\n\n| Rule | Version |\n| --- | --- |\n| practices/testing/a | 1.10.0 |\n| practices/testing/b | 1.5.0 |\n\n</details>",
		},
		{
			name: "first library release, which adds its library-wide files without the shared-files sentence",
			record: trailguide.ReleaseRecord{
				Release:      1,
				LibraryFiles: []string{"config.yaml", "practices/testing/_group.yaml"},
				Rules:        map[string]trailguide.RuleVersion{"practices/testing/a": trailguide.FirstRuleVersion, "techs/go/b": trailguide.FirstRuleVersion},
				Changes:      map[string]trailguide.RecordedChange{"practices/testing/a": {Change: trailguide.ChangeNew, Summaries: []string{"Add the rule."}}, "techs/go/b": {Change: trailguide.ChangeNew, Summaries: []string{"Add the rule."}}},
			},
			want: "Library release 1 publishes 2 rules.\n\n## New rules\n\n- **practices/testing/a** `1.0.0`\n- **techs/go/b** `1.0.0`\n\n<details>\n<summary>All rule versions in this library release</summary>\n\n| Rule | Version |\n| --- | --- |\n| practices/testing/a | 1.0.0 |\n| techs/go/b | 1.0.0 |\n\n</details>",
		},
		{
			name: "library-wide files only",
			record: trailguide.ReleaseRecord{
				Release:      6,
				Rules:        map[string]trailguide.RuleVersion{"practices/testing/a": *version(1, 2, 3)},
				LibraryFiles: []string{"assets/diagram.svg"},
			},
			want: "Library release 6 changes no rules. It updates shared files, such as group descriptions or shared assets.\n\n<details>\n<summary>All rule versions in this library release</summary>\n\n| Rule | Version |\n| --- | --- |\n| practices/testing/a | 1.2.3 |\n\n</details>",
		},
		{
			name:   "library-wide files in a library without rules",
			record: trailguide.ReleaseRecord{Release: 1, Rules: map[string]trailguide.RuleVersion{}, LibraryFiles: []string{"config.yaml"}},
			want:   "Library release 1 publishes no rules, only shared files, such as group descriptions or shared assets.",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if notes := renderReleaseNotes(test.record, skillContext{}); notes != test.want {
				t.Fatalf("notes:\n%s\nwant:\n%s", notes, test.want)
			}
		})
	}
}

// TestReleaseMessage_WritesNotesThenARecordThatReadsBack puts the record after a --- line, sorted and without
// empty sections, and parses it back to the same record.
func TestReleaseMessage_WritesNotesThenARecordThatReadsBack(t *testing.T) {
	record := trailguide.ReleaseRecord{
		Release: 2,
		Rules:   map[string]trailguide.RuleVersion{"practices/testing/c": trailguide.FirstRuleVersion, "practices/testing/a": *version(2, 0, 0)},
		Changes: map[string]trailguide.RecordedChange{
			"practices/testing/c": {Change: trailguide.ChangeNew, Summaries: []string{"Replace b with c."}},
			"practices/testing/a": {Change: trailguide.ChangeMajor, From: version(1, 0, 0), Summaries: []string{"Fix a typo.", "Replace b with c."}},
		},
		Retired: map[string]trailguide.RetiredRule{"practices/testing/b": {LastVersion: trailguide.FirstRuleVersion, ReplacedBy: "practices/testing/c", Summaries: []string{"Replace b with c."}}},
	}
	message, err := releaseMessage("Notes.\n\n---\n\nMore notes.", record)
	if err != nil {
		t.Fatal(err)
	}
	want := "Notes.\n\n---\n\nMore notes.\n---\nschemaVersion: 1\nrelease: 2\nrules:\n  practices/testing/a: 2.0.0\n  practices/testing/c: 1.0.0\nchanges:\n  practices/testing/a:\n    change: major\n    from: 1.0.0\n    summaries:\n      - Fix a typo.\n      - Replace b with c.\n  practices/testing/c:\n    change: new\n    summaries:\n      - Replace b with c.\nretired:\n  practices/testing/b:\n    lastVersion: 1.0.0\n    replacedBy: practices/testing/c\n    summaries:\n      - Replace b with c.\n"
	if string(message) != want {
		t.Fatalf("message:\n%s\nwant:\n%s", message, want)
	}
	record.LibraryFiles = []string{"practices/testing/_group.yaml", "assets/diagram.svg"}
	message, err = releaseMessage("Notes.", record)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(message), "libraryFiles:\n  - practices/testing/_group.yaml\n  - assets/diagram.svg\n") {
		t.Fatalf("library-wide files keep their order:\n%s", message)
	}
}

// TestReleaseMessage_RefusesARecordTheParserRejects never produces a tag message that projects can't read.
func TestReleaseMessage_RefusesARecordTheParserRejects(t *testing.T) {
	record := trailguide.ReleaseRecord{
		Release:      2,
		Rules:        map[string]trailguide.RuleVersion{"practices/testing/a": *version(1, 1, 0)},
		Changes:      map[string]trailguide.RecordedChange{"practices/testing/a": {Change: trailguide.ChangeMajor, From: version(1, 0, 0), Summaries: []string{"Tighten a."}}},
		LibraryFiles: []string{"practices/testing/a.md"},
	}
	if _, err := releaseMessage("Notes.", record); err == nil {
		t.Fatal("expected an inconsistent record to fail")
	}
}
