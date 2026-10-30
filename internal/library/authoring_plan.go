// Plan library authoring with shared live-state checks and no locks held across prompts.

package library

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/fabricahq/trailguide/internal/filetxn"
	"github.com/fabricahq/trailguide/internal/librarytree"
	"github.com/fabricahq/trailguide/internal/rules"
	"github.com/fabricahq/trailguide/internal/skills"
	"github.com/fabricahq/trailguide/trailguide"
)

// GroupPlan identifies a new group in an initialized library and owns no filesystem resources.
type GroupPlan struct {
	id      string
	options Options
}

// RulePlan identifies a new library rule; Commit rechecks that its group still exists.
type RulePlan struct {
	id      string
	options Options
	// released records that a library release was reachable from HEAD when planning.
	released bool
}

// PlanGroup checks the group target before collecting metadata and anchors the library directory.
func PlanGroup(ctx context.Context, id string, options Options) (*GroupPlan, error) {
	options, err := planAuthoring(ctx, options, func(root *os.Root, _ Options) error { return checkNewGroup(ctx, root, id) })
	if err != nil {
		return nil, err
	}
	return &GroupPlan{id, options}, nil
}

// Commit validates metadata and repeats target checks under writer ownership before publishing.
func (p *GroupPlan) Commit(ctx context.Context, metadata trailguide.GroupMetadata) (AuthoringResult, error) {
	if p == nil || p.id == "" {
		return AuthoringResult{}, failure("invalid-operation", "expected a planned group", nil)
	}
	return AddGroup(ctx, p.id, metadata, p.options)
}

// PlanRule checks the path, parent group, and target before collecting rule details. In a Git repository,
// it also finds whether the library has a library release, so the new rule needs a change note; it fails
// in a shallow clone, where that can't be known.
func PlanRule(ctx context.Context, id string, options Options) (*RulePlan, error) {
	released := false
	options, err := planAuthoring(ctx, options, func(root *os.Root, options Options) error {
		if err := checkNewRule(ctx, root, id, options); err != nil {
			return err
		}
		git, err := openLibraryGit(ctx, root.Name(), options.Git)
		if err != nil {
			return err
		}
		tags, err := git.releaseTags(ctx)
		released = len(tags) > 0
		return err
	})
	if err != nil {
		return nil, err
	}
	return &RulePlan{id, options, released}, nil
}

// NeedsChangeNote reports that the library had a library release when planning, so the new rule needs a
// change note before library check passes.
func (p *RulePlan) NeedsChangeNote() bool { return p != nil && p.released }

// Commit revalidates live state before creating a complete rule or marked draft when body is nil.
func (p *RulePlan) Commit(ctx context.Context, metadata rules.RuleMetadata, body *string) (AuthoringResult, error) {
	if p == nil || p.id == "" {
		return AuthoringResult{}, failure("invalid-operation", "expected a planned rule", nil)
	}
	return AddRule(ctx, p.id, metadata, RuleOptions{Options: p.options, Body: body})
}

// planAuthoring retains only an absolute location; no root or lock survives input collection.
func planAuthoring(ctx context.Context, options Options, check func(*os.Root, Options) error) (Options, error) {
	directory, err := filepath.Abs(options.Directory)
	if err != nil {
		return Options{}, err
	}
	options.Directory = directory
	root, err := openLibrary(ctx, options, false)
	if err != nil {
		return Options{}, err
	}
	defer root.Close()
	if _, _, err = libraryManifest(ctx, root); err != nil {
		return Options{}, err
	}
	return options, check(root, options)
}

func checkNewGroup(ctx context.Context, root *os.Root, id string) error {
	if err := librarytree.ValidateGroupID(id, "group"); err != nil {
		return err
	}
	return filetxn.RequireAbsent(ctx, root, librarytree.RepositoryPath(id+"/_group.yaml"), librarytree.RepositoryPath(id+"/README.md"))
}

func checkNewRule(ctx context.Context, root *os.Root, id string, options Options) error {
	if strings.HasSuffix(id, ".md") {
		return failure("invalid-rule-path", "use a rule path without the .md extension", nil)
	}
	group, err := librarytree.GroupFromPath(id+".md", "rule")
	if err != nil {
		return err
	}
	exists, err := hasGroupMetadata(ctx, root, group)
	if err != nil {
		return err
	}
	if !exists {
		directory, err := filepath.Abs(options.Directory)
		if err != nil {
			return err
		}
		command := "trailguide library add group " + group + " --directory='" + strings.ReplaceAll(directory, "'", "'\"'\"'") + "'"
		return failure("missing-group", "group "+group+" does not exist; create it first with "+command+", then retry adding the rule", nil)
	}
	return filetxn.RequireAbsent(ctx, root, librarytree.RepositoryPath(id+".md"))
}

// SkillPlan identifies a new skill in the library's .trailguide/skills/; Commit rechecks that its folder is still
// absent.
type SkillPlan struct {
	name, folder string
	options      Options
	released     bool
}

// PlanSkill checks the skill's name and that the library has no skill folder by that name before collecting the
// skill's description. In a Git repository, it also finds whether the library has a library release, so the new
// skill needs a change note; it fails in a shallow clone, where that can't be known.
func PlanSkill(ctx context.Context, name string, options Options) (*SkillPlan, error) {
	released, folder := false, ""
	options, err := planAuthoring(ctx, options, func(root *os.Root, options Options) error {
		var err error
		if folder, err = newSkillFolder(ctx, root, name); err != nil {
			return err
		}
		git, err := openLibraryGit(ctx, root.Name(), options.Git)
		if err != nil {
			return err
		}
		tags, err := git.releaseTags(ctx)
		released = len(tags) > 0
		return err
	})
	if err != nil {
		return nil, err
	}
	return &SkillPlan{name, folder, options, released}, nil
}

// Folder returns the path in the library of the planned skill's folder, such as .trailguide/skills/acme/fix-bug.
func (p *SkillPlan) Folder() string { return p.folder }

// NeedsChangeNote reports that the library had a library release when planning, so the new skill needs a change
// note before library check passes.
func (p *SkillPlan) NeedsChangeNote() bool { return p != nil && p.released }

// Commit revalidates live state before creating the skill's SKILL.md, a marked draft with description and kind, a
// trailguide-skill-type or empty, in its frontmatter.
func (p *SkillPlan) Commit(ctx context.Context, description, kind string) (AuthoringResult, error) {
	if p == nil || p.name == "" {
		return AuthoringResult{}, failure("invalid-operation", "expected a planned skill", nil)
	}
	data, err := draftSkill(p.name, description, kind)
	if err != nil {
		return AuthoringResult{}, err
	}
	root, err := openLibrary(ctx, p.options, false)
	if err != nil {
		return AuthoringResult{}, err
	}
	defer root.Close()
	dir, err := openTrailGuideDirectory(ctx, root, false)
	if err != nil {
		return AuthoringResult{}, err
	}
	defer dir.Close()
	changes, err := filetxn.EditIn(ctx, dir, root, func() ([]filetxn.File, error) {
		if _, _, err := libraryManifest(ctx, root); err != nil {
			return nil, err
		}
		folder, err := newSkillFolder(ctx, root, p.name)
		if err != nil {
			return nil, err
		}
		return withSkillsGuide(ctx, root, []filetxn.File{{Path: path.Join(folder, skills.File), Content: data}})
	})
	return authoringResult(changes, err)
}

// draftSkill returns the SKILL.md draft of the skill name, failing at the flag that supplied an invalid description
// or kind.
func draftSkill(name, description, kind string) ([]byte, error) {
	if err := skills.ValidateDescription(description); err != nil {
		return nil, &ValidationError{Location: "--description", Problem: err.Error()}
	}
	if err := skills.ValidateType(kind); err != nil {
		return nil, &ValidationError{Location: "--type", Problem: err.Error()}
	}
	return skills.Draft(name, description, kind, rules.DraftMarker)
}

// newSkillFolder returns the path in the library of the folder of the library's own new skill name, in the origin
// folder its library.name names, such as .trailguide/skills/acme/fix-bug. It fails when name isn't a valid skill
// name, the library has no manifest, or it already has a skill named name in any origin folder.
func newSkillFolder(ctx context.Context, root *os.Root, name string) (string, error) {
	if !skills.ValidName(name) {
		return "", &ValidationError{Location: "skill name", Problem: "a skill name uses 1 to 64 lowercase letters, digits, and hyphens, without a leading, trailing, or doubled hyphen, such as fix-bug"}
	}
	origins, err := skillOrigins(ctx, root)
	if err != nil {
		return "", err
	}
	return requireNoSkillNamed(ctx, root, origins.Own, name)
}

// skillOrigins returns the skill origins of the library's configuration, which says which origin folder of skills/
// each skill belongs in. It fails when the library has no manifest, or its configuration is invalid.
func skillOrigins(ctx context.Context, root *os.Root) (skills.Origins, error) {
	data, err := readOptionalBelow(ctx, root, manifestPath)
	if err != nil {
		return skills.Origins{}, err
	}
	if data == nil {
		return skills.Origins{}, failure("needs-init", "missing "+manifestPath+"; run trailguide library init first", nil)
	}
	config, err := rules.ParseConfigurationYAML(data)
	if err != nil {
		return skills.Origins{}, err
	}
	return config.SkillOrigins(), nil
}

// requireNoSkillNamed returns the path in the library of the folder of skill name in the origin folder origin. It
// fails when that folder exists, or the library has a skill named name in any other origin folder, since a skill's
// name is its ID.
func requireNoSkillNamed(ctx context.Context, root *os.Root, origin, name string) (string, error) {
	folder := path.Join(skillsDirectory, skills.Folder(origin, name))
	if err := filetxn.RequireAbsent(ctx, root, folder); err != nil {
		return "", err
	}
	existing, err := skillFolderPaths(root, skills.AuthoredID(name))
	if err != nil {
		return "", err
	}
	if len(existing) > 0 {
		return "", failure("already-exists", existing[0]+": already a skill named "+name+", and a skill's name is its ID; choose a different name", nil)
	}
	return folder, nil
}
