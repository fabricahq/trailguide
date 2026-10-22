// Package project owns a consuming project's Trail Guide state: initialization, local authoring,
// library adoption, offline generation, and read-only freshness checks. Storage protocols stay private.
// The lockfile, lock.json, records what each source resolved to: sync restores the recorded rule versions and chooses
// one only where configuration asks for something the record lacks, so only update moves rules to newer versions on
// its own. Operations that need input return a plan first; the CLI prompts before the plan's Commit or Apply, which
// revalidates under writer ownership.
package project

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/fabricahq/trailguide/internal/filetxn"
	"github.com/fabricahq/trailguide/internal/librarytree"
	"github.com/fabricahq/trailguide/internal/rules"
	"github.com/fabricahq/trailguide/trailguide"
)

// AuthoringResult lists the absolute paths of the authored files an operation created and changed, and cleanup
// warnings. Errors return no result.
type AuthoringResult struct {
	Added   []string `json:"added"`
	Changed []string `json:"changed"`
	// Warnings describe cleanup failures after all requested files were committed.
	Warnings []string `json:"warnings"`
}

// Written returns every path the operation wrote: its created files, then its changed ones.
func (r AuthoringResult) Written() []string { return append(slices.Clone(r.Added), r.Changed...) }

// RuleOptions distinguishes a supplied body from an unfinished draft in an existing group.
type RuleOptions struct {
	Options
	Body *string
}

// Initialize creates missing scaffolding and refreshes an unmodified managed guide; valid configuration and authored
// files are preserved. A new configuration installs skills into both skill targets, claude and agents. It fails,
// writing nothing, when the project still has a local/ folder, or when AGENTS.md has an edited section or broken markers.
// Then it adds or refreshes the Trail Guide section of the repository's AGENTS.md, as build does, listing the file among
// the changed files, or the added ones when it created it; a failure there comes after the other files were written,
// and running init again finishes it.
func Initialize(ctx context.Context, options Options) (AuthoringResult, error) {
	return InitializeWithSkillTargets(ctx, options, nil)
}

// DefaultSkillTargets are the skill targets a new configuration installs skills into.
var DefaultSkillTargets = []string{"claude", "agents"}

// InitializeWithSkillTargets is Initialize, except that a non-nil skillTargets, a list of skill target names that
// may be empty, sets skillTargets in a new configuration and replaces it in an existing one.
func InitializeWithSkillTargets(ctx context.Context, options Options, skillTargets []string) (AuthoringResult, error) {
	root, err := openProject(ctx, options, true)
	if err != nil {
		return AuthoringResult{}, err
	}
	defer root.Close()
	changes, err := filetxn.Edit(ctx, root, func() ([]filetxn.File, error) {
		var files []filetxn.File
		old, err := filetxn.ReadOptional(ctx, root, configurationFile)
		if err != nil {
			return nil, err
		}
		var config rules.Configuration
		if old != nil {
			config, err = rules.ParseConfigurationYAML(old)
			if err != nil {
				return nil, err
			}
			if skillTargets != nil && !slices.Equal(config.SkillTargets, skillTargets) {
				data, err := rules.SetConfigurationSkillTargets(old, skillTargets)
				if err != nil {
					return nil, err
				}
				files = append(files, filetxn.File{Path: configurationFile, Content: data, Previous: old})
			}
		}
		if _, err = filetxn.ReadTree(ctx, root, "rules"); err != nil {
			return nil, err
		}
		readme, err := filetxn.ReadOptional(ctx, root, "rules/README.md")
		if err != nil {
			return nil, err
		}
		guide, err := prepareProjectGuide(ctx, root)
		if err != nil {
			return nil, err
		}
		// Refuse an edited section or broken markers before writing anything; initializeAgents writes the section.
		agents, err := planAgentsSection(ctx, root, config, options)
		if err != nil {
			return nil, err
		}
		agents.close()
		if old == nil {
			if skillTargets == nil {
				skillTargets = DefaultSkillTargets
			}
			data, err := rules.SetConfigurationSkillTargets([]byte("schemaVersion: 2\nsources: {}\n"), skillTargets)
			if err != nil {
				return nil, err
			}
			files = append(files, filetxn.File{Path: configurationFile, Content: data})
		}
		if readme == nil {
			files = append(files, filetxn.File{Path: "rules/README.md", Content: renderLocalReadme()})
		}
		if guide != nil {
			files = append(files, *guide)
		}
		return withSkillsGuide(ctx, root, files)
	})
	result, err := authoringResult(changes, err)
	if err != nil {
		return AuthoringResult{}, err
	}
	return initializeAgents(ctx, root, options, result)
}

// initializeAgents adds or refreshes the AGENTS.md section of the initialized project at root in a recoverable
// transaction, adding its path to result, unless its configuration turns off manageAgentsFile.
func initializeAgents(ctx context.Context, root *os.Root, options Options, result AuthoringResult) (AuthoringResult, error) {
	var changes FileChanges
	err := filetxn.WithWriter(ctx, root, func(w *filetxn.Writer) error {
		_, config, err := configuration(ctx, root)
		if err != nil {
			return err
		}
		agents, err := planAgentsSection(ctx, root, config, options)
		if err != nil {
			return err
		}
		defer agents.close()
		targets := map[filetxn.Target]map[string][]byte{}
		agents.include(targets, &changes)
		return w.Apply(targets, func() error { return agents.requireUnchanged(ctx) })
	})
	if err != nil {
		return AuthoringResult{}, err
	}
	if changes.Agents != nil {
		written := filepath.Join(filepath.Dir(root.Name()), changes.Agents.Path)
		if changes.Agents.Action == "created" {
			result.Added = append(result.Added, written)
		} else {
			result.Changed = append(result.Changed, written)
		}
	}
	return result, nil
}

// editProject revalidates configuration and rules/ under exclusive ownership before publishing authored files.
func editProject(ctx context.Context, options Options, prepare func(*os.Root, []byte, rules.Configuration) ([]filetxn.File, error)) (AuthoringResult, error) {
	root, err := openProject(ctx, options, false)
	if err != nil {
		return AuthoringResult{}, err
	}
	defer root.Close()
	changes, err := filetxn.Edit(ctx, root, func() ([]filetxn.File, error) {
		var files []filetxn.File
		original, config, err := configuration(ctx, root)
		if err != nil {
			return nil, err
		}
		if _, err = filetxn.ReadTree(ctx, root, "rules"); err != nil {
			return nil, err
		}
		files, err = prepare(root, original, config)
		if err != nil {
			return nil, err
		}
		return files, nil
	})
	return authoringResult(changes, err)
}

// AddLocalGroup creates one complete authored group definition in rules/ without overwriting existing metadata.
func AddLocalGroup(ctx context.Context, id string, metadata trailguide.GroupMetadata, options Options) (AuthoringResult, error) {
	if err := librarytree.ValidateGroupID(id, "group"); err != nil {
		return AuthoringResult{}, err
	}
	data, err := rules.RenderGroup(metadata)
	if err != nil {
		return AuthoringResult{}, err
	}
	return editProject(ctx, options, func(root *os.Root, _ []byte, _ rules.Configuration) ([]filetxn.File, error) {
		if err := checkLocalGroup(ctx, root, id); err != nil {
			return nil, err
		}
		guideName, _ := projectGuide()
		return groupFiles(path.Join("rules", id), id, data, guideName), nil
	})
}

// groupAvailable accepts authored metadata or a selected group from a verified persisted source snapshot.
func groupAvailable(ctx context.Context, root *os.Root, config rules.Configuration, id string) (bool, error) {
	local, err := filetxn.ReadTree(ctx, root, "rules")
	if err != nil {
		return false, err
	}
	if local != nil {
		if data, ok := local.Files[id+"/_group.yaml"]; ok {
			_, err = trailguide.ParseGroupMetadata(data, id)
			return err == nil, err
		}
	}
	stored, vendor, _, err := readStored(ctx, root, config)
	if err != nil || vendor == nil {
		return false, err
	}
	snapshots, err := decodeSnapshots(config, stored, vendor.Files)
	if err != nil {
		return false, err
	}
	for _, source := range config.Sources {
		snapshot := snapshots[source.Name]
		reached := slices.Contains(snapshot.Groups, id)
		for rule := range snapshot.Rules {
			reached = reached || strings.HasPrefix(rule, id+"/")
		}
		if !reached {
			continue
		}
		if data, ok := snapshot.Files[id+"/_group.yaml"]; ok {
			_, err = trailguide.ParseGroupMetadata(data, id)
			return err == nil, err
		}
	}
	return false, nil
}

// AddLocalRule creates a validated rule or canonical unfinished draft in rules/, in an existing group.
func AddLocalRule(ctx context.Context, id string, metadata rules.RuleMetadata, options RuleOptions) (AuthoringResult, error) {
	data, err := rules.RenderRule(id, metadata, options.Body)
	if err != nil {
		return AuthoringResult{}, err
	}

	return editProject(ctx, options.Options, func(root *os.Root, _ []byte, config rules.Configuration) ([]filetxn.File, error) {
		if err := checkLocalRule(ctx, root, config, id); err != nil {
			return nil, err
		}
		return []filetxn.File{{Path: path.Join("rules", id+".md"), Content: data}}, nil
	})
}

// AddSource records a validated source declaration without Git access, preserving other selections and exceptions.
func AddSource(ctx context.Context, alias string, input SourceInput, options Options) (AuthoringResult, error) {
	return editProject(ctx, options, func(_ *os.Root, original []byte, config rules.Configuration) ([]filetxn.File, error) {
		if err := checkSourceAlias(config, alias); err != nil {
			return nil, err
		}
		source, err := parseSourceInput(input)
		if err != nil {
			return nil, err
		}
		data, err := rules.AppendConfigurationSource(original, alias, source)
		if err != nil {
			return nil, err
		}
		return []filetxn.File{{Path: configurationFile, Content: data, Previous: original}}, nil
	})
}

// authoringResult returns published paths and cleanup warnings only after a successful write.
func authoringResult(changes filetxn.Changes, err error) (AuthoringResult, error) {
	if err != nil {
		return AuthoringResult{}, err
	}
	return AuthoringResult{Added: changes.Added, Changed: changes.Changed, Warnings: changes.Warnings}, nil
}

func failure(code, problem string, cause error) error {
	return &filetxn.Error{Code: code, Problem: problem, Cause: cause}
}

func groupFiles(directory, id string, metadata []byte, projectGuideName string) []filetxn.File {
	instructions := fmt.Sprintf("## Add or edit rules\n\nFollow [the project guide](../../../%s) for commands to run from the project root.\nUse `trailguide project add rule %s/<rule-name>` to add a rule. Edit existing rule files directly.\nRun trailguide project build and trailguide project check from the project root after local changes, then inspect [the resolved rules](../../../%s/RULES.md).\nUse the resolved rules when working on the project: they include imported guidance and apply exclusions and replacements.\n", projectGuideName, id, librarytree.Generated)
	return []filetxn.File{{Path: path.Join(directory, "_group.yaml"), Content: metadata}, {Path: path.Join(directory, "README.md"), Content: rules.GroupGuide(id, instructions)}}
}
