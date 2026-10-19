// Fetch the skills a library curates from other repositories, from those repositories, at revisions each project
// resolves for itself.

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
	"github.com/fabricahq/trailguide/internal/skills"
)

// curating says which revision of each source a library curates from an import takes skills from.
type curating struct {
	// recorded holds each source's snapshot from the last sync, keyed by source name. Its curated revisions stay
	// while the library still names the same repository and ref for them, since sync never moves a source.
	recorded map[string]library.Snapshot
	// planned maps the name of a library source to the curated revisions an update planned for it, by the commit
	// that supplies the library's skills, then by curated source name, which the import takes as they are.
	planned map[string]map[string]map[string]library.CuratedSource
}

// curatedSkill is a curated skill a project takes from a library source: its ID in the library, such as
// mattpocock:skills/to-spec, and the level the library lists it at. included reports that the project's selection
// takes it, and usedBy lists, sorted, the library's own installed skills that use it, by their IDs in the library.
type curatedSkill struct {
	id, level string
	included  bool
	usedBy    []string
}

// curatedSkills returns, by curated source name, the curated skills source takes from manifest, its library's:
// every required one, the recommended ones when skills.include names recommended, each offered one that it
// names or includes with "*", which, as for a source's own skills, leaves out one in a folder whose name starts with
// a dot, and each one that used, as curatedUses gives it for the library's skills the source installs, says one of
// them uses. It fails when skills.include or skills.exclude names a curated skill the library
// doesn't list, since a curated skill reaches projects only through the library's lists, when skills.exclude
// names a required one, since a project that uses the library can't diverge from it, and when an installed skill
// of the library uses a curated skill the library doesn't list.
func curatedSkills(manifest *rules.Library, source rules.Source, release int, used map[string][]string) (map[string][]curatedSkill, error) {
	listed := rules.SkillLevels{}
	if manifest != nil {
		listed = manifest.Skills
	}
	curated := listed.Curated()
	for _, field := range []struct {
		name string
		ids  []string
	}{{"include", source.Skills}, {"exclude", slices.Sorted(maps.Keys(source.SkillExclude))}} {
		for _, id := range field.ids {
			if _, _, isCurated := library.SplitCurated(id); isCurated && !slices.Contains(curated, id) {
				return nil, &ValidationError{Location: "sources." + source.Name + ".skills." + field.name, Problem: fmt.Sprintf("the library doesn't list %s at any level, and a library's curated skills reach projects only through its lists; check the skill ID, or ask the library's maintainer to list it", id)}
			}
		}
	}
	for _, id := range slices.Sorted(maps.Keys(source.SkillExclude)) {
		if slices.Contains(listed.Required, id) {
			return nil, &ValidationError{Location: "sources." + source.Name + ".skills.exclude." + id, Problem: rules.RequiredSkillProblem(source.Name, id, release)}
		}
	}
	for _, id := range slices.Sorted(maps.Keys(used)) {
		if !slices.Contains(curated, id) {
			return nil, fail("unsupported-content", fmt.Sprintf("sources.%s: the library's skill %s uses %s, which the library doesn't list at any level, so Trail Guide can't install it. Ask the library's maintainer to list it in library.skills; trailguide library check catches this.", source.Name, used[id][0], id), nil)
		}
	}
	result := map[string][]curatedSkill{}
	for _, id := range curated {
		level := listed.Level(id)
		_, folder, _ := library.SplitCurated(id)
		included := level == skills.LevelRequired ||
			slices.Contains(source.Skills, id) ||
			slices.Contains(source.Skills, skills.Wildcard) && !library.HiddenSkill(folder) ||
			level == skills.LevelRecommended && slices.Contains(source.Skills, skills.Recommended)
		if !included && len(used[id]) == 0 {
			continue
		}
		name, _, _ := library.SplitCurated(id)
		result[name] = append(result[name], curatedSkill{id: id, level: level, included: included, usedBy: used[id]})
	}
	return result, nil
}

// curateSkills adds to imported, source's import, the curated skills the source takes from its library, with their
// files from the repositories the library curates from, and records each curated source's revision. all holds every
// source's import, so a project source that declares a curated repository supplies its revision instead of the
// library's ref.
func curateSkills(ctx context.Context, configuration rules.Configuration, source rules.Source, imported *Library, all map[string]Library, curate curating, options Options) error {
	if imported.Snapshot.NonLibrary {
		return nil
	}
	manifest, err := rules.ParseLibraryManifest(imported.Snapshot.Files[librarytree.Manifest], source.Name+"/"+librarytree.RepositoryPath(librarytree.Manifest))
	if err != nil {
		return err
	}
	used, err := curatedUses(imported.Snapshot.Skills, imported.Snapshot.Files, "vendor/"+source.Name)
	if err != nil {
		return err
	}
	selected, err := curatedSkills(manifest, source, imported.Snapshot.Release, used)
	if err != nil {
		return err
	}
	// The skills the library lists are what it offers of each curated repository, whether the project takes them or not.
	for _, id := range manifest.Skills.Curated() {
		imported.Snapshot.AvailableSkills[id] = library.ImportedSkill{Level: manifest.Skills.Level(id)}
	}
	for _, name := range slices.Sorted(maps.Keys(selected)) {
		origin := curatedSource{library: source.Name, name: name, declared: manifest.Curated[name], exclude: curatedExclusions(source, name)}
		origin.override = overridingSource(configuration, source.Name, origin.declared.Repository)
		if origin.override != nil {
			origin.overridden = all[origin.override.Name].Snapshot
		}
		recorded := curate.recorded[source.Name].Curated[name]
		planned, isPlanned := curate.planned[source.Name][imported.Snapshot.Commit][name]
		if err := origin.curate(ctx, imported, selected[name], recorded, planned, isPlanned, options); err != nil {
			return err
		}
	}
	slices.Sort(imported.Snapshot.Executable)
	return nil
}

// overridingSource returns the project's source, other than the library source named library, that declares
// repository, or nil when none does. Repository identity, not the source's name, decides the match.
func overridingSource(configuration rules.Configuration, library, repository string) *rules.Source {
	identity := repositoryIdentity(repository)
	for i, source := range configuration.Sources {
		if source.Name != library && identity != "" && repositoryIdentity(source.Repository) == identity {
			return &configuration.Sources[i]
		}
	}
	return nil
}

// repositoryIdentity returns the identity of a repository address, or "" for one that doesn't parse.
func repositoryIdentity(address string) string {
	parsed, err := rules.ParseRepository(quote(address), "repository")
	if err != nil {
		return ""
	}
	return parsed.Identity
}

// curatedSource is one source a library curates skills from, as one project resolves it.
type curatedSource struct {
	// library names the project's library source, and name the curated source in the library's manifest, which
	// declares it as declared.
	library, name string
	declared      rules.CuratedSource
	// override is the project's own source that declares the same repository, and overridden its import; override
	// is nil when no source does.
	override   *rules.Source
	overridden library.Snapshot
	// maintained reports that the library's own maintainer is reading the source, such as library check does, so
	// advice names the library's configuration as theirs to fix rather than a project's.
	maintained bool
	// exclude holds the library source's exclusions of the skills it takes from the curated source, by their IDs in
	// its repository; a replacement's basedOn is a commit of that repository.
	exclude map[string]rules.SkillExclusion
}

// curate fetches the curated source's revision and adds the files of chosen, its skills the project takes, to
// imported. The revision is the overriding source's, when there is one; else planned, when an update planned one;
// else recorded's, when it records the ref the library names; else the library's ref, resolved now.
func (l curatedSource) curate(ctx context.Context, imported *Library, chosen []curatedSkill, recorded library.CuratedSource, planned library.CuratedSource, isPlanned bool, options Options) (err error) {
	ctx, cancel, err := withTimeout(ctx, options)
	if err != nil {
		return err
	}
	defer cancel()
	address := l.declared.Repository
	if l.override != nil {
		address = l.override.Repository
	}
	repo, err := openRepository(ctx, rules.Source{Name: l.name, Repository: address}, options)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := repo.Close(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}()
	revision, kept, err := l.revision(ctx, repo, recorded, planned, isPlanned)
	if err != nil {
		return err
	}
	if err := repo.requireBasedOn(ctx, replacements(l.exclude), basedOnField(l.library, l.name)); err != nil {
		return err
	}
	retained := 0
	for _, data := range imported.Snapshot.Files {
		retained += len(data)
	}
	read, err := l.read(ctx, repo, revision, chosen, kept, retained)
	if err != nil {
		return err
	}
	// The library's own skills use a curated skill by its ID in the library, which they're recorded under too.
	ownUsers := map[string][]string{}
	for _, skill := range chosen {
		ownUsers[skill.id] = skill.usedBy
	}
	for id, skill := range read.skills {
		usedBy := slices.Clone(ownUsers[l.name+":"+id])
		for _, user := range skill.UsedBy {
			usedBy = append(usedBy, l.name+":"+user)
		}
		if usedBy == nil {
			usedBy = []string{}
		}
		slices.Sort(usedBy)
		imported.Snapshot.Skills[l.name+":"+id] = library.ImportedSkill{Level: skill.Level, Included: skill.Included, UsedBy: usedBy}
	}
	prefix := library.CuratedDirectory + "/" + l.name + "/"
	for name, data := range read.files {
		imported.Snapshot.Files[prefix+name] = data
	}
	for _, name := range read.executable {
		imported.Snapshot.Executable = append(imported.Snapshot.Executable, prefix+name)
	}
	if imported.Snapshot.Curated == nil {
		imported.Snapshot.Curated = map[string]library.CuratedSource{}
	}
	imported.Snapshot.Curated[l.name] = revision
	return nil
}

// read reads, from repo at revision, the curated source's skills that chosen names and every skill they use, as a
// source's own skills bring the skills they use, by their IDs in the curated repository, with their files by path
// in that repository. kept and retained are as missing and readFiles take them.
func (l curatedSource) read(ctx context.Context, repo *repository, revision library.CuratedSource, chosen []curatedSkill, kept bool, retained int) (importedSkills, error) {
	tree, err := repo.repositoryTree(ctx, revision.Commit)
	if err != nil {
		return importedSkills{}, err
	}
	folders := skillFolders(tree, !revision.NonLibrary)
	ids, levels, usedOnly := []string{}, map[string]string{}, map[string]bool{}
	for _, skill := range chosen {
		_, id, _ := library.SplitCurated(skill.id)
		if _, ok := folders[id]; !ok {
			return importedSkills{}, l.missing(skill.id, revision, kept)
		}
		ids, levels[id], usedOnly[id] = append(ids, id), skill.level, !skill.included
	}
	origin := rules.Source{Name: l.library}
	plan := sourcePlan{release: revision.Release, commit: revision.Commit}
	// A curated library's skills can use skills it curates in turn, which levels don't chain to, so they aren't
	// followed.
	uses, err := repo.skillUses(ctx, origin, folders, ids, !revision.NonLibrary)
	if err != nil {
		return importedSkills{}, err
	}
	installed, err := installedSkills(origin, plan, ids, uses)
	if err != nil {
		return importedSkills{}, err
	}
	result := importedSkills{skills: map[string]library.ImportedSkill{}}
	files := map[string]treeEntry{}
	for _, id := range slices.Sorted(maps.Keys(installed)) {
		level, ok := levels[id]
		if !ok {
			level = skills.LevelOffered
		}
		// A skill the library's own skills use, which the project's selection doesn't take, is installed for them.
		result.skills[id] = library.ImportedSkill{Level: level, Included: installed[id].Included && !usedOnly[id], UsedBy: installed[id].UsedBy}
		maps.Copy(files, folders[id])
	}
	result.files, result.executable, err = repo.readFiles(ctx, l.library, files, retained)
	return result, err
}

// revision returns the revision the curated source's skills come from, fetched, and reports whether a source record
// locked it, which sync never moves.
func (l curatedSource) revision(ctx context.Context, repo *repository, recorded, planned library.CuratedSource, isPlanned bool) (library.CuratedSource, bool, error) {
	if l.override != nil {
		revision := library.CuratedSource{Repository: l.override.Repository, Ref: l.override.Ref, Release: l.overridden.Release, Commit: l.overridden.Commit, Source: l.override.Name, NonLibrary: l.overridden.NonLibrary}
		return revision, true, repo.fetchCommits(ctx, []string{revision.Commit}, fmt.Sprintf("The commit that vendor/%s/_source.json records for sources.%s is missing from its repository, so the repository may have rewritten its history. Run trailguide project sync again, or delete vendor/%s and run trailguide project sync to resolve sources.%s again.", l.override.Name, l.override.Name, l.override.Name, l.override.Name))
	}
	if isPlanned && l.declares(planned) {
		return planned, false, repo.fetchCommits(ctx, []string{planned.Commit}, l.vanished("trailguide project update "+l.library))
	}
	if recorded.Source == "" && l.declares(recorded) {
		missing := fmt.Sprintf("The commit that vendor/%s/_source.json records for the skills library %s curates from %s is missing from that repository, so it may have rewritten its history. Run trailguide project update %s to resolve the library's ref for %s again.", l.library, l.library, l.name, l.library, l.name)
		if err := repo.fetchCommits(ctx, []string{recorded.Commit}, missing); err != nil {
			return library.CuratedSource{}, false, err
		}
		isLibrary, err := repo.isLibrary(ctx, recorded.Commit)
		recorded.NonLibrary = !isLibrary
		return recorded, true, err
	}
	revision, err := l.resolve(ctx, repo)
	return revision, false, err
}

// vanished explains a commit of the curated source that its repository no longer has, although Trail Guide just read it,
// and names command, what to run again.
func (l curatedSource) vanished(command string) string {
	return fmt.Sprintf("A commit of the repository library %s curates skills from as %s disappeared while Trail Guide read it, so the repository's history changed. Run %s again.", l.library, l.name, command)
}

// declares reports whether revision records the repository and ref the library declares for the curated source.
func (l curatedSource) declares(revision library.CuratedSource) bool {
	return revision.Commit != "" && revision.Repository == l.declared.Repository && revision.Ref.Equal(l.declared.Ref)
}

// resolve resolves the ref the library declares for the curated source, or, without one, the repository's newest
// library release, and fetches the revision.
func (l curatedSource) resolve(ctx context.Context, repo *repository) (library.CuratedSource, error) {
	p := &planner{ctx: ctx, repo: repo, source: rules.Source{Name: l.name, Repository: l.declared.Repository, Ref: l.declared.Ref}}
	revision := library.CuratedSource{Repository: l.declared.Repository, Ref: l.declared.Ref}
	var err error
	if l.declared.Ref.IsZero() {
		history, historyErr := p.releases()
		if historyErr != nil {
			return library.CuratedSource{}, historyErr
		}
		newest := history.newest()
		if newest == nil {
			return library.CuratedSource{}, fail("ref-required", fmt.Sprintf("%s curates skills from its source %s, whose repository has no release/<number> tags, so the library must name a ref for it. %s", l.subject(), l.name, l.fixRef("set")), nil)
		}
		revision.Release, revision.Commit = newest.number, newest.commit
		if err := repo.fetchCommits(ctx, []string{revision.Commit}, l.vanished("the command")); err != nil {
			return library.CuratedSource{}, err
		}
	} else if revision.Release, revision.Commit, err = p.resolveRef(); err != nil {
		return library.CuratedSource{}, l.libraryRef(err)
	} else if err := repo.fetchCommits(ctx, []string{revision.Commit}, l.vanished("the command")); err != nil {
		return library.CuratedSource{}, err
	}
	isLibrary, err := repo.isLibrary(ctx, revision.Commit)
	revision.NonLibrary = !isLibrary
	return revision, err
}

// libraryRef rewords a failure resolving the ref the library declares, which names sources.<name>.ref as if it
// were the project's, to name the library's configuration.
func (l curatedSource) libraryRef(err error) error {
	var failure *Error
	if !errors.As(err, &failure) || !strings.HasPrefix(failure.Problem, "sources."+l.name+".ref") {
		return err
	}
	return fail(failure.Code, fmt.Sprintf("%s curates skills from its source %s at ref %s, which the repository doesn't resolve: %s %s", l.subject(), l.name, l.declared.Ref, strings.TrimSpace(strings.TrimPrefix(failure.Problem, "sources."+l.name+".ref:")), l.fixRef("fix")), nil)
}

// subject names the library in advice: by its source name in a project, and as the library to its maintainer.
func (l curatedSource) subject() string {
	if l.maintained {
		return "The library"
	}
	return "Library " + l.library
}

// fixRef advises how to fix the ref the library declares for the curated source, with verb, such as set or fix:
// the maintainer edits the library's configuration, and a project asks them to or overrides the ref.
func (l curatedSource) fixRef(verb string) string {
	if l.maintained {
		return fmt.Sprintf("%s sources.%s.ref in .trailguide/config.yaml.", strings.ToUpper(verb[:1])+verb[1:], l.name)
	}
	return fmt.Sprintf("Ask the library's maintainer to %s sources.%s.ref in its .trailguide/config.yaml, or declare the repository as a source of your own with a ref, which overrides the library's.", verb, l.name)
}

// missing explains a curated skill, id, that the curated source's revision doesn't have: a revision a record or an
// overriding source locks needs an update or another ref, and any other means the library lists a skill its
// curated repository lacks.
func (l curatedSource) missing(id string, revision library.CuratedSource, kept bool) error {
	where := revisionName(revision.Release, revision.Commit)
	switch {
	case l.override != nil:
		return fail("skill-not-in-revision", fmt.Sprintf("Library %s lists %s, which isn't in %s, the revision of sources.%s, which declares the same repository and so supplies the skills the library curates from it. Run trailguide project update %s, or set sources.%s.ref to a revision that has the skill, then run trailguide project sync.", l.library, id, where, l.override.Name, l.override.Name, l.override.Name), nil)
	case kept:
		return fail("skill-not-in-revision", fmt.Sprintf("Library %s lists %s, which isn't in %s, the revision vendor/%s/_source.json records for the skills it curates from %s. Sync never moves that revision; run trailguide project update %s, which resolves the library's ref for %s again.", l.library, id, where, l.library, l.name, l.library, l.name), nil)
	}
	return &ValidationError{Location: "sources." + l.library, Problem: fmt.Sprintf("library %s lists %s, which its repository %s doesn't have at %s; ask the library's maintainer to fix library.skills or sources.%s.ref", l.library, id, l.declared.Repository, where, l.name)}
}

// planCurated plans the revision of each source a library curates skills from, given manifest, the library's
// .trailguide/config.yaml at a revision the update can take source's skills from, used, the curated skills the
// library's skills that the source installs there use, as curatedUses gives them, and recorded, the source's snapshot,
// or nil. It resolves the ref the library names again, as an update of the whole source does, unless keep is set,
// which keeps the recorded revision while the library still names its repository and ref, as sync does. A curated repository that
// a project source declares too comes from overridden, that source's revision once the update is applied, by source
// name. A non-nil only limits the plan to the curated sources it reports true for, given each one's name and the
// project source that overrides it, or nil. It returns the revisions it
// resolved, by curated source name, leaving out overridden ones, and a preview row for each curated source whose
// commit or skills differ from those recorded, including one the project no longer takes any skills from.
func planCurated(ctx context.Context, configuration rules.Configuration, source rules.Source, manifest []byte, used map[string][]string, recorded *library.Snapshot, keep bool, overridden map[string]library.Snapshot, only func(name string, override *rules.Source) bool, options Options) (map[string]library.CuratedSource, []CuratedUpdate, error) {
	parsed, err := rules.ParseLibraryManifest(manifest, source.Name+"/"+librarytree.RepositoryPath(librarytree.Manifest))
	if err != nil {
		return nil, nil, err
	}
	selected, err := curatedSkills(parsed, source, 0, used)
	if err != nil {
		return nil, nil, err
	}
	before := library.Snapshot{}
	if recorded != nil {
		before = *recorded
	}
	revisions, rows := map[string]library.CuratedSource{}, []CuratedUpdate{}
	names := maps.Clone(selected)
	for name := range before.Curated {
		if _, ok := names[name]; !ok {
			names[name] = nil
		}
	}
	for _, name := range slices.Sorted(maps.Keys(names)) {
		was := before.CuratedFrom(name)
		origin := curatedSource{library: source.Name, name: name, declared: parsed.Curated[name], exclude: curatedExclusions(source, name)}
		origin.override = overridingSource(configuration, source.Name, origin.declared.Repository)
		if only != nil && !only(name, origin.override) {
			continue
		}
		if len(selected[name]) == 0 {
			// The project takes no skills from the source any more, so the update removes those it installed.
			gone := before.Curated[name]
			row := curatedMove(name, gone, library.CuratedSource{Repository: gone.Repository, Ref: gone.Ref}, skillChanges(&was, importedSkills{}))
			for i := range row.Changes {
				row.Changes[i].ID = name + ":" + row.Changes[i].ID
			}
			rows = append(rows, row)
			continue
		}
		if origin.override != nil {
			origin.overridden = overridden[origin.override.Name]
		}
		revision, row, err := origin.plan(ctx, selected[name], was, before.Curated[name], keep, options)
		if err != nil {
			return nil, nil, err
		}
		if origin.override == nil {
			revisions[name] = revision
		}
		if row != nil {
			rows = append(rows, *row)
		}
	}
	return revisions, rows, nil
}

// plan fetches the curated source's revision in a repository of its own, within one import deadline, and returns it,
// and the preview row for it when its commit or the skills the project takes from it differ from was's, which holds
// the skills the project recorded from the source, as Snapshot.CuratedFrom gives them. The revision is the
// overriding source's, when there is one; else recorded, when keep is set and it records the ref the library names;
// else the library's ref, resolved now. The row compares the files of chosen, the skills the project takes, and of
// the skills they use, at the two commits, as skillChanges compares a source's own skills.
func (l curatedSource) plan(ctx context.Context, chosen []curatedSkill, was library.Snapshot, recorded library.CuratedSource, keep bool, options Options) (_ library.CuratedSource, _ *CuratedUpdate, err error) {
	ctx, cancel, err := withTimeout(ctx, options)
	if err != nil {
		return library.CuratedSource{}, nil, err
	}
	defer cancel()
	address := l.declared.Repository
	if l.override != nil {
		address = l.override.Repository
	}
	repo, err := openRepository(ctx, rules.Source{Name: l.name, Repository: address}, options)
	if err != nil {
		return library.CuratedSource{}, nil, err
	}
	defer func() {
		if closeErr := repo.Close(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}()
	recordedRevision := recorded
	if !keep {
		recorded = library.CuratedSource{}
	}
	revision, kept, err := l.revision(ctx, repo, recorded, library.CuratedSource{}, false)
	if err != nil {
		return library.CuratedSource{}, nil, err
	}
	read, err := l.read(ctx, repo, revision, chosen, kept, 0)
	if err != nil {
		return library.CuratedSource{}, nil, err
	}
	// A skill the library source excludes isn't installed, so its changes aren't listed; a replaced one's are
	// listed against the copy its replacement is based on instead.
	changes := slices.DeleteFunc(skillChanges(&was, read), func(change SkillUpdate) bool {
		_, excluded := l.exclude[change.ID]
		return excluded
	})
	row := curatedMove(l.name, recordedRevision, revision, changes)
	if row.Replaced, err = repo.replacedRows(ctx, l.library, replacements(l.exclude), basedOnField(l.library, l.name), l.name+":", recordedRevision.Commit, revision.Commit); err != nil {
		return library.CuratedSource{}, nil, err
	}
	if row.Commit == nil && len(row.Changes) == 0 && len(row.Replaced) == 0 {
		return revision, nil, nil
	}
	for i := range row.Changes {
		row.Changes[i].ID = l.name + ":" + row.Changes[i].ID
	}
	return revision, &row, nil
}

// CuratedSkillNames resolves the ref a library declares for its curated source name, or, without one, the
// repository's newest library release, as a project that uses the library would when it updates, for the library's
// maintainer. It returns, by the skill's ID in the repository, the name each skill of ids installs under, as project
// sync names it, and the skills of the repository it uses, and the same for each skill those use, which a project
// installs with them, leaving out the IDs the repository has no skill for there. It also returns the revision's name,
// such as main or release/3.
func CuratedSkillNames(ctx context.Context, name string, declared rules.CuratedSource, ids []string, options Options) (_ map[string]library.CuratedSkill, _ string, err error) {
	ctx, cancel, err := withTimeout(ctx, options)
	if err != nil {
		return nil, "", err
	}
	defer cancel()
	repo, err := openRepository(ctx, rules.Source{Name: name, Repository: declared.Repository}, options)
	if err != nil {
		return nil, "", err
	}
	defer func() {
		if closeErr := repo.Close(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}()
	origin := curatedSource{name: name, declared: declared, maintained: true}
	revision, err := origin.resolve(ctx, repo)
	if err != nil {
		return nil, "", err
	}
	tree, err := repo.repositoryTree(ctx, revision.Commit)
	if err != nil {
		return nil, "", err
	}
	at := revisionName(revision.Release, revision.Commit)
	if !declared.Ref.IsZero() {
		at = declared.Ref.String()
	}
	folders := skillFolders(tree, !revision.NonLibrary)
	result := map[string]library.CuratedSkill{}
	for pending := ids; len(pending) > 0; {
		entries := map[string]treeEntry{}
		for _, id := range pending {
			if folder, ok := folders[id]; ok {
				file := library.SkillFolder(id) + "/" + skills.File
				entries[file] = folder[file]
			}
		}
		files, _, err := repo.readFiles(ctx, name, entries, 0)
		if err != nil {
			return nil, "", err
		}
		next := []string{}
		for _, id := range pending {
			data, ok := files[library.SkillFolder(id)+"/"+skills.File]
			if _, seen := result[id]; !ok || seen {
				continue
			}
			location := name + ":" + id + "/" + skills.File
			metadata, err := skills.ReadMetadata(data, location)
			if err != nil {
				return nil, "", err
			}
			installName, err := skills.InstallName(id, metadata, location)
			if err != nil {
				return nil, "", err
			}
			// A skill of the repository it uses has no source name; one with a source name comes from elsewhere.
			uses := []string{}
			for _, used := range metadata.SkillsUsed {
				if !strings.Contains(used, ":") {
					uses = append(uses, used)
					next = append(next, used)
				}
			}
			result[id] = library.CuratedSkill{Name: installName, Uses: uses}
		}
		pending = next
	}
	return result, at, nil
}

// curatedMove returns the preview row of the curated source name moving from the revision from, which the project
// recorded, or the zero CuratedSource for one it didn't curate from before, to the revision to, whose Commit is empty
// when the update removes the source's skills, with the changes to its skills.
func curatedMove(name string, from, to library.CuratedSource, changes []SkillUpdate) CuratedUpdate {
	row := CuratedUpdate{Name: name, Repository: to.Repository, Ref: to.Ref.String(), Changes: changes, Replaced: []ReplacedSkillUpdate{}}
	if from.Commit == to.Commit {
		return row
	}
	row.Commit = &CommitMove{From: from.Commit, To: to.Commit}
	if from.Release != 0 && to.Release != 0 && to.Ref.IsZero() {
		row.Release = &SharedFilesUpdate{From: from.Release, To: to.Release}
	}
	row.TagMoved = from.Commit != "" && to.Commit != "" && to.Ref.Kind() == rules.GitRefTag && from.Ref.Equal(to.Ref)
	return row
}

// manifestAt returns the .trailguide/config.yaml of a fetched commit, or nil when it has none.
func (r *repository) manifestAt(ctx context.Context, commit string) ([]byte, error) {
	tree, err := r.repositoryTree(ctx, commit)
	if err != nil {
		return nil, err
	}
	data, _, err := r.manifest(ctx, tree)
	return data, err
}
