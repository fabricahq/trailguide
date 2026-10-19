// Fork a skill a source installs into skills/, and replace the source's skill with it.

package project

import (
	"bytes"
	"context"
	"fmt"
	"maps"
	"os"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/fabricahq/trailguide/internal/filetxn"
	"github.com/fabricahq/trailguide/internal/library"
	"github.com/fabricahq/trailguide/internal/rules"
	"github.com/fabricahq/trailguide/internal/skills"
)

// SkillForkSource names the configured source and the skill in it that a fork copies.
type SkillForkSource struct {
	Source string
	// Skill is the skill's ID in the source, its folder's path there, such as skills/fix-bug.
	Skill string
}

// String returns SOURCE:SKILL.
func (s SkillForkSource) String() string { return s.Source + ":" + s.Skill }

// ParseSkillForkSource reads SOURCE:SKILL, splitting at the first colon, because source names have none.
func ParseSkillForkSource(value string) (SkillForkSource, error) {
	source, skill, found := strings.Cut(value, ":")
	if !found || source == "" || skill == "" {
		return SkillForkSource{}, &ValidationError{Location: "--from", Problem: fmt.Sprintf("%q: expected SOURCE:SKILL, a configured source and the skill's ID in it, such as fabrica:skills/fix-bug", value)}
	}
	if err := skills.ValidateID(skill, "--from"); err != nil {
		return SkillForkSource{}, err
	}
	return SkillForkSource{Source: source, Skill: skill}, nil
}

// SkillForkPlan is a planned fork of a skill a source installs. Planning reads the project without writing; Commit
// writes the fork.
type SkillForkPlan struct {
	from    SkillForkSource
	options Options
	// configBytes is the configuration planning read, and record the source record it copied the skill as, which
	// digests every vendored file; Commit refuses to write when either changed.
	configBytes, record []byte
	// name is the skill's install name, which names the fork's folder, and commit the commit of the source's
	// repository that the source record locks, which the exclusion records as basedOn.
	name, commit string
	// renamed reports whether the source's skills.rename gave the skill its install name; Commit then removes that
	// entry, since the exclusion replaces it.
	renamed bool
	// files holds the fork's files, by path relative to the Trail Guide directory, with their executable bits.
	files []filetxn.File
}

// PlanSkillFork checks the project and prepares a fork of the skill from names, copied from vendor/ as the source's
// last sync recorded it, with its SKILL.md rewritten as forkedSkillFile rewrites it, into skills/<name>/, where name
// is the skill's install name, the one the source's skills.rename gives it when it has one, without writing. It fails
// when the source isn't configured, when its record doesn't match its configuration, when its last sync didn't
// install the skill, when the source's library requires the skill, when the source already excludes the skill, when
// the project already has an authored skill with that name or another exclusion names it as a replacement, since
// existing authored skills and exclusions are never replaced, or when forkedSkillFile fails.
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
		if _, _, curated := library.SplitCurated(from.Skill); curated {
			return &ValidationError{Location: "--from", Problem: fmt.Sprintf("%s is a skill library %s curates from another repository, which a fork can't record a revision for; declare that repository as a source of your own, run trailguide project sync, and fork the skill from that source", from.Skill, source.Name)}
		}
		if _, excluded := source.SkillExclude[from.Skill]; excluded {
			return &ValidationError{Location: "sources." + source.Name + ".skills.exclude." + from.Skill, Problem: "the source already excludes this skill, and a fork never replaces an existing exclusion; delete the entry to fork the skill"}
		}
		vendor, err := filetxn.ReadTree(ctx, root, "vendor")
		if err != nil {
			return err
		}
		snapshots, err := decodeSnapshots(config, treeFiles(vendor))
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
		plan.record = treeFiles(vendor)[source.Name+"/_source.json"]
		if _, installed := snapshot.Skills[from.Skill]; !installed {
			return &ValidationError{Location: "--from", Problem: fmt.Sprintf("source %s doesn't install skill %s, as its last sync recorded; name a skill its skills.include selects, and if you just added it, run trailguide project sync first", source.Name, from.Skill)}
		}
		files := snapshot.SkillFiles(from.Skill)
		location := "vendor/" + source.Name + "/" + from.Skill + "/" + skills.File
		metadata, err := skills.ReadMetadata(files[skills.File], location)
		if err != nil {
			return err
		}
		if name, renamed := source.SkillRename[from.Skill]; renamed {
			plan.name, plan.renamed = name, true
		} else if plan.name, err = skills.InstallName(from.Skill, metadata, location); err != nil {
			return err
		}
		plan.commit = snapshot.Commit
		folder := skills.AuthoredID(plan.name)
		if err := requireSkillForkTarget(ctx, root, config, folder); err != nil {
			return err
		}
		executable := snapshot.SkillExecutable(from.Skill)
		for _, name := range slices.Sorted(maps.Keys(files)) {
			content := files[name]
			if name == skills.File {
				if plan.renamed {
					content = skills.Rename(content, plan.name)
					if metadata, err = skills.ReadMetadata(content, location); err != nil {
						return err
					}
				}
				if content, err = forkedSkillFile(content, metadata, plan.name, source.Name, !snapshot.NonLibrary, source.SkillUses[from.Skill], location); err != nil {
					return err
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
// skills/<name>, or when an exclusion already names that authored skill as its replacement.
func requireSkillForkTarget(ctx context.Context, root *os.Root, config rules.Configuration, folder string) error {
	for _, source := range config.Sources {
		for _, id := range slices.Sorted(maps.Keys(source.SkillExclude)) {
			if source.SkillExclude[id].ReplacedBy == folder {
				return &ValidationError{Location: "sources." + source.Name + ".skills.exclude." + id + ".replacedBy", Problem: fmt.Sprintf("already names %s, the fork's folder, as a replacement, and each authored skill replaces one skill; remove that replacement first", folder)}
			}
		}
	}
	return filetxn.RequireAbsent(ctx, root, folder)
}

// Name returns the fork's name, which is the skill's install name, so its ID is skills/<name>.
func (p *SkillForkPlan) Name() string { return p.name }

// BasedOn returns the full commit SHA the fork copies the skill from, which the exclusion records as basedOn.
func (p *SkillForkPlan) BasedOn() string { return p.commit }

// Commit writes the fork under writer ownership, and, in the same edit, adds the source's exclusion of the skill
// with reason, the fork as replacedBy, and the commit it copied as basedOn, removing the skill's skills.rename entry, so the fork installs in place of the
// source's skill and updates list the source's later changes to it. It writes nothing when reason is blank, when the
// configuration or the source record changed after planning, such as when an update moved the source to another
// commit, or when any of the fork's files exists.
func (p *SkillForkPlan) Commit(ctx context.Context, reason string) (AuthoringResult, error) {
	if p == nil || p.name == "" {
		return AuthoringResult{}, failure("invalid-operation", "expected a planned skill fork", nil)
	}
	if strings.TrimSpace(reason) == "" {
		return AuthoringResult{}, &ValidationError{Location: "--reason", Problem: "give the reason the project uses the fork instead of the skill it installs from " + p.from.Source}
	}
	basedOn, err := rules.ParseGitRef(p.commit, "vendor/"+p.from.Source+"/_source.json")
	if err != nil {
		return AuthoringResult{}, err
	}
	folder := skills.AuthoredID(p.name)
	return editProject(ctx, p.options, func(root *os.Root, original []byte, config rules.Configuration) ([]filetxn.File, error) {
		if !bytes.Equal(original, p.configBytes) {
			return nil, failure("concurrent-change", "the project's configuration changed after the fork was planned; run the command again", nil)
		}
		record, err := filetxn.ReadOptional(ctx, root, "vendor/"+p.from.Source+"/_source.json")
		if err != nil {
			return nil, err
		}
		if !bytes.Equal(record, p.record) {
			return nil, failure("concurrent-change", "the source's imported files changed after the fork was planned; run the command again", nil)
		}
		if err := requireSkillForkTarget(ctx, root, config, folder); err != nil {
			return nil, err
		}
		edit := rules.SourceEdit{SkillExclude: map[string]rules.SkillExclusion{p.from.Skill: {Reason: reason, ReplacedBy: folder, BasedOn: basedOn}}}
		if p.renamed {
			edit.SkillUnrename = []string{p.from.Skill}
		}
		edited, err := rules.EditConfigurationSource(original, p.from.Source, edit)
		if err != nil {
			return nil, err
		}
		return append(slices.Clone(p.files), filetxn.File{Path: configurationFile, Content: edited, Previous: original}), nil
	})
}

// forkedSkillFile returns data, the SKILL.md of a skill the source named source supplies, as its fork, the authored
// skill skills/<name>, needs it: with frontmatter that names the skill name, which a SKILL.md from a repository that
// isn't a library can lack; with the source's name added to each bare ID its trailguide-skills-used lists, and, when the
// source is a library, its trailguide-rules-used, since in the project a bare ID names the project's own skill or rule,
// and each ID of a skill the library curates, such as anthropic:skills/skill-creator, since the project names that
// skill with the library source's name first;
// and with uses, the IDs the source's skills.uses adds for the skill, added to trailguide-skills-used the same way, since
// skills.uses applies only to the source's own skills. metadata is data's, read at location. It rewrites only what
// differs, replacing a list it changes with a block string. It fails, at location, when it can't rewrite the
// frontmatter to say exactly that, such as a list inside a flow mapping, or when the result fails the checks build
// applies to an authored skill, such as a type other than playbook or review.
func forkedSkillFile(data []byte, metadata skills.Metadata, name, source string, library bool, uses []string, location string) ([]byte, error) {
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
	text := string(data)
	eol := "\n"
	if first, _, _ := strings.Cut(text, "\n"); strings.HasSuffix(first, "\r") {
		eol = "\r\n"
	}
	// A quoted name stays a string even when it reads as a number or a boolean, such as 2048.
	nameLine := "name: " + strconv.Quote(name) + eol
	switch {
	case !metadata.Frontmatter:
		text = "---" + eol + nameLine + "---" + eol + text
	case metadata.Name == "":
		opening, rest, _ := strings.Cut(text, "\n")
		text = opening + "\n" + nameLine + rest
	}
	if !slices.Equal(skillsUsed, metadata.SkillsUsed) {
		text = setMetadataList(text, skills.SkillsUsedKey, skillsUsed, eol)
	}
	if !slices.Equal(rulesUsed, metadata.RulesUsed) {
		text = setMetadataList(text, skills.RulesUsedKey, rulesUsed, eol)
	}
	forked, err := skills.ReadMetadata([]byte(text), location)
	if err != nil || forked.Name != name || !slices.Equal(forked.SkillsUsed, skillsUsed) || !slices.Equal(forked.RulesUsed, rulesUsed) || forked.Description != metadata.Description || forked.Type != metadata.Type || !slices.Equal(forked.ReservedKeys, metadata.ReservedKeys) {
		return nil, &ValidationError{Location: location, Problem: fmt.Sprintf("Trail Guide can't rewrite this frontmatter for the fork, which must name the skill %s and cite the source's skills and rules with the source's name, such as %s:skills/tdd, because it isn't written as block mappings Trail Guide can edit. %s", name, source, forkByHand(name))}
	}
	if problems := skills.Check(name, forked, true); len(problems) > 0 {
		return nil, &ValidationError{Location: location, Problem: problems[0] + ", and build refuses a skill you author, as a fork is, with this problem. " + forkByHand(name)}
	}
	return []byte(text), nil
}

// forkByHand tells how to fork the skill named name when the command can't.
func forkByHand(name string) string {
	return "To fork the skill, copy it into .trailguide/skills/" + name + "/, correct its frontmatter, and add the exclusion to config.yaml by hand"
}

// setMetadataList returns text, a SKILL.md whose frontmatter starts at its first line, with key of the top-level
// metadata block mapping set to ids, one per line, as a literal block string, adding the key, and the mapping, when
// missing. eol ends each line it writes. It returns text unchanged when the frontmatter has no closing line or
// metadata isn't a block mapping, which the caller detects by reading the result.
func setMetadataList(text, key string, ids []string, eol string) string {
	lines := strings.SplitAfter(text, "\n")
	content := func(i int) string { return strings.TrimSpace(lines[i]) }
	indent := func(i int) int { return len(lines[i]) - len(strings.TrimLeft(lines[i], " ")) }
	closing := slices.IndexFunc(lines[1:], func(line string) bool { return strings.TrimRight(line, "\r\n") == "---" }) + 1
	if closing == 0 {
		return text
	}
	block := func(at int) []string {
		result := []string{strings.Repeat(" ", at) + key + ": |" + eol}
		for _, id := range ids {
			result = append(result, strings.Repeat(" ", at+2)+id+eol)
		}
		return result
	}
	mapping := slices.IndexFunc(lines[1:closing], func(line string) bool { return strings.HasPrefix(line, "metadata:") }) + 1
	if mapping == 0 {
		return strings.Join(slices.Insert(lines, closing, append([]string{"metadata:" + eol}, block(2)...)...), "")
	}
	if value := strings.TrimSpace(strings.TrimPrefix(content(mapping), "metadata:")); value != "" && !strings.HasPrefix(value, "#") {
		return text
	}
	// last is the mapping's last line, and child the indentation of its keys, or -1 before its first key.
	last, child := mapping, -1
	for i := mapping + 1; i < closing && (content(i) == "" || indent(i) > 0); i++ {
		if content(i) == "" {
			continue
		}
		if child < 0 && !strings.HasPrefix(content(i), "#") {
			child = indent(i)
		}
		last = i
	}
	if child < 0 {
		child = 2
	}
	for i := mapping + 1; i <= last; i++ {
		if indent(i) != child || !strings.HasPrefix(content(i), key+":") {
			continue
		}
		// The value runs to the last line indented more than the key, which covers every scalar style.
		end := i
		for j := i + 1; j <= last && (content(j) == "" || indent(j) > child); j++ {
			if content(j) != "" {
				end = j
			}
		}
		return strings.Join(slices.Replace(lines, i, end+1, block(child)...), "")
	}
	return strings.Join(slices.Insert(lines, last+1, block(child)...), "")
}
