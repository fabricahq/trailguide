// Fork a skill a source supplies into skills/, and replace the source's skill with it.

package project

import (
	"bytes"
	"context"
	"fmt"
	"maps"
	"net/url"
	"os"
	"path"
	"slices"
	"strings"

	"github.com/fabricahq/trailguide/internal/build"
	"github.com/fabricahq/trailguide/internal/filetxn"
	"github.com/fabricahq/trailguide/internal/library"
	"github.com/fabricahq/trailguide/internal/librarytree"
	"github.com/fabricahq/trailguide/internal/rules"
	"github.com/fabricahq/trailguide/internal/skillrefs"
	"github.com/fabricahq/trailguide/internal/skills"
)

// SkillForkSource names the configured source and the skill in it that a fork copies.
type SkillForkSource struct {
	Source string
	// Skill is the skill's ID in the source, its folder's path there, such as skills/fix-bug, or the ID of a skill the
	// source's library curates, such as mattpocock:skills/to-spec.
	Skill string
}

// String returns SOURCE:SKILL.
func (s SkillForkSource) String() string { return s.Source + ":" + s.Skill }

// ParseSkillForkSource reads SOURCE:SKILL, splitting at the first colon, because source names have none. SKILL
// can itself name a curated skill, such as mattpocock:skills/to-spec.
func ParseSkillForkSource(value string) (SkillForkSource, error) {
	source, skill, found := strings.Cut(value, ":")
	curatedFrom, id, curated := library.SplitCurated(skill)
	if !found || source == "" || skill == "" || curated && (curatedFrom == "" || id == "") {
		return SkillForkSource{}, &ValidationError{Location: "--from", Problem: fmt.Sprintf("%q: expected SOURCE:SKILL, a configured source and the skill's ID in it, such as fabrica:skills/fix-bug, or fabrica:mattpocock:skills/to-spec for a skill its library curates", value)}
	}
	if !curated {
		id = skill
	}
	if err := skills.ValidateID(id, "--from"); err != nil {
		return SkillForkSource{}, err
	}
	return SkillForkSource{Source: source, Skill: skill}, nil
}

// SkillForkPlan is a planned fork of a skill a source installs. Planning reads the project without writing; Commit
// writes the fork.
type SkillForkPlan struct {
	from    SkillForkSource
	options Options
	// configBytes is the configuration planning read, and lock the lockfile whose entry it copied the skill as,
	// which digests every vendored file; Commit refuses to write when either changed.
	configBytes, lock []byte
	// name is the skill's install name, which names the fork's folder, and commit the commit of the source's
	// repository that the source's lock entry records, which the fork records as basedOn.
	name, commit string
	// renamed reports whether the source's skills.rename gave the skill its install name; Commit then removes that
	// entry, since the fork replaces it.
	renamed bool
	// files holds the fork's files, by path relative to the Trail Guide directory, with their executable bits.
	files []filetxn.File
	// fixes says what the author must change in the fork's SKILL.md that Trail Guide couldn't, in which case the
	// fork's SKILL.md carries the draft marker, so build refuses it until the author fixes it.
	fixes []string
	// linksOutside reports that the skill links to files outside every installed skill, which the source's last sync
	// kept for it, so build refuses them until a sync drops them.
	linksOutside bool
}

// PlanSkillFork checks the project and prepares a fork of the skill from names, which can be one the source's
// library curates, copied from vendor/ as the source's last sync recorded it, with its SKILL.md rewritten as
// forkedSkillFile rewrites it, into skills/<name>/, where name is the skill's install name, the one the source's
// skills.rename gives it when it has one, without writing. It fails when the source isn't configured, when its record
// doesn't match its configuration, when its last sync didn't install the skill, when the source's library requires
// the skill, when the source already excludes or forks the skill, or when the project already has an authored skill
// with that name or another fork installs as it, since existing authored skills, exclusions, and forks are never
// replaced. When Trail Guide can't make the fork's SKILL.md what the project needs, because forkedSkillFile can't, or
// because the library curates the skill from another library and the skill cites rules, which the project can't
// name, the fork's SKILL.md carries the draft marker, and Fixes says what the author must change.
func PlanSkillFork(ctx context.Context, from SkillForkSource, options Options) (*SkillForkPlan, error) {
	plan := &SkillForkPlan{from: from}
	options, err := planProjectAuthoring(ctx, options, func(root *os.Root, _ rules.Configuration) error {
		original, config, err := configuration(ctx, root)
		if err != nil {
			return err
		}
		plan.configBytes = original
		index := slices.IndexFunc(config.Sources, func(source rules.Source) bool { return source.Name == from.Source })
		if index < 0 {
			return &ValidationError{Location: "--from", Problem: "no source named " + from.Source + " in .trailguide/config.yaml; name a configured source"}
		}
		source := config.Sources[index]
		if _, excluded := source.SkillExclude[from.Skill]; excluded {
			return &ValidationError{Location: "sources." + source.Name + ".skills.exclude." + from.Skill, Problem: "the source already excludes this skill, and a fork never replaces an existing exclusion; delete the entry to fork the skill"}
		}
		if _, forked := source.SkillForks[from.Skill]; forked {
			return &ValidationError{Location: "sources." + source.Name + ".skills.forks." + from.Skill, Problem: "the project already forks this skill, and a fork never replaces an existing one; delete the entry and the fork's folder to fork the skill again"}
		}
		stored, vendor, lock, err := readStored(ctx, root, config)
		if err != nil {
			return err
		}
		snapshots, err := decodeSnapshots(config, stored, treeFiles(vendor))
		if err != nil {
			return err
		}
		if err := verifyExecutable(snapshots, vendor); err != nil {
			return err
		}
		snapshot := snapshots[source.Name]
		if snapshot.Skills[from.Skill].Level == skills.LevelRequired {
			return &ValidationError{Location: "--from", Problem: rules.RequiredSkillProblem(source.Name, from.Skill, snapshot.Release)}
		}
		plan.lock = lock
		if _, installed := snapshot.Skills[from.Skill]; !installed {
			return &ValidationError{Location: "--from", Problem: fmt.Sprintf("source %s doesn't install skill %s, as its last sync recorded; name a skill its skills.include selects, and if you just added it, run trailguide project sync first", source.Name, from.Skill)}
		}
		files := snapshot.SkillFiles(from.Skill)
		location := librarytree.Vendor + "/" + source.Name + "/" + library.SkillFolder(from.Skill) + "/" + skills.File
		metadata, err := skills.ReadMetadata(files[skills.File], location)
		if err != nil {
			return err
		}
		// A curated skill comes from the repository its library curates it from, at the commit the source record
		// locks for it there, and its trailguide-skills-used names skills of that repository, which the library
		// curates with it. skills.uses applies only to the source's own skills.
		id, prefix, qualifyRules, uses := from.Skill, source.Name, !snapshot.NonLibrary, source.SkillUses[from.Skill]
		plan.commit = snapshot.Commit
		if curatedFrom, curatedID, curated := library.SplitCurated(from.Skill); curated {
			origin := snapshot.Curated[curatedFrom]
			// A curated skill the library doesn't list reaches the project only with a listed skill that uses it, and
			// sync accepts exclusions only of skills the library lists.
			if _, listed := snapshot.AvailableSkills[from.Skill]; !listed {
				users := []string{}
				for _, user := range snapshot.Skills[from.Skill].UsedBy {
					users = append(users, source.Name+":"+user)
				}
				return &ValidationError{Location: "--from", Problem: fmt.Sprintf("%s's library doesn't list %s, so the project installs it only because %s uses it, and can't fork it on its own; fork the skill that uses it instead, or ask the library's maintainer to list it", source.Name, from.Skill, strings.Join(users, " and "))}
			}
			id, prefix, qualifyRules, uses = curatedID, source.Name+":"+curatedFrom, false, nil
			plan.commit = origin.Commit
			if !origin.NonLibrary && len(metadata.RulesUsed) > 0 {
				plan.fixes = append(plan.fixes, fmt.Sprintf("Remove the rules of %s from %s: the project takes no rules from a library %s curates a skill from, so it can't cite them.", curatedFrom, skills.RulesUsedKey, source.Name))
			}
		}
		if name, renamed := source.SkillRename[from.Skill]; renamed {
			plan.name, plan.renamed = name, true
		} else if plan.name, err = skills.InstallName(id, metadata, location); err != nil {
			return err
		}
		folder := skills.AuthoredID(plan.name)
		if err := requireSkillForkTarget(ctx, root, config, folder); err != nil {
			return err
		}
		executable := snapshot.SkillExecutable(from.Skill)
		layout, err := readLayout(root)
		if err != nil {
			return err
		}
		record, err := filetxn.ReadOptional(ctx, root, path.Join(string(layout.generated), build.InstalledSkillsFile))
		if err != nil {
			return err
		}
		linked, fixes, err := forkLinks(snapshot, source, from.Skill, plan.name, files, build.InstalledSkills(record))
		if err != nil {
			return err
		}
		plan.fixes, plan.linksOutside = append(plan.fixes, fixes...), len(fixes) > 0
		for _, name := range slices.Sorted(maps.Keys(files)) {
			content := files[name]
			if data, ok := linked[name]; ok {
				content = data
			}
			if name == skills.File {
				if plan.renamed {
					content = skills.Rename(content, plan.name)
					if metadata, err = skills.ReadMetadata(content, location); err != nil {
						return err
					}
				}
				var fixes []string
				if content, fixes, err = forkedSkillFile(content, metadata, plan.name, prefix, qualifyRules, uses, location); err != nil {
					return err
				}
				if plan.fixes = append(plan.fixes, fixes...); len(plan.fixes) > 0 {
					content = skills.Mark(content, rules.DraftMarker)
				}
			}
			plan.files = append(plan.files, filetxn.File{Path: path.Join(folder, name), Content: content, Executable: slices.Contains(executable, name)})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	plan.options = options
	return plan, nil
}

// requireSkillForkTarget fails when the Trail Guide directory already has folder, the fork's authored skill folder,
// skills/<name>, or when another fork already installs as that authored skill.
func requireSkillForkTarget(ctx context.Context, root *os.Root, config rules.Configuration, folder string) error {
	for _, source := range config.Sources {
		for _, id := range slices.Sorted(maps.Keys(source.SkillForks)) {
			if source.SkillForks[id].As == folder {
				return &ValidationError{Location: "sources." + source.Name + ".skills.forks." + id + ".as", Problem: fmt.Sprintf("already names %s, the new fork's folder, and each authored skill replaces one skill; remove that fork first", folder)}
			}
		}
	}
	return filetxn.RequireAbsent(ctx, root, folder)
}

// Name returns the fork's name, which is the skill's install name, so its ID is skills/<name>.
func (p *SkillForkPlan) Name() string { return p.name }

// Fixes says what the author must change in the fork's SKILL.md, which then carries the draft marker, or is empty
// when the fork is complete.
func (p *SkillForkPlan) Fixes() []string { return p.fixes }

// LinksOutside reports that the skill links to files outside every skill the project installs, which the source's
// last sync kept for the skill and the next sync drops, so the fork installs with sync rather than build.
func (p *SkillForkPlan) LinksOutside() bool { return p.linksOutside }

// BasedOn returns the full commit SHA the fork copies the skill from, which skills.forks records as basedOn.
func (p *SkillForkPlan) BasedOn() string { return p.commit }

// Commit writes the fork under writer ownership, and, in the same edit, adds the skill to the source's skills.forks,
// with the fork as as, the commit it copied as basedOn, and reason, removing the skill's skills.rename entry, so the
// fork installs in place of the source's skill and updates list the source's later changes to it. It writes nothing when reason is blank, when the
// configuration or the source record changed after planning, such as when an update moved the source to another
// commit, or when any of the fork's files exists.
func (p *SkillForkPlan) Commit(ctx context.Context, reason string) (AuthoringResult, error) {
	if p == nil || p.name == "" {
		return AuthoringResult{}, failure("invalid-operation", "expected a planned skill fork", nil)
	}
	if strings.TrimSpace(reason) == "" {
		return AuthoringResult{}, &ValidationError{Location: "--reason", Problem: "give the reason the project uses the fork instead of the skill it installs from " + p.from.Source}
	}
	basedOn, err := rules.ParseGitRef(p.commit, entryLocation(p.from.Source)+".commit")
	if err != nil {
		return AuthoringResult{}, err
	}
	folder := skills.AuthoredID(p.name)
	return editProject(ctx, p.options, func(root *os.Root, original []byte, config rules.Configuration) ([]filetxn.File, error) {
		if !bytes.Equal(original, p.configBytes) {
			return nil, failure("concurrent-change", "the project's configuration changed after the fork was planned; run the command again", nil)
		}
		lock, err := filetxn.ReadOptional(ctx, root, lockFile)
		if err != nil {
			return nil, err
		}
		if !bytes.Equal(lock, p.lock) {
			return nil, failure("concurrent-change", "the source's imported files changed after the fork was planned; run the command again", nil)
		}
		if err := requireSkillForkTarget(ctx, root, config, folder); err != nil {
			return nil, err
		}
		edit := rules.SourceEdit{SkillForks: map[string]rules.SkillFork{p.from.Skill: {As: folder, Reason: reason, BasedOn: basedOn}}}
		if p.renamed {
			edit.SkillUnrename = []string{p.from.Skill}
		}
		edited, err := rules.EditConfigurationSource(original, p.from.Source, edit)
		if err != nil {
			return nil, err
		}
		return withSkillsGuide(ctx, root, append(slices.Clone(p.files), filetxn.File{Path: configurationFile, Content: edited, Previous: original}))
	})
}

// forkedSkillFile returns data, the SKILL.md of a skill a source supplies, as its fork, the authored skill
// skills/<name>, needs it: with frontmatter that names the skill name, which a SKILL.md from a repository that isn't
// a library can lack; with source, which starts the skill's project ID, such as team, or team:pocock for a skill
// team's library curates from pocock, and a colon added to each bare ID its trailguide-skills-used lists, and, when
// library is true, its trailguide-rules-used, since in the project a bare ID names the project's own skill or rule;
// when library is true, also with source added to each ID of a skill the library curates, such as
// anthropic:skills/skill-creator, since the project names that skill with the library source's name first;
// and with uses, the IDs the source's skills.uses adds for the skill, added to trailguide-skills-used the same way,
// since skills.uses applies only to the source's own skills. metadata is data's, read at location. It rewrites only
// what differs, replacing a list it changes with a block string. When it can't rewrite the frontmatter to say
// exactly that, such as a list inside a flow mapping, it returns data unchanged, and when the result fails the
// checks build applies to an authored skill, such as a type other than playbook or review, the result; either way
// with fixes, what the author must change.
func forkedSkillFile(data []byte, metadata skills.Metadata, name, source string, library bool, uses []string, location string) ([]byte, []string, error) {
	// curated reports that a qualified ID names a skill the source's library curates, which needs the source's name too.
	qualify := func(ids []string, curated bool) []string {
		result := []string{}
		for _, id := range ids {
			if curated || !strings.Contains(id, ":") {
				id = source + ":" + id
			}
			if !slices.Contains(result, id) {
				result = append(result, id)
			}
		}
		return result
	}
	skillsUsed, rulesUsed := qualify(slices.Concat(metadata.SkillsUsed, uses), library), metadata.RulesUsed
	if library {
		rulesUsed = qualify(metadata.RulesUsed, false)
	}
	text, ok := skills.Fork(data, metadata, name, skillsUsed, rulesUsed)
	if !ok {
		fix := fmt.Sprintf("Name the skill %s, and cite the source's skills and rules with the source's name, such as %s:skills/tdd; Trail Guide edits only frontmatter written as block mappings.", name, source)
		if len(skillsUsed) > 0 {
			fix += fmt.Sprintf(" List exactly these in %s: %s.", skills.SkillsUsedKey, strings.Join(skillsUsed, ", "))
		}
		return data, []string{fix}, nil
	}
	forked, err := skills.ReadMetadata(text, location)
	if err != nil {
		return nil, nil, err
	}
	fixes := []string{}
	for _, problem := range skills.Check(name, forked, true) {
		fixes = append(fixes, "Fix what build reports: "+problem+".")
	}
	return text, fixes, nil
}

// forkLinks returns, by path in the skill's folder, the Markdown files of files, the folder of skill id in snapshot,
// the snapshot of source, whose links leave the folder, with each link to a skill the project installs, as installed
// lists them, or forks pointing at it by the name it installs as, so the fork, skills/<name>, reaches that skill's
// installed copy. A link that reaches any other file outside the folder, such as a shared document, or a skill the
// project doesn't install, has nothing the fork can reach, so it returns a fix for each.
func forkLinks(snapshot library.Snapshot, source rules.Source, id, name string, files map[string][]byte, installed []build.InstalledSkill) (map[string][]byte, []string, error) {
	folder := library.SkillFolder(id)
	origin := snapshot.Origins()[snapshot.OriginOf(folder+"/")]
	installedAs := map[string]string{}
	for _, skill := range installed {
		if other, ok := strings.CutPrefix(skill.ID, source.Name+":"); ok {
			installedAs[other] = skill.Name
		}
	}
	for other, fork := range source.SkillForks {
		installedAs[other] = strings.TrimPrefix(fork.As, skills.Directory+"/")
	}
	result := map[string][]byte{}
	fixes := []string{}
	for _, file := range slices.Sorted(maps.Keys(files)) {
		if !skillrefs.IsMarkdown(file) {
			continue
		}
		from, _ := origin.Path(folder + "/" + file)
		links, err := skillrefs.Links(from, files[file])
		if err != nil {
			return nil, nil, err
		}
		changed := false
		rewritten := skillrefs.Rewrite(files[file], links, func(link skillrefs.Link) string {
			key, _ := origin.Key(link.Target)
			if strings.HasPrefix(key, folder+"/") {
				return link.Destination
			}
			for _, other := range slices.Sorted(maps.Keys(snapshot.Skills)) {
				rest, inside := strings.CutPrefix(key, library.SkillFolder(other)+"/")
				if !inside {
					continue
				}
				if as, ok := installedAs[other]; ok && other != id {
					forked := librarytree.Directory + "/" + skills.AuthoredID(name) + "/" + file
					relative, err := url.PathUnescape(rules.RelativeLink(forked, librarytree.Directory+"/"+skills.AuthoredID(as)+"/"+rest))
					if err == nil {
						changed = changed || relative != link.Destination
						return relative
					}
				}
				break
			}
			fixes = append(fixes, fmt.Sprintf("In %s, replace the link to %s. It reaches %s in %s, which is outside the skill and every skill the project installs, so the fork can't reach it. Copy the file into the fork and link to the copy, or remove the link.", file, link.Destination, link.Target, source.Name))
			return link.Destination
		})
		if changed {
			result[file] = rewritten
		}
	}
	return result, fixes, nil
}
