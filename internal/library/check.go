// Validate all authored library content, including unreferenced assets, without generating or modifying files.

package library

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"iter"
	"maps"
	"os"
	"path"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/fabricahq/trailguide/internal/filetxn"
	"github.com/fabricahq/trailguide/internal/librarytree"
	"github.com/fabricahq/trailguide/internal/rules"
	"github.com/fabricahq/trailguide/internal/skills"
	"golang.org/x/text/unicode/norm"
)

// CheckResult reports complete adoption counts, explicit licensing and change note caveats, and the next library release.
type CheckResult struct {
	GroupCount     int            `json:"groupCount"`
	RuleCount      int            `json:"ruleCount"`
	Warnings       []string       `json:"warnings"`
	PendingRelease PendingRelease `json:"pendingRelease"`
	// ForkedSkills lists, by source and then ID, the skills the library forked from its sources whose copies at the
	// revisions the sources' refs name now differ from the copies the forks are based on; it is empty, never nil,
	// when there are none.
	ForkedSkills []ForkedSkill `json:"forkedSkills"`
}

// Check validates a captured library snapshot, ignoring unrelated files such as .git.
// In a Git repository, it also compares rules with the latest library release reachable from HEAD and
// requires a pending change note for each difference; a shallow clone fails. A library outside Git, or
// before its first library release, needs no notes, and its first library release gives every rule 1.0.0.
// A final comparison rejects observed changes; ordinary editors are not locked out.
func Check(ctx context.Context, options Options) (CheckResult, error) {
	root, err := openLibrary(ctx, options, false)
	if err != nil {
		return CheckResult{}, err
	}
	defer root.Close()
	if err = requireIdle(ctx, root); err != nil {
		return CheckResult{}, err
	}
	git, err := openLibraryGit(ctx, root.Name(), options.Git)
	if err != nil {
		return CheckResult{}, err
	}
	checked, err := checkLibrary(ctx, root, git, options.Curated, options.Forks)
	if err != nil {
		return CheckResult{}, err
	}
	if err = requireLibraryUnchanged(ctx, root, checked.input); err != nil {
		return CheckResult{}, err
	}
	if err = requireIdle(ctx, root); err != nil {
		return CheckResult{}, err
	}
	return checked.result, nil
}

// checkedLibrary is a library that passed library check: what check read, how its rules changed since the
// latest library release, and what the next library release would publish.
type checkedLibrary struct {
	result  CheckResult
	input   checkInput
	changes libraryChanges
	plan    releasePlan
}

// checkLibrary validates the library under root and its change notes against git's library releases; a nil
// git is a library outside Git. curated reads the sources the library curates skills from, or is nil to leave
// them unchecked, and forks compares the skills the library forked from its sources with them, or is nil to leave
// them uncompared. The caller confirms that the library didn't change while it was read.
func checkLibrary(ctx context.Context, root *os.Root, git *libraryGit, curated CuratedReader, forks ForkReader) (checkedLibrary, error) {
	input, err := libraryCheckInput(ctx, root)
	if err != nil {
		return checkedLibrary{}, err
	}
	if err = validateLibraryInventory(ctx, input.tree.Files, rules.LicensePaths(input.license)); err != nil {
		return checkedLibrary{}, err
	}
	if err = requireLeveledSkills(root, input.tree.Files[librarytree.Manifest]); err != nil {
		return checkedLibrary{}, err
	}
	if err = requireCuratedSkills(ctx, input.tree.Files[librarytree.Manifest], input.skills, curated); err != nil {
		return checkedLibrary{}, err
	}
	catalog, err := LoadSource(ctx, capturedLibrary{input.tree}, "library", rules.GroupSelection{Pattern: "*"}, nil)
	if err != nil {
		return checkedLibrary{}, err
	}
	result := CheckResult{GroupCount: len(catalog.Groups), Warnings: []string{}}
	if result.ForkedSkills, err = forkedSkills(ctx, input.tree.Files[librarytree.Manifest], forks); err != nil {
		return checkedLibrary{}, err
	}
	current := []string{}
	for _, group := range catalog.Groups {
		result.RuleCount += len(group.Rules)
		for _, rule := range group.Rules {
			current = append(current, strings.TrimSuffix(rule.Path, ".md"))
		}
	}
	slices.Sort(current)
	changes, warnings, err := git.compare(ctx, input.tree.Files, current, input.notes)
	if err != nil {
		return checkedLibrary{}, err
	}
	if problems := changes.review(); len(problems) > 0 {
		return checkedLibrary{}, failure("change-notes", "change notes don't match the rule changes since "+changes.history.latest.tagName()+":\n  - "+strings.Join(problems, "\n  - "), nil)
	}
	declaration, err := rules.ParseLibraryManifest(input.tree.Files[librarytree.Manifest], manifestPath)
	if err != nil {
		return checkedLibrary{}, err
	}
	curatedSkills := []string{}
	if declaration != nil {
		curatedSkills = declaration.Skills.Curated()
	}
	if problems := skillProblems(input.skills, current, curatedSkills, changes); len(problems) > 0 {
		return checkedLibrary{}, failure("invalid-skill", "skills don't pass library check:\n  - "+strings.Join(problems, "\n  - "), nil)
	}
	plan, err := changes.plan()
	if err != nil {
		return checkedLibrary{}, err
	}
	result.PendingRelease = plan.preview()
	if result.PendingRelease.LibraryFiles, err = git.pendingLibraryFiles(ctx, changes.history.latest, input.tree.Files, rules.LicensePaths(input.license)); err != nil {
		return checkedLibrary{}, err
	}
	if input.license == nil {
		result.Warnings = append(result.Warnings, "License is undeclared. Decide terms before sharing this library.")
	} else if input.license.SPDXExpression == nil {
		result.Warnings = append(result.Warnings, "The license has no SPDX expression. Declare the library terms explicitly.")
	}
	result.Warnings = append(result.Warnings, warnings...)
	if err = ctx.Err(); err != nil {
		return checkedLibrary{}, err
	}
	return checkedLibrary{result: result, input: input, changes: changes, plan: plan}, nil
}

// parseChangeNote validates one note's format.
func parseChangeNote(name string, data []byte) (pendingNote, error) {
	note, err := rules.ParseChangeNote(data, name)
	return pendingNote{path: name, note: note}, err
}

// compare finds the notes added since the latest library release and the rules whose versioned content
// differs from it, with a warning for each published note that was edited or deleted. It validates the format
// of pending notes only: a published note's edits have no effect, so an invalid edit only warns. Before the first
// library release every note is pending. A nil receiver has no history.
func (g *libraryGit) compare(ctx context.Context, files map[string][]byte, current []string, notes map[string][]byte) (libraryChanges, []string, error) {
	history, err := g.history(ctx)
	if err != nil {
		return libraryChanges{}, nil, err
	}
	changes := libraryChanges{history: history, current: current, changed: map[string]bool{}}
	latest := history.latest
	published := map[string]string{}
	if latest != nil {
		published = latest.files
	}
	var warnings []string
	var releasedNotes []string
	for _, name := range slices.Sorted(maps.Keys(notes)) {
		if _, ok := published[name]; ok {
			releasedNotes = append(releasedNotes, name)
			continue
		}
		pending, err := parseChangeNote(name, notes[name])
		if err != nil {
			return libraryChanges{}, nil, err
		}
		if latest != nil {
			changes.pending = append(changes.pending, pending)
		}
	}
	if latest == nil {
		return changes, nil, nil
	}
	working := map[string][]string{}
	for _, id := range current {
		if _, versioned := latest.record.Rules[id]; versioned {
			working[id] = ruleFiles(id, maps.Keys(files))
		}
	}
	if changes.changed, err = g.changedRules(ctx, latest, working); err != nil {
		return libraryChanges{}, nil, err
	}
	hashes, err := g.hashFiles(ctx, releasedNotes)
	if err != nil {
		return libraryChanges{}, nil, err
	}
	for _, name := range releasedNotes {
		if hashes[name] != published[name] {
			warnings = append(warnings, name+" changed after a library release published it. Editing a published note has no effect.")
		}
	}
	for _, name := range slices.Sorted(maps.Keys(latest.files)) {
		if _, kept := notes[name]; strings.HasPrefix(name, changesDirectory+"/") && !kept {
			warnings = append(warnings, name+" was deleted after a library release published it. Notes are never deleted; restore it.")
		}
	}
	return changes, append(warnings, changes.retiredReplacements()...), nil
}

// retiredReplacements warns about each pending retirement of a rule that an earlier library release named as a
// retired rule's replacement: updates point projects still importing the earlier rule on to the replacement the
// note names, or, without one, at a retired rule.
func (c libraryChanges) retiredReplacements() []string {
	_, retiring := c.namedRules()
	var warnings []string
	for _, old := range slices.Sorted(maps.Keys(c.history.replacedBy)) {
		replacement := c.history.replacedBy[old]
		changes := retiring[replacement]
		if len(changes) == 0 {
			continue
		}
		retires := "The pending retirement of " + replacement + " retires the replacement that release/" + strconv.Itoa(c.history.retired[old]) + " named for " + old
		if onward := changes[0].ReplacedBy; onward != "" {
			warnings = append(warnings, retires+", so updates will point projects still importing "+old+" on to "+onward+", the replacement the note names.")
			continue
		}
		warnings = append(warnings, retires+", so projects still importing "+old+" would be pointed at a retired rule. To let them follow it, name a replacement for "+replacement+" as replacedBy in the note that retires it.")
	}
	return warnings
}

// ruleFiles returns a rule's versioned files among names, in sorted order: its Markdown file and its asset directory's files.
func ruleFiles(id string, names iter.Seq[string]) []string {
	assets := rules.RuleAssetDirectory(id + ".md")
	var files []string
	for name := range names {
		if name == id+".md" || strings.HasPrefix(name, assets) {
			files = append(files, name)
		}
	}
	slices.Sort(files)
	return files
}

// validateLibraryInventory catches malformed unused assets, invalid files, draft markers, and missing local destinations.
func validateLibraryInventory(ctx context.Context, files map[string][]byte, terms []string) error {
	spellings := map[string]string{}
	for _, name := range slices.Sorted(maps.Keys(files)) {
		if err := ctx.Err(); err != nil {
			return err
		}
		parts := strings.Split(name, "/")
		for n := 1; n <= len(parts); n++ {
			prefix := strings.Join(parts[:n], "/")
			key := norm.NFC.String(strings.ToUpper(strings.ToLower(norm.NFC.String(prefix))))
			if old, ok := spellings[key]; ok && old != prefix {
				return failure("path-collision", name+": collides with "+old, nil)
			}
			spellings[key] = prefix
		}
		if name == librarytree.Manifest || slices.Contains(terms, name) || librarytree.IsGroupReadme(name) {
			continue
		}
		asset := slices.Contains(parts, "assets")
		if asset {
			directory := rules.AssetDirectory(name)
			if directory == "" {
				return failure("invalid-asset", name+": invalid asset directory", nil)
			}
			if directory != "assets/" {
				ownerParts := strings.Split(strings.TrimSuffix(directory, "/"), "/")
				owner := strings.Join(ownerParts[:len(ownerParts)-2], "/") + "/" + ownerParts[len(ownerParts)-1] + ".md"
				if _, ok := files[owner]; !ok {
					return failure("invalid-asset", name+": missing rule owner "+owner, nil)
				}
			}
		} else {
			if len(parts) < 3 {
				return failure("invalid-library", name+": expected a group document", nil)
			}
			group := strings.Join(parts[:2], "/")
			if err := librarytree.ValidateGroupID(group, name); err != nil {
				return err
			}
			if name != group+"/_group.yaml" {
				if _, err := librarytree.GroupFromPath(name, name); err != nil {
					return failure("invalid-library", name+": use rule Markdown, _group.yaml, or a conventional assets directory", err)
				}
				if rules.HasDraftMarker(files[name]) {
					return failure("incomplete-rule", name+": complete the draft and remove its trailguide:draft marker", nil)
				}
			}
		}
		if strings.HasSuffix(name, ".md") {
			if !utf8.Valid(files[name]) {
				return failure("invalid-library", name+": expected UTF-8 Markdown", nil)
			}
			targets, err := rules.LibraryTargets(string(files[name]), name)
			if err != nil {
				return err
			}
			for _, target := range targets {
				if err = rules.RequireAllowedTarget(name, target, terms); err != nil {
					return err
				}
				if _, ok := files[target]; !ok {
					return failure("missing-link", name+": missing link destination "+target, nil)
				}
			}
		}
		if strings.HasPrefix(strings.ReplaceAll(string(files[name][:min(len(files[name]), 128)]), "\r\n", "\n"), "version https://git-lfs.github.com/spec/v1\n") {
			return failure("invalid-library", path.Clean(name)+": Git LFS pointers are unsupported", nil)
		}
	}
	return nil
}

// checkInput is a bounded snapshot of every library-owned file and change note a complete check considers.
type checkInput struct {
	tree *filetxn.Tree
	// skills is the library's skills/ folder, by path in it, or nil when the library has none.
	skills  *filetxn.Tree
	license *rules.LicenseDeclaration
	// notes maps each change note, by its library path under changes/, to its bytes; notes stay out of catalogs and snapshots.
	notes map[string][]byte
}

// libraryCheckInput captures every manifest, term, library-owned file, and change note considered by a complete check.
func libraryCheckInput(ctx context.Context, root *os.Root) (checkInput, error) {
	inventory, err := readLocalInventory(ctx, root)
	if err != nil {
		return checkInput{}, err
	}
	notes, err := readChangeNotes(ctx, repositoryFiles{rootFiles{ctx: ctx, root: root}})
	if err != nil {
		return checkInput{}, err
	}
	authoredSkills, err := filetxn.ReadTree(ctx, root, skillsDirectory)
	if err != nil {
		return checkInput{}, err
	}
	return checkInput{tree: &filetxn.Tree{Files: inventory.Files, Directories: inventory.Directories}, skills: authoredSkills, license: inventory.License, notes: notes}, nil
}

// requireLibraryUnchanged rejects differences observed after validating the captured snapshot.
func requireLibraryUnchanged(ctx context.Context, root *os.Root, before checkInput) error {
	after, err := libraryCheckInput(ctx, root)
	if err != nil {
		return failure("changed-input", "library changed during validation; retry library check", err)
	}
	if !maps.EqualFunc(before.tree.Files, after.tree.Files, bytes.Equal) || !slices.Equal(before.tree.Directories, after.tree.Directories) || before.skills.Digest() != after.skills.Digest() || !maps.EqualFunc(before.notes, after.notes, bytes.Equal) {
		return failure("changed-input", "library changed during validation; retry library check", nil)
	}
	return nil
}

// requireLeveledSkills fails when manifest, the library's configuration, gives a level to an own skill the library
// doesn't have: a folder in .trailguide/skills/ with a SKILL.md, in the repository at root.
func requireLeveledSkills(root *os.Root, manifest []byte) error {
	declaration, err := rules.ParseLibraryManifest(manifest, manifestPath)
	if err != nil || declaration == nil {
		return err
	}
	for _, id := range declaration.Skills.Listed() {
		// A curated skill is in the repository the library curates it from, which each project resolves for itself.
		if strings.Contains(id, ":") {
			continue
		}
		file := librarytree.Directory + "/" + id + "/" + skills.File
		info, err := root.Lstat(file)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("inspect %s: %v", file, err)
		}
		if err != nil || !info.Mode().IsRegular() {
			return &ValidationError{Location: manifestPath + ": library.skills." + declaration.Skills.Level(id), Problem: id + " names no skill of the library, because " + file + " doesn't exist; add the skill, or remove it from the list"}
		}
	}
	return nil
}
