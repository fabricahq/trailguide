// Test that installed skills' local links reach their files offline, with referenced files retained for reading.

package project

import (
	"context"
	"errors"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/build"
	"github.com/fabricahq/trailguide/internal/imports"
	"github.com/fabricahq/trailguide/internal/skillrefs"
	"github.com/fabricahq/trailguide/internal/test/gitfixture"
	"github.com/fabricahq/trailguide/trailguide"
)

// The Greenfield regressions: two selected skills compare themselves with skills the project doesn't select, in
// links that the workflows never call.
const (
	attackSkill   = "---\nname: principle-attack-the-premise\ndescription: Attack the premise.\n---\n# Attack the premise\n\nCompare with [Redesign from First Principles](../principle-redesign-from-first-principles/SKILL.md).\n\nRun `cat ../principle-missing/SKILL.md` to see nothing:\n\n```sh\ncat [a path](../principle-also-missing/SKILL.md)\n```\n\nRead [the essay](https://example.invalid/private/essay.md).\n"
	leverSkill    = "---\nname: principle-build-the-lever\ndescription: Build the lever.\n---\n# Build the lever\n\nCompare with [Encode Lessons in Structure](../principle-encode-lessons-in-structure/SKILL.md).\n"
	redesignSkill = "---\nname: principle-redesign-from-first-principles\ndescription: Redesign from first principles.\n---\n# Redesign\n\nFollow [the steps](references/steps.md#first-step), twice: [again](references/steps.md).\n"
	encodeSkill   = "---\nname: principle-encode-lessons-in-structure\ndescription: Encode lessons in structure.\n---\n# Encode lessons\n\nThe opposite of [Build the Lever](../principle-build-the-lever/SKILL.md).\n"
	stepsDocument = "# Steps\n\nUse [the shared checklist](../../../../docs/checklist.md).\n"
	checklistDoc  = "# Checklist\n\n![The flow](images/flow.png)\n\nBack to [Redesign](../.trailguide/skills/principle-redesign-from-first-principles/SKILL.md#redesign).\n"
)

// referenceFiles returns a library's files, by repository path as gitfixture.Library takes them, for the
// Greenfield regressions: two skills a project selects, the two they compare themselves with, a reference document,
// and a shared checklist and image outside .trailguide/.
func referenceFiles() map[string][]byte {
	return map[string][]byte{
		"config.yaml":          []byte(`{"schemaVersion":2,"sources":{},"library":{"schemaVersion":1}}`),
		"techs/go/_group.yaml": []byte(projectMetadata),
		"techs/go/errors.md":   []byte(projectRule),
		".trailguide/skills/principle-attack-the-premise/SKILL.md":                        []byte(attackSkill),
		".trailguide/skills/principle-build-the-lever/SKILL.md":                           []byte(leverSkill),
		".trailguide/skills/principle-redesign-from-first-principles/SKILL.md":            []byte(redesignSkill),
		".trailguide/skills/principle-redesign-from-first-principles/references/steps.md": []byte(stepsDocument),
		".trailguide/skills/principle-encode-lessons-in-structure/SKILL.md":               []byte(encodeSkill),
		"docs/checklist.md":    []byte(checklistDoc),
		"docs/images/flow.png": []byte("\x89PNG flow"),
		"docs/unreferenced.md": []byte("# Not linked\n"),
	}
}

// referenceLibrary returns a library whose release/1 publishes files, as referenceFiles returns them.
func referenceLibrary(t *testing.T, files map[string][]byte) *gitfixture.Fixture {
	t.Helper()
	ctx := context.Background()
	f, err := gitfixture.New(ctx, gitfixture.Library(files))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	if err := f.Release(ctx, 1, firstRelease); err != nil {
		t.Fatal(err)
	}
	return f
}

// selectedPrinciples is a configuration that installs the two Greenfield skills, with fields added to the source.
func selectedPrinciples(fields string) string {
	return `{"schemaVersion":2,"skillTargets":["claude","agents"],"sources":{"team":{"repository":REPOSITORY,"skills":{"include":["skills/principle-attack-the-premise","skills/principle-build-the-lever"]` + fields + `}}}}`
}

// linkedFile returns the path, from the project's repository root, of the file that the link with text label in
// the file at from, also a path from the root, names, failing the test when the file has no such link or its
// destination isn't a file.
func linkedFile(t *testing.T, options Options, from, label string) string {
	t.Helper()
	text := readRepositoryFile(t, options, from)
	_, rest, found := strings.Cut(text, "["+label+"](")
	destination, _, closed := strings.Cut(rest, ")")
	if !found || !closed {
		t.Fatalf("%s has no link [%s]:\n%s", from, label, text)
	}
	destination, _, _ = strings.Cut(destination, "#")
	target := path.Join(path.Dir(from), strings.ReplaceAll(destination, "%20", " "))
	if info, err := os.Stat(filepath.Join(options.Directory, filepath.FromSlash(target))); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("%s links [%s] to %s, which isn't a file: %v", from, label, destination, err)
	}
	return target
}

// projectBytes returns every file under the project's repository root, by path, with its mode and contents.
func projectBytes(t *testing.T, options Options) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(options.Directory, func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		relative, _ := filepath.Rel(options.Directory, name)
		files[filepath.ToSlash(relative)] = info.Mode().String() + "\n" + string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// requireReferenceFailure checks that err is a *skillrefs.Error with reason whose message has each of want, and
// that the project's files are still before.
func requireReferenceFailure(t *testing.T, options Options, err error, reason string, before map[string]string, want ...string) {
	t.Helper()
	var failure *skillrefs.Error
	if !errors.As(err, &failure) || failure.Reason != reason {
		t.Fatalf("got %v; want a %s failure", err, reason)
	}
	for _, text := range want {
		if !strings.Contains(err.Error(), text) {
			t.Errorf("the failure lacks %q:\n%s", text, err)
		}
	}
	if after := projectBytes(t, options); !maps.Equal(before, after) {
		t.Error("the failed command changed the project's files")
	}
}

func TestSync_KeepsTheGreenfieldComparisonLinksReadable(t *testing.T) {
	ctx := context.Background()
	f := referenceLibrary(t, referenceFiles())
	options, git := skillProject(t, f, selectedPrinciples(""))
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{".claude/skills", ".agents/skills"} {
		redesign := linkedFile(t, options, target+"/principle-attack-the-premise/SKILL.md", "Redesign from First Principles")
		steps := linkedFile(t, options, redesign, "the steps")
		if again := linkedFile(t, options, redesign, "again"); again != steps {
			t.Errorf("two links to one file reach %s and %s", steps, again)
		}
		checklist := linkedFile(t, options, steps, "the shared checklist")
		if readRepositoryFile(t, options, checklist) == "" || linkedFile(t, options, checklist, "Redesign") != redesign {
			t.Errorf("the shared checklist doesn't link back to the redesign skill's copy")
		}
		linkedFile(t, options, checklist, "The flow")
		encode := linkedFile(t, options, target+"/principle-build-the-lever/SKILL.md", "Encode Lessons in Structure")
		// The cycle back to an installed skill reaches its installed copy.
		if lever := linkedFile(t, options, encode, "Build the Lever"); lever != ".agents/skills/principle-build-the-lever/SKILL.md" {
			t.Errorf("the reference copy of encode-lessons links to %s, want the installed build-the-lever", lever)
		}
		installed := readRepositoryFile(t, options, target+"/principle-attack-the-premise/SKILL.md")
		for _, kept := range []string{"`cat ../principle-missing/SKILL.md`", "cat [a path](../principle-also-missing/SKILL.md)", "[the essay](https://example.invalid/private/essay.md)"} {
			if !strings.Contains(installed, kept) {
				t.Errorf("the installed copy changed %s:\n%s", kept, installed)
			}
		}
	}
	if !strings.Contains(readRepositoryFile(t, options, ".trailguide/_managed/generated/references/team/skills/principle-redesign-from-first-principles/SKILL.md"), "](references/steps.md#first-step)") {
		t.Error("the reference copy lost the fragment of a link that needed no rewriting")
	}
	for _, name := range []string{"principle-redesign-from-first-principles", "principle-encode-lessons-in-structure"} {
		if exists(t, options, ".claude/skills/"+name) || exists(t, options, ".agents/skills/"+name) || strings.Contains(readRepositoryFile(t, options, ".trailguide/_managed/generated/SKILLS.md"), name) {
			t.Errorf("%s, linked only for reading, was installed as a skill", name)
		}
	}
	references := []string{}
	err := filepath.WalkDir(filepath.Join(options.Directory, ".trailguide/_managed/generated/references"), func(name string, entry fs.DirEntry, err error) error {
		if err == nil && !entry.IsDir() {
			relative, _ := filepath.Rel(filepath.Join(options.Directory, ".trailguide/_managed/generated/references"), name)
			references = append(references, filepath.ToSlash(relative))
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"README.md", "team/_repository/docs/checklist.md", "team/_repository/docs/images/flow.png", "team/skills/principle-encode-lessons-in-structure/SKILL.md", "team/skills/principle-redesign-from-first-principles/SKILL.md", "team/skills/principle-redesign-from-first-principles/references/steps.md"}; !slices.Equal(references, want) {
		t.Errorf("reference copies = %v, want %v", references, want)
	}
	entry := lockedEntry(t, options, "team")
	if entry.Files["_repository/docs/checklist.md"] == "" || entry.Files["_repository/docs/unreferenced.md"] != "" || len(entry.Skills) != 2 {
		t.Errorf("lock entry = %+v; want the linked files retained beside the two installed skills", entry)
	}
	before := projectBytes(t, options)
	for range 2 {
		if changes, err := Build(ctx, options); err != nil || len(changes.Added)+len(changes.Changed)+len(changes.Removed) != 0 {
			t.Fatalf("build from the retained input = %+v, %v; want no changes", changes, err)
		}
		if result, err := Check(ctx, options); err != nil || !result.Current() {
			t.Fatalf("check = %+v, %v", result, err)
		}
	}
	if !maps.Equal(before, projectBytes(t, options)) {
		t.Error("build or check changed files")
	}
}

func TestSync_FailsWithoutChangingFilesForALinkItCantDeliver(t *testing.T) {
	for name, test := range map[string]struct {
		files  map[string][]byte
		fields string
		reason string
		want   []string
	}{
		"missing destination": {map[string][]byte{".trailguide/skills/principle-redesign-from-first-principles/references/steps.md": []byte("See [the gone file](gone.md).\n")}, "", skillrefs.Missing, []string{
			"sources.team: .trailguide/skills/principle-redesign-from-first-principles/references/steps.md: links to gone.md",
			"The link is reached through .trailguide/skills/principle-attack-the-premise/SKILL.md -> .trailguide/skills/principle-redesign-from-first-principles/SKILL.md -> .trailguide/skills/principle-redesign-from-first-principles/references/steps.md.",
		}},
		"destination outside the repository": {map[string][]byte{"docs/checklist.md": []byte("[up](../../outside.md)\n")}, "", skillrefs.Escapes, []string{"outside the repository"}},
		"folder":                             {map[string][]byte{"docs/checklist.md": []byte("[images](images)\n")}, "", skillrefs.Unsupported, []string{"is a folder"}},
		"rule file":                          {map[string][]byte{"docs/checklist.md": []byte("[rule](../.trailguide/rules/techs/go/errors.md)\n")}, "", skillrefs.Unsupported, []string{"trailguide-rules-used"}},
		"missing file an excluded skill links to": {map[string][]byte{".trailguide/skills/principle-redesign-from-first-principles/references/steps.md": []byte("See [the gone file](gone.md).\n")}, `,"exclude":[{"id":"skills/principle-redesign-from-first-principles","reason":"We redesign our own way."}]`, skillrefs.Missing, []string{
			".trailguide/skills/principle-redesign-from-first-principles/references/steps.md: links to gone.md",
		}},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			files := referenceFiles()
			maps.Copy(files, test.files)
			f := referenceLibrary(t, files)
			options, git := skillProject(t, f, `{"schemaVersion":2,"skillTargets":["claude"],"sources":{"team":{"repository":REPOSITORY,"skills":{"include":["skills/principle-build-the-lever"]}}}}`)
			if _, err := Sync(ctx, options, git); err != nil {
				t.Fatal(err)
			}
			include := `"skills/principle-attack-the-premise","skills/principle-build-the-lever"`
			if test.fields != "" {
				include = `"*"`
			}
			root, err := openProject(ctx, options, false)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			writeFixture(t, root, configurationFile, strings.ReplaceAll(`{"schemaVersion":2,"skillTargets":["claude"],"sources":{"team":{"repository":REPOSITORY,"ref":"release/1","skills":{"include":[`+include+`]`+test.fields+`}}}}`, "REPOSITORY", quoteJSON(f.Repository)))
			before := projectBytes(t, options)
			_, err = Sync(ctx, options, git)
			requireReferenceFailure(t, options, err, test.reason, before, test.want...)
		})
	}
}

func TestSync_KeepsWhatIncludedSkillsLinkToInAnExcludedSkillForReading(t *testing.T) {
	const redesign = "skills/principle-redesign-from-first-principles"
	for name, test := range map[string]struct {
		levels, include, exclude string
	}{
		// An exclusion by ID names a skill the source's include selects.
		"excluded by ID":    {"", `"*"`, `[{"id":"` + redesign + `","reason":"We redesign our own way."}]`},
		"excluded by level": {`,"skills":{"required":["skills/principle-attack-the-premise"],"recommended":["` + redesign + `"]}`, `"skills/principle-attack-the-premise","skills/principle-build-the-lever"`, `[{"id":"recommended","reason":"Not yet."}]`},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			files := referenceFiles()
			files["config.yaml"] = []byte(`{"schemaVersion":2,"sources":{},"library":{"schemaVersion":1` + test.levels + `}}`)
			// Build the Lever, and Encode Lessons when it's installed too, reach Redesign only through one document,
			// which links to it twice.
			files[".trailguide/skills/principle-build-the-lever/SKILL.md"] = []byte(leverSkill + "\nRead [the comparison](../../../docs/compare.md).\n")
			files[".trailguide/skills/principle-encode-lessons-in-structure/SKILL.md"] = []byte(encodeSkill + "\nRead [the comparison](../../../docs/compare.md).\n")
			files["docs/compare.md"] = []byte("# Compare\n\nSee [Redesign](../.trailguide/" + redesign + "/SKILL.md) and [its steps](../.trailguide/" + redesign + "/references/steps.md).\n")
			f := referenceLibrary(t, files)
			options, git := skillProject(t, f, `{"schemaVersion":2,"skillTargets":["claude","agents"],"sources":{"team":{"repository":REPOSITORY,"skills":{"include":[`+test.include+`],"exclude":`+test.exclude+`}}}}`)
			changes, err := Sync(ctx, options, git)
			if err != nil {
				t.Fatal(err)
			}
			want := []build.ExcludedSkillLink{
				{Skill: "team:skills/principle-attack-the-premise", Excluded: "team:" + redesign, Chain: []string{".trailguide/skills/principle-attack-the-premise/SKILL.md", ".trailguide/" + redesign + "/SKILL.md"}},
				{Skill: "team:skills/principle-build-the-lever", Excluded: "team:" + redesign, Chain: []string{".trailguide/skills/principle-build-the-lever/SKILL.md", "docs/compare.md", ".trailguide/" + redesign + "/SKILL.md"}},
			}
			if test.include == `"*"` {
				want = append(want, build.ExcludedSkillLink{Skill: "team:skills/principle-encode-lessons-in-structure", Excluded: "team:" + redesign, Chain: []string{".trailguide/skills/principle-encode-lessons-in-structure/SKILL.md", "docs/compare.md", ".trailguide/" + redesign + "/SKILL.md"}})
			}
			if !reflect.DeepEqual(changes.ExcludedSkillLinks, want) {
				t.Errorf("excluded skill links = %+v, want %+v", changes.ExcludedSkillLinks, want)
			}
			copy := ".trailguide/_managed/generated/references/team/" + redesign + "/SKILL.md"
			for _, target := range []string{".claude/skills", ".agents/skills"} {
				if linked := linkedFile(t, options, target+"/principle-attack-the-premise/SKILL.md", "Redesign from First Principles"); linked != copy {
					t.Errorf("%s's attack links to %s, want the reading copy %s", target, linked, copy)
				}
				linkedFile(t, options, copy, "the steps")
				if exists(t, options, target+"/principle-redesign-from-first-principles") {
					t.Errorf("%s installs the excluded skill", target)
				}
			}
			// SKILLS.md lists the skill among the excluded ones only, and the record of installed skills not at all.
			if index := readRepositoryFile(t, options, ".trailguide/_managed/generated/SKILLS.md"); strings.Contains(index, "### principle-redesign-from-first-principles") {
				t.Errorf("SKILLS.md lists the excluded skill as available:\n%s", index)
			}
			if record := readRepositoryFile(t, options, ".trailguide/_managed/generated/"+build.InstalledSkillsFile); strings.Contains(record, redesign) {
				t.Errorf("the installed skills record lists the excluded skill:\n%s", record)
			}
			before := projectBytes(t, options)
			built, err := Build(ctx, options)
			if err != nil || len(built.Added)+len(built.Changed)+len(built.Removed) != 0 || !reflect.DeepEqual(built.ExcludedSkillLinks, want) {
				t.Fatalf("build = %+v, %v; want no changes and the same excluded skill links", built, err)
			}
			if result, err := Check(ctx, options); err != nil || !result.Current() {
				t.Fatalf("check = %+v, %v", result, err)
			}
			if !maps.Equal(before, projectBytes(t, options)) {
				t.Error("build or check changed files")
			}
		})
	}
}

func TestSync_ReportsASkillThatUsesAndLinksToAnExcludedSkillOnce(t *testing.T) {
	ctx := context.Background()
	files := referenceFiles()
	files[".trailguide/skills/principle-attack-the-premise/SKILL.md"] = []byte(strings.Replace(attackSkill, "description: Attack the premise.\n", "description: Attack the premise.\nmetadata:\n  trailguide-skills-used: |-\n    skills/principle-redesign-from-first-principles\n", 1))
	f := referenceLibrary(t, files)
	options, git := skillProject(t, f, `{"schemaVersion":2,"skillTargets":["claude"],"sources":{"team":{"repository":REPOSITORY,"skills":{"include":["*"],"exclude":[{"id":"skills/principle-redesign-from-first-principles","reason":"Ours."}]}}}}`)
	changes, err := Sync(ctx, options, git)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes.ExcludedSkillLinks) != 1 || !changes.ExcludedSkillLinks[0].Uses {
		t.Errorf("excluded skill links = %+v; want one that also uses the skill", changes.ExcludedSkillLinks)
	}
	for _, warning := range changes.Warnings {
		if strings.Contains(warning, "principle-redesign-from-first-principles") {
			t.Errorf("the link's report doesn't replace the use's warning: %s", warning)
		}
	}
}

func TestSync_LinksToTheForkThatReplacesALinkedSkill(t *testing.T) {
	ctx := context.Background()
	files := referenceFiles()
	files[".trailguide/skills/principle-attack-the-premise/SKILL.md"] = []byte(attackSkill + "\nAnd [its steps](../principle-redesign-from-first-principles/references/steps.md).\n")
	f := referenceLibrary(t, files)
	commit, err := f.Command(ctx, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	options, git := skillProject(t, f, `{"schemaVersion":2,"skillTargets":["claude"],"sources":{"team":{"repository":REPOSITORY,"skills":{"include":["skills/principle-attack-the-premise"],"forks":{"skills/principle-redesign-from-first-principles":{"as":"skills/principle-redesign-from-first-principles","basedOn":"`+strings.TrimSpace(commit)+`","reason":"Ours."}}}}}}`)
	root, err := openProject(ctx, options, false)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	writeFixture(t, root, "skills/principle-redesign-from-first-principles/SKILL.md", "---\nname: principle-redesign-from-first-principles\ndescription: Our redesign.\n---\n# Our redesign\n")
	before := projectBytes(t, options)
	_, err = Sync(ctx, options, git)
	requireReferenceFailure(t, options, err, skillrefs.ReplacementMissing, before, "which the project forks as skills/principle-redesign-from-first-principles, and the fork has no file at that path")
	writeFixture(t, root, "skills/principle-redesign-from-first-principles/references/steps.md", "# Our steps\n")
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	if redesign := linkedFile(t, options, ".claude/skills/principle-attack-the-premise/SKILL.md", "Redesign from First Principles"); redesign != ".claude/skills/principle-redesign-from-first-principles/SKILL.md" || !strings.Contains(readRepositoryFile(t, options, redesign), "Our redesign") {
		t.Errorf("the link reaches %s, want the installed fork", redesign)
	}
	if steps := linkedFile(t, options, ".claude/skills/principle-attack-the-premise/SKILL.md", "its steps"); readRepositoryFile(t, options, steps) != "# Our steps\n" {
		t.Errorf("the link reaches %s, want the fork's file", steps)
	}
}

func TestSync_KeepsLinksBetweenRenamedSkillsInEveryTarget(t *testing.T) {
	ctx := context.Background()
	f := referenceLibrary(t, referenceFiles())
	options, git := skillProject(t, f, `{"schemaVersion":2,"skillTargets":["claude","agents"],"sources":{"team":{"repository":REPOSITORY,"skills":{"include":["skills/principle-build-the-lever","skills/principle-encode-lessons-in-structure"],"rename":{"skills/principle-build-the-lever":"lever","skills/principle-encode-lessons-in-structure":"encode"}}}}}`)
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{".claude/skills", ".agents/skills"} {
		if encode := linkedFile(t, options, target+"/lever/SKILL.md", "Encode Lessons in Structure"); encode != target+"/encode/SKILL.md" {
			t.Errorf("lever links to %s", encode)
		}
		if lever := linkedFile(t, options, target+"/encode/SKILL.md", "Build the Lever"); lever != target+"/lever/SKILL.md" {
			t.Errorf("encode links to %s", lever)
		}
	}
	if exists(t, options, ".trailguide/_managed/generated/references") {
		t.Error("links between installed skills got reference copies")
	}
}

func TestSync_ResolvesEachSourcesLinksInItsOwnRepository(t *testing.T) {
	ctx := context.Background()
	sources := map[string]*gitfixture.Fixture{}
	for _, name := range []string{"one", "two"} {
		f, err := gitfixture.New(ctx, map[string][]byte{
			"skills/a/SKILL.md": []byte("---\nname: a\ndescription: A.\n---\nSee [B](../b/SKILL.md).\n"),
			"skills/b/SKILL.md": []byte("---\nname: b\ndescription: B from " + name + ".\n---\n# B from " + name + "\n"),
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = f.Close() })
		sources[name] = f
	}
	environment, err := sources["one"].Route(map[string]*gitfixture.Fixture{"one": sources["one"], "two": sources["two"]})
	if err != nil {
		t.Fatal(err)
	}
	git := imports.Options{GitPath: sources["one"].GitPath, Environment: environment}
	root := openTestProject(t)
	options := Options{Directory: filepath.Dir(root.Name()), ToolVersion: "1.2.3"}
	if _, err := Initialize(ctx, options); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, root, configurationFile, `{"schemaVersion":2,"skillTargets":["claude"],"sources":{"one":{"repository":"git@fixture.invalid:one","ref":"v1.0.0","skills":{"include":["skills/a"],"rename":{"skills/a":"one-a"}}},"two":{"repository":"git@fixture.invalid:two","ref":"v1.0.0","skills":{"include":["skills/a"],"rename":{"skills/a":"two-a"}}}}}`)
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"one", "two"} {
		b := linkedFile(t, options, ".claude/skills/"+name+"-a/SKILL.md", "B")
		if text := readRepositoryFile(t, options, b); !strings.Contains(text, "# B from "+name) {
			t.Errorf("%s's link reaches %s:\n%s", name, b, text)
		}
	}
}

func TestSync_ResolvesACuratedSkillsLinksInTheRepositoryItCameFrom(t *testing.T) {
	ctx := context.Background()
	pocock, err := gitfixture.New(ctx, map[string][]byte{
		"skills/engineering/to-spec/SKILL.md":   []byte("---\nname: to-spec\ndescription: Write a spec.\n---\nCompare [grilling](../../productivity/grilling/SKILL.md).\n"),
		"skills/productivity/grilling/SKILL.md": []byte("---\nname: grilling\ndescription: Grill.\n---\n# Grilling from pocock\n"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pocock.Close() })
	if _, err := pocock.Command(ctx, "tag", "stable", pocock.FirstCommit); err != nil {
		t.Fatal(err)
	}
	team, err := gitfixture.New(ctx, gitfixture.Library(map[string][]byte{
		"config.yaml":          []byte("schemaVersion: 2\nsources:\n  pocock:\n    repository: git@fixture.invalid:pocock\n    ref: stable\nlibrary:\n  schemaVersion: 1\n  skills:\n    offered: [pocock:skills/engineering/to-spec]\n"),
		"techs/go/_group.yaml": []byte(projectMetadata),
		"techs/go/errors.md":   []byte(projectRule),
		// The curating library's own layout has a file at the same relative path, which must not satisfy the link.
		".trailguide/skills/productivity/grilling/SKILL.md": []byte("---\nname: grilling\ndescription: Not this one.\n---\n"),
	}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = team.Close() })
	if err := team.Release(ctx, 1, firstRelease); err != nil {
		t.Fatal(err)
	}
	environment, err := team.Route(map[string]*gitfixture.Fixture{"team": team, "pocock": pocock})
	if err != nil {
		t.Fatal(err)
	}
	git := imports.Options{GitPath: team.GitPath, Environment: environment}
	root := openTestProject(t)
	options := Options{Directory: filepath.Dir(root.Name()), ToolVersion: "1.2.3"}
	if _, err := Initialize(ctx, options); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, root, configurationFile, `{"schemaVersion":2,"skillTargets":["claude"],"sources":{"team":{"repository":"git@fixture.invalid:team","ref":"release/1","skills":{"include":["pocock:skills/engineering/to-spec"]}}}}`)
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	grilling := linkedFile(t, options, ".claude/skills/to-spec/SKILL.md", "grilling")
	if grilling != ".trailguide/_managed/generated/references/team/curated/pocock/skills/productivity/grilling/SKILL.md" || !strings.Contains(readRepositoryFile(t, options, grilling), "# Grilling from pocock") {
		t.Errorf("the curated skill's link reaches %s", grilling)
	}
	if result, err := Check(ctx, options); err != nil || !result.Current() {
		t.Fatalf("check = %+v, %v", result, err)
	}
}

func TestBuild_UsesTheRecordedRevisionAfterTheSourceBranchMoves(t *testing.T) {
	ctx := context.Background()
	f, err := gitfixture.New(ctx, map[string][]byte{
		"skills/a/SKILL.md":  []byte("---\nname: a\ndescription: A.\n---\nSee [the notes](../../docs/notes.md).\n"),
		"docs/notes.md":      []byte("# First notes\n"),
		"docs/unrelated.txt": []byte("x\n"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	options, git := skillProject(t, f, `{"schemaVersion":2,"skillTargets":["claude"],"sources":{"skills":{"repository":REPOSITORY,"ref":"main","skills":{"include":["skills/a"]}}}}`)
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Commit(ctx, f.Worktree(), "Rewrite the notes", map[string][]byte{"docs/notes.md": []byte("# Second notes\n")}); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if changes, err := Build(ctx, options); err != nil || len(changes.Added)+len(changes.Changed)+len(changes.Removed) != 0 {
			t.Fatalf("build = %+v, %v", changes, err)
		}
		if result, err := Check(ctx, options); err != nil || !result.Current() {
			t.Fatalf("check = %+v, %v", result, err)
		}
	}
	if notes := linkedFile(t, options, ".claude/skills/a/SKILL.md", "the notes"); readRepositoryFile(t, options, notes) != "# First notes\n" {
		t.Errorf("the offline build read %s from somewhere other than the recorded revision", notes)
	}
	plan, err := PlanUpdate(ctx, options, git, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plan.Apply(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if notes := linkedFile(t, options, ".claude/skills/a/SKILL.md", "the notes"); readRepositoryFile(t, options, notes) != "# Second notes\n" {
		t.Errorf("the update didn't adopt the branch's new notes")
	}
}

func TestCheck_ReportsAChangedOrMissingReferenceCopy(t *testing.T) {
	ctx := context.Background()
	f := referenceLibrary(t, referenceFiles())
	options, git := skillProject(t, f, selectedPrinciples(""))
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	copy := filepath.Join(options.Directory, ".trailguide/_managed/generated/references/team/_repository/docs/checklist.md")
	if err := os.Remove(copy); err != nil {
		t.Fatal(err)
	}
	result, err := Check(ctx, options)
	if err != nil || result.Current() || !slices.ContainsFunc(result.Problems, func(problem Problem) bool {
		return strings.HasSuffix(problem.Path, "references/team/_repository/docs/checklist.md")
	}) {
		t.Fatalf("check after deleting a reference copy = %+v, %v", result, err)
	}
	if _, err := Build(ctx, options); err != nil {
		t.Fatal(err)
	}
	vendored := filepath.Join(options.Directory, ".trailguide/_managed/vendor/team/_repository/docs/checklist.md")
	if err := os.WriteFile(vendored, []byte("# Edited\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Check(ctx, options); err == nil || !strings.Contains(err.Error(), "missing or modified imported content") {
		t.Fatalf("check after editing the vendored file = %v", err)
	}
}

func TestBuild_AsksForAnExplicitSyncWhenAnOlderSnapshotLacksLinkedFiles(t *testing.T) {
	ctx := context.Background()
	f := referenceLibrary(t, referenceFiles())
	options, git := skillProject(t, f, selectedPrinciples(""))
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	// Take the project back to what a Trail Guide that kept only the installed skills' folders recorded.
	lockPath := filepath.Join(options.Directory, ".trailguide", lockFile)
	data, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	lock, err := trailguide.ParseLock(data)
	if err != nil {
		t.Fatal(err)
	}
	entry := lock.Sources["team"]
	for file := range entry.Files {
		if !strings.HasPrefix(file, "skills/principle-attack-the-premise/") && !strings.HasPrefix(file, "skills/principle-build-the-lever/") && file != "config.yaml" {
			delete(entry.Files, file)
			if err := os.Remove(filepath.Join(options.Directory, ".trailguide/_managed/vendor/team", filepath.FromSlash(file))); err != nil {
				t.Fatal(err)
			}
		}
	}
	lock.Sources["team"] = entry
	if data, err = lock.Encode(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lockPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	before := projectBytes(t, options)
	_, err = Build(ctx, options)
	requireReferenceFailure(t, options, err, skillrefs.NotRetained, before, "Run trailguide project sync", "sources.team: .trailguide/skills/principle-attack-the-premise/SKILL.md")
	if _, err := Check(ctx, options); err == nil {
		t.Fatal("check passed a snapshot that lacks linked files")
	}
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if result, err := Check(ctx, options); err != nil || !result.Current() {
			t.Fatalf("check after the refresh = %+v, %v", result, err)
		}
	}
}

func TestSync_FailsWithoutChangingFilesForALinkedFileOverTheImportLimit(t *testing.T) {
	ctx := context.Background()
	files := referenceFiles()
	files["docs/images/flow.png"] = make([]byte, 9<<20)
	f := referenceLibrary(t, files)
	options, git := skillProject(t, f, selectedPrinciples(""))
	before := projectBytes(t, options)
	_, err := Sync(ctx, options, git)
	if err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("got %v; want the import limit", err)
	}
	if !maps.Equal(before, projectBytes(t, options)) {
		t.Error("the failed sync changed files")
	}
}

func TestSync_KeepsLinkedFilesInRootFoldersNamedLikeTheLibrarysOwn(t *testing.T) {
	ctx := context.Background()
	files := gitfixture.Library(referenceFiles())
	files[".trailguide/skills/principle-attack-the-premise/SKILL.md"] = []byte("---\nname: principle-attack-the-premise\ndescription: Attack the premise.\n---\nRead [the notes](../../../assets/notes.md), [the guide](../../../skills/guide.md), [the rules](../../../rules/README.md), [the config](../../../config.yaml), and [the shared asset](../../assets/notes.md).\n")
	files["assets/notes.md"] = []byte("# Notes at the repository root\n")
	files["skills/guide.md"] = []byte("# A guide at the repository root\n")
	files["rules/README.md"] = []byte("# Rules at the repository root\n")
	files["config.yaml"] = []byte("root: true\n")
	files[".trailguide/assets/notes.md"] = []byte("# The library's shared notes\n")
	f, err := gitfixture.New(ctx, files)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	if err := f.Release(ctx, 1, firstRelease); err != nil {
		t.Fatal(err)
	}
	options, git := skillProject(t, f, selectedPrinciples(""))
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	for label, want := range map[string]string{"the notes": "# Notes at the repository root\n", "the guide": "# A guide at the repository root\n", "the rules": "# Rules at the repository root\n", "the config": "root: true\n", "the shared asset": "# The library's shared notes\n"} {
		for _, target := range []string{".claude/skills", ".agents/skills"} {
			if got := readRepositoryFile(t, options, linkedFile(t, options, target+"/principle-attack-the-premise/SKILL.md", label)); got != want {
				t.Errorf("%s in %s reaches %q, want %q", label, target, got, want)
			}
		}
	}
	if result, err := Check(ctx, options); err != nil || !result.Current() {
		t.Fatalf("check = %+v, %v", result, err)
	}
}

func TestSync_FollowsTheLinksOfAForkThatReplacesALinkedSkill(t *testing.T) {
	ctx := context.Background()
	f := referenceLibrary(t, referenceFiles())
	commit, err := f.Command(ctx, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	options, git := skillProject(t, f, `{"schemaVersion":2,"skillTargets":["claude","agents"],"sources":{"team":{"repository":REPOSITORY,"skills":{"include":["skills/principle-attack-the-premise"],"forks":{"skills/principle-redesign-from-first-principles":{"as":"skills/our-redesign","basedOn":"`+strings.TrimSpace(commit)+`","reason":"Ours."}}}}}}`)
	root, err := openProject(ctx, options, false)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	writeFixture(t, root, "skills/our-redesign/SKILL.md", "---\nname: our-redesign\ndescription: Our redesign.\n---\n# Our redesign\n\nRead [the notes](missing.md).\n")
	before := projectBytes(t, options)
	_, err = Sync(ctx, options, git)
	requireReferenceFailure(t, options, err, skillrefs.Missing, before, ".trailguide/skills/our-redesign/SKILL.md: links to missing.md", "isn't in the project")
	writeFixture(t, root, "skills/our-redesign/SKILL.md", "---\nname: our-redesign\ndescription: Our redesign.\n---\n# Our redesign\n\nRead [the notes](../../../docs/notes.md) and [the asset](../../assets/flow.svg).\n")
	writeFixture(t, root, "assets/flow.svg", "<svg/>\n")
	if err := os.MkdirAll(filepath.Join(options.Directory, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(options.Directory, "docs/notes.md"), []byte("# Our notes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{".claude/skills", ".agents/skills"} {
		redesign := linkedFile(t, options, target+"/principle-attack-the-premise/SKILL.md", "Redesign from First Principles")
		if !strings.HasSuffix(redesign, "skills/our-redesign/SKILL.md") || !strings.Contains(readRepositoryFile(t, options, redesign), "Our redesign") {
			t.Errorf("the link reaches %s, want the installed fork", redesign)
		}
		if notes := linkedFile(t, options, target+"/our-redesign/SKILL.md", "the notes"); notes != "docs/notes.md" {
			t.Errorf("the fork's link in %s reaches %s, want the project's docs/notes.md", target, notes)
		}
		if asset := linkedFile(t, options, target+"/our-redesign/SKILL.md", "the asset"); asset != ".trailguide/assets/flow.svg" {
			t.Errorf("the fork's link in %s reaches %s, want the project's shared asset", target, asset)
		}
	}
	if result, err := Check(ctx, options); err != nil || !result.Current() {
		t.Fatalf("check = %+v, %v", result, err)
	}
	if err := os.Remove(filepath.Join(options.Directory, "docs/notes.md")); err != nil {
		t.Fatal(err)
	}
	var failure *skillrefs.Error
	if _, err := Check(ctx, options); !errors.As(err, &failure) || failure.Reason != skillrefs.Missing {
		t.Fatalf("check after the linked file was deleted = %v; want a missing reference", err)
	}
	if _, err := Build(ctx, options); !errors.As(err, &failure) || failure.Reason != skillrefs.Missing {
		t.Fatalf("build after the linked file was deleted = %v; want a missing reference", err)
	}
}

func TestSync_RefusesAnAuthoredLinkToAnInstalledCopyItWouldRemove(t *testing.T) {
	ctx := context.Background()
	f := referenceLibrary(t, referenceFiles())
	options, git := skillProject(t, f, `{"schemaVersion":2,"skillTargets":["claude"],"sources":{"team":{"repository":REPOSITORY,"skills":{"include":["skills/principle-build-the-lever"]}}}}`)
	root, err := openProject(ctx, options, false)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	writeFixture(t, root, "skills/ours/SKILL.md", "---\nname: ours\ndescription: Ours.\n---\nUse [the lever](../../../.claude/skills/principle-build-the-lever/SKILL.md).\n")
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	if lever := linkedFile(t, options, ".claude/skills/ours/SKILL.md", "the lever"); lever != ".claude/skills/principle-build-the-lever/SKILL.md" {
		t.Fatalf("the authored skill links to %s", lever)
	}
	writeFixture(t, root, "config.yaml", `{"schemaVersion":2,"skillTargets":["claude"],"sources":{}}`)
	before := projectBytes(t, options)
	_, err = Sync(ctx, options, git)
	requireReferenceFailure(t, options, err, skillrefs.Missing, before, ".trailguide/skills/ours/SKILL.md: links to ../../../.claude/skills/principle-build-the-lever/SKILL.md", "among the files Trail Guide installs and generates")
}

func TestSync_KeepsAnAuthoredLinkToTheSkillsIndexWithoutSkillTargets(t *testing.T) {
	ctx := context.Background()
	f := referenceLibrary(t, referenceFiles())
	options, git := skillProject(t, f, `{"schemaVersion":2,"skillTargets":[],"sources":{"team":{"repository":REPOSITORY,"skills":{"include":["skills/principle-build-the-lever"]}}}}`)
	root, err := openProject(ctx, options, false)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	writeFixture(t, root, "skills/ours/SKILL.md", "---\nname: ours\ndescription: Ours.\n---\nPick from [the index](../../_managed/generated/SKILLS.md).\n")
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	if index := readRepositoryFile(t, options, ".trailguide/_managed/generated/SKILLS.md"); !strings.Contains(index, "../vendor/team/") {
		t.Fatalf("the index doesn't link to the vendor snapshot:\n%s", index)
	}
	if _, err := Build(ctx, options); err != nil {
		t.Fatal(err)
	}
	if _, err := Check(ctx, options); err != nil {
		t.Fatal(err)
	}
}

func TestBuild_ChecksAuthoredLinksWithoutSkillTargets(t *testing.T) {
	ctx := context.Background()
	root := openTestProject(t)
	options := Options{Directory: filepath.Dir(root.Name()), ToolVersion: "1.2.3"}
	if _, err := Initialize(ctx, options); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, root, "config.yaml", `{"schemaVersion":2,"skillTargets":[],"sources":{}}`)
	writeFixture(t, root, "skills/ours/SKILL.md", "---\nname: ours\ndescription: Ours.\n---\nRead [the notes](../../../docs/notes.md).\n")
	var failure *skillrefs.Error
	if _, err := Build(ctx, options); !errors.As(err, &failure) || failure.Reason != skillrefs.Missing {
		t.Fatalf("build = %v; want a missing reference", err)
	}
}

func TestRequireLinkedFilesUnchanged_RefusesAChangedOrMissingFile(t *testing.T) {
	ctx := context.Background()
	repository := t.TempDir()
	directory := filepath.Join(repository, ".trailguide")
	if err := os.MkdirAll(filepath.Join(repository, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	notes := filepath.Join(repository, "docs/notes.md")
	if err := os.WriteFile(notes, []byte("# Notes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	linked := map[string][]byte{"docs/notes.md": []byte("# Notes\n")}
	if err := requireLinkedFilesUnchanged(ctx, directory, linked); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func() error{func() error { return os.WriteFile(notes, []byte("# Edited\n"), 0o644) }, func() error { return os.Remove(notes) }} {
		if err := change(); err != nil {
			t.Fatal(err)
		}
		if err := requireLinkedFilesUnchanged(ctx, directory, linked); !code("concurrent-change")(err) {
			t.Errorf("got %v; want concurrent-change", err)
		}
	}
}

// The bytes a reference check validates are the ones the write later requires unchanged, so a file that changes
// after its links were checked fails the write instead of becoming the baseline.
func TestProjectView_KeepsTheBytesItFirstRead(t *testing.T) {
	repository := t.TempDir()
	notes := filepath.Join(repository, "notes.md")
	if err := os.WriteFile(notes, []byte("[ok](ok.md)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(repository)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	view := &projectView{ctx: context.Background(), root: root}
	first, err := view.Read("notes.md")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notes, []byte("[broken](gone.md)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if again, err := view.Read("notes.md"); err != nil || string(again) != string(first) {
		t.Fatalf("a second read = %q, %v; want the first read's %q", again, err, first)
	}
	if err := requireLinkedFilesUnchanged(context.Background(), filepath.Join(repository, ".trailguide"), map[string][]byte{"notes.md": first}); !code("concurrent-change")(err) {
		t.Fatalf("got %v; want concurrent-change for the file that changed after it was checked", err)
	}
}
