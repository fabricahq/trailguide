// Test that a skills/ folder's layout finds each skill in its origin folder and reports each skill in the wrong place.

package skills_test

import (
	"maps"
	"reflect"
	"slices"
	"testing"

	"github.com/fabricahq/trailguide/internal/skills"
)

func TestDiscoverLayoutReadsOriginFoldersAndTheFlatLayout(t *testing.T) {
	tests := map[string]struct {
		files []string
		want  skills.Layout
	}{
		"origin folders": {
			[]string{"README.md", "acme/fix/SKILL.md", "acme/fix/scripts/run.sh", "pstack/show/SKILL.md"},
			skills.Layout{"fix": "acme", "show": "pstack"},
		},
		"library schema version 1": {[]string{"fix/SKILL.md", "fix/references/SKILL.md"}, skills.Layout{"fix": ""}},
		"a name two origins share": {[]string{"zeta/fix/SKILL.md", "acme/fix/SKILL.md"}, skills.Layout{"fix": "acme"}},
		"invalid names":            {[]string{"Acme/fix/SKILL.md", "acme/Fix/SKILL.md", "acme/notes/README.md"}, skills.Layout{}},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			layout := skills.DiscoverLayout(test.files)
			if !reflect.DeepEqual(layout, test.want) {
				t.Fatalf("got %v; want %v", layout, test.want)
			}
			for skill := range layout {
				if got, inside, ok := layout.Split(layout.Path(skill, "SKILL.md")); !ok || got != skill || inside != "SKILL.md" {
					t.Fatalf("split %s = %s, %s, %v", layout.Path(skill, "SKILL.md"), got, inside, ok)
				}
			}
		})
	}
}

func TestLayoutProblemsNamesTheFolderEachSkillBelongsIn(t *testing.T) {
	origins := skills.Origins{Own: "acme", Forks: map[string]string{"show": "pstack"}}
	tests := map[string]struct {
		files, directories []string
		want               []skills.LayoutProblem
	}{
		"a valid layout": {[]string{"README.md", "acme/fix/SKILL.md", "pstack/show/SKILL.md"}, nil, []skills.LayoutProblem{}},
		"a file directly in skills/": {[]string{"notes.md"}, nil, []skills.LayoutProblem{
			{"skills/notes.md", "skills/ holds a folder for each origin, each holding skill folders, and its README.md; move this file into a skill's folder"},
		}},
		"a skill directly in skills/": {[]string{"fix/SKILL.md", "show/SKILL.md"}, nil, []skills.LayoutProblem{
			{"skills/fix", "a skill's folder sits in the folder for its origin, so move it to skills/acme/fix/"},
			{"skills/show", "a skill's folder sits in the folder for its origin, so move it to skills/pstack/show/"},
		}},
		"a file directly in an origin folder": {[]string{"acme/notes.md"}, nil, []skills.LayoutProblem{
			{"skills/acme/notes.md", "an origin folder holds only skill folders; move this file into a skill's folder"},
		}},
		"an unknown origin": {[]string{"other/fix/SKILL.md"}, nil, []skills.LayoutProblem{
			{"skills/other", "an origin folder is named for the repository's own skills, acme, or for a source a fork comes from; move its skills to the folder for their origin"},
		}},
		"an invalid or incomplete skill folder": {[]string{"acme/Fix/SKILL.md", "acme/tidy/notes.md"}, []string{"acme/empty"}, []skills.LayoutProblem{
			{"skills/acme/Fix", "a skill's folder name is its name, which uses 1 to 64 lowercase letters, digits, and hyphens, without a leading, trailing, or doubled hyphen; rename the folder"},
			{"skills/acme/empty", "missing SKILL.md; each folder in an origin folder is a skill, so add its SKILL.md or move the folder out of skills/"},
			{"skills/acme/tidy", "missing SKILL.md; each folder in an origin folder is a skill, so add its SKILL.md or move the folder out of skills/"},
		}},
		"a skill in the wrong origin": {[]string{"acme/show/SKILL.md", "pstack/fix/SKILL.md"}, nil, []skills.LayoutProblem{
			{"skills/acme/show", "this skill forks a skill of the source pstack, so it sits in that source's folder; move it to skills/pstack/show/"},
			{"skills/pstack/fix", "this skill forks no skill of the source pstack, so it's one of the repository's own skills; move it to skills/acme/fix/, or record the fork in sources.pstack.skills.forks"},
		}},
		"two skills with one name": {[]string{"acme/show/SKILL.md", "pstack/show/SKILL.md"}, nil, []skills.LayoutProblem{
			{"skills/acme/show", "this skill forks a skill of the source pstack, so it sits in that source's folder; move it to skills/pstack/show/"},
			{"skills/pstack/show", "skills/acme/show is also a skill named show, and a skill's name is its ID, skills/show; rename one of them"},
		}},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := skills.LayoutProblems(test.files, test.directories, origins); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("got %+v\nwant %+v", got, test.want)
			}
		})
	}
}

func TestSplitFolderTakesASourceNameAndASkillName(t *testing.T) {
	for folder, want := range map[string]bool{
		"pstack/show-me-your-work": true,
		"team--tools/hello":        true,
		"team/hello--there":        false,
		"Team/hello":               false,
		"team-/hello":              true,
		"hello":                    false,
		"team/hello/nested":        false,
	} {
		if _, _, ok := skills.SplitFolder(folder); ok != want {
			t.Errorf("SplitFolder(%q) reports %v; want %v", folder, ok, want)
		}
	}
}

func TestReadReturnsEachSkillWithItsOriginOrEveryProblem(t *testing.T) {
	origins := skills.Origins{Own: "acme", Forks: map[string]string{"show": "team--tools"}}
	files := map[string]string{"README.md": "guide", "acme/fix/SKILL.md": "fix", "acme/fix/scripts/run.sh": "run", "team--tools/show/SKILL.md": "show"}
	found, problems := skills.Read(files, []string{"acme/fix/scripts/run.sh"}, nil, origins)
	want := map[string]skills.Skill[string]{
		"skills/fix":  {Name: "fix", Origin: "acme", Files: map[string]string{"SKILL.md": "fix", "scripts/run.sh": "run"}, Executable: []string{"scripts/run.sh"}},
		"skills/show": {Name: "show", Origin: "team--tools", Files: map[string]string{"SKILL.md": "show"}, Executable: []string{}},
	}
	if len(problems) != 0 || !reflect.DeepEqual(found, want) {
		t.Fatalf("got %+v, %+v; want %+v", found, problems, want)
	}
	if folder := found["skills/show"].Folder(); folder != "team--tools/show" || found["skills/show"].ID() != "skills/show" {
		t.Fatalf("folder %s, ID %s", folder, found["skills/show"].ID())
	}
	// A second skill named fix, in an origin folder origins don't name: DiscoverLayout would keep one of them, but
	// Read keeps no skill and reports the folder.
	files["aaa/fix/SKILL.md"] = "other fix"
	found, problems = skills.Read(files, nil, nil, origins)
	if found != nil || len(problems) != 1 || problems[0].Location != "skills/aaa" {
		t.Fatalf("got %+v, %+v; want the unknown origin", found, problems)
	}
	if layout := skills.DiscoverLayout(slices.Collect(maps.Keys(files))); layout["fix"] != "aaa" {
		t.Fatalf("DiscoverLayout placed fix in %q; the test expects it to pick aaa", layout["fix"])
	}
}
