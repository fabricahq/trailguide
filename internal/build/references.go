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
)

// ReferencesDirectory is the folder, in generated/, of the reading copies of the files installed skills link to
// that no installed skill holds, one folder per source, each file at its path in the source's vendor snapshot.
const ReferencesDirectory = "references"

// generatedRoot is the generated folder's path from the repository root.
const generatedRoot = librarytree.Directory + "/" + librarytree.Generated

// deliverReferences resolves every local link in the Markdown of each source's installed skills, in the
// repository and revision that supplied the file, and in the reading copies those links reach, until no new file
// is reached. A destination in an installed skill is that skill's installed copy, under its install name; one in a
// skill the project forks is the fork's file at the same path; one in a skill the project excludes, by ID or by
// level, fails; and any other is a reading copy in generated/references/. It rewrites the links of the installed
// copies in resolved, when the project has skill targets, and of the reading copies, to reach the delivered files,
// and returns the reading copies by path in generated/. aliases maps a project source's skill that
// withoutCuratedCopies dropped to the curated skill installed in its place. Authored skills' links are the
// project's own and stay as written. It fails with a *skillrefs.Error for a link whose destination the project
// can't deliver.
func deliverReferences(resolved []resolvedSkill, config rules.Configuration, libraries map[string]Library, aliases map[string]string) (map[string][]byte, error) {
	copies := map[string][]byte{}
	byID := map[string]int{}
	for i, skill := range resolved {
		byID[skill.ID] = i
	}
	// delivered holds the path, from the repository root, of every file the delivery writes, for the final check.
	delivered := map[string]bool{}
	for _, skill := range resolved {
		for name := range skill.Files.Files {
			delivered[skillRoot(skill)+"/"+name] = true
		}
	}
	links := map[string][]linkDelivery{}
	for _, source := range config.Sources {
		d := delivery{source: source, snapshot: libraries[source.Name].Snapshot, resolved: resolved, byID: byID, aliases: aliases, via: map[string]string{}, files: map[string]map[string]string{}}
		if err := d.walk(); err != nil {
			return nil, err
		}
		for _, key := range slices.Sorted(maps.Keys(d.copies)) {
			delivered[generatedRoot+"/"+d.copyPath(key)] = true
		}
		for key, targets := range d.files {
			from := d.locate(key)
			for _, target := range slices.Sorted(maps.Keys(targets)) {
				links[from] = append(links[from], linkDelivery{key: key, target: target, destination: targets[target]})
			}
		}
		for i, skill := range resolved {
			if skill.Source != source.Name || len(skill.Targets) == 0 {
				continue
			}
			files := maps.Clone(skill.Files.Files)
			for name, data := range files {
				if skillrefs.IsMarkdown(name) {
					files[name] = d.rewrite(skill.Vendored+"/"+name, data)
				}
			}
			resolved[i].Files.Files = files
		}
		for key := range d.copies {
			data := d.snapshot.Files[key]
			if skillrefs.IsMarkdown(key) {
				data = d.rewrite(key, data)
			}
			copies[d.copyPath(key)] = data
		}
	}
	// Every rewritten link must reach a file the delivery writes, in the layout agents read.
	for _, from := range slices.Sorted(maps.Keys(links)) {
		if !strings.HasPrefix(from, generatedRoot+"/") && strings.HasPrefix(from, librarytree.Directory+"/") {
			continue
		}
		for _, link := range links[from] {
			if !delivered[link.destination] {
				return nil, invalid(from, fmt.Sprintf("links to %s, which the project doesn't deliver; report this as a Trail Guide bug", link.destination))
			}
		}
	}
	return copies, nil
}

// linkDelivery is one link of a delivered file: key is the linking file's path in its source's snapshot, target the
// repository path the link names, and destination the delivered file it reaches, by path from the repository root.
type linkDelivery struct {
	key, target, destination string
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

// delivery resolves the links of one source's installed skills.
type delivery struct {
	source   rules.Source
	snapshot library.Snapshot
	resolved []resolvedSkill
	byID     map[string]int
	aliases  map[string]string
	// copies holds the paths, in the snapshot, of the files that get reading copies; via maps each file followed to
	// the file whose link reached it, or "" for an installed skill's file.
	copies map[string]bool
	via    map[string]string
	// files maps each followed file's snapshot path to its links' targets, by repository path, and the delivered
	// file each reaches, by path from the repository root.
	files map[string]map[string]string
}

// walk follows the links of the source's installed skills' Markdown, and of the reading copies they reach.
func (d *delivery) walk() error {
	d.copies = map[string]bool{}
	queue := []string{}
	for _, skill := range d.resolved {
		if skill.Source != d.source.Name {
			continue
		}
		for _, name := range slices.Sorted(maps.Keys(skill.Files.Files)) {
			if skillrefs.IsMarkdown(name) {
				key := skill.Vendored + "/" + name
				d.via[key] = ""
				queue = append(queue, key)
			}
		}
	}
	for len(queue) > 0 {
		key := queue[0]
		queue = queue[1:]
		origin := d.snapshot.Origins()[d.snapshot.OriginOf(key)]
		file, _ := origin.Path(key)
		links, err := skillrefs.Links(file, d.snapshot.Files[key])
		if err != nil {
			return d.fail(err.(*skillrefs.Error), key)
		}
		d.files[key] = map[string]string{}
		for _, link := range links {
			target, ok := origin.Key(link.Target)
			destination, follow, failure := "", false, (*skillrefs.Error)(nil)
			if ok {
				destination, follow, failure = d.classify(target)
			} else {
				failure = &skillrefs.Error{Reason: skillrefs.NotRetained, Problem: "the project's copy of the source doesn't retain. Run trailguide project sync, which takes the files installed skills link to from the revision .trailguide/lock.json records."}
			}
			if failure != nil {
				failure.File, failure.Destination, failure.Target = file, link.Destination, link.Target
				return d.fail(failure, key)
			}
			d.files[key][link.Target] = destination
			if follow {
				if _, seen := d.via[target]; !seen {
					d.via[target] = key
					queue = append(queue, target)
				}
			}
		}
	}
	return nil
}

// classify returns the delivered file that the file at path key in the snapshot is, by path from the repository
// root, and whether its links are followed, which they are for a reading copy of a Markdown file. It fails, with
// the reason and problem of a *skillrefs.Error, for a file in a skill the project excludes, a fork's missing file,
// and a file the snapshot doesn't hold.
func (d *delivery) classify(key string) (string, bool, *skillrefs.Error) {
	for _, id := range slices.Sorted(maps.Keys(d.snapshot.Skills)) {
		folder := library.SkillFolder(id)
		name, inside := strings.CutPrefix(key, folder+"/")
		if !inside {
			continue
		}
		installed := d.source.Name + ":" + id
		if exclusion, excluded := d.source.SkillExclude[id]; excluded {
			return "", false, &skillrefs.Error{Reason: skillrefs.Excluded, Problem: skillrefs.Problem(skillrefs.Excluded, fmt.Sprintf("%s, which sources.%s.skills.exclude excludes: %s", installed, d.source.Name, exclusion.Reason))}
		}
		if fork, forked := d.source.SkillForks[id]; forked {
			installed = fork.As
			if _, ok := d.resolved[d.byID[fork.As]].Files.Files[name]; !ok {
				return "", false, &skillrefs.Error{Reason: skillrefs.ReplacementMissing, Problem: skillrefs.Problem(skillrefs.ReplacementMissing, fmt.Sprintf("%s:%s, which the project forks as %s", d.source.Name, id, fork.As))}
			}
		} else if alias, ok := d.aliases[installed]; ok {
			installed = alias
		}
		i, ok := d.byID[installed]
		if _, has := d.resolved[i].Files.Files[name]; !ok || !has {
			return "", false, &skillrefs.Error{Reason: skillrefs.NotRetained, Problem: "the project's copy of the source doesn't retain. Run trailguide project sync, which takes the files installed skills link to from the revision .trailguide/lock.json records."}
		}
		return skillRoot(d.resolved[i]) + "/" + name, false, nil
	}
	for _, id := range slices.Sorted(maps.Keys(d.snapshot.AvailableSkills)) {
		if level := d.snapshot.AvailableSkills[id].Level; strings.HasPrefix(key, library.SkillFolder(id)+"/") && d.source.ExcludesLevel(id, level) {
			by := "skills.exclude"
			if level != "" {
				by += ", which leaves out its level, " + level
			}
			return "", false, &skillrefs.Error{Reason: skillrefs.Excluded, Problem: skillrefs.Problem(skillrefs.Excluded, fmt.Sprintf("%s:%s, which sources.%s excludes with %s", d.source.Name, id, d.source.Name, by))}
		}
	}
	if _, ok := d.snapshot.Files[key]; !ok {
		return "", false, &skillrefs.Error{Reason: skillrefs.NotRetained, Problem: "the project's copy of the source doesn't retain. Run trailguide project sync, which takes the files installed skills link to from the revision .trailguide/lock.json records."}
	}
	d.copies[key] = true
	return generatedRoot + "/" + d.copyPath(key), skillrefs.IsMarkdown(key), nil
}

// copyPath returns the path, in generated/, of the reading copy of the file at path key in the snapshot.
func (d *delivery) copyPath(key string) string {
	return ReferencesDirectory + "/" + d.source.Name + "/" + key
}

// locate returns where the file at path key in the snapshot is delivered, by path from the repository root: its
// installed skill's copy, or its reading copy.
func (d *delivery) locate(key string) string {
	if d.copies[key] {
		return generatedRoot + "/" + d.copyPath(key)
	}
	for _, skill := range d.resolved {
		if name, inside := strings.CutPrefix(key, skill.Vendored+"/"); skill.Source == d.source.Name && inside {
			return skillRoot(skill) + "/" + name
		}
	}
	return librarytree.Directory + "/" + librarytree.Vendor + "/" + d.source.Name + "/" + key
}

// rewrite returns data, the file at path key in the snapshot, with each local link pointing from where the file
// is delivered to where its destination is.
func (d *delivery) rewrite(key string, data []byte) []byte {
	origin := d.snapshot.Origins()[d.snapshot.OriginOf(key)]
	file, _ := origin.Path(key)
	links, err := skillrefs.Links(file, data)
	if err != nil {
		return data
	}
	from := d.locate(key)
	return skillrefs.Rewrite(data, links, func(link skillrefs.Link) string {
		destination, ok := d.files[key][link.Target]
		if !ok {
			return ""
		}
		relative := rules.RelativeLink(from, destination)
		if decoded, err := url.PathUnescape(relative); err == nil {
			return decoded
		}
		return ""
	})
}

// fail completes failure with the source, revision, and chain of the file at path key in the snapshot.
func (d *delivery) fail(failure *skillrefs.Error, key string) error {
	name := d.snapshot.OriginOf(key)
	failure.Source, failure.Revision = "sources."+d.source.Name, d.snapshot.Commit
	if name != "" {
		failure.Source, failure.Revision = "sources."+d.source.Name+", curated from "+name, d.snapshot.Curated[name].Commit
	}
	origin := d.snapshot.Origins()[name]
	via := map[string]string{}
	for file, from := range d.via {
		path, _ := origin.Path(file)
		via[path], _ = origin.Path(from)
	}
	file, _ := origin.Path(key)
	failure.Chain = skillrefs.Chain(file, via)
	return failure
}

// referencesReadme explains the reading copies to people who open generated/references/.
const referencesReadme = "# Referenced files\n\nInstalled skills link to these files, which no installed skill holds, such as a skill the project doesn't install, or a document beside the skills in their source's repository. Trail Guide keeps a copy of each one here, from the revision `.trailguide/lock.json` records for its source, at its path in the source's vendor snapshot, so the links work offline. These copies aren't skills agents load; read them where a skill links to them. Edit the configuration and rebuild to change them.\n"
