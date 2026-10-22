// List every rule the project holds of its sources, and its authored rules, with how the project uses each.

package project

import (
	"context"
	"maps"
	"slices"
	"strings"

	"github.com/fabricahq/trailguide/internal/build"
	"github.com/fabricahq/trailguide/trailguide/projectview"
)

// RuleListing lists the rules the project holds of each source, and its authored rules, in project ID order.
type RuleListing struct {
	Rules []projectview.Rule `json:"rules"`
}

// emptyLock is the lockfile of a project with no sources, which needs none.
var emptyLock = []byte(`{"lockVersion": 1, "sources": {}}`)

// ListRules lists the rules of the project's sources from its configuration and lockfile, through
// projectview.Read, and adds its authored rules, without fetching sources or writing files. It fails, as build does,
// when the lockfile doesn't match the configuration, so the listing never describes a configuration sync hasn't
// applied.
func ListRules(ctx context.Context, options Options) (RuleListing, error) {
	if err := ctx.Err(); err != nil {
		return RuleListing{}, err
	}
	root, err := openProject(ctx, options, false)
	if err != nil {
		return RuleListing{}, err
	}
	defer root.Close()
	state, err := readProject(ctx, root)
	if err != nil {
		return RuleListing{}, err
	}
	if _, err := state.records(); err != nil {
		return RuleListing{}, err
	}
	lock := state.lock
	if lock == nil && len(state.config.Sources) == 0 {
		lock = emptyLock
	}
	view, err := projectview.Read(state.configBytes, lock)
	if err != nil {
		return RuleListing{}, err
	}
	listed := map[string]bool{}
	for _, rule := range view.Rules {
		listed[rule.ID] = true
	}
	for _, id := range build.AuthoredRules(slices.Collect(maps.Keys(treeFiles(state.local)))) {
		if !listed[projectview.LocalSource+":"+id] {
			view.Rules = append(view.Rules, projectview.Rule{ID: projectview.LocalSource + ":" + id, Path: id, Active: true})
		}
	}
	slices.SortFunc(view.Rules, func(a, b projectview.Rule) int { return strings.Compare(a.ID, b.ID) })
	return RuleListing{Rules: view.Rules}, ctx.Err()
}
