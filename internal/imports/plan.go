// Choose each imported rule's version from a source's configuration, its recorded snapshot, and its library releases.

package imports

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/fabricahq/trailguide/internal/library"
	"github.com/fabricahq/trailguide/internal/librarytree"
	"github.com/fabricahq/trailguide/internal/rules"
	"github.com/fabricahq/trailguide/trailguide"
)

// sourcePlan is what one source imports: the revision that supplies its library-wide files, and each imported
// rule's version and the commit that supplies its files. Resolving it may read the library's release history;
// importing it fetches the commits it names and reads their files.
type sourcePlan struct {
	// release is the library release that supplies the library-wide files, or 0 when ref isn't a library release.
	// For a source without ref, it is never older than the library release of a rule the plan imports.
	release int
	commit  string
	rules   map[string]library.ImportedRule
	// individual lists the individually selected rules to load; it omits entries naming retired rules.
	individual []string
	warnings   []string
	// retired lists, sorted, the rules the library retired that the source selects; it is empty, never nil.
	retired []string
	// ref is the source's ref as the snapshot records it: the recorded spelling when it names the same revision as
	// the configured one, such as release/2 for refs/tags/release/2, so an equivalent spelling never rewrites the
	// record; otherwise the configured one.
	ref rules.GitRef
	// nonLibrary reports that commit holds no library manifest, so the source imports only skills.
	nonLibrary bool
	// followsReleases reports that the plan imports rule versions from library releases: the source has no ref, or its
	// ref chose a library release, release/<number> or latest-release, which the record then keeps no ref for.
	followsReleases bool
	// locked is the commit the source's record holds, which sync keeps, or empty for a source without one.
	locked string
	// resolved is what the source's ref named when the plan resolved it again, a commit, a tag, or a branch, as for
	// a bare name the repository decides; it is empty when the plan kept the recorded commit or follows library
	// releases.
	resolved rules.GitRefKind
}

// planner chooses versions for one source, reading the library's release history only when a choice needs it.
type planner struct {
	ctx    context.Context
	repo   *repository
	source rules.Source
	// recorded is the source's snapshot from the last sync, or nil for a new source or a changed repository.
	recorded *library.Snapshot
	// history is nil until first needed. It holds every library release; releases returns those up to ceiling.
	history *releaseHistory
	// ceiling is the library release a ref such as release/4 names, newer releases being invisible to sync, or 0.
	ceiling int
	// again resolves a source's ref again, as an update of a source that follows its ref does, instead of keeping the
	// commit recorded holds.
	again bool
	// updating plans an update, which takes the library release a ref such as release/7 names when it differs from
	// the one recorded holds; sync refuses that ref.
	updating bool
}

// planSource chooses what source imports, as project sync does: recorded versions stay, and versions are chosen
// only for new sources, changed repositories, newly selected rules, added or changed pins, and a changed ref.
// It fails when a library release tag that a recorded rule names has moved.
func planSource(ctx context.Context, repo *repository, source rules.Source, recorded *library.Snapshot) (sourcePlan, error) {
	p := newPlanner(ctx, repo, source, recorded)
	plan, err := p.choose()
	if err != nil {
		return sourcePlan{}, err
	}
	return plan, p.requireUnmoved(plan)
}

// newPlanner plans source against recorded, which it ignores when it records another repository.
func newPlanner(ctx context.Context, repo *repository, source rules.Source, recorded *library.Snapshot) *planner {
	if recorded != nil && recorded.Repository != source.Repository {
		recorded = nil
	}
	return &planner{ctx: ctx, repo: repo, source: source, recorded: recorded}
}

// choose plans what the source imports as project sync does, without fetching the commits the plan names that
// choosing didn't need.
func (p *planner) choose() (sourcePlan, error) {
	choose := p.planRevision
	switch {
	case p.source.Ref.IsZero():
		choose = p.planVersions
	case p.source.Ref.FollowsReleases():
		choose = p.planReleases
	}
	chosen, err := choose()
	if err != nil {
		return sourcePlan{}, err
	}
	chosen.warnings = append(redundantRules(p.source, chosen.rules), chosen.warnings...)
	if !chosen.followsReleases {
		chosen.ref = p.source.Ref
		if p.recorded != nil && p.recorded.Ref.Equal(p.source.Ref) {
			chosen.ref = p.recorded.Ref
		}
	}
	if p.recorded != nil {
		chosen.locked = p.recorded.Commit
	}
	if chosen.nonLibrary {
		chosen.retired = []string{}
		return chosen, nil
	}
	chosen.retired, err = p.retiredRules(chosen)
	return chosen, err
}

// redundantRules warns about each imported rule the source selects individually although another of its include
// selectors already selects the rule's group, and about each group selector its wildcard already covers, since
// those entries change nothing. An entry naming a retired rule gets its own warning instead.
func redundantRules(source rules.Source, imported map[string]library.ImportedRule) []string {
	warnings := []string{}
	wildcard := rules.GroupSelection{Pattern: source.Groups.Pattern}
	for _, group := range source.Groups.Groups {
		if wildcard.Includes(group) {
			warnings = append(warnings, fmt.Sprintf("sources.%s.rules.include names %s/*, a group that %s in the same list already selects, so the entry changes nothing; delete it.", source.Name, group, quoteSelector(source.Groups.Pattern)))
		}
	}
	for _, id := range source.Rules {
		if _, ok := imported[id]; ok && source.Groups.Includes(ruleGroup(id)) {
			covering := source.Groups.Pattern
			if slices.Contains(source.Groups.Groups, ruleGroup(id)) {
				covering = ruleGroup(id) + "/*"
			}
			warnings = append(warnings, fmt.Sprintf("sources.%s.rules.include names %s, a rule that %s in the same list already selects, so the entry changes nothing; delete it.", source.Name, id, quoteSelector(covering)))
		}
	}
	return warnings
}

// quoteSelector returns a wildcard selector as YAML needs it written: "*" in quotes, since a bare * starts an alias,
// and other selectors unchanged.
func quoteSelector(text string) string {
	if text == "*" {
		return `"*"`
	}
	return text
}

// retiredRules returns the retired rules a sync that chose plan records. When the source selects what recorded did,
// from the same revision, with the same shared files, it keeps recorded's list, so that a sync changing nothing
// else, such as an exclusion the list already covers, never rewrites the record, whatever the planner happened to
// read; it adds only the retired rules that sync just found its pins and exclusions, selectors included, match,
// which the record didn't cover, so offline checks accept them too. Otherwise it returns the release history's, as freshRetiredRules does.
func (p *planner) retiredRules(plan sourcePlan) ([]string, error) {
	recorded := p.recorded
	if recorded == nil || !recorded.Selection.Equal(p.source.Groups) || !slices.Equal(recorded.RuleSelection, p.source.Rules) || !plan.ref.Equal(recorded.Ref) || plan.release != recorded.Release || plan.commit != recorded.Commit {
		return p.freshRetiredRules()
	}
	retired := slices.Clone(recorded.RetiredRules)
	if p.history != nil {
		// Validating such a pin or exclusion read the history, which records why it only warns.
		for _, entry := range slices.Concat(slices.Sorted(maps.Keys(p.source.Pins)), slices.Sorted(maps.Keys(p.source.Exclude))) {
			for _, id := range p.retiredMatches(entry, p.history.upTo(p.ceiling)) {
				if _, imported := plan.rules[id]; !imported && !slices.Contains(retired, id) {
					retired = append(retired, id)
				}
			}
		}
	}
	slices.Sort(retired)
	return retired, nil
}

// freshRetiredRules returns, sorted, the rules the library's release history retired that the source would
// otherwise import, as configuration entries naming them only warn: those its groups or rules list selects, and
// those recorded imported or listed as retired, so an entry left after deselecting a retired rule stays a warning
// offline too. A retired rule the plan still imports, such as one a pin keeps at its last version, is listed too, so
// the record says it's retired. It reads the history when the planner hasn't yet.
func (p *planner) freshRetiredRules() ([]string, error) {
	history, err := p.releases()
	if err != nil {
		return nil, err
	}
	retired := []string{}
	for _, release := range history.releases {
		for id := range release.record.Retired {
			if p.wouldImport(id) {
				retired = append(retired, id)
			}
		}
	}
	slices.Sort(retired)
	return slices.Compact(retired), nil
}

// releases returns the library's release history, reading it on first use; it may be empty. With a ceiling, it
// holds only the library releases up to it.
func (p *planner) releases() (releaseHistory, error) {
	if p.history == nil {
		history, err := p.repo.loadHistory(p.ctx)
		if err != nil {
			return releaseHistory{}, err
		}
		p.history = &history
	}
	return p.history.upTo(p.ceiling), nil
}

// planReleases plans a source whose ref chooses a library release: release/<number> takes that library release,
// and latest-release the newest one when no library release is recorded, and otherwise keeps the recorded one, as a
// branch keeps its recorded commit until an update. A ref edited to name another library release than the record's
// fails, as editedRelease decides, or plans the recorded release for the update to move from. A record of that
// library release keeps its rule versions, as
// planVersions keeps them; otherwise each selected rule takes its pinned version, or else its version in that
// release. Releases after the one the ref names stay out of view. A ref shaped like a library release that the
// library doesn't have, such as a branch named release/5, or the tag of a repository that isn't a library, imports
// that revision, as planRevision does.
func (p *planner) planReleases() (sourcePlan, error) {
	ref, recorded := p.source.Ref, p.recorded
	if !ref.LatestRelease() && recorded != nil && (recorded.NonLibrary || recorded.Release == 0) && ref.Equal(recorded.Ref) {
		return p.planRevision()
	}
	// A record of the library release the ref names keeps it without reading the history, even after its tag moved
	// or was deleted, as long as the source selects nothing new.
	if number := ref.Release(); number != 0 && recorded != nil && !recorded.NonLibrary && recorded.Release == number {
		p.ceiling = number
		return p.planVersions()
	}
	if ref.LatestRelease() && recorded != nil && !recorded.NonLibrary && recorded.Release > 0 {
		p.ceiling = recorded.Release
		return p.planVersions()
	}
	// A bare name such as release/5 names a branch of that name when the repository has one, as resolveRef reads it.
	if ref.Bare() && !ref.LatestRelease() {
		name, err := p.repo.bareRef(p.ctx, p.source)
		var failure *Error
		switch {
		case err == nil && strings.HasPrefix(name, "refs/heads/"):
			return p.planRevision()
		case err != nil && (!errors.As(err, &failure) || failure.Code != "version-not-found"):
			return sourcePlan{}, err
		}
	}
	// A ref edited to name another library release than the record's is reviewed with an update, which plans from the
	// recorded versions, choosing that release's versions only for newly selected rules, as sync chooses them.
	if number := ref.Release(); number != 0 && recorded != nil && !recorded.NonLibrary && recorded.Release != 0 {
		if err := p.editedRelease(number); err != nil {
			return sourcePlan{}, err
		}
		p.ceiling = number
		return p.planVersions()
	}
	history, err := p.releases()
	if err != nil {
		return sourcePlan{}, err
	}
	target := ref.Release()
	if ref.LatestRelease() {
		newest := history.newest()
		if newest == nil {
			return sourcePlan{}, fail("ref-required", fmt.Sprintf("sources.%s.ref is latest-release, the newest library release, but the repository has no release/<number> tags: it isn't a library, or hasn't published its first library release. Set sources.%s.ref to the branch, tag, or full commit SHA to take it from.", p.source.Name, p.source.Name), nil)
		}
		target = newest.number
	} else if history.release(target) == nil {
		return p.planRevision()
	}
	p.ceiling = target
	// A record of another library release, or of a revision other than one, keeps only its pinned rules: every other
	// rule moves to the release the ref names now.
	if recorded != nil && (recorded.NonLibrary || recorded.Release != target) {
		p.recorded = pinnedOnly(recorded, p.source.Pins)
		defer func() { p.recorded = recorded }()
	}
	return p.planVersions()
}

// editedRelease fails with code ref-changed for a ref that names library release number, other than the one the
// source's record holds, unless an update plans it and number is later: sync doesn't move a source, and an update
// moves one only forward.
func (p *planner) editedRelease(number int) error {
	name, recorded := p.source.Name, p.recorded.Release
	switch {
	case number < recorded:
		return fail("ref-changed", fmt.Sprintf("sources.%s.ref names release/%d, older than library release %d, which the project records; sync doesn't move a source, and trailguide project update moves a source only forward. To go back, restore .trailguide/config.yaml and vendor/%s/ from the commit that recorded release %d, such as with git revert, then run trailguide project sync; or set sources.%s.ref back to release/%d.", name, number, recorded, name, number, name, recorded), nil)
	case !p.updating:
		return fail("ref-changed", fmt.Sprintf("sources.%s.ref names release/%d, but the project records library release %d, and sync doesn't move a source to another release. Run trailguide project update %s to review the move to release %d and take it, or set sources.%s.ref back to release/%d.", name, number, recorded, name, number, name, recorded), nil)
	}
	return nil
}

// pinnedOnly returns a copy of recorded that holds only the rules pins names, with no library release, groups, or
// individual selection, so planning it chooses every other rule again; it is nil for a record of a repository that
// isn't a library.
func pinnedOnly(recorded *library.Snapshot, pins map[string]rules.Pin) *library.Snapshot {
	if recorded.NonLibrary {
		return nil
	}
	kept := *recorded
	kept.Release, kept.Commit, kept.Selection, kept.Groups, kept.RuleSelection = 0, "", rules.GroupSelection{}, []string{}, []string{}
	kept.Rules = map[string]library.ImportedRule{}
	for id, rule := range recorded.Rules {
		if _, pinned := pins[id]; pinned {
			kept.Rules[id] = rule
		}
	}
	return &kept
}

// versioned returns the release history, up to the planner's ceiling, failing with code ref-required when the
// repository has no library release to follow: a library before its first library release, or a repository that
// isn't a library.
func (p *planner) versioned() (releaseHistory, error) {
	history, err := p.releases()
	if err != nil {
		return releaseHistory{}, err
	}
	if p.ceiling != 0 && history.release(p.ceiling) == nil {
		return releaseHistory{}, fail("version-not-found", fmt.Sprintf("sources.%s.ref names release/%d, which the library no longer has, so Trail Guide can't choose versions for newly selected or pinned rules. Ask the library's maintainer to restore the tag, or change sources.%s.ref.", p.source.Name, p.ceiling, p.source.Name), nil)
	}
	if history.newest() == nil {
		return releaseHistory{}, fail("ref-required", fmt.Sprintf("the repository has no release/<number> tags, so Trail Guide can't choose a library release to take the source from: it isn't a library, or hasn't published its first library release. Set sources.%s.ref to the branch, tag, or full commit SHA to take it from.", p.source.Name), nil)
	}
	return history, nil
}

// planVersions plans a source that follows rule versions: recorded rules keep their versions unless a pin moves
// them or they have none, and newly selected rules get their pinned or newest version.
func (p *planner) planVersions() (sourcePlan, error) {
	plan := sourcePlan{rules: map[string]library.ImportedRule{}, individual: []string{}, warnings: []string{}, followsReleases: true}
	recorded := p.recorded
	if recorded != nil {
		for _, id := range slices.Sorted(maps.Keys(recorded.Rules)) {
			if p.selected(id) {
				plan.rules[id] = recorded.Rules[id]
			}
		}
	}
	for _, id := range slices.Sorted(maps.Keys(plan.rules)) {
		rule := plan.rules[id]
		if pin, pinned := p.source.Pins[id]; pinned && (rule.Version == nil || *rule.Version != pin.Version) {
			chosen, err := p.pinnedVersion(id, pin)
			if err != nil {
				return sourcePlan{}, err
			}
			plan.rules[id] = chosen
		} else if rule.Version == nil {
			// Only a removed ref leaves unreleased rules; each gets its newest version, if it has one.
			history, err := p.versioned()
			if err != nil {
				return sourcePlan{}, err
			}
			if _, published := history.newest().record.Rules[id]; !published {
				delete(plan.rules, id)
				continue
			}
			if plan.rules[id], err = p.newestVersion(id); err != nil {
				return sourcePlan{}, err
			}
		}
	}
	chosen := []string{}
	if recorded == nil || !recorded.Selection.Equal(p.source.Groups) {
		history, err := p.versioned()
		if err != nil {
			return sourcePlan{}, err
		}
		// A group is newly selected when the snapshot didn't import it in full, whatever its selector matched:
		// a wildcard's snapshot lacks the groups later library releases added.
		for _, id := range slices.Sorted(maps.Keys(history.newest().record.Rules)) {
			group := ruleGroup(id)
			if _, imported := plan.rules[id]; !imported && p.source.Groups.Includes(group) && (recorded == nil || !slices.Contains(recorded.Groups, group)) {
				chosen = append(chosen, id)
			}
		}
	}
	for _, id := range p.source.Rules {
		if _, imported := plan.rules[id]; imported || slices.Contains(chosen, id) {
			plan.individual = append(plan.individual, id)
			continue
		}
		if recorded != nil && slices.Contains(recorded.RuleSelection, id) && !hasRule(recorded, id) {
			plan.warnings = append(plan.warnings, p.retiredEntry("rules.include", id))
			continue
		}
		history, err := p.versioned()
		if err != nil {
			return sourcePlan{}, err
		}
		if _, current := history.newest().record.Rules[id]; current {
			chosen = append(chosen, id)
			plan.individual = append(plan.individual, id)
			continue
		}
		if err := p.unimported("rules.include", id, &plan); err != nil {
			return sourcePlan{}, err
		}
	}
	for _, id := range chosen {
		var err error
		if pin, pinned := p.source.Pins[id]; pinned {
			plan.rules[id], err = p.pinnedVersion(id, pin)
		} else {
			plan.rules[id], err = p.newestVersion(id)
		}
		if err != nil {
			return sourcePlan{}, err
		}
	}
	if err := p.requireEntries(&plan); err != nil {
		return sourcePlan{}, err
	}
	if recorded == nil || len(chosen) > 0 || selectsMore(recorded, p.source) {
		return plan, p.newestSharedFiles(&plan)
	}
	plan.release, plan.commit = recorded.Release, recorded.Commit
	if plan.release == 0 {
		// Only a removed ref that named a revision other than a library release records none.
		return plan, p.newestSharedFiles(&plan)
	}
	raiseSharedFiles(&plan)
	return plan, nil
}

// newestSharedFiles takes the plan's library-wide files from the newest library release, which is at least as new
// as every rule version.
func (p *planner) newestSharedFiles(plan *sourcePlan) error {
	history, err := p.versioned()
	if err != nil {
		return err
	}
	plan.release, plan.commit = history.newest().number, history.newest().commit
	return nil
}

// raiseSharedFiles moves the plan's library-wide files to the newest library release among its rule versions when
// that is newer, such as after a pin moved a rule up, because a rule version can rely on the shared files and group
// metadata its own library release published.
func raiseSharedFiles(plan *sourcePlan) {
	for _, id := range slices.Sorted(maps.Keys(plan.rules)) {
		if rule := plan.rules[id]; rule.Release > plan.release {
			plan.release, plan.commit = rule.Release, rule.Commit
		}
	}
}

// selectsMore reports whether source selects a rule or group that recorded's selection didn't: an individually
// selected rule recorded's lacks, a listed group recorded didn't import in full, or a wildcard wider than recorded's.
func selectsMore(recorded *library.Snapshot, source rules.Source) bool {
	for _, id := range source.Rules {
		if !slices.Contains(recorded.RuleSelection, id) {
			return true
		}
	}
	if recorded.Selection.Equal(source.Groups) {
		return false
	}
	if source.Groups.Pattern != "" && !recorded.Selection.Covers(rules.GroupSelection{Pattern: source.Groups.Pattern}) {
		return true
	}
	for _, group := range source.Groups.Groups {
		if !slices.Contains(recorded.Groups, group) {
			return true
		}
	}
	return false
}

// planRevision plans a source that imports one revision with ref. A recorded snapshot of the same ref, however it
// is written, keeps its commit and rule versions, unless the planner resolves the ref again; otherwise the ref is
// resolved again. Rules newly selected at the revision record the version its library release record lists, or, for
// any other revision, the newest published version whose files match, or none. A ref that names a branch of a
// library warns that the source takes the library's unreleased changes.
func (p *planner) planRevision() (sourcePlan, error) {
	plan := sourcePlan{rules: map[string]library.ImportedRule{}, individual: []string{}, warnings: []string{}}
	recorded := p.recorded
	if recorded != nil && p.source.Ref.Equal(recorded.Ref) && !p.again {
		plan.release, plan.commit = recorded.Release, recorded.Commit
		if err := p.repo.fetchCommits(p.ctx, []string{plan.commit}, ""); err != nil {
			// Explaining the loss can ask the repository what a bare ref names, so only a lost commit does it.
			var failure *Error
			if errors.As(err, &failure) && failure.Code == "version-not-found" {
				return sourcePlan{}, fail("version-not-found", p.lostCommit(), nil)
			}
			return sourcePlan{}, err
		}
	} else {
		recorded = nil
		resolved, err := p.resolveRef()
		if err != nil {
			return sourcePlan{}, err
		}
		plan.release, plan.commit, plan.resolved = resolved.release, resolved.commit, resolved.kind
	}
	isLibrary, err := p.repo.isLibrary(p.ctx, plan.commit)
	if err != nil {
		return sourcePlan{}, err
	}
	if !isLibrary {
		if p.source.HasRules() {
			return sourcePlan{}, &ValidationError{Location: "sources." + p.source.Name + ".rules", Problem: fmt.Sprintf("the repository has no library manifest, a .trailguide/config.yaml with a library section, at %s, so it isn't a library and has no rules to include; remove rules, and include its skills with skills.include", revisionName(plan.release, plan.commit))}
		}
		if len(p.source.Skills) == 0 && !p.source.Curated {
			return sourcePlan{}, &ValidationError{Location: "sources." + p.source.Name, Problem: fmt.Sprintf("the repository has no library manifest at %s, so it isn't a library and requires no skills; include its skills with skills.include, such as include: [\"*\"]", revisionName(plan.release, plan.commit))}
		}
		plan.nonLibrary = true
		return plan, nil
	}
	tree, err := p.repo.tree(p.ctx, plan.commit)
	if err != nil {
		return sourcePlan{}, err
	}
	present := rulesInTree(tree)
	imported := []string{}
	for _, id := range present {
		if p.source.Groups.Includes(ruleGroup(id)) {
			imported = append(imported, id)
		}
	}
	for _, id := range p.source.Rules {
		switch {
		case slices.Contains(present, id):
			plan.individual = append(plan.individual, id)
			if !slices.Contains(imported, id) {
				imported = append(imported, id)
			}
		case recorded != nil && slices.Contains(recorded.RuleSelection, id) && !hasRule(recorded, id):
			plan.warnings = append(plan.warnings, p.retiredEntry("rules.include", id))
		default:
			if err := p.unimported("rules.include", id, &plan); err != nil {
				return sourcePlan{}, err
			}
		}
	}
	slices.Sort(imported)
	for _, id := range imported {
		if recorded != nil && hasRule(recorded, id) {
			plan.rules[id] = recorded.Rules[id]
			continue
		}
		if plan.rules[id], err = p.revisionVersion(id, plan); err != nil {
			return sourcePlan{}, err
		}
	}
	if err := p.requireEntries(&plan); err != nil {
		return sourcePlan{}, err
	}
	branch := plan.resolved == rules.GitRefBranch || p.source.Ref.Kind() == rules.GitRefBranch
	if !branch && plan.resolved == "" && plan.release == 0 {
		// Keeping the recorded commit doesn't resolve a bare name, which may name a branch, so the branch warning
		// stays on every sync.
		branch = p.bareBranch()
	}
	if branch || plan.release == 0 {
		var current map[string]trailguide.RuleVersion
		if branch {
			history, err := p.releases()
			if err != nil {
				return sourcePlan{}, err
			}
			if newest := history.newest(); newest != nil {
				current = newest.record.Rules
			}
		}
		if warning := unreleasedWarning(p.source, plan.rules, branch, current); warning != "" {
			plan.warnings = append(plan.warnings, warning)
		}
	}
	return plan, nil
}

// lostCommit explains that the repository no longer has the commit the source record holds for the source's ref.
// A source whose ref certainly moves, a branch, or a tag of a repository that isn't a library, takes the commit its
// ref names now with trailguide project update.
func (p *planner) lostCommit() string {
	name := p.source.Name
	kind := p.source.Ref.Kind()
	branch := kind == rules.GitRefBranch || !p.recorded.NonLibrary && p.recorded.Release == 0 && p.bareBranch()
	if branch || p.recorded.NonLibrary && kind != rules.GitRefCommit {
		return fmt.Sprintf("The commit that vendor/%s/_source.json records for sources.%s.ref, %s, is missing from the source's repository, so the branch or tag may have been force-pushed, moved, or deleted. Run trailguide project update %s to take the commit sources.%s.ref names now.", name, name, p.source.Ref, name, name)
	}
	return fmt.Sprintf("The commit that vendor/%s/_source.json records for sources.%s.ref is missing from the library's repository, so the library may have rewritten its history. Ask the library's maintainer to restore it, or set sources.%s.ref to a tag or commit the library still has, then run trailguide project sync. As a last resort, delete vendor/%s and run trailguide project sync to resolve the ref again, which may import different content.", name, name, name, name)
}

// bareBranch reports whether the source's ref is a bare name that the repository has as a branch now. A name it
// can't resolve, such as a deleted branch, reports false.
func (p *planner) bareBranch() bool {
	if !p.source.Ref.Bare() {
		return false
	}
	name, err := p.repo.bareRef(p.ctx, p.source)
	return err == nil && strings.HasPrefix(name, "refs/heads/")
}

// resolvedRef is the revision a source's ref names in its repository now.
type resolvedRef struct {
	// release is the library release the ref names, or 0 when it isn't a library release tag.
	release int
	commit  string
	// kind is what the ref named: a bare name's is GitRefTag or GitRefBranch, as the repository resolved it.
	kind rules.GitRefKind
}

// resolveRef returns the revision the source's ref names now. A missing branch, tag, or commit fails with code
// version-not-found, and a bare name of both a branch and a tag with ambiguous-ref. A bare name shaped like a library
// release, such as release/5, names that release only when the repository has no branch of that name.
func (p *planner) resolveRef() (resolvedRef, error) {
	if ref := p.source.Ref; ref.Kind() == rules.GitRefTag {
		if number, err := trailguide.ParseReleaseTag(ref.Name()); err == nil {
			if ref.Bare() {
				name, err := p.repo.bareRef(p.ctx, p.source)
				var failure *Error
				switch {
				case err == nil && strings.HasPrefix(name, "refs/heads/"):
					commit, kind, err := p.repo.fetchName(p.ctx, p.source, name)
					return resolvedRef{commit: commit, kind: kind}, err
				// The release history explains a release the library doesn't have.
				case err != nil && (!errors.As(err, &failure) || failure.Code != "version-not-found"):
					return resolvedRef{}, err
				}
			}
			history, err := p.releases()
			if err != nil {
				return resolvedRef{}, err
			}
			release := history.release(number)
			if release == nil {
				return resolvedRef{}, fail("version-not-found", fmt.Sprintf("sources.%s.ref: the library has no %s; check the ref.", p.source.Name, p.source.Ref), nil)
			}
			return resolvedRef{release: number, commit: release.commit, kind: rules.GitRefTag}, nil
		}
	}
	commit, kind, err := p.repo.fetchRef(p.ctx, p.source)
	return resolvedRef{commit: commit, kind: kind}, err
}

// revisionVersion chooses the version of rule id imported at the plan's revision.
func (p *planner) revisionVersion(id string, plan sourcePlan) (library.ImportedRule, error) {
	history, err := p.releases()
	if err != nil {
		return library.ImportedRule{}, err
	}
	if plan.release != 0 {
		release := history.release(plan.release)
		if release == nil {
			return library.ImportedRule{}, fail("version-not-found", fmt.Sprintf("sources.%s.ref names release/%d, which the library no longer has, so newly selected rule %s has no version to import. Ask the library's maintainer to restore the tag, or change sources.%s.ref.", p.source.Name, plan.release, id, p.source.Name), nil)
		}
		version, listed := release.record.Rules[id]
		if !listed {
			return library.ImportedRule{}, fail("invalid-release-tag", fmt.Sprintf("Library release release/%d contains rule %s, but its release record doesn't list it. Don't create or move release tags by hand.", plan.release, id), nil)
		}
		return p.publishedVersion(id, version)
	}
	files, err := p.repo.ruleFiles(p.ctx, plan.commit, id)
	if err != nil {
		return library.ImportedRule{}, err
	}
	for _, release := range history.published(id) {
		published, err := p.repo.ruleFiles(p.ctx, release.commit, id)
		if err != nil {
			return library.ImportedRule{}, err
		}
		if maps.Equal(files, published) {
			version := release.record.Rules[id]
			return library.ImportedRule{Version: &version, Release: release.number, Commit: release.commit}, nil
		}
	}
	return library.ImportedRule{Commit: plan.commit}, nil
}

// pinnedVersion returns the pinned version of rule id, failing with code version-not-found when the rule never
// published it.
func (p *planner) pinnedVersion(id string, pin rules.Pin) (library.ImportedRule, error) {
	history, err := p.versioned()
	if err != nil {
		return library.ImportedRule{}, err
	}
	if history.publisher(id, pin.Version) == nil {
		if later := p.history.publisher(id, pin.Version); later != nil {
			return library.ImportedRule{}, fail("version-not-found", fmt.Sprintf("sources.%s.rules.pins.%s: version %s comes from library release %d, after release/%d, which sources.%s.ref names. Set sources.%s.ref to release/%d or later, or pin a version that release/%d has: %s.", p.source.Name, id, pin.Version, later.number, p.ceiling, p.source.Name, p.source.Name, later.number, p.ceiling, history.versionList(id)), nil)
		}
		return library.ImportedRule{}, fail("version-not-found", fmt.Sprintf("sources.%s.rules.pins.%s: the rule never published version %s; check the pin. Its published versions, newest first: %s.", p.source.Name, id, pin.Version, history.versionList(id)), nil)
	}
	return p.publishedVersion(id, pin.Version)
}

// newestVersion returns the version of rule id in the newest library release.
func (p *planner) newestVersion(id string) (library.ImportedRule, error) {
	history, err := p.versioned()
	if err != nil {
		return library.ImportedRule{}, err
	}
	return p.publishedVersion(id, history.newest().record.Rules[id])
}

// publishedVersion returns version of rule id with the library release that published it.
func (p *planner) publishedVersion(id string, version trailguide.RuleVersion) (library.ImportedRule, error) {
	history, err := p.releases()
	if err != nil {
		return library.ImportedRule{}, err
	}
	release := history.publisher(id, version)
	if release == nil {
		return library.ImportedRule{}, fail("invalid-release-tag", fmt.Sprintf("No library release's record publishes version %s of rule %s, although a later record lists it. Don't create or move release tags by hand.", version, id), nil)
	}
	return library.ImportedRule{Version: &version, Release: release.number, Commit: release.commit}, nil
}

// requireEntries checks that each pin and exclusion names an imported rule, and that the library published each
// replacement's basedOn version. An entry naming a retired rule adds a warning; any other fails.
func (p *planner) requireEntries(plan *sourcePlan) error {
	if err := p.requireBasedOn(); err != nil {
		return err
	}
	for _, id := range slices.Sorted(maps.Keys(p.source.Pins)) {
		if _, imported := plan.rules[id]; imported {
			continue
		}
		// A pin of a rule the last sync recorded as retired, and didn't import, needs no release history to warn.
		if recorded := p.recorded; recorded != nil && !hasRule(recorded, id) && slices.Contains(recorded.RetiredRules, id) {
			plan.warnings = append(plan.warnings, p.retiredEntry("rules.pins", id))
			continue
		}
		if err := p.unimported("rules.pins", id, plan); err != nil {
			return err
		}
	}
	for _, id := range slices.Sorted(maps.Keys(p.source.Exclude)) {
		matched := slices.ContainsFunc(slices.Collect(maps.Keys(plan.rules)), func(imported string) bool { return rules.SelectorMatches(id, imported) })
		if matched {
			continue
		}
		if err := p.unimported("rules.exclude", id, plan); err != nil {
			return err
		}
	}
	return nil
}

// requireBasedOn fails with code version-not-found when a replacement's basedOn names a version its rule never
// published, listing the versions it did. It reads the release history only when a replacement records basedOn.
func (p *planner) requireBasedOn() error {
	for _, id := range slices.Sorted(maps.Keys(p.source.Exclude)) {
		based := p.source.Exclude[id].BasedOn
		if based == nil {
			continue
		}
		if _, err := p.releases(); err != nil {
			return err
		}
		// A replacement can be based on a version newer than the library release a ref names.
		history := *p.history
		if history.publisher(id, *based) == nil {
			return fail("version-not-found", fmt.Sprintf("sources.%s.rules.exclude.%s.basedOn: the rule never published version %s; check basedOn. Its published versions, newest first: %s.", p.source.Name, id, based, history.versionList(id)), nil)
		}
	}
	return nil
}

// unimported handles a configuration entry naming a rule, or for an exclusion a selector, that matches no rule the
// source imports: a warning when it matches only rules the library retired that the source would otherwise import,
// through its include selectors or its recorded snapshot, and a validation error for any other.
func (p *planner) unimported(field, id string, plan *sourcePlan) error {
	history, err := p.releases()
	if err != nil {
		return err
	}
	if len(p.retiredMatches(id, history)) > 0 {
		plan.warnings = append(plan.warnings, p.retiredEntry(field, id))
		return nil
	}
	location := "sources." + p.source.Name + "." + field + "." + id
	switch {
	case field == "rules.include":
		return &ValidationError{Location: "sources." + p.source.Name + ".rules.include", Problem: "the library has no rule " + id + " to import; check the rule ID"}
	case strings.HasSuffix(id, "*"):
		return &ValidationError{Location: location, Problem: "matches no rule this source imports; name rules that its rules.include selects"}
	}
	return &ValidationError{Location: location, Problem: "rule is not imported by this source; name a rule that its rules.include selects"}
}

// retiredMatches returns, unordered, the rules the library retired that the source would otherwise import and that
// the configuration entry id, a rule ID or an exclusion's selector, matches.
func (p *planner) retiredMatches(id string, history releaseHistory) []string {
	if !strings.HasSuffix(id, "*") {
		if p.wouldImport(id) && history.retired(id) {
			return []string{id}
		}
		return nil
	}
	matches := []string{}
	for _, release := range history.releases {
		for retired := range release.record.Retired {
			if rules.SelectorMatches(id, retired) && p.wouldImport(retired) && !slices.Contains(matches, retired) {
				matches = append(matches, retired)
			}
		}
	}
	return matches
}

// wouldImport reports whether the source would import rule id if the library still published it: its include
// selectors select it, or recorded imported it or listed it as retired.
func (p *planner) wouldImport(id string) bool {
	return p.selected(id) || p.recorded != nil && (hasRule(p.recorded, id) || slices.Contains(p.recorded.RetiredRules, id))
}

// retiredEntry warns that a configuration entry names a retired rule, or, for a selector, matches only retired rules.
func (p *planner) retiredEntry(field, id string) string {
	if strings.HasSuffix(id, "*") {
		return fmt.Sprintf("sources.%s.%s names %s, which matches only rules the library retired, so the entry no longer does anything; delete it.", p.source.Name, field, id)
	}
	return fmt.Sprintf("sources.%s.%s names %s, a rule the library retired, so the entry no longer does anything; delete it.", p.source.Name, field, id)
}

// requireUnmoved checks the plan's library releases and versions, which it may have kept from the source record,
// against the library's release history. For a source that follows rule versions, it reads the history when the
// planner hasn't yet: a rule version its library release doesn't publish, or a library release that supplies a rule
// or the plan's library-wide files but that the library doesn't have, fails as an invalid source record, as a hand
// edit or a merge resolution of _source.json can leave it, and a library release tag that now names a different
// commit than the plan records fails with invalid-release-tag. A source that uses ref keeps the commit its ref named
// when recorded, even after the tag moves or is gone, so it reads the history only when choosing versions needed it,
// and then checks only the versions of library releases the history has. Sync reads only records it wrote, whose
// checksum matches, so these exemptions never cover a record changed outside sync.
func (p *planner) requireUnmoved(plan sourcePlan) error {
	// A ref that names a library release keeps its recorded commit, as other refs do.
	usesRef := !p.source.Ref.IsZero()
	versioned := plan.release != 0 || slices.ContainsFunc(slices.Collect(maps.Values(plan.rules)), func(rule library.ImportedRule) bool { return rule.Version != nil })
	if !versioned || usesRef && p.history == nil {
		return nil
	}
	history, err := p.releases()
	if err != nil {
		return err
	}
	record := "vendor/" + p.source.Name + "/_source.json"
	missing := func(location string, number int) error {
		return &ValidationError{Location: location, Problem: fmt.Sprintf("records library release %d, which the library doesn't have; restore %s, such as from version control, or, if the library deleted its release/%d tag, ask the library's maintainer to restore it; then run trailguide project sync again", number, record, number)}
	}
	for _, id := range slices.Sorted(maps.Keys(plan.rules)) {
		rule := plan.rules[id]
		if rule.Version == nil {
			continue
		}
		release := history.release(rule.Release)
		if release == nil {
			if usesRef {
				continue
			}
			return missing(record+".rules."+id, rule.Release)
		}
		if published, ok := release.record.Rules[id]; !ok || published != *rule.Version {
			publishes := "which doesn't publish the rule"
			if ok {
				publishes = "which publishes version " + published.String() + " of the rule"
			}
			return &ValidationError{Location: record + ".rules." + id, Problem: fmt.Sprintf("records version %s from library release %d, %s; restore %s, such as from version control, then run trailguide project sync again", rule.Version, rule.Release, publishes, record)}
		}
	}
	if plan.release != 0 && history.release(plan.release) == nil && !usesRef {
		return missing(record+".release", plan.release)
	}
	// A recorded ref keeps its commit, wherever its tags point now.
	if usesRef && p.recorded != nil && (p.recorded.Ref.Equal(p.source.Ref) || plan.followsReleases && plan.release == p.recorded.Release) {
		return nil
	}
	if release := history.release(plan.release); plan.release != 0 && release != nil && release.commit != plan.commit {
		return fail("invalid-release-tag", fmt.Sprintf("Library release tag release/%d now names a different commit than vendor/%s/_source.json records for the library's shared files. Library release tags must not move; ask the library's maintainer, or delete vendor/%s and run trailguide project sync to use the tag's current commit.", plan.release, p.source.Name, p.source.Name), nil)
	}
	for _, id := range slices.Sorted(maps.Keys(plan.rules)) {
		rule := plan.rules[id]
		if release := history.release(rule.Release); release != nil && release.commit != rule.Commit {
			return fail("invalid-release-tag", fmt.Sprintf("Library release tag release/%d now names a different commit than vendor/%s/_source.json records for %s. Library release tags must not move; ask the library's maintainer, or delete vendor/%s and run trailguide project sync to use the tag's current commit.", rule.Release, p.source.Name, id, p.source.Name), nil)
		}
	}
	return nil
}

// selected reports whether the source's include selectors select rule id, through a group or wildcard or by its ID.
func (p *planner) selected(id string) bool {
	return p.source.Groups.Includes(ruleGroup(id)) || slices.Contains(p.source.Rules, id)
}

// unreleasedWarning names a source that imports a revision other than a library release, and its unreleased rules.
// It is empty when every imported rule matches a published version, as at a library release's commit. For a branch
// it always warns, and lists as changed since the last release each rule that is unreleased or whose version isn't
// the one current, the newest library release's rule versions, lists, such as a rule reverted to an older version.
func unreleasedWarning(source rules.Source, imported map[string]library.ImportedRule, branch bool, current map[string]trailguide.RuleVersion) string {
	unreleased := []string{}
	for _, id := range slices.Sorted(maps.Keys(imported)) {
		if imported[id].Version == nil {
			unreleased = append(unreleased, id)
		}
	}
	listed := ""
	if len(unreleased) > 0 {
		listed = " Unreleased rules: " + strings.Join(unreleased, ", ") + "."
	}
	if branch {
		differ := slices.DeleteFunc(slices.Sorted(maps.Keys(imported)), func(id string) bool {
			version, listed := current[id]
			return imported[id].Version != nil && (!listed || *imported[id].Version == version)
		})
		changed := ""
		if len(differ) > 0 {
			changed = " Rules changed since the last release: " + strings.Join(differ, ", ") + "."
		}
		return fmt.Sprintf("Source %s follows the %s branch of its library. A library publishes its changes in releases: numbered snapshots with change notes that say what changed in each rule and skill. On %s, you get every change as soon as it's made, but rules and skills changed since the last release have no change notes. Set sources.%s.ref to a library release, such as release/4, to follow releases instead.%s", source.Name, source.Ref.Name(), source.Ref.Name(), source.Name, changed)
	}
	if listed == "" {
		return ""
	}
	return fmt.Sprintf("Source %s imports %s, which isn't a library release, so rules with unreleased changes have no version.%s", source.Name, source.Ref, listed)
}

// ruleGroup returns the group of a valid library rule ID.
func ruleGroup(id string) string {
	parts := strings.SplitN(id, "/", 3)
	return parts[0] + "/" + parts[1]
}

// hasRule reports whether the snapshot imported rule id.
func hasRule(snapshot *library.Snapshot, id string) bool {
	_, ok := snapshot.Rules[id]
	return ok
}

// rulesInTree returns, sorted, the IDs of the rules whose Markdown files tree holds.
func rulesInTree(tree map[string]treeEntry) []string {
	ids := []string{}
	for file := range tree {
		if id, ok := librarytree.VersionedRule(file); ok && file == id+".md" {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	return ids
}
