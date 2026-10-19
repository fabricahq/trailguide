// Plan project update: move rules to their newest versions, preview each change, and import the planned versions.

package imports

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/fabricahq/trailguide/internal/errs"
	"github.com/fabricahq/trailguide/internal/gitexec"
	"github.com/fabricahq/trailguide/internal/library"
	"github.com/fabricahq/trailguide/internal/rules"
	"github.com/fabricahq/trailguide/trailguide"
)

// UpdateTarget names what an update moves: every rule of Source when Rule is empty, or only the library rule Rule.
type UpdateTarget struct {
	Source string
	Rule   string
}

// UpdateChange is how an update affects one rule, as the update preview lists it.
type UpdateChange string

// The update preview's changes, in the order it lists them.
const (
	UpdateMajor    UpdateChange = "major"
	UpdateMinor    UpdateChange = "minor"
	UpdatePatch    UpdateChange = "patch"
	UpdateNew      UpdateChange = "new"
	UpdateRetired  UpdateChange = "retired"
	UpdateReplaced UpdateChange = "replaced"
	UpdatePinned   UpdateChange = "pinned"
)

var updateChangeOrder = []UpdateChange{UpdateMajor, UpdateMinor, UpdatePatch, UpdateNew, UpdateRetired, UpdateReplaced, UpdatePinned}

// SourceUpdate is one source's part of the update preview.
type SourceUpdate struct {
	Name string `json:"name"`
	// Ref is the source's ref, which update doesn't move, so Rules is empty; it is empty for other sources.
	Ref string `json:"ref,omitempty"`
	// Rules is sorted by change, in the order of the UpdateChange constants, then by rule ID. It is empty, never
	// nil, when nothing changes.
	Rules []RuleUpdate `json:"rules"`
	// SharedFiles is how the update moves the source's library-wide files, or nil when they stay.
	SharedFiles *SharedFilesUpdate `json:"sharedFiles,omitempty"`
	// Skills compares the skills the source installs once the update is applied with those its source record
	// locks, as skillChanges does. It is empty, never nil, when they don't differ.
	Skills []SkillUpdate `json:"skills"`
	// ReplacedSkills lists, sorted by ID, the skills the project replaces with authored skills whose copy at the
	// revision the update takes the source's skills from differs from the copy the replacement is based on. It is
	// empty, never nil, when there are none.
	ReplacedSkills []ReplacedSkillUpdate `json:"replacedSkills"`
	// SkillLevels lists, sorted by ID, the skills whose level the library changes at the revision the update takes
	// the source's skills from. It is empty, never nil, when there are none.
	SkillLevels []SkillLevelUpdate `json:"skillLevels"`
	// Curated lists, sorted by name, the sources the library curates skills from whose revision or skills the update
	// changes; it is omitted when there are none. A library source the update doesn't name is listed with only these
	// rows when a source the update names overrides a repository it curates from.
	Curated []CuratedUpdate `json:"curated,omitempty"`
}

// SkillLevelUpdate is a row of the update preview for a skill whose level in its library changes: a skill whose
// level differs between the revisions, and a required skill the library adds or removes, which installs or
// uninstalls whatever the project includes.
type SkillLevelUpdate struct {
	ID string `json:"id"`
	// From is the level at the commit the source record locks, and To the level at the commit the update takes
	// skills from; each is empty when the library has no such skill at that commit.
	From string `json:"from"`
	To   string `json:"to"`
}

// CuratedUpdate moves the revision of a source a library curates skills from, which the project resolves from the
// ref the library names: a branch's newest commit, a tag's commit, or, without a ref, the repository's newest library
// release.
type CuratedUpdate struct {
	Name       string `json:"name"`
	Repository string `json:"repository"`
	// Ref is the ref the library names, or empty when it follows the newest library release.
	Ref string `json:"ref,omitempty"`
	// Commit is the move from the commit the project recorded to the one the update takes the curated skills from;
	// it is nil when the commit doesn't move. Its From is empty for a source the project didn't curate from before,
	// and its To when the project no longer takes any skills from the source, so the update removes those it
	// installed. Release is the same move in library releases, when both commits are library releases and Ref is
	// empty, and is nil otherwise.
	Commit  *CommitMove        `json:"commit,omitempty"`
	Release *SharedFilesUpdate `json:"release,omitempty"`
	// TagMoved reports that Ref names a tag that now names a different commit than at the last update.
	TagMoved bool `json:"tagMoved,omitempty"`
	// Changes compares the curated skills the project installs from the source, and the skills they use, at the new
	// commit with those at the recorded one, as SourceUpdate.Skills compares a source's own skills, by their IDs in
	// the library. It is empty, never nil, when they don't differ.
	Changes []SkillUpdate `json:"changes"`
}

// CommitMove is the move of a source's revision between two commits.
type CommitMove struct {
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
}

// SharedFilesUpdate moves a source's library-wide files, such as group metadata and shared assets, from the library
// release that supplies them to a newer one.
type SharedFilesUpdate struct {
	From int `json:"from"`
	To   int `json:"to"`
}

// RuleUpdate is one row of the update preview.
type RuleUpdate struct {
	ID     string       `json:"id"`
	Change UpdateChange `json:"change"`
	// From is the version the project imports; it is nil for a new rule.
	From *trailguide.RuleVersion `json:"from,omitempty"`
	// To is the version the update installs; it is nil for retired and pinned rules, which don't move, including a
	// pinned replaced rule.
	To *trailguide.RuleVersion `json:"to,omitempty"`
	// Newest is a pinned rule's newest version, including a pinned replaced rule's, and LastVersion a retired
	// rule's; each is nil otherwise.
	Newest      *trailguide.RuleVersion `json:"newest,omitempty"`
	LastVersion *trailguide.RuleVersion `json:"lastVersion,omitempty"`
	// Summaries holds one line per change note, oldest first: every version after From up to To, every version of
	// a new rule, or the retirement. It is empty, never nil, for a pinned rule. SummaryVersions holds the version
	// each summary belongs to, in the same order, a retired rule's being its last version.
	Summaries       []string                 `json:"summaries"`
	SummaryVersions []trailguide.RuleVersion `json:"summaryVersions"`
	// ReplacedBy is the library rule that replaces a retired rule, when there is one.
	ReplacedBy string `json:"replacedBy,omitempty"`
	// ReplacementRetired reports that the library later retired ReplacedBy too; CurrentReplacement is then the rule
	// that its replacements lead to, or empty when they lead to none.
	ReplacementRetired bool   `json:"replacementRetired,omitempty"`
	CurrentReplacement string `json:"currentReplacement,omitempty"`
	// ReplacementLocalRule is the project's local rule that already replaces ReplacedBy, when the project replaced
	// it, relative to the Trail Guide directory, so the row doesn't recommend a rule agents don't read.
	ReplacementLocalRule string `json:"replacementLocalRule,omitempty"`
	// LocalRule is the project's local rule that replaces a replaced rule, relative to the Trail Guide directory.
	LocalRule string `json:"localRule,omitempty"`
	// BasedOn is the version of a replaced rule that its local rule incorporates, as the exclusion records it; the
	// row then lists the changes after it, From being the imported version. It is nil otherwise.
	BasedOn *trailguide.RuleVersion `json:"basedOn,omitempty"`
	// Pin is the configured pin of a pinned rule, or of a retired rule a pin keeps.
	Pin *rules.Pin `json:"pin,omitempty"`
	// Decision is "keep" when the project pins the rule at From instead of applying the change, "exclude" when it
	// excludes a new rule, or "update-fork" when it replaces a replaced rule's local rule with a fork of the
	// version ReviewedVersion returns; Reason is recorded with a pin or exclusion. Planning leaves both empty.
	Decision string `json:"decision,omitempty"`
	Reason   string `json:"reason,omitempty"`
	// Overwrites and Removes list, for an update-fork decision, the local files that replacing the fork writes again
	// and deletes: of the local rule and the files in its asset directory, those the new fork has and those it
	// doesn't, relative to the Trail Guide directory. The project's preview leaves both empty, never nil, for other
	// rows; planning leaves them nil.
	Overwrites []string `json:"overwrites"`
	Removes    []string `json:"removes"`
}

// ReviewedVersion returns the newest version a replaced row lists changes up to, which replacing its fork forks and
// records as basedOn: To, or Newest when a pin keeps the imported copy. It is nil for other rows.
func (r RuleUpdate) ReviewedVersion() *trailguide.RuleVersion {
	if r.Change != UpdateReplaced {
		return nil
	}
	if r.To != nil {
		return r.To
	}
	return r.Newest
}

// Update is a planned project update: each source's preview, and what installing it imports.
type Update struct {
	// Sources lists, in configuration order, the sources the update names, and each library source it doesn't name
	// whose curated skills come from one it does, with only its curated rows. A source that uses ref has no rule
	// rows.
	Sources []SourceUpdate
	// Warnings explain configuration the update tolerates, as sync reports them, for the sources it names. It is
	// empty, never nil, when there are none.
	Warnings []string
	// plans holds each source the update names; recorded holds every recorded snapshot, from which the sources it
	// doesn't name import what sync would.
	plans    map[string]plannedSource
	recorded map[string]library.Snapshot
	// curated holds the preview rows of the library sources the update doesn't name whose curated skills come from
	// a source it does name, which overrides the library's ref, by library source name.
	curated map[string][]CuratedUpdate
}

// plannedSource is one source an update names, with what its planner read, so applying the project's decisions
// reads nothing from the library again.
type plannedSource struct {
	// before is what project sync would import.
	before sourcePlan
	// moved maps every rule the source imports after the update, before the project's decisions, to its version.
	// It is nil for a source that uses ref, which the update doesn't move.
	moved map[string]library.ImportedRule
	// scoped reports that the update names some of the source's rules rather than the whole source.
	scoped   bool
	recorded *library.Snapshot
	history  releaseHistory
	// skills maps each commit that can supply the source's skills once the project decides, the shared files'
	// commit, to the preview of the skills the source installs from it.
	skills map[string]skillPreview
}

// sharedFiles returns the library release, and its commit, that supplies the library-wide files once the update
// installs rules: the newest library release for an update of the whole source; for an update scoped to some
// rules, before's, raised only as far as the newest library release among the rules the update actually moves,
// since a rule version can rely on the shared files its library release published.
func (s plannedSource) sharedFiles(rules map[string]library.ImportedRule) (int, string) {
	if !s.scoped {
		return s.history.newest().number, s.history.newest().commit
	}
	shared := sourcePlan{release: s.before.release, commit: s.before.commit, rules: rules}
	raiseSharedFiles(&shared)
	return shared.release, shared.commit
}

// SharedFiles returns how the update moves the shared files of source, which it names, once the rules kept lists
// stay at their versions from before the update, or nil when they stay.
func (u Update) SharedFiles(source string, kept []string) *SharedFilesUpdate {
	planned, named := u.plans[source]
	if !named || planned.moved == nil {
		return nil
	}
	if release, _ := planned.keeping(kept); release != planned.before.release {
		return &SharedFilesUpdate{From: planned.before.release, To: release}
	}
	return nil
}

// Skills returns the preview rows of the skills of source, which the update names, once the rules kept lists stay at
// their versions from before the update: the skills come from the revision that then supplies the shared files.
// It is empty, never nil, when the skills don't change or the update doesn't name source, and fails as installing
// that revision would when planning couldn't read its skills, such as one skills.include names that it lacks.
func (u Update) Skills(source string, kept []string) ([]SkillUpdate, error) {
	planned, named := u.plans[source]
	if !named {
		return []SkillUpdate{}, nil
	}
	rows, err := planned.skillRows(kept)
	if err != nil {
		return nil, sourceError(source, err)
	}
	return rows, nil
}

// skillRows returns a copy of the preview rows of the revision that supplies the source's skills once the rules kept
// lists stay at their versions from before the update, or the failure reading them. Planning reads every revision
// that keeping rules can choose, as skillRevisions lists them.
func (s plannedSource) skillRows(kept []string) ([]SkillUpdate, error) {
	_, commit := s.keeping(kept)
	preview, read := s.skills[commit]
	switch {
	case !read:
		return nil, fmt.Errorf("the update preview didn't read the skills at commit %s", commit)
	case preview.err != nil:
		return nil, preview.err
	}
	return slices.Clone(preview.rows), nil
}

// ReplacedSkills returns the replaced skill rows of source, which the update names, once the rules kept lists stay
// at their versions from before the update, as Skills chooses the revision. It is empty, never nil, when there are
// none or the update doesn't name source.
func (u Update) ReplacedSkills(source string, kept []string) []ReplacedSkillUpdate {
	planned, named := u.plans[source]
	if !named {
		return []ReplacedSkillUpdate{}
	}
	return planned.replacedRows(kept)
}

// replacedRows returns a copy of the replaced skill rows of the revision that supplies the source's skills once the
// rules kept lists stay at their versions from before the update; empty, never nil, when planning read none there.
func (s plannedSource) replacedRows(kept []string) []ReplacedSkillUpdate {
	_, commit := s.keeping(kept)
	rows := slices.Clone(s.skills[commit].replaced)
	if rows == nil {
		return []ReplacedSkillUpdate{}
	}
	return rows
}

// SkillLevels returns the level change rows of source, which the update names, once the rules kept lists stay at
// their versions from before the update, as Skills chooses the revision. It is empty, never nil, when no level
// changes, the update doesn't name source, or source uses ref, which the update doesn't move.
func (u Update) SkillLevels(source string, kept []string) []SkillLevelUpdate {
	planned, named := u.plans[source]
	if !named || planned.moved == nil {
		return []SkillLevelUpdate{}
	}
	return planned.levelRows(kept)
}

// levelRows returns a copy of the level change rows of the revision that supplies the source's skills once the rules
// kept lists stay at their versions from before the update; empty, never nil, when planning read none there.
func (s plannedSource) levelRows(kept []string) []SkillLevelUpdate {
	_, commit := s.keeping(kept)
	rows := slices.Clone(s.skills[commit].levels)
	if rows == nil {
		return []SkillLevelUpdate{}
	}
	return rows
}

// keeping returns the library release, and its commit, that supplies the library-wide files and skills once the
// rules kept lists stay at their versions from before the update. A source that uses ref keeps its revision.
func (s plannedSource) keeping(kept []string) (int, string) {
	if s.moved == nil {
		return s.before.release, s.before.commit
	}
	decided := maps.Clone(s.moved)
	for _, id := range kept {
		if rule, ok := s.before.rules[id]; ok {
			decided[id] = rule
		}
	}
	return s.sharedFiles(decided)
}

// skillRevisions returns a plan of each revision that can supply the source's skills once the project decides: the
// shared files' revision before the update for a scoped update, since keeping every rule it moves keeps them there,
// and every library release a moved rule comes from, since keeping the others raises them only that far. after is
// the plan without decisions, whose revision every whole-source update takes.
func (s plannedSource) skillRevisions(after sourcePlan) []sourcePlan {
	plans := []sourcePlan{after}
	if !s.scoped {
		return plans
	}
	revision := func(release int, commit string) sourcePlan {
		return sourcePlan{release: release, commit: commit, locked: s.before.locked, nonLibrary: s.before.nonLibrary}
	}
	plans = append(plans, revision(s.before.release, s.before.commit))
	for _, id := range slices.Sorted(maps.Keys(s.moved)) {
		if rule := s.moved[id]; rule.Release > s.before.release {
			plans = append(plans, revision(rule.Release, rule.Commit))
		}
	}
	return plans
}

// decided returns what the source imports once the update is applied with source, its configuration plus the pins
// and exclusions the project decided on: the moved rules, except that a rule source pins keeps its version from
// before the update. A source that uses ref imports what sync would.
func (s plannedSource) decided(source rules.Source) (sourcePlan, error) {
	if !source.Ref.IsZero() {
		return s.before, nil
	}
	plan := sourcePlan{rules: maps.Clone(s.moved), individual: []string{}, warnings: []string{}}
	for id := range source.Pins {
		if rule, ok := s.before.rules[id]; ok {
			plan.rules[id] = rule
		}
	}
	plan.release, plan.commit = s.sharedFiles(plan.rules)
	// Settling reads nothing but the history, which planning already read, so this planner needs no repository.
	p := &planner{source: source, recorded: s.recorded, history: &s.history}
	return plan, p.settle(&plan)
}

// ForkedRule is a version of a rule that a project forks during an update, from a source the update names.
type ForkedRule struct {
	Source, ID string
	Version    trailguide.RuleVersion
}

// Import imports every source of configuration, the planned configuration plus the pins and exclusions the
// project decided on, or returns no partial result, as ImportLibraries does. Each source the update names imports
// exactly its planned versions, except that a rule configuration pins keeps its version from before the update,
// and fails when a library release tag those versions, or the versions in forked, come from has moved since
// planning; any other source imports what sync would.
func (u Update) Import(ctx context.Context, configuration rules.Configuration, options Options, forked []ForkedRule) (map[string]Library, error) {
	return importSources(ctx, configuration, options, curating{recorded: u.recorded, planned: u.curatedRevisions()}, func(ctx context.Context, repo *repository, source rules.Source) (sourcePlan, error) {
		planned, named := u.plans[source.Name]
		if !named {
			return planSource(ctx, repo, source, recordedSnapshot(u.recorded, source.Name))
		}
		plan, err := planned.decided(source)
		if err != nil {
			return sourcePlan{}, err
		}
		// A fork's library release needs the same check even when a pin or the update's scope keeps the imported copy
		// at an older one.
		releases := map[int]string{}
		for _, fork := range forked {
			if fork.Source != source.Name {
				continue
			}
			release, err := publishingRelease(planned.history, fork.ID, fork.Version)
			if err != nil {
				return sourcePlan{}, err
			}
			releases[release.number] = release.commit
		}
		return plan, repo.requireTagsUnmoved(ctx, source, plan, releases)
	})
}

// ReadFork reads version of rule id from source, which the update names, as ReadPublishedRule does, from the
// library release that planning found published it. It fails with invalid-release-tag, before reading the rule's
// files, when that release's tag now names another commit, so a fork never holds content the preview didn't plan.
func (u Update) ReadFork(ctx context.Context, source rules.Source, id string, version trailguide.RuleVersion, options Options) (PublishedRule, error) {
	planned, named := u.plans[source.Name]
	if !named {
		return PublishedRule{}, fmt.Errorf("source %s isn't part of the update", source.Name)
	}
	expected, err := publishingRelease(planned.history, id, version)
	if err != nil {
		return PublishedRule{}, err
	}
	return readPublishedRule(ctx, source, id, version, options, func(release *libraryRelease) error {
		if release.number != expected.number || release.commit != expected.commit {
			return fail("invalid-release-tag", fmt.Sprintf("Library release tag release/%d, which published version %s of %s, now names a different commit than when trailguide project update previewed source %s. Library release tags must not move; ask the library's maintainer, then run trailguide project update again.", expected.number, version, id, source.Name), nil)
		}
		return nil
	})
}

// requireTagsUnmoved fails with code invalid-release-tag when a library release tag that the plan's rules or
// library-wide files come from, or one of releases, which maps library release numbers to the commits planning
// read, now names another commit than recorded, so an update never installs versions its preview read from a tag
// that has moved since. A tag the library deleted passes, as it does for sync, and a plan that depends on no library
// release reads no tags.
func (r *repository) requireTagsUnmoved(ctx context.Context, source rules.Source, plan sourcePlan, releases map[int]string) error {
	recorded := maps.Clone(releases)
	if plan.release != 0 {
		recorded[plan.release] = plan.commit
	}
	for _, rule := range plan.rules {
		if rule.Release != 0 {
			recorded[rule.Release] = rule.Commit
		}
	}
	// A plan of a commit whose rules are all unreleased depends on no tag, so it reads none, as sync doesn't.
	if len(recorded) == 0 {
		return nil
	}
	advertised, err := r.listReleases(ctx)
	if err != nil {
		return err
	}
	current := map[int]string{}
	for _, release := range advertised {
		current[release.number] = release.commit
	}
	for _, number := range slices.Sorted(maps.Keys(recorded)) {
		if commit, listed := current[number]; listed && commit != recorded[number] {
			return fail("invalid-release-tag", fmt.Sprintf("Library release tag release/%d now names a different commit than when trailguide project update previewed source %s. Library release tags must not move; ask the library's maintainer, then run trailguide project update again.", number, source.Name), nil)
		}
	}
	return nil
}

// PlanUpdate plans moving the rules that targets name to their newest versions, reading each named source's
// release history without fetching rule files or writing anything. No targets names every source. It starts from
// what project sync would import from configuration and recorded, the snapshots from the last sync with only their
// skills' files, so the preview never repeats changes sync makes on its own. It also reads the skills each named
// source installs once updated, so the preview compares them with recorded's and flags every script the update adds
// or changes; a skill that skills.include names and that revision lacks fails as installing it would. It fails
// without a partial result when a target names an unknown source, a source that uses ref, or a rule the source
// doesn't import.
func PlanUpdate(ctx context.Context, configuration rules.Configuration, recorded map[string]library.Snapshot, targets []UpdateTarget, options Options) (Update, error) {
	if err := ctx.Err(); err != nil {
		return Update{}, gitexec.ContextFailure(err)
	}
	scopes, err := updateScopes(configuration, targets)
	if err != nil {
		return Update{}, err
	}
	update := Update{Sources: []SourceUpdate{}, Warnings: []string{}, plans: map[string]plannedSource{}, recorded: maps.Clone(recorded), curated: map[string][]CuratedUpdate{}}
	for _, source := range configuration.Sources {
		scope, named := scopes[source.Name]
		if !named {
			continue
		}
		planned, preview, warnings, err := planSourceUpdate(ctx, source, recordedSnapshot(recorded, source.Name), scope, options)
		if err != nil {
			return Update{}, sourceError(source.Name, err)
		}
		update.plans[source.Name] = planned
		update.Sources = append(update.Sources, preview)
		update.Warnings = append(update.Warnings, warnings...)
	}
	// Curated skills come from other repositories, or from a source that overrides the library's ref, whose
	// revision once the update is applied is known only once every source is planned. Only an update of a whole
	// source resolves the library's refs for them again; a scoped one, and one of a source that uses ref, which the
	// update doesn't move, keep them where sync would.
	overridden := map[string]library.Snapshot{}
	for name, snapshot := range recorded {
		overridden[name] = snapshot
	}
	for name, planned := range update.plans {
		release, commit := planned.keeping(nil)
		overridden[name] = library.Snapshot{Release: release, Commit: commit, NonLibrary: planned.before.nonLibrary}
	}
	for i, source := range namedSources(configuration, update.plans) {
		planned := update.plans[source.Name]
		for _, commit := range slices.Sorted(maps.Keys(planned.skills)) {
			at := planned.skills[commit]
			if at.manifest == nil || at.err != nil {
				continue
			}
			at.revisions, at.curated, err = planCurated(ctx, configuration, source, at.manifest, at.used, recordedSnapshot(recorded, source.Name), planned.scoped || !source.Ref.IsZero(), overridden, nil, options)
			if ctxErr := ctx.Err(); ctxErr != nil {
				return Update{}, gitexec.ContextFailure(ctxErr)
			}
			if err != nil {
				// Only choosing this revision fails, as for a skill it lacks.
				at.err, at.revisions, at.curated = sourceError(source.Name, err), nil, nil
			}
			planned.skills[commit] = at
		}
		update.Sources[i].Curated = planned.curatedRows(nil)
	}
	for _, source := range configuration.Sources {
		if _, named := update.plans[source.Name]; named {
			continue
		}
		rows, err := planOverriddenCurating(ctx, configuration, source, recordedSnapshot(recorded, source.Name), update.plans, overridden, options)
		if err != nil {
			return Update{}, sourceError(source.Name, err)
		}
		if len(rows) > 0 {
			update.curated[source.Name] = rows
		}
	}
	// A library source the update doesn't name still gets a preview of the curated skills it installs from a named
	// source, in configuration order.
	sources := []SourceUpdate{}
	for _, source := range configuration.Sources {
		if rows, moved := update.curated[source.Name]; moved {
			sources = append(sources, SourceUpdate{Name: source.Name, Rules: []RuleUpdate{}, Skills: []SkillUpdate{}, ReplacedSkills: []ReplacedSkillUpdate{}, SkillLevels: []SkillLevelUpdate{}, Curated: rows})
			continue
		}
		if i := slices.IndexFunc(update.Sources, func(preview SourceUpdate) bool { return preview.Name == source.Name }); i >= 0 {
			sources = append(sources, update.Sources[i])
		}
	}
	update.Sources = sources
	return update, nil
}

// namedSources returns, in configuration order, the sources of configuration that plans names.
func namedSources(configuration rules.Configuration, plans map[string]plannedSource) []rules.Source {
	named := []rules.Source{}
	for _, source := range configuration.Sources {
		if _, ok := plans[source.Name]; ok {
			named = append(named, source)
		}
	}
	return named
}

// planOverriddenCurating returns the preview rows of the curated skills that source, a library source the update
// doesn't name, whose snapshot is recorded, installs from a source plans names, which overrides the library's ref,
// so moving that source moves them, including one the project declared since the last sync. overridden is as planCurated takes it. It returns no rows when no source the
// update names overrides one the library curates from.
func planOverriddenCurating(ctx context.Context, configuration rules.Configuration, source rules.Source, recorded *library.Snapshot, plans map[string]plannedSource, overridden map[string]library.Snapshot, options Options) (_ []CuratedUpdate, err error) {
	// The project's current sources decide the override, by repository identity, whatever overrode it at the last
	// sync.
	moves := func(override *rules.Source) bool {
		if override == nil {
			return false
		}
		_, named := plans[override.Name]
		return named
	}
	if recorded == nil || recorded.NonLibrary || !slices.ContainsFunc(slices.Collect(maps.Values(recorded.Curated)), func(curated library.CuratedSource) bool {
		return moves(overridingSource(configuration, source.Name, curated.Repository))
	}) {
		return []CuratedUpdate{}, nil
	}
	ctx, cancel, err := withTimeout(ctx, options)
	if err != nil {
		return nil, err
	}
	defer cancel()
	repo, err := openRepository(ctx, source, options)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := repo.Close(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}()
	if err := repo.fetchCommits(ctx, []string{recorded.Commit}, fmt.Sprintf("The commit that vendor/%s/_source.json records is missing from its repository, so the repository may have rewritten its history. Run trailguide project sync, then trailguide project update again.", source.Name)); err != nil {
		return nil, err
	}
	manifest, err := repo.manifestAt(ctx, recorded.Commit)
	if err != nil || manifest == nil {
		return []CuratedUpdate{}, err
	}
	used, err := curatedUses(recorded.Skills, recorded.Files, "vendor/"+source.Name)
	if err != nil {
		return nil, err
	}
	// Only the curated sources a named source overrides move; sync keeps the others where they are.
	_, rows, err := planCurated(ctx, configuration, source, manifest, used, recorded, true, overridden, func(_ string, override *rules.Source) bool {
		return moves(override)
	}, options)
	return rows, err
}

// Curated returns the preview rows of the sources library source source curates skills from, once the rules kept
// lists stay at their versions from before the update, as Skills chooses the revision. It is empty, never nil, when
// none moves.
func (u Update) Curated(source string, kept []string) []CuratedUpdate {
	if rows, ok := u.curated[source]; ok {
		return slices.Clone(rows)
	}
	planned, named := u.plans[source]
	if !named {
		return []CuratedUpdate{}
	}
	return planned.curatedRows(kept)
}

// curatedRows returns the curated rows of the revision kept chooses, as levelRows does.
func (s plannedSource) curatedRows(kept []string) []CuratedUpdate {
	_, commit := s.keeping(kept)
	rows := slices.Clone(s.skills[commit].curated)
	if rows == nil {
		return []CuratedUpdate{}
	}
	return rows
}

// curatedRevisions returns the curated revisions the update planned, by library source name, then by the commit of
// the revision that supplies the library's skills, then by curated source name, so the import takes the ones of the
// revision the project's decisions choose.
func (u Update) curatedRevisions() map[string]map[string]map[string]library.CuratedSource {
	revisions := map[string]map[string]map[string]library.CuratedSource{}
	for name, planned := range u.plans {
		revisions[name] = map[string]map[string]library.CuratedSource{}
		for commit, preview := range planned.skills {
			revisions[name][commit] = preview.revisions
		}
	}
	return revisions
}

// sourceError names source before err, a failure importing or updating it, unless err already names it or is a
// cancellation: a validation error of the source's configuration, of its source record, or of a SOURCE:RULE argument
// naming it, or a message about one of its fields. Other failures, such as an invalid release record in the library, need it.
func sourceError(source string, err error) error {
	var validation errs.ValidationError
	if errors.As(err, &validation) && err == error(validation) {
		location := validation.ValidationLocation()
		if location == "sources."+source || strings.HasPrefix(location, "sources."+source+".") || strings.HasPrefix(location, source+":") || strings.HasPrefix(location, "vendor/"+source+"/") {
			return err
		}
	}
	if strings.HasPrefix(err.Error(), "sources."+source+".") || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return fmt.Errorf("source %s: %w", source, err)
}

// updateScopes maps each source the targets name to the rules they name, or to nil for every rule. No targets
// names every source.
func updateScopes(configuration rules.Configuration, targets []UpdateTarget) (map[string][]string, error) {
	scopes := map[string][]string{}
	whole := map[string]bool{}
	for _, source := range configuration.Sources {
		if len(targets) == 0 {
			scopes[source.Name] = nil
		}
	}
	for _, target := range targets {
		where := target.Source
		if target.Rule != "" {
			where += ":" + target.Rule
		}
		index := slices.IndexFunc(configuration.Sources, func(source rules.Source) bool { return source.Name == target.Source })
		if index < 0 {
			return nil, &ValidationError{Location: where, Problem: "no source named " + target.Source + " in .trailguide/config.yaml"}
		}
		if !configuration.Sources[index].Ref.IsZero() {
			return nil, &ValidationError{Location: where, Problem: "the source imports one revision with ref, so update doesn't move it; change sources." + target.Source + ".ref and run trailguide project sync"}
		}
		if target.Rule == "" {
			whole[target.Source] = true
			scopes[target.Source] = nil
		} else if !whole[target.Source] && !slices.Contains(scopes[target.Source], target.Rule) {
			scopes[target.Source] = append(scopes[target.Source], target.Rule)
		}
	}
	return scopes, nil
}

// planSourceUpdate plans one source within the same deadline as an import, and returns its preview and the
// warnings that installing it without decisions would give. A nil scope moves every rule and adds new ones;
// otherwise it moves only the rules scope names. A source that uses ref stays as sync would import it. Either way,
// the preview lists the replaced skills whose copy at the revision the update takes skills from, before the
// project's decisions, differs from the copy their replacement is based on; the plan keeps those rows for every
// revision the decisions can choose.
func planSourceUpdate(ctx context.Context, source rules.Source, recorded *library.Snapshot, scope []string, options Options) (_ plannedSource, _ SourceUpdate, _ []string, err error) {
	ctx, cancel, err := withTimeout(ctx, options)
	if err != nil {
		return plannedSource{}, SourceUpdate{}, nil, err
	}
	defer cancel()
	repo, err := openRepository(ctx, source, options)
	if err != nil {
		return plannedSource{}, SourceUpdate{}, nil, err
	}
	// Joining only a failed Close keeps a validation error unwrapped, so its location reads on its own.
	defer func() {
		if closeErr := repo.Close(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}()
	p := newPlanner(ctx, repo, source, recorded)
	before, err := p.choose()
	if err != nil {
		return plannedSource{}, SourceUpdate{}, nil, err
	}
	preview := SourceUpdate{Name: source.Name, Ref: source.Ref.String(), Rules: []RuleUpdate{}, Skills: []SkillUpdate{}, SkillLevels: []SkillLevelUpdate{}}
	if !source.Ref.IsZero() {
		planned := plannedSource{before: before}
		if planned.skills, err = repo.previewSkills(ctx, source, recorded, []sourcePlan{before}); err != nil {
			return plannedSource{}, SourceUpdate{}, nil, err
		}
		if preview.Skills, err = planned.skillRows(nil); err != nil {
			return plannedSource{}, SourceUpdate{}, nil, err
		}
		preview.ReplacedSkills = planned.replacedRows(nil)
		return planned, preview, before.warnings, nil
	}
	for _, id := range scope {
		if _, imported := before.rules[id]; !imported {
			return plannedSource{}, SourceUpdate{}, nil, &ValidationError{Location: source.Name + ":" + id, Problem: "source " + source.Name + " doesn't import this rule; name a rule it imports"}
		}
	}
	moved, rows, err := p.update(before, scope)
	if err != nil {
		return plannedSource{}, SourceUpdate{}, nil, err
	}
	planned := plannedSource{before: before, moved: moved, scoped: scope != nil, recorded: p.recorded, history: *p.history}
	if release, _ := planned.sharedFiles(moved); release != before.release {
		preview.SharedFiles = &SharedFilesUpdate{From: before.release, To: release}
	}
	after, err := planned.decided(source)
	if err != nil {
		return plannedSource{}, SourceUpdate{}, nil, err
	}
	if err := p.requireUnmoved(before); err != nil {
		return plannedSource{}, SourceUpdate{}, nil, err
	}
	if planned.skills, err = repo.previewSkills(ctx, source, recorded, planned.skillRevisions(after)); err != nil {
		return plannedSource{}, SourceUpdate{}, nil, err
	}
	if preview.Skills, err = planned.skillRows(nil); err != nil {
		return plannedSource{}, SourceUpdate{}, nil, err
	}
	preview.ReplacedSkills = planned.replacedRows(nil)
	preview.SkillLevels = planned.levelRows(nil)
	preview.Rules = rows
	return planned, preview, after.warnings, nil
}

// update moves the rules in scope, or every rule when scope is nil, to their newest versions and drops retired
// ones, except that pins keep rules where they are. A nil scope also adds the rules the library added to the
// selected groups. It returns every rule the source then imports, with its version, and a preview row for each
// change: none for a rule excluded without a replacement, and replaced for one with a replacement. A replacement
// is listed whenever the newest version is newer than its basedOn, or the imported version when it records none,
// with the changes after that version, even when a pin keeps the imported copy where it is.
func (p *planner) update(before sourcePlan, scope []string) (map[string]library.ImportedRule, []RuleUpdate, error) {
	history, err := p.versioned()
	if err != nil {
		return nil, nil, err
	}
	newest := history.newest().record
	after := maps.Clone(before.rules)
	rows := []RuleUpdate{}
	ids := scope
	if ids == nil {
		ids = slices.Sorted(maps.Keys(before.rules))
	}
	for _, id := range ids {
		current := before.rules[id]
		latest, published := newest.Rules[id]
		exclusion, excluded := p.source.Exclusion(id)
		listed := !excluded || exclusion.ReplacedBy != ""
		pin, pinned := p.source.Pins[id]
		switch {
		// A replacement is reviewed against its basedOn, or else the imported version, and a pin keeps only the
		// imported copy, never the review.
		case published && excluded && exclusion.ReplacedBy != "":
			if !pinned && latest.Compare(*current.Version) > 0 {
				if after[id], err = p.publishedVersion(id, latest); err != nil {
					return nil, nil, err
				}
			}
			baseline := current.Version
			if exclusion.BasedOn != nil {
				baseline = exclusion.BasedOn
			}
			if latest.Compare(*baseline) > 0 {
				row := RuleUpdate{ID: id, Change: UpdateReplaced, From: current.Version, To: &latest, LocalRule: exclusion.ReplacementFile(), BasedOn: exclusion.BasedOn}
				if pinned {
					row.To, row.Newest, row.Pin = nil, &latest, &pin
				}
				row.Summaries, row.SummaryVersions = history.summaries(id, baseline, latest)
				rows = append(rows, row)
			}
		case pinned && !listed:
		case pinned && !published:
			row, err := retiredRow(id, current, history)
			if err != nil {
				return nil, nil, err
			}
			row.Pin = &pin
			rows = append(rows, p.withReplacementLocalRule(row))
		case pinned && latest.Compare(*current.Version) > 0:
			rows = append(rows, RuleUpdate{ID: id, Change: UpdatePinned, From: current.Version, Newest: &latest, Summaries: []string{}, SummaryVersions: []trailguide.RuleVersion{}, Pin: &pin})
		case pinned:
		case !published:
			delete(after, id)
			if listed {
				row, err := retiredRow(id, current, history)
				if err != nil {
					return nil, nil, err
				}
				rows = append(rows, p.withReplacementLocalRule(row))
			}
		case latest.Compare(*current.Version) > 0:
			if after[id], err = p.publishedVersion(id, latest); err != nil {
				return nil, nil, err
			}
			if !listed {
				continue
			}
			row := RuleUpdate{ID: id, Change: versionChange(*current.Version, latest), From: current.Version, To: &latest}
			row.Summaries, row.SummaryVersions = history.summaries(id, current.Version, latest)
			rows = append(rows, row)
		}
	}
	if scope == nil {
		for _, id := range slices.Sorted(maps.Keys(newest.Rules)) {
			if _, imported := before.rules[id]; imported || !p.source.Groups.Includes(ruleGroup(id)) {
				continue
			}
			version := newest.Rules[id]
			if after[id], err = p.publishedVersion(id, version); err != nil {
				return nil, nil, err
			}
			// A selector that excludes the new rule decided it already, so the preview doesn't ask.
			if _, excluded := p.source.Exclusion(id); excluded {
				continue
			}
			row := RuleUpdate{ID: id, Change: UpdateNew, To: &version}
			row.Summaries, row.SummaryVersions = history.summaries(id, nil, version)
			rows = append(rows, row)
		}
	}
	slices.SortFunc(rows, func(a, b RuleUpdate) int {
		if order := slices.Index(updateChangeOrder, a.Change) - slices.Index(updateChangeOrder, b.Change); order != 0 {
			return order
		}
		return strings.Compare(a.ID, b.ID)
	})
	return after, rows, nil
}

// settle completes a plan whose rules and library-wide files an update chose: individually selected rules to load,
// warnings for entries naming rules the library retired, and the retired rules the source selects.
func (p *planner) settle(plan *sourcePlan) error {
	plan.warnings = append(plan.warnings, redundantRules(p.source, plan.rules)...)
	for _, id := range p.source.Rules {
		if _, imported := plan.rules[id]; imported {
			plan.individual = append(plan.individual, id)
		} else {
			plan.warnings = append(plan.warnings, p.retiredEntry("rules.include", id))
		}
	}
	if err := p.requireEntries(plan); err != nil {
		return err
	}
	var err error
	// An update refreshes the recorded retirements, as it refreshes the rules.
	plan.retired, err = p.freshRetiredRules()
	return err
}

// retiredRow previews the retirement of rule id, which the project imports at current.
func retiredRow(id string, current library.ImportedRule, history releaseHistory) (RuleUpdate, error) {
	retired := history.retirement(id)
	if retired == nil {
		return RuleUpdate{}, fail("invalid-release-tag", fmt.Sprintf("Rule %s is missing from release/%d, but no library release retired it. Don't create or move release tags by hand.", id, history.newest().number), nil)
	}
	last := retired.LastVersion
	row := RuleUpdate{ID: id, Change: UpdateRetired, From: current.Version, LastVersion: &last, Summaries: slices.Clone(retired.Summaries), SummaryVersions: []trailguide.RuleVersion{}, ReplacedBy: retired.ReplacedBy}
	for range row.Summaries {
		row.SummaryVersions = append(row.SummaryVersions, last)
	}
	if row.ReplacedBy != "" && history.retired(row.ReplacedBy) {
		row.ReplacementRetired, row.CurrentReplacement = true, history.currentReplacement(row.ReplacedBy)
	}
	return row, nil
}

// withReplacementLocalRule names, in a retired row, the project's local rule that already replaces the retired rule's
// current replacement, when the project replaced it.
func (p *planner) withReplacementLocalRule(row RuleUpdate) RuleUpdate {
	replacement := row.ReplacedBy
	if row.ReplacementRetired {
		replacement = row.CurrentReplacement
	}
	if exclusion, excluded := p.source.Exclusion(replacement); excluded && replacement != "" {
		row.ReplacementLocalRule = exclusion.ReplacementFile()
	}
	return row
}

// versionChange classifies moving from one version to a newer one by the largest component that changed.
func versionChange(from, to trailguide.RuleVersion) UpdateChange {
	switch {
	case to.Major != from.Major:
		return UpdateMajor
	case to.Minor != from.Minor:
		return UpdateMinor
	}
	return UpdatePatch
}
