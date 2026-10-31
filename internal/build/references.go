// Deliver what installed skills link to: point each local link at the delivered copy of its destination, and
// generate reading copies of the files no installed skill holds.

package build

import (
	"fmt"
	"maps"
	"net/url"
	"slices"
	"strings"

	"github.com/fabricahq/trailguide/internal/library"
	"github.com/fabricahq/trailguide/internal/librarytree"
	"github.com/fabricahq/trailguide/internal/rules"
	"github.com/fabricahq/trailguide/internal/skillrefs"
	"github.com/fabricahq/trailguide/internal/skills"
)

// ReferencesDirectory is the folder, in generated/, of the reading copies of the files installed skills link to
// that no installed skill holds, one folder per source, each file at its path in the source's vendor snapshot.
const ReferencesDirectory = "references"

// generatedRoot is the generated folder's path from the repository root.
const generatedRoot = librarytree.Directory + "/" + librarytree.Generated

// deliveredReferences is what delivering installed skills' links produces. skills maps the project ID of each
// installed skill that has skill targets to its files, with each local link pointing at the delivered copy of its
// destination from every target; copies holds the reading copies by path in generated/; and excluded holds, sorted
// by skill and then excluded skill, one ExcludedSkillLink for each installed skill whose links reach a skill the
// project excludes.
type deliveredReferences struct {
	skills   map[string]map[string][]byte
	copies   map[string][]byte
	excluded []ExcludedSkillLink
}

// deliverReferences analyzes the local links in the Markdown of each source's installed skills, in the repository
// and revision that supplied each file, and in the Markdown they reach, and delivers each destination: a file of an
// installed skill as that skill's installed copy, under its install name; one of a skill the project forks as the
// fork's file at the same path; and any other, including a file of a skill the project excludes, as a reading copy.
// Authored skills' links name the project's own files, which the caller checked, and their installed copies point
// at them in place, or at another authored skill's installed copy. aliases maps a project source's skill that
// withoutCuratedCopies dropped to the curated skill installed in its place. It leaves resolved unchanged, and fails
// with a *skillrefs.Error for a link whose destination the project can't deliver.
func deliverReferences(resolved []resolvedSkill, config rules.Configuration, libraries map[string]Library, aliases map[string]string) (deliveredReferences, error) {
	authored, err := authoredLinks(resolved)
	if err != nil {
		return deliveredReferences{}, err
	}
	result := deliveredReferences{skills: authored, copies: map[string][]byte{}, excluded: []ExcludedSkillLink{}}
	byID := map[string]int{}
	for i, skill := range resolved {
		byID[skill.ID] = i
	}
	for _, source := range config.Sources {
		d := delivery{source: source, snapshot: libraries[source.Name].Snapshot, resolved: resolved, byID: byID, aliases: aliases}
		if err := d.analyze(); err != nil {
			return deliveredReferences{}, err
		}
		for _, skill := range resolved {
			if skill.Source == source.Name && len(skill.Targets) > 0 {
				result.skills[skill.ID] = d.installedFiles(skill)
			}
		}
		maps.Copy(result.copies, d.readingCopies())
		result.excluded = append(result.excluded, d.excludedLinks()...)
	}
	slices.SortFunc(result.excluded, func(a, b ExcludedSkillLink) int {
		return strings.Compare(a.Skill+"\x00"+a.Excluded, b.Skill+"\x00"+b.Excluded)
	})
	return result, nil
}

// authoredLinks returns, by project ID, the files of each authored skill in resolved that has skill targets, with
// each local link rewritten to reach, from every target, the same file it names in skills/: the installed copy of
// an authored skill's file, under its install name, or the project's file in place. It fails as skillrefs.Links
// does, which the caller's check of the authored skills' links rules out.
func authoredLinks(resolved []resolvedSkill) (map[string]map[string][]byte, error) {
	// A link into skills/<origin>/<name> names the skill installed as name: the authored skill in that folder, or,
	// when origin is a source, that source's skill, which has no authored folder, as a fork's link to a sibling skill
	// of its source does.
	installed := map[string]resolvedSkill{}
	for _, skill := range resolved {
		if skill.Source != "" && len(skill.Targets) > 0 {
			installed[librarytree.Directory+"/"+skills.Directory+"/"+skills.Folder(skill.Source, skill.Name)] = skill
		}
	}
	for _, skill := range resolved {
		if skill.Source == "" {
			installed[librarytree.Directory+"/"+skill.Folder] = skill
		}
	}
	result := map[string]map[string][]byte{}
	for _, skill := range resolved {
		if skill.Source != "" || len(skill.Targets) == 0 {
			continue
		}
		files := maps.Clone(skill.Files.Files)
		for name, data := range files {
			if !skillrefs.IsMarkdown(name) {
				continue
			}
			links, err := skillrefs.Links(librarytree.Directory+"/"+skill.Folder+"/"+name, data)
			if err != nil {
				return nil, err
			}
			files[name] = rewriteLinks(data, links, skillRoot(skill)+"/"+name, func(target string) string {
				for folder, other := range installed {
					if rest, inside := strings.CutPrefix(target, folder+"/"); inside {
						return skillRoot(other) + "/" + rest
					}
				}
				return target
			})
		}
		result[skill.ID] = files
	}
	return result, nil
}

// rewriteLinks returns data, a Markdown file delivered at from, by path from the repository root, with each of
// links pointing from there to the file destination returns for its target, also by path from the root.
func rewriteLinks(data []byte, links []skillrefs.Link, from string, destination func(target string) string) []byte {
	return skillrefs.Rewrite(data, links, func(link skillrefs.Link) string {
		relative, err := url.PathUnescape(rules.RelativeLink(from, destination(link.Target)))
		if err != nil {
			return ""
		}
		return relative
	})
}

// skillRoot returns where skill's files are delivered, by path from the repository root: its first skill target's
// folder, since every target folder is two levels below the root and holds the same copy, or its source folder,
// in the vendor snapshot or skills/, when the project has no skill targets.
func skillRoot(skill resolvedSkill) string {
	switch {
	case len(skill.Targets) > 0:
		return skill.Targets[0]
	case skill.Source == "":
		return librarytree.Directory + "/" + skill.Folder
	}
	return librarytree.Directory + "/" + librarytree.Vendor + "/" + skill.Source + "/" + skill.Vendored
}

// delivery delivers the links of one source's installed skills. graphs holds, by the name Snapshot.Origins gives
// each repository the snapshot holds files of, the analysis of the links its installed skills' Markdown reaches.
type delivery struct {
	source   rules.Source
	snapshot library.Snapshot
	resolved []resolvedSkill
	byID     map[string]int
	aliases  map[string]string
	graphs   map[string]*skillrefs.Graph
}

// analyze analyzes, in each repository the snapshot holds files of, the links of the source's installed skills'
// Markdown from there, following the Markdown files that get reading copies. It fails as skillrefs.Analyze does,
// for a link whose destination place can't deliver.
func (d *delivery) analyze() error {
	d.graphs = map[string]*skillrefs.Graph{}
	origins := d.snapshot.Origins()
	for _, name := range slices.Sorted(maps.Keys(origins)) {
		origin := origins[name]
		start := []string{}
		for _, skill := range d.skills(name) {
			for _, file := range slices.Sorted(maps.Keys(skill.Files.Files)) {
				if skillrefs.IsMarkdown(file) {
					path, _ := origin.Path(skill.Vendored + "/" + file)
					start = append(start, path)
				}
			}
		}
		if len(start) == 0 {
			continue
		}
		source, revision := "sources."+d.source.Name, d.snapshot.Commit
		if name != "" {
			source, revision = "sources."+d.source.Name+", curated from "+name, d.snapshot.Curated[name].Commit
		}
		graph, err := skillrefs.Analyze(deliveryReader{d, origin}, start, func(_ string, link skillrefs.Link) (bool, *skillrefs.Error) {
			key, ok := origin.Key(link.Target)
			if !ok {
				return false, &skillrefs.Error{Reason: skillrefs.NotRetained, Problem: notRetainedProblem}
			}
			_, copied, failure := d.place(key)
			return copied && skillrefs.IsMarkdown(key), failure
		}, source, revision)
		if err != nil {
			return err
		}
		d.graphs[name] = graph
	}
	return nil
}

// skills returns the source's installed skills whose files come from the repository the snapshot's origins name.
func (d *delivery) skills(origin string) []resolvedSkill {
	return slices.DeleteFunc(slices.Clone(d.resolved), func(skill resolvedSkill) bool {
		return skill.Source != d.source.Name || d.snapshot.OriginOf(skill.Vendored+"/") != origin
	})
}

// deliveryReader lends one origin's files to the analysis: an installed skill's file as the skill installs it, and
// any other from the snapshot.
type deliveryReader struct {
	d      *delivery
	origin skillrefs.Origin
}

// Read returns the bytes of the file at repository path file.
func (r deliveryReader) Read(file string) ([]byte, error) {
	key, _ := r.origin.Key(file)
	for _, skill := range r.d.resolved {
		if name, inside := strings.CutPrefix(key, skill.Vendored+"/"); skill.Source == r.d.source.Name && inside {
			return skill.Files.Files[name], nil
		}
	}
	return r.d.snapshot.Files[key], nil
}

// notRetainedProblem explains a destination the project's copy of a source lacks.
const notRetainedProblem = "the project's copy of the source doesn't retain. Run trailguide project sync, which takes the files installed skills link to from the revision .trailguide/lock.json records."

// place returns where the project delivers the file at path key in the snapshot, by path from the repository root,
// and whether that is a reading copy, which every file but an installed or forked skill's gets. It fails, with the
// reason and problem of a *skillrefs.Error, for a fork's missing file and a file the snapshot doesn't hold.
func (d *delivery) place(key string) (string, bool, *skillrefs.Error) {
	for _, id := range slices.Sorted(maps.Keys(d.snapshot.Skills)) {
		name, inside := strings.CutPrefix(key, d.snapshot.SkillFolder(id)+"/")
		if !inside {
			continue
		}
		if _, excluded := d.source.SkillExclude[id]; excluded {
			break
		}
		installed := d.source.Name + ":" + id
		if fork, forked := d.source.SkillForks[id]; forked {
			installed = fork.As
			if _, ok := d.resolved[d.byID[fork.As]].Files.Files[name]; !ok {
				return "", false, &skillrefs.Error{Reason: skillrefs.ReplacementMissing, Problem: skillrefs.ReplacementProblem(fmt.Sprintf("%s:%s, which the project forks as %s", d.source.Name, id, fork.As))}
			}
		} else if alias, ok := d.aliases[installed]; ok {
			installed = alias
		}
		i, ok := d.byID[installed]
		if _, has := d.resolved[i].Files.Files[name]; !ok || !has {
			return "", false, &skillrefs.Error{Reason: skillrefs.NotRetained, Problem: notRetainedProblem}
		}
		return skillRoot(d.resolved[i]) + "/" + name, false, nil
	}
	if _, ok := d.snapshot.Files[key]; !ok {
		return "", false, &skillrefs.Error{Reason: skillrefs.NotRetained, Problem: notRetainedProblem}
	}
	return generatedRoot + "/" + d.copyPath(key), true, nil
}

// destination returns where the project delivers the file at repository path target of origin, a link's target
// the analysis accepted.
func (d *delivery) destination(origin skillrefs.Origin, target string) string {
	key, _ := origin.Key(target)
	destination, _, _ := d.place(key)
	return destination
}

// installedFiles returns skill's files, an installed skill of the source, with each link of its Markdown pointing
// from its first skill target at the delivered copy of its destination.
func (d *delivery) installedFiles(skill resolvedSkill) map[string][]byte {
	name := d.snapshot.OriginOf(skill.Vendored + "/")
	origin, graph := d.snapshot.Origins()[name], d.graphs[name]
	files := maps.Clone(skill.Files.Files)
	for file := range files {
		if path, _ := origin.Path(skill.Vendored + "/" + file); skillrefs.IsMarkdown(file) {
			files[file] = rewriteLinks(graph.Data(path), graph.Links(path), skillRoot(skill)+"/"+file, func(target string) string { return d.destination(origin, target) })
		}
	}
	return files
}

// readingCopies returns, by path in generated/, a reading copy of each file the source's installed skills' links
// reach that no installed skill holds, a Markdown one with its links pointing at the delivered copies of their
// destinations.
func (d *delivery) readingCopies() map[string][]byte {
	copies := map[string][]byte{}
	origins := d.snapshot.Origins()
	for name, graph := range d.graphs {
		origin := origins[name]
		followed := map[string]bool{}
		for _, file := range graph.Files() {
			followed[file] = true
		}
		for _, target := range graph.Targets() {
			key, _ := origin.Key(target)
			destination, copied, _ := d.place(key)
			switch {
			case !copied:
			case followed[target]:
				copies[d.copyPath(key)] = rewriteLinks(graph.Data(target), graph.Links(target), destination, func(target string) string { return d.destination(origin, target) })
			default:
				copies[d.copyPath(key)] = d.snapshot.Files[key]
			}
		}
	}
	return copies
}

// excludedSkill returns the project ID of the skill the project excludes, by ID or by level, whose folder holds the
// file at path key in the snapshot, or "" when no such skill does.
func (d *delivery) excludedSkill(key string) string {
	for id := range d.source.SkillExclude {
		if strings.HasPrefix(key, d.snapshot.SkillFolder(id)+"/") {
			return d.source.Name + ":" + id
		}
	}
	for id, skill := range d.snapshot.AvailableSkills {
		// A skill the snapshot records is installed, whatever its level.
		if _, installed := d.snapshot.Skills[id]; !installed && strings.HasPrefix(key, d.snapshot.SkillFolder(id)+"/") && d.source.ExcludesLevel(id, skill.Level) {
			return d.source.Name + ":" + id
		}
	}
	return ""
}

// excludedLinks returns, for each of the source's installed skills, the first chain of links from its Markdown,
// through reading copies, to each skill the project excludes. Each skill walks the links on its own, so a file
// several skills reach counts for each of them, and another installed skill's files are that skill's to report.
func (d *delivery) excludedLinks() []ExcludedSkillLink {
	links := []ExcludedSkillLink{}
	origins := d.snapshot.Origins()
	for _, skill := range d.resolved {
		if skill.Source != d.source.Name {
			continue
		}
		name := d.snapshot.OriginOf(skill.Vendored + "/")
		origin, graph := origins[name], d.graphs[name]
		start := []string{}
		for _, file := range slices.Sorted(maps.Keys(skill.Files.Files)) {
			path, _ := origin.Path(skill.Vendored + "/" + file)
			start = append(start, path)
		}
		reached := map[string]bool{}
		copied := func(file string) bool {
			key, _ := origin.Key(file)
			_, copied, _ := d.place(key)
			return copied
		}
		graph.Reach(start, copied, func(target string, chain []string) {
			key, _ := origin.Key(target)
			if excluded := d.excludedSkill(key); excluded != "" && !reached[excluded] {
				reached[excluded] = true
				links = append(links, ExcludedSkillLink{Skill: skill.ID, Excluded: excluded, Chain: append(chain, target)})
			}
		})
	}
	return links
}

// copyPath returns the path, in generated/, of the reading copy of the file at path key in the snapshot.
func (d *delivery) copyPath(key string) string {
	return ReferencesDirectory + "/" + d.source.Name + "/" + key
}

// referencesReadme explains the reading copies to people who open generated/references/.
const referencesReadme = "# Referenced files\n\nInstalled skills link to these files, which no installed skill holds, such as a skill the project doesn't install, or a document beside the skills in their source's repository. Trail Guide keeps a copy of each one here, from the revision `.trailguide/lock.json` records for its source, at its path in the source's vendor snapshot, so the links work offline. These copies aren't skills agents load; read them where a skill links to them. Edit the configuration and rebuild to change them.\n"
