// Read and write the project's lockfile, .trailguide/lock.json, and migrate a project from per-source records.

package project

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/fabricahq/trailguide/internal/filetxn"
	"github.com/fabricahq/trailguide/internal/librarytree"
	"github.com/fabricahq/trailguide/internal/rules"
	"github.com/fabricahq/trailguide/trailguide"
)

// lockFile is the lockfile's path in the Trail Guide directory.
const lockFile = trailguide.LockFile

// legacyRecord is the name of the per-source record in each vendor folder that the lockfile replaced.
const legacyRecord = "_source.json"

// entryLocation locates the lock entry of the source name in diagnostics.
func entryLocation(name string) string { return lockFile + ".sources." + name }

// retakeAdvice is the next step for a source whose lock entry or record can't be used: resolving it again at its
// ref, as if it were new.
func retakeAdvice(name string) string {
	return "run trailguide project sync --retake " + name + " to resolve the source again at its ref, which imports its unpinned rules at their newest versions"
}

// sourceRecords is how a project records what its sources resolved to. records holds a parsed record for every
// lock entry, or, when legacy is set, for every per-source record in vendor/ that sync is migrating from. refused
// holds, by name, the refusal of each lock entry of a source configuration no longer has that was changed outside
// Trail Guide, which nothing may use but sync may remove.
type sourceRecords struct {
	records map[string]parsedRecord
	refused map[string]error
	legacy  bool
}

// readRecords returns the project's records from its lockfile, data, which is nil when the project has none, and
// vendor, the vendor tree's files. Projects that sync last wrote with Trail Guide 0.5 keep a record in each source's
// vendor folder instead; only sync, with migrate set, reads them, and it fails, without writing, for every state
// a finished migration can't start from: a configured source whose vendor folder has files but no record, a record
// changed outside sync or whose files don't match it, or a vendor folder of a source configuration no longer has.
// The source names in retake are left out, so sync resolves them again. A project with no lockfile whose vendor/
// has files, but no records, lost its lockfile; it fails rather than resolving sources again, which could move
// them. A lockfile together with any legacy record is a mixed layout, which fails.
func readRecords(config rules.Configuration, data []byte, vendor map[string][]byte, migrate bool, retake []string) (sourceRecords, error) {
	legacy := []string{}
	for _, file := range slices.Sorted(maps.Keys(vendor)) {
		if name, ok := strings.CutSuffix(file, "/"+legacyRecord); ok && !strings.Contains(name, "/") {
			legacy = append(legacy, name)
		}
	}
	if data != nil && len(legacy) > 0 {
		return sourceRecords{}, failure("mixed-layout", fmt.Sprintf(".trailguide/%s and per-source records from Trail Guide 0.5, such as .trailguide/vendor/%s/%s, are both present. Sync never writes both, so a merge combined a migrated branch with one that wasn't. Take one branch's whole .trailguide/vendor/ folder and %s, then run trailguide project sync", lockFile, legacy[0], legacyRecord, lockFile), nil)
	}
	if data != nil {
		return lockRecords(config, data, retake)
	}
	if len(legacy) > 0 {
		if !migrate {
			return sourceRecords{}, failure("lock-migration-needed", "the project was last synced by Trail Guide 0.5, which kept a record in each source's vendor folder instead of .trailguide/"+lockFile+". Run trailguide project sync once to migrate it, which moves no source, and commit the result", nil)
		}
		return legacyRecords(config, vendor, retake)
	}
	result := sourceRecords{records: map[string]parsedRecord{}}
	if len(vendor) == 0 {
		return result, nil
	}
	for _, source := range config.Sources {
		if !slices.Contains(retake, source.Name) {
			return sourceRecords{}, failure("lock-missing", ".trailguide/"+lockFile+" is missing, though .trailguide/vendor/ has files a sync wrote. Restore it, such as with git checkout -- .trailguide/"+lockFile+", or, for a project Trail Guide 0.5 last synced, restore each source's .trailguide/vendor/<source>/"+legacyRecord+", then run trailguide project sync. Sync never resolves a source again on its own, which could move it; to resolve every source again at its ref, run trailguide project sync with --retake for each source, such as trailguide project sync --retake "+config.Sources[0].Name, nil)
		}
	}
	return result, nil
}

// readStored reads the vendor tree and lockfile under root and returns the records of a project whose configuration
// is config, as projectState.records does, with the vendor tree and the lockfile's bytes.
func readStored(ctx context.Context, root *os.Root, config rules.Configuration) (sourceRecords, *filetxn.Tree, []byte, error) {
	vendor, err := filetxn.ReadTree(ctx, root, "vendor")
	if err != nil {
		return sourceRecords{}, nil, nil, err
	}
	lock, err := filetxn.ReadOptional(ctx, root, lockFile)
	if err != nil {
		return sourceRecords{}, nil, nil, err
	}
	stored, err := readRecords(config, lock, treeFiles(vendor), false, nil)
	return stored, vendor, lock, err
}

// lockRecords parses the lockfile data and checks each entry as the record it converts to, leaving out the sources
// in retake, whose entries may be invalid. It fails for an invalid entry of a configured source, and keeps the
// refusal of an invalid entry of a source configuration no longer has in refused.
func lockRecords(config rules.Configuration, data []byte, retake []string) (sourceRecords, error) {
	lock, refused, err := trailguide.ParseLockEntries(data)
	var conflict *trailguide.LockConflictError
	var newer *trailguide.UnsupportedLockError
	switch {
	case errors.As(err, &conflict):
		return sourceRecords{}, failure("lock-conflict", conflict.Error(), nil)
	case errors.As(err, &newer):
		return sourceRecords{}, failure("unsupported-lock", newer.Error(), nil)
	case err != nil:
		return sourceRecords{}, lockError(err, "restore .trailguide/"+lockFile+" from Git, such as with git checkout -- .trailguide/"+lockFile+", and run trailguide project sync")
	}
	configured := func(name string) bool {
		return slices.ContainsFunc(config.Sources, func(source rules.Source) bool { return source.Name == name })
	}
	result := sourceRecords{records: map[string]parsedRecord{}, refused: map[string]error{}}
	names := slices.Sorted(maps.Keys(lock.Sources))
	for name := range refused {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		if slices.Contains(retake, name) {
			continue
		}
		err := refused[name]
		if err == nil {
			var record parsedRecord
			if record, err = lockedRecord(name, lock.Sources[name]); err == nil {
				result.records[name] = record
				continue
			}
		}
		err = lockError(err, retakeAdvice(name))
		if configured(name) {
			return sourceRecords{}, err
		}
		result.refused[name] = err
	}
	return result, nil
}

// lockError returns err, a lockfile's validation error, as this package reports it, adding next, the next step: as
// an alternative when the problem already says to restore the entry, or as the only one when it names none.
func lockError(err error, next string) error {
	var public *trailguide.ValidationError
	if errors.As(err, &public) {
		err = invalidSnapshot(public.Location, public.Problem)
	}
	var invalid *ValidationError
	switch {
	case !errors.As(err, &invalid):
		return err
	case strings.Contains(invalid.Problem, "restore the entry"):
		return invalidSnapshot(invalid.Location, invalid.Problem+"; or "+next)
	case !strings.Contains(invalid.Problem, "trailguide project sync"):
		return invalidSnapshot(invalid.Location, invalid.Problem+"; "+next)
	}
	return err
}

// legacyRecords reads and verifies the per-source records of a project sync is migrating, as readRecords documents.
func legacyRecords(config rules.Configuration, vendor map[string][]byte, retake []string) (sourceRecords, error) {
	result := sourceRecords{records: map[string]parsedRecord{}, legacy: true}
	configured := map[string]bool{}
	for _, source := range config.Sources {
		configured[source.Name] = true
	}
	folders := map[string]bool{}
	for file := range vendor {
		folder, _, _ := strings.Cut(file, "/")
		folders[folder] = true
	}
	for _, folder := range slices.Sorted(maps.Keys(folders)) {
		if !configured[folder] {
			return sourceRecords{}, failure("lock-migration-blocked", fmt.Sprintf(".trailguide/vendor/%s/ belongs to no configured source, so the project can't migrate to .trailguide/%s. Add the source back to .trailguide/config.yaml, or delete the folder, then run trailguide project sync", folder, lockFile), nil)
		}
	}
	for _, source := range config.Sources {
		name := source.Name
		if slices.Contains(retake, name) || !folders[name] {
			continue
		}
		data, ok := vendor[name+"/"+legacyRecord]
		if !ok {
			return sourceRecords{}, failure("record-missing", fmt.Sprintf(".trailguide/vendor/%s/ has files but no %s, so the project can't migrate to .trailguide/%s without resolving the source again, which could move it. Restore the record, such as with git checkout -- .trailguide/vendor/%s/%s, and run trailguide project sync; or %s", name, legacyRecord, lockFile, name, legacyRecord, retakeAdvice(name)), nil)
		}
		record, err := parseSourceRecord(data, name)
		if err != nil {
			return sourceRecords{}, err
		}
		for _, file := range slices.Sorted(maps.Keys(record.digests)) {
			stored := name + "/" + librarytree.SnapshotPath(file)
			if contents, ok := vendor[stored]; !ok || digest(contents) != record.digests[file] {
				return sourceRecords{}, invalidSnapshot("vendor/"+stored, "missing or changed since sync imported it, so the project can't migrate to .trailguide/"+lockFile+" from this record. Restore the file, such as with git checkout -- .trailguide/vendor/"+stored+", and run trailguide project sync; or "+retakeAdvice(name))
			}
		}
		result.records[name] = record
	}
	return result, nil
}

// encodeLock returns the lockfile for records, the records of config's sources by name, each recorded with the ref
// configuration names for it, spelled as in its previous record when that names the same ref, so respelling a ref
// changes nothing.
func encodeLock(config rules.Configuration, records map[string]sourceRecord, previous sourceRecords) ([]byte, error) {
	lock := trailguide.Lock{Sources: map[string]trailguide.LockedSource{}}
	for _, source := range config.Sources {
		ref := source.Ref
		if recorded := previous.records[source.Name].lockedRef; recorded.Equal(ref) && !recorded.IsZero() {
			ref = recorded
		}
		entry, err := lockedSource(records[source.Name], ref.String(), entryLocation(source.Name))
		if err != nil {
			return nil, err
		}
		lock.Sources[source.Name] = entry
	}
	data, err := lock.Encode()
	if err != nil {
		var invalid *trailguide.ValidationError
		if errors.As(err, &invalid) {
			return nil, invalidSnapshot(invalid.Location, invalid.Problem)
		}
		return nil, err
	}
	return data, nil
}

// lockedSource converts record, a record encodeSnapshots built, to its lock entry, recording ref. Each rule's commit
// moves to the entry's releaseCommits, so it fails when rules from one library release record different commits,
// or an unreleased rule's commit isn't the record's.
func lockedSource(record sourceRecord, ref, where string) (trailguide.LockedSource, error) {
	entry := trailguide.LockedSource{Kind: record.Kind, Repository: record.Repository, Ref: ref, Release: record.Release, Commit: record.Commit, Selectors: trailguide.LockSelectors{Skills: record.SkillInclude}, Skills: lockedSkills(record.Skills), AvailableSkills: map[string]trailguide.LockedAvailableSkill{}, Files: record.Files, Executable: record.Executable}
	for id, skill := range record.AvailableSkills {
		entry.AvailableSkills[id] = trailguide.LockedAvailableSkill{Level: skill.Level}
	}
	for name, curated := range record.Curated {
		if entry.Curated == nil {
			entry.Curated = map[string]trailguide.LockedCurated{}
		}
		entry.Curated[name] = trailguide.LockedCurated{Kind: curated.Kind, Repository: curated.Repository, Ref: curated.Ref, Release: curated.Release, Commit: curated.Commit, Source: curated.Source, Skills: lockedSkills(curated.Skills), Files: curated.Files, Executable: curated.Executable}
	}
	if record.Kind == nonLibraryKind {
		return entry, nil
	}
	entry.Selectors.Groups = record.Selection
	entry.Selectors.Rules = nonNil(record.RuleSelection)
	entry.Groups, entry.RetiredRules = record.Groups, record.RetiredRules
	entry.Rules = map[string]trailguide.LockedRule{}
	entry.ReleaseCommits = map[int]string{}
	if record.Release != 0 {
		entry.ReleaseCommits[record.Release] = record.Commit
	}
	for _, id := range slices.Sorted(maps.Keys(record.Rules)) {
		rule := record.Rules[id]
		entry.Rules[id] = trailguide.LockedRule{Version: rule.Version, Release: rule.Release, LegacyLayout: rule.LegacyLayout}
		if rule.Release == nil {
			if rule.Commit != record.Commit {
				return trailguide.LockedSource{}, invalidSnapshot(where+".rules."+id, "the rule's commit differs from the source's commit, though no library release published it")
			}
			continue
		}
		if commit, seen := entry.ReleaseCommits[*rule.Release]; seen && commit != rule.Commit {
			return trailguide.LockedSource{}, invalidSnapshot(where+".rules."+id, fmt.Sprintf("rules from library release %d record different commits", *rule.Release))
		}
		entry.ReleaseCommits[*rule.Release] = rule.Commit
	}
	return entry, nil
}

// lockedSkills converts installed skills' records to lock entries.
func lockedSkills(records map[string]recordSkill) map[string]trailguide.LockedSkill {
	result := map[string]trailguide.LockedSkill{}
	for id, skill := range records {
		result[id] = trailguide.LockedSkill{Level: skill.Level, Included: skill.Included, UsedBy: skill.UsedBy, UsedByLibrary: skill.UsedByLibrary}
	}
	return result
}

// lockedRecord converts the lock entry of the source name to the record it holds and checks it as checkRecord does.
// The parsed record's Ref is the entry's ref, except for a library on a library release whose ref follows releases,
// which a snapshot records without one, as sync imports it; lockedRef always holds the entry's ref.
func lockedRecord(name string, entry trailguide.LockedSource) (parsedRecord, error) {
	where := entryLocation(name)
	ref, err := rules.ParseGitRef(entry.Ref, where+".ref")
	if err != nil {
		return parsedRecord{}, err
	}
	record := entrySourceRecord(entry)
	if entry.Kind == trailguide.KindLibrary && entry.Release != 0 && ref.FollowsReleases() {
		record.Ref = ""
	}
	parsed, err := checkRecord(record, name, where)
	if err != nil {
		return parsedRecord{}, err
	}
	parsed.lockedRef = ref
	return parsed, nil
}

// entrySourceRecord returns the record a lock entry holds, unchecked. Its rules take their commits from the entry's
// releaseCommits, or the entry's commit when no library release published them.
func entrySourceRecord(entry trailguide.LockedSource) sourceRecord {
	record := sourceRecord{Kind: entry.Kind, Repository: entry.Repository, Ref: entry.Ref, Release: entry.Release, Commit: entry.Commit, Selection: entry.Selectors.Groups, RuleSelection: entry.Selectors.Rules, Groups: entry.Groups, RetiredRules: entry.RetiredRules, SkillInclude: entry.Selectors.Skills, Skills: recordSkills(entry.Skills), AvailableSkills: map[string]availableSkill{}, Files: maps.Clone(entry.Files), Executable: entry.Executable, Rules: map[string]recordRule{}}
	if entry.Kind == trailguide.KindNonLibrary {
		record.Selection = jsonText([]string{})
	}
	for id, skill := range entry.AvailableSkills {
		record.AvailableSkills[id] = availableSkill{Level: skill.Level}
	}
	for name, curated := range entry.Curated {
		if record.Curated == nil {
			record.Curated = map[string]recordCurated{}
		}
		record.Curated[name] = recordCurated{Kind: curated.Kind, Repository: curated.Repository, Ref: curated.Ref, Release: curated.Release, Commit: curated.Commit, Source: curated.Source, Skills: recordSkills(curated.Skills), Files: maps.Clone(curated.Files), Executable: curated.Executable}
	}
	for id, rule := range entry.Rules {
		commit := entry.Commit
		if rule.Release != nil {
			commit = entry.ReleaseCommits[*rule.Release]
		}
		record.Rules[id] = recordRule{Version: rule.Version, Release: rule.Release, Commit: commit, LegacyLayout: rule.LegacyLayout}
	}
	return record
}

// recordSkills converts installed skills' lock entries to records, keeping usedByLibrary absent when empty.
func recordSkills(entries map[string]trailguide.LockedSkill) map[string]recordSkill {
	result := map[string]recordSkill{}
	for id, skill := range entries {
		var usedByLibrary []string
		if len(skill.UsedByLibrary) > 0 {
			usedByLibrary = skill.UsedByLibrary
		}
		result[id] = recordSkill{Level: skill.Level, Included: skill.Included, UsedBy: skill.UsedBy, UsedByLibrary: usedByLibrary}
	}
	return result
}
