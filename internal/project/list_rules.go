// List every rule the project holds of its sources, and its authored rules, with how the project uses each.

package project

import (
	"context"
	"maps"
	"slices"
	"strings"

	"github.com/fabricahq/trailguide/internal/build"
	"github.com/fabricahq/trailguide/internal/rules"
	"github.com/fabricahq/trailguide/trailguide/projectview"
)

// RuleListing lists the rules the project holds of each source, and its authored rules, in project ID order.
type RuleListing struct {
	Rules []ListedRule `json:"rules"`
}

// ListedRule is one rule of a RuleListing: the project view's rule, and whether an authored rule is still a draft,
// which build refuses until its author completes it, so agents don't receive it yet.
type ListedRule struct {
	projectview.Rule
	Draft bool `json:"draft,omitempty"`
}

// emptyLock is the lockfile of a project with no sources, which needs none.
var emptyLock = []byte(`{"lockVersion": 1, "sources": {}}`)

// ListRules lists the rules of the project's sources from its configuration and lockfile, through
// projectview.Read, and adds its authored rules, marking a draft inactive, without fetching sources or writing files. It fails, as build does,
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
	authored := treeFiles(state.local)
	listing := RuleListing{Rules: []ListedRule{}}
	listed := map[string]bool{}
	for _, rule := range view.Rules {
		listed[rule.ID] = true
		listing.Rules = append(listing.Rules, ListedRule{Rule: rule})
	}
	for _, id := range build.AuthoredRules(slices.Collect(maps.Keys(authored))) {
		if !listed[projectview.LocalSource+":"+id] {
			listing.Rules = append(listing.Rules, ListedRule{Rule: projectview.Rule{ID: projectview.LocalSource + ":" + id, Path: id, Active: true}})
		}
	}
	for i := range listing.Rules {
		rule := &listing.Rules[i]
		if rule.Source == "" && rules.HasDraftMarker(authored[rule.Path+".md"]) {
			rule.Active, rule.Draft = false, true
		}
	}
	slices.SortFunc(listing.Rules, func(a, b ListedRule) int { return strings.Compare(a.ID, b.ID) })
	return listing, ctx.Err()
}
