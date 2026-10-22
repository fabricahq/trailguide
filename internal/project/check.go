// Determine complete project freshness without applying repairs or exposing storage details.

package project

import (
	"context"
	"os"
	"path"

	"github.com/fabricahq/trailguide/internal/librarytree"
	"github.com/fabricahq/trailguide/internal/rules"
)

// ProblemKind distinguishes the observed mismatch without relying on presentation text.
type ProblemKind string

const (
	MissingFile    ProblemKind = "missing-file"
	StaleContents  ProblemKind = "stale-contents"
	UnexpectedFile ProblemKind = "unexpected-file"
	OutdatedGuide  ProblemKind = "outdated-readme"
	// OutdatedAgents means the repository's AGENTS.md lacks the Trail Guide section, or holds an older one.
	OutdatedAgents ProblemKind = "outdated-agents-section"
	// MissingSkill, ModifiedSkill, and UnexpectedSkill report an installed skill folder that is missing, differs from
	// the copy of its source build writes, or belongs to a skill the project no longer installs.
	MissingSkill    ProblemKind = "missing-skill"
	ModifiedSkill   ProblemKind = "modified-skill"
	UnexpectedSkill ProblemKind = "unexpected-skill"
)

// RepairAction identifies the project operation that can repair an observed mismatch.
type RepairAction string

const (
	Rebuild      RepairAction = "rebuild"
	RefreshGuide RepairAction = "refresh-guide"
)

// Problem identifies a path relative to the configuration directory, such as ../AGENTS.md, and its intended repair.
// Refreshing a guide still requires preserving manual edits; Check never authorizes overwriting them.
type Problem struct {
	Kind ProblemKind
	// Path is relative to the configuration directory, so an installed skill folder's starts with ../, such as
	// ../.claude/skills/tdd.
	Path   string
	Repair RepairAction
	// Skill is the project ID of the skill an installed skill folder problem is about, or empty for another problem.
	Skill string
}

// CheckResult lists all freshness problems in deterministic order; an empty list means current.
// Invalid, unreadable, or concurrently changing inputs return an error instead of this report.
type CheckResult struct {
	Problems []Problem
	// Warnings never make the project out of date: one for each personal skill with the name of a skill the
	// project installs. It is empty, never nil, when there are none.
	Warnings []string
}

// Current reports whether generated output, the managed guide, and the AGENTS.md section match the selected inputs.
func (r CheckResult) Current() bool { return len(r.Problems) == 0 }

// Check compares generated output, the installed skill folders, the managed guide, and the Trail Guide section of the
// repository's AGENTS.md in one optimistic snapshot. It performs no writes, locks, recovery, Git, or network access.
// Staleness is a result, not an error; invalid inputs, including an authored skill that cites a rule its library
// retired, an edited section, or broken markers in AGENTS.md, are errors. It also warns about personal skills in
// options.Home with the name of a skill generated/ records as installed.
func Check(ctx context.Context, options Options) (CheckResult, error) {
	guideName, guideBytes := projectGuide()
	var agents agentsPlan
	defer func() { agents.close() }()
	checked, err := checkProjectFiles(ctx, options, map[string][]byte{guideName: guideBytes}, func(root *os.Root, config rules.Configuration) error {
		var err error
		agents, err = planAgentsSection(ctx, root, config, options)
		return err
	})
	if err != nil {
		return CheckResult{}, err
	}
	// The snapshot covers the Trail Guide directory; AGENTS.md, outside it, must also be as planned when it ends.
	if err := agents.requireUnchanged(ctx); err != nil {
		return CheckResult{}, err
	}
	changes, guideChanges := checked.generated, checked.files
	result := CheckResult{Problems: []Problem{}, Warnings: changes.Warnings}
	for _, group := range []struct {
		kind  ProblemKind
		paths []string
	}{
		{MissingFile, changes.Added}, {StaleContents, changes.Changed}, {UnexpectedFile, changes.Removed},
	} {
		for _, name := range group.paths {
			result.Problems = append(result.Problems, Problem{Kind: group.kind, Path: path.Join(librarytree.Generated, name), Repair: Rebuild})
		}
	}
	for _, folder := range checked.skills {
		result.Problems = append(result.Problems, Problem{Kind: folder.Kind, Path: "../" + folder.Folder, Repair: Rebuild, Skill: folder.Skill})
	}
	if len(guideChanges.Added)+len(guideChanges.Changed) > 0 {
		result.Problems = append(result.Problems, Problem{Kind: OutdatedGuide, Path: guideName, Repair: RefreshGuide})
	}
	if agents.content != nil {
		result.Problems = append(result.Problems, Problem{Kind: OutdatedAgents, Path: path.Join("..", agentsFile), Repair: Rebuild})
	}
	return result, nil
}
