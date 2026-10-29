// Package build generates complete rule guidance from validated inputs without filesystem or network access.
package build

import (
	"github.com/fabricahq/trailguide/internal/library"
	"github.com/fabricahq/trailguide/internal/rules"
)

// Library supplies a validated selected catalog and its caller-verified snapshot record: the revision that
// supplied its library-wide files and every skill, each imported rule's version and commit, and each installed
// skill. Of Snapshot.Files, only the skills' files are used. Snapshot freshness and Git authenticity belong to the
// import/snapshot boundary.
type Library struct {
	Catalog  library.Catalog
	Snapshot library.Snapshot
}

// Output owns generated-root-relative file contents, including byte-exact license and notice copies, and the
// installed skill folders.
type Output struct {
	Files map[string][]byte `json:"files"`
	// Skills maps each installed skill folder, by path from the repository root, such as .claude/skills/tdd, to
	// its files. It is empty, never nil, when the project installs no skill or has no skill targets.
	Skills map[string]SkillFiles `json:"skills"`
	// SkillIDs lists, sorted, the project ID of every skill the project installs, including when it has no skill
	// targets to install copies in.
	SkillIDs []string `json:"skillIds"`
	// RetiredCitations reports, as a validation error at the skill's SKILL.md, each rule an authored skill cites that
	// its library retired, in skill order. The output is still complete: build and sync warn about them, and check
	// fails. It is nil when there are none.
	RetiredCitations []*ValidationError `json:"retiredCitations"`
	// ExcludedUses warns, in skill order, about each use of a source's skill that the project excludes without a
	// replacement, which isn't installed. It is empty, never nil, when there are none.
	ExcludedUses []string `json:"excludedUses"`
	// RenamedSkills lists, sorted by project ID, the skills the project installs under a name skills.rename gives
	// them instead of their own; it is empty, never nil, when it renames none.
	RenamedSkills []RenamedSkill `json:"renamedSkills"`
}

// RenamedSkill is a source's skill that the project installs under another name.
type RenamedSkill struct {
	// ID is the skill's project ID, source:folder; Source and Folder are its parts.
	ID, Source, Folder string
	// From is the name the skill's SKILL.md declares, and To the name it's installed under.
	From, To string
}

// Options provides the Markdown line limit and inline byte limit, and the project's authored skills.
type Options struct {
	// IndexMaxLines defaults to 750 when zero; negative limits are invalid.
	IndexMaxLines int
	// GroupInlineMaxBytes defaults to 8 KiB when nil; zero forces summary-only group pages.
	GroupInlineMaxBytes *int
	// LocalSkills holds the project's authored skills by ID, skills/<name>, each with its files.
	LocalSkills map[string]SkillFiles
}

// Generate applies adoption policies and renders the complete generated file tree.
// Configuration and catalogs must be validated; callers verify each library's source identity. localFiles holds the
// project's authored files keyed as a library keys its own: rules, group metadata, and rule assets by their path in
// rules/, and shared assets by their path in the Trail Guide directory, assets/<path>. It installs every skill the
// sources' snapshots record and every authored skill into each skill target, each installed SKILL.md ending with
// links to the project's version of every rule the skill cites. It leaves inputs unchanged, preserves retained
// bytes, and returns no partial output on failure; two skills with one install name fail with *ClashError, and an
// authored skill whose frontmatter or citations are invalid fails with a *ValidationError at its SKILL.md.
func Generate(config rules.Configuration, libraries map[string]Library, localFiles map[string][]byte, options Options) (Output, error) {
	resolved, err := resolve(config, libraries, localFiles)
	if err != nil {
		return Output{}, err
	}
	var excludedUses []string
	if resolved.Skills, excludedUses, resolved.References, err = resolveSkills(config, libraries, options.LocalSkills); err != nil {
		return Output{}, err
	}
	retired, err := citeRules(&resolved, config, libraries)
	if err != nil {
		return Output{}, err
	}
	if options.IndexMaxLines == 0 {
		options.IndexMaxLines = defaultIndexMaxLines
	}
	output, err := prepare(resolved, options)
	if err != nil {
		return Output{}, err
	}
	output.RetiredCitations, output.ExcludedUses = retired, excludedUses
	return output, nil
}
