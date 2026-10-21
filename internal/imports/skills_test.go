// Test how a source's skills are found in a commit's tree and chosen by skills.include, without Git.

package imports

import (
	"errors"
	"io/fs"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/rules"
	"github.com/fabricahq/trailguide/trailguide"
)

// entries returns a tree of regular files at names, keyed by repository path.
func entries(names ...string) map[string]treeEntry {
	tree := map[string]treeEntry{}
	for _, name := range names {
		tree[name] = treeEntry{name: name, mode: 0o644, object: "0"}
	}
	return tree
}

func TestSkillFoldersFindsEachSkillAndItsFiles(t *testing.T) {
	tests := map[string]struct {
		tree      map[string]treeEntry
		isLibrary bool
		want      map[string][]string
	}{
		"library skills are folders directly in .trailguide/skills": {
			tree: entries(".trailguide/skills/build/SKILL.md", ".trailguide/skills/build/scripts/run.sh", ".trailguide/skills/notes.md", ".trailguide/skills/empty/README.md",
				".trailguide/skills/group/nested/SKILL.md", ".trailguide/rules/techs/go/errors.md", "tools/review/SKILL.md"),
			isLibrary: true,
			want:      map[string][]string{"skills/build": {"skills/build/SKILL.md", "skills/build/scripts/run.sh"}},
		},
		"another repository's skills are folders with a SKILL.md": {
			tree:      entries("SKILL.md", "README.md", "tools/review/SKILL.md", "tools/review/guide.md", "skills/engineering/to-spec/SKILL.md", ".claude/skills/local/SKILL.md"),
			isLibrary: false,
			want: map[string][]string{
				"tools/review":               {"tools/review/SKILL.md", "tools/review/guide.md"},
				"skills/engineering/to-spec": {"skills/engineering/to-spec/SKILL.md"},
				".claude/skills/local":       {".claude/skills/local/SKILL.md"},
			},
		},
		"a skill inside another skill's folder belongs to it": {
			tree:      entries("outer/SKILL.md", "outer/inner/SKILL.md", "outer/inner/notes.md", "outer-two/SKILL.md"),
			isLibrary: false,
			want:      map[string][]string{"outer": {"outer/SKILL.md", "outer/inner/SKILL.md", "outer/inner/notes.md"}, "outer-two": {"outer-two/SKILL.md"}},
		},
		"no skills": {tree: entries("README.md"), want: map[string][]string{}},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			folders := skillFolders(test.tree, test.isLibrary)
			got := map[string][]string{}
			for id, files := range folders {
				got[id] = slices.Sorted(maps.Keys(files))
				for stored, entry := range files {
					if entry.name != stored {
						t.Errorf("%s: entry name %s, want its snapshot path %s", id, entry.name, stored)
					}
				}
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("skillFolders = %v, want %v", got, test.want)
			}
		})
	}
}

func TestChooseSkillsTakesEverySkillFromThePlannedRevision(t *testing.T) {
	const locked, newer = "1111111111111111111111111111111111111111", "2222222222222222222222222222222222222222"
	available := []string{".claude/skills/local", "skills/build", "skills/review"}
	release := func(_ *testing.T, include ...string) rules.Source {
		return rules.Source{Name: "team", Skills: include}
	}
	pinned := func(t *testing.T, include ...string) rules.Source {
		return rules.Source{Name: "team", Skills: include, Ref: gitRef(t, "v1.0.0")}
	}
	tests := map[string]struct {
		source  func(*testing.T, ...string) rules.Source
		include []string
		plan    sourcePlan
		want    []string
		code    string
		problem string
	}{
		"wildcard skips dot folders":           {release, []string{"*"}, sourcePlan{commit: locked}, []string{"skills/build", "skills/review"}, "", ""},
		"named skills":                         {release, []string{"skills/review"}, sourcePlan{commit: locked, locked: locked}, []string{"skills/review"}, "", ""},
		"a dot folder can be named":            {release, []string{".claude/skills/local"}, sourcePlan{commit: locked}, []string{".claude/skills/local"}, "", ""},
		"missing from the locked release":      {release, []string{"skills/new"}, sourcePlan{release: 1, commit: locked, locked: locked}, nil, "skill-not-in-revision", "Run trailguide project update team"},
		"missing from the locked ref":          {pinned, []string{"skills/new"}, sourcePlan{commit: locked, locked: locked}, nil, "skill-not-in-revision", "set sources.team.ref to a tag or commit that has it"},
		"missing from a newly chosen revision": {release, []string{"skills/new"}, sourcePlan{release: 2, commit: newer, locked: locked}, nil, "", "the source has no skill skills/new at release/2"},
		"missing without a record":             {release, []string{"skills/new"}, sourcePlan{commit: newer}, nil, "", "the source has no skill skills/new at commit " + newer},
		"a curated ID outside a library":       {release, []string{"pocock:skills/to-spec"}, sourcePlan{commit: locked}, nil, "", "only a library's curated skills have IDs with a colon"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := chooseSkills(test.source(t, test.include...), test.plan, available, nil)
			if test.problem == "" {
				if err != nil || !slices.Equal(got, test.want) {
					t.Fatalf("chooseSkills = %v, %v; want %v", got, err, test.want)
				}
				return
			}
			var failure *Error
			var validation *ValidationError
			switch {
			case test.code != "" && (!errors.As(err, &failure) || failure.Code != test.code):
				t.Fatalf("got %v; want code %s", err, test.code)
			case test.code == "" && (!errors.As(err, &validation) || validation.Location != "sources.team.skills.include"):
				t.Fatalf("got %v; want a validation error at sources.team.skills.include", err)
			case !strings.Contains(err.Error(), test.problem):
				t.Fatalf("got %v; want %q", err, test.problem)
			}
		})
	}
}

func TestSkillFoldersKeepsFileModes(t *testing.T) {
	tree := entries("tools/run/SKILL.md")
	tree["tools/run/run.sh"] = treeEntry{name: "tools/run/run.sh", mode: 0o755, object: "1"}
	tree["tools/run/link"] = treeEntry{name: "tools/run/link", mode: fs.ModeSymlink, object: "2"}
	files := skillFolders(tree, false)["tools/run"]
	if files["tools/run/run.sh"].mode.Perm()&0o100 == 0 || files["tools/run/link"].mode.IsRegular() {
		t.Fatalf("modes = %v", files)
	}
}

func TestRequireConfiguredUsesNeedsEachUsedSkillInTheRevision(t *testing.T) {
	const locked = "1111111111111111111111111111111111111111"
	available := []string{"skills/architect", "skills/how"}
	source := func(uses map[string][]string) rules.Source {
		return rules.Source{Name: "team", Skills: []string{"skills/architect"}, SkillUses: uses}
	}
	if err := requireConfiguredUses(source(map[string][]string{"skills/architect": {"skills/how"}}), sourcePlan{commit: locked, locked: locked}, available); err != nil {
		t.Fatal(err)
	}
	err := requireConfiguredUses(source(map[string][]string{"skills/architect": {"skills/gone"}}), sourcePlan{release: 1, commit: locked, locked: locked}, available)
	var failure *Error
	if !errors.As(err, &failure) || failure.Code != "skill-not-in-revision" || !strings.HasPrefix(failure.Error(), "sources.team.skills.uses.skills/architect names skills/gone, which isn't in release/1") {
		t.Fatalf("locked revision: got %v", err)
	}
	err = requireConfiguredUses(source(map[string][]string{"skills/architect": {"skills/gone"}}), sourcePlan{commit: locked}, available)
	var validation *ValidationError
	if !errors.As(err, &validation) || validation.Location != "sources.team.skills.uses.skills/architect" || !strings.Contains(validation.Problem, "the source has no skill skills/gone") {
		t.Fatalf("new revision: got %v", err)
	}
}

func TestInstalledSkillsReportsTheSkillsAMissingUseOrCycleInvolves(t *testing.T) {
	plan := sourcePlan{release: 3, commit: "1111111111111111111111111111111111111111"}
	tests := map[string]struct {
		uses       map[string][]string
		configured map[string][]string
		location   string
		problem    string
	}{
		"an included skill's missing use": {
			map[string][]string{"skills/architect": {"skills/how"}}, nil, "sources.team.skills",
			"skills/architect uses skills/how, which isn't a skill in release/3: skills/architect names it in trailguide-skills-used. Trail Guide installs every skill an installed skill uses, so it can't install skills/architect. Ask the source's maintainer to correct trailguide-skills-used, or stop including skills/architect",
		},
		"a used skill's missing use": {
			map[string][]string{"skills/architect": {"skills/how"}, "skills/how": {"skills/gone"}}, nil, "sources.team.skills",
			"skills/how uses skills/gone, which isn't a skill in release/3: skills/how names it in trailguide-skills-used. Trail Guide installs every skill an installed skill uses, so it can't install skills/how, which skills/architect uses. Ask the source's maintainer to correct trailguide-skills-used, or stop including skills/architect",
		},
		"a cycle": {
			map[string][]string{"skills/architect": {"skills/how"}, "skills/how": {"skills/architect"}}, nil, "sources.team.skills",
			"the skills the source installs use each other in a cycle: skills/architect -> skills/how -> skills/architect. Trail Guide installs every skill an installed skill uses, so uses can't form a cycle. Ask the source's maintainer to correct trailguide-skills-used, or remove the use from sources.team.skills.uses",
		},
		"a uses key the source doesn't install": {
			map[string][]string{"skills/architect": nil}, map[string][]string{"skills/other": {"skills/architect"}}, "sources.team.skills.uses.skills/other",
			"skills/other isn't a skill the source installs, so it has no uses to declare; a key of skills.uses names a skill that skills.include selects, or one those skills use. Include the skill, or remove the entry",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			source := rules.Source{Name: "team", Skills: []string{"skills/architect"}, SkillUses: test.configured}
			_, err := installedSkills(source, plan, source.Skills, test.uses)
			var validation *ValidationError
			if !errors.As(err, &validation) || validation.Location != test.location || validation.Problem != test.problem {
				t.Fatalf("got %v\nwant %s: %s", err, test.location, test.problem)
			}
		})
	}
}

func TestCuratedSkillsWildcardSkipsDotFolders(t *testing.T) {
	manifest := &rules.Library{Skills: trailguide.SkillLevels{Required: []string{}, Recommended: []string{}, Offered: []string{"pocock:.claude/skills/local", "pocock:skills/grilling"}}}
	for _, test := range []struct {
		include []string
		want    []string
	}{
		{[]string{"*"}, []string{"pocock:skills/grilling"}},
		{[]string{"*", "pocock:.claude/skills/local"}, []string{"pocock:.claude/skills/local", "pocock:skills/grilling"}},
	} {
		got, err := curatedSkills(manifest, rules.Source{Name: "team", Skills: test.include}, 1, nil)
		ids := []string{}
		for _, skill := range got["pocock"] {
			ids = append(ids, skill.id)
		}
		if err != nil || !slices.Equal(ids, test.want) {
			t.Errorf("include %v: curatedSkills = %v, %v; want %v", test.include, ids, err, test.want)
		}
	}
}

func TestCuratedSkillsAddsTheCuratedSkillsTheLibrarysSkillsUse(t *testing.T) {
	manifest := &rules.Library{Skills: trailguide.SkillLevels{Required: []string{}, Recommended: []string{"anthropic:skills/skill-creator"}, Offered: []string{"pocock:skills/grilling"}}}
	got, err := curatedSkills(manifest, rules.Source{Name: "team", Skills: []string{"pocock:skills/grilling"}}, 1, map[string][]string{"anthropic:skills/skill-creator": {"skills/add-skill"}})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]curatedSkill{
		"anthropic": {{id: "anthropic:skills/skill-creator", level: "recommended", usedBy: []string{"skills/add-skill"}}},
		"pocock":    {{id: "pocock:skills/grilling", level: "offered", included: true}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("curatedSkills = %+v; want %+v", got, want)
	}
	_, err = curatedSkills(manifest, rules.Source{Name: "team"}, 1, map[string][]string{"anthropic:skills/unlisted": {"skills/add-skill"}})
	if err == nil || !strings.Contains(err.Error(), "the library's skill skills/add-skill uses anthropic:skills/unlisted, which the library doesn't list at any level or as a dependency") {
		t.Errorf("got %v; want the unlisted curated skill refused", err)
	}
}

func TestChooseSkillsLeavesOutTheExcludedLevels(t *testing.T) {
	const locked = "1111111111111111111111111111111111111111"
	available := []string{"skills/build", "skills/lint", "skills/review", "skills/ship"}
	levels := &trailguide.SkillLevels{Required: []string{"skills/ship"}, Recommended: []string{"skills/review"}, Offered: []string{"skills/build"}}
	source := func(include []string, exclude ...string) rules.Source {
		excluded := map[string]string{}
		for _, id := range exclude {
			excluded[id] = "Ours."
		}
		return rules.Source{Name: "team", Skills: include, SkillExcludeLevels: excluded}
	}
	tests := map[string]struct {
		source rules.Source
		levels *trailguide.SkillLevels
		want   []string
	}{
		"every skill but the recommended":     {source([]string{"*"}, "recommended"), levels, []string{"skills/build", "skills/lint", "skills/ship"}},
		"only the required":                   {source([]string{"*"}, "*"), levels, []string{"skills/ship"}},
		"a named skill of an excluded level":  {source([]string{"skills/review"}, "recommended"), levels, []string{"skills/ship"}},
		"a library's required skills":         {rules.Source{Name: "team", Skills: []string{"*"}, SkillExcludeLevels: map[string]string{"*": "Ours."}, LibraryRequires: []string{"skills/lint"}}, levels, []string{"skills/lint", "skills/ship"}},
		"a skill also excluded by ID":         {rules.Source{Name: "team", Skills: []string{"*"}, SkillExcludeLevels: map[string]string{"*": "Ours."}, SkillExclude: map[string]rules.SkillExclusion{"skills/build": {Reason: "Ours."}}}, levels, []string{"skills/build", "skills/ship"}},
		"a forked skill of an excluded level": {rules.Source{Name: "team", Skills: []string{"*"}, SkillExcludeLevels: map[string]string{"*": "Ours."}, SkillForks: map[string]rules.SkillFork{"skills/review": {As: "skills/our-review", Reason: "Ours."}}}, levels, []string{"skills/review", "skills/ship"}},
		"a fork of a deleted skill":           {rules.Source{Name: "team", Skills: []string{}, SkillForks: map[string]rules.SkillFork{"skills/gone": {As: "skills/gone", Reason: "Ours."}}}, levels, []string{"skills/ship"}},
		"none from another repository":        {source([]string{"*"}, "*"), nil, []string{}},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := chooseSkills(test.source, sourcePlan{commit: locked}, available, test.levels)
			if err != nil || !slices.Equal(got, test.want) {
				t.Fatalf("chooseSkills = %v, %v; want %v", got, err, test.want)
			}
		})
	}
	_, err := chooseSkills(source([]string{"*"}, "recommended"), sourcePlan{commit: locked}, available, nil)
	var validation *ValidationError
	if !errors.As(err, &validation) || validation.Location != "sources.team.skills.exclude.recommended" || !strings.Contains(validation.Problem, "isn't a library") {
		t.Fatalf("got %v; want the non-library recommended exclusion to fail", err)
	}
}
