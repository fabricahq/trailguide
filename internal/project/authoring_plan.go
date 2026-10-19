// Plan project authoring without retaining roots or locks; commit revalidates live state.

package project

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/fabricahq/trailguide/internal/build"
	"github.com/fabricahq/trailguide/internal/filetxn"
	"github.com/fabricahq/trailguide/internal/librarytree"
	"github.com/fabricahq/trailguide/internal/rules"
	"github.com/fabricahq/trailguide/internal/skills"
	"github.com/fabricahq/trailguide/trailguide"
)

// LocalGroupPlan identifies a new project-only group. It owns no filesystem resources.
type LocalGroupPlan struct {
	id      string
	options Options
}

// LocalRulePlan identifies a new project-only rule in an existing local or imported group.
type LocalRulePlan struct {
	id      string
	options Options
}

// SourcePlan reserves no alias; Commit refuses an alias taken after planning.
type SourcePlan struct {
	alias   string
	options Options
}

// PlanLocalGroup checks the target before input collection and anchors the project directory.
func PlanLocalGroup(ctx context.Context, id string, options Options) (*LocalGroupPlan, error) {
	options, err := planProjectAuthoring(ctx, options, func(root *os.Root, _ rules.Configuration) error { return checkLocalGroup(ctx, root, id) })
	if err != nil {
		return nil, err
	}
	return &LocalGroupPlan{id, options}, nil
}

// Commit validates metadata and repeats target checks under writer ownership before publishing.
func (p *LocalGroupPlan) Commit(ctx context.Context, metadata trailguide.GroupMetadata) (AuthoringResult, error) {
	if p == nil || p.id == "" {
		return AuthoringResult{}, failure("invalid-operation", "expected a planned group", nil)
	}
	return AddLocalGroup(ctx, p.id, metadata, p.options)
}

// PlanLocalRule checks path, group availability, and target absence before collecting metadata.
func PlanLocalRule(ctx context.Context, id string, options Options) (*LocalRulePlan, error) {
	options, err := planProjectAuthoring(ctx, options, func(root *os.Root, config rules.Configuration) error { return checkLocalRule(ctx, root, config, id) })
	if err != nil {
		return nil, err
	}
	return &LocalRulePlan{id, options}, nil
}

// Commit rechecks the group and target before creating a complete rule or draft when body is nil.
func (p *LocalRulePlan) Commit(ctx context.Context, metadata rules.RuleMetadata, body *string) (AuthoringResult, error) {
	if p == nil || p.id == "" {
		return AuthoringResult{}, failure("invalid-operation", "expected a planned rule", nil)
	}
	return AddLocalRule(ctx, p.id, metadata, RuleOptions{Options: p.options, Body: body})
}

// PlanSource rejects an occupied source name before collecting its declaration; it never fetches.
func PlanSource(ctx context.Context, alias string, options Options) (*SourcePlan, error) {
	options, err := planProjectAuthoring(ctx, options, func(_ *os.Root, config rules.Configuration) error { return checkSourceAlias(config, alias) })
	if err != nil {
		return nil, err
	}
	return &SourcePlan{alias, options}, nil
}

// Commit validates and stores a source declaration, rechecking the alias under writer ownership.
func (p *SourcePlan) Commit(ctx context.Context, declaration SourceInput) (AuthoringResult, error) {
	if p == nil || p.alias == "" {
		return AuthoringResult{}, failure("invalid-operation", "expected a planned source name", nil)
	}
	return AddSource(ctx, p.alias, declaration, p.options)
}

// planProjectAuthoring closes its advisory snapshot before prompts; only commit owns recovery and writes.
func planProjectAuthoring(ctx context.Context, options Options, check func(*os.Root, rules.Configuration) error) (Options, error) {
	directory, err := filepath.Abs(options.Directory)
	if err != nil {
		return Options{}, err
	}
	options.Directory = directory
	root, err := openProject(ctx, options, false)
	if err != nil {
		return Options{}, err
	}
	defer root.Close()
	_, config, err := configuration(ctx, root)
	if err != nil {
		return Options{}, err
	}
	return options, check(root, config)
}

func checkLocalGroup(ctx context.Context, root *os.Root, id string) error {
	if err := librarytree.ValidateGroupID(id, "group"); err != nil {
		return err
	}
	return filetxn.RequireAbsent(ctx, root, path.Join("rules", id, "_group.yaml"), path.Join("rules", id, "README.md"))
}

func checkLocalRule(ctx context.Context, root *os.Root, config rules.Configuration, id string) error {
	if strings.HasSuffix(id, ".md") {
		return failure("invalid-rule-path", "use a rule path without the .md extension", nil)
	}
	group, err := librarytree.GroupFromPath(id+".md", "rule")
	if err != nil {
		return err
	}
	available, err := groupAvailable(ctx, root, config, group)
	if err != nil {
		return err
	}
	if !available {
		return failure("missing-group", "group "+group+" does not exist; create it first with trailguide project add group "+group+", then retry adding the rule", nil)
	}
	return filetxn.RequireAbsent(ctx, root, path.Join("rules", id+".md"))
}

func checkSourceAlias(config rules.Configuration, alias string) error {
	for _, source := range config.Sources {
		if source.Name == alias {
			return failure("source-exists", "source "+alias+" already exists; edit .trailguide/config.yaml or choose a different source name", nil)
		}
	}
	return nil
}

// LocalSkillPlan identifies a new authored skill in the project's skills/; Commit rechecks that its folder is still
// absent.
type LocalSkillPlan struct {
	name    string
	options Options
}

// PlanLocalSkill checks the skill's name and that the project has no authored skill folder by that name before
// collecting the skill's description.
func PlanLocalSkill(ctx context.Context, name string, options Options) (*LocalSkillPlan, error) {
	options, err := planProjectAuthoring(ctx, options, func(root *os.Root, _ rules.Configuration) error { return checkLocalSkill(ctx, root, name) })
	if err != nil {
		return nil, err
	}
	return &LocalSkillPlan{name, options}, nil
}

// Commit rechecks the target before creating the skill's SKILL.md, a marked draft with description and kind, a
// trailguide-skill-type or empty, in its frontmatter. It warns when a skill the project last installed from a source
// has the same name, unless the configuration now excludes that skill, since sync and build then fail
// until one of them is renamed or excluded.
func (p *LocalSkillPlan) Commit(ctx context.Context, description, kind string) (AuthoringResult, error) {
	if p == nil || p.name == "" {
		return AuthoringResult{}, failure("invalid-operation", "expected a planned skill", nil)
	}
	if err := skills.ValidateDescription(description); err != nil {
		return AuthoringResult{}, &ValidationError{Location: "--description", Problem: err.Error()}
	}
	if err := skills.ValidateType(kind); err != nil {
		return AuthoringResult{}, &ValidationError{Location: "--type", Problem: err.Error()}
	}
	data, err := skills.Draft(p.name, description, kind, rules.DraftMarker)
	if err != nil {
		return AuthoringResult{}, err
	}
	var clash string
	result, err := editProject(ctx, p.options, func(root *os.Root, _ []byte, config rules.Configuration) ([]filetxn.File, error) {
		if err := checkLocalSkill(ctx, root, p.name); err != nil {
			return nil, err
		}
		record, err := filetxn.ReadOptional(ctx, root, path.Join("generated", build.InstalledSkillsFile))
		if err != nil {
			return nil, err
		}
		for _, installed := range build.InstalledSkills(record) {
			source, id, fromSource := strings.Cut(installed.ID, ":")
			if installed.Name == p.name && fromSource && !excludes(config, source, id) {
				clash = installed.ID
				break
			}
		}
		return []filetxn.File{{Path: path.Join(skills.AuthoredID(p.name), skills.File), Content: data}}, nil
	})
	if err == nil && clash != "" {
		result.Warnings = append(result.Warnings, fmt.Sprintf("The project installs %s as %s too, so trailguide project sync and build fail with skill-name-clash until you rename this skill's folder and name, or exclude %s.", clash, p.name, clash))
	}
	return result, err
}

// excludes reports whether config's source excludes its skill id, so build no longer installs it, whatever the last
// build's record lists.
func excludes(config rules.Configuration, source, id string) bool {
	for _, declared := range config.Sources {
		if declared.Name == source {
			_, excluded := declared.SkillExclude[id]
			return excluded
		}
	}
	return false
}

func checkLocalSkill(ctx context.Context, root *os.Root, name string) error {
	if !skills.ValidName(name) {
		return &ValidationError{Location: "skill name", Problem: "a skill name uses 1 to 64 lowercase letters, digits, and hyphens, without a leading, trailing, or doubled hyphen, such as release-checklist"}
	}
	return filetxn.RequireAbsent(ctx, root, skills.AuthoredID(name))
}
