// Encode, read, and verify persisted source snapshots, the lockfile that sync restores and offline builds verify.

package project

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/fabricahq/trailguide/internal/errs"
	"github.com/fabricahq/trailguide/internal/filetxn"
	"github.com/fabricahq/trailguide/internal/library"
	"github.com/fabricahq/trailguide/internal/librarytree"
	"github.com/fabricahq/trailguide/internal/rules"
	"github.com/fabricahq/trailguide/internal/skills"
	"github.com/fabricahq/trailguide/trailguide"
)

// snapshot is the library-owned value persisted by project snapshot encoding.
type snapshot = library.Snapshot

// sourceRecordFormat is the only supported _source.json format.
const sourceRecordFormat = 3

// The kinds of source a record names: a library, whose repository has a library manifest at the recorded commit, or
// another repository, which supplies only skills.
const (
	libraryKind    = "library"
	nonLibraryKind = "non-library"
)

// sourceRecord is the format 3 record in vendor/<source>/_source.json: the source's kind and configuration when it
// was recorded, the revision that supplied its library-wide files and every skill, each imported rule's version, each
// installed skill's level, and file digests and modes. A record of a source that isn't a library has no release, and
// empty group and rule fields.
type sourceRecord struct {
	FormatVersion int `json:"formatVersion"`
	// Checksum is the SHA-256 of the record's other fields, so offline checks can tell a record changed outside
	// sync, such as by hand or in a merge, from one sync wrote.
	Checksum      string                 `json:"checksum,omitempty"`
	Kind          string                 `json:"kind"`
	Repository    string                 `json:"repository"`
	Ref           string                 `json:"ref,omitempty"`
	Release       int                    `json:"release,omitempty"`
	Commit        string                 `json:"resolvedCommit"`
	Selection     json.RawMessage        `json:"groupSelection"`
	RuleSelection []string               `json:"ruleSelection,omitempty"`
	Groups        []string               `json:"groups"`
	Rules         map[string]recordRule  `json:"rules"`
	RetiredRules  []string               `json:"retiredRules"`
	SkillInclude  []string               `json:"skillInclude"`
	Skills        map[string]recordSkill `json:"skills"`
	// AvailableSkills holds every skill the source offers at the commit, installed or not.
	AvailableSkills map[string]availableSkill `json:"availableSkills"`
	Files           map[string]string         `json:"files"`
	Executable      []string                  `json:"executable"`
	// Curated records, by name, the revision of each source the library curates skills from; it is omitted when
	// the source curates none.
	Curated map[string]recordCurated `json:"curated,omitempty"`
}

// recordCurated is the revision a project resolved for one source a library curates skills from, and what the
// project installs from it. Kind is that repository's kind at the commit, and Source names the project's own source
// that supplied the revision, when one declares the same repository. Skills, Files, and Executable record the
// curated skills the project installs and their files as the source record's own fields do, by their IDs and paths
// in the curated repository; the files are stored in the curated source's folder of the library's snapshot.
type recordCurated struct {
	Kind       string                 `json:"kind"`
	Repository string                 `json:"repository"`
	Ref        string                 `json:"ref,omitempty"`
	Release    int                    `json:"release,omitempty"`
	Commit     string                 `json:"resolvedCommit"`
	Source     string                 `json:"source,omitempty"`
	Skills     map[string]recordSkill `json:"skills"`
	Files      map[string]string      `json:"files"`
	Executable []string               `json:"executable"`
}

// recordSkill is one installed skill's record. Level is null for a skill from a source that isn't a library.
// Included reports that skills.include, or the library's required level, selects the skill, and UsedBy lists, sorted, the source's installed skills
// that use it; a skill that isn't included has at least one. A curated skill's UsedBy names skills of its curated
// source by their IDs there, and UsedByLibrary lists, sorted, the library's own skills that use it, by their IDs in
// the library, which only a curated skill records.
type recordSkill struct {
	Level         *string  `json:"level"`
	Included      bool     `json:"included"`
	UsedBy        []string `json:"usedBy"`
	UsedByLibrary []string `json:"usedByLibrary,omitempty"`
}

// availableSkill is the record of a skill the source offers at the commit, installed or not. Level is null for a
// skill from a source that isn't a library.
type availableSkill struct {
	Level *string `json:"level"`
}

// nullableLevel returns level, or nil for the empty level of a skill from a source that isn't a library.
func nullableLevel(level string) *string {
	if level == "" {
		return nil
	}
	return &level
}

// recordRule is one imported rule's version record; Version and Release are null for an unreleased rule.
type recordRule struct {
	Version *trailguide.RuleVersion `json:"version"`
	Release *int                    `json:"release"`
	Commit  string                  `json:"commit"`
	// LegacyLayout is library.ImportedRule's.
	LegacyLayout bool `json:"legacyLayout,omitempty"`
}

// unsupportedRecord explains how to replace a source record this version can't read.
const unsupportedRecord = "unsupported source record; delete .trailguide/vendor/ and run trailguide project sync to import it again, which imports unpinned rules at their newest versions"

// reimport is the next step for a source record sync can't read: resolving the source again, as if it were new.
func reimport(name string) string { return retakeAdvice(name) }

// changedOutsideSync refuses the source record of the source name, whose checksum shows it was changed outside sync,
// since nothing in it can be trusted, and says how to recover: restore a record sync wrote, or resolve the source
// again.
func changedOutsideSync(name string) error {
	record := ".trailguide/vendor/" + name + "/_source.json"
	return invalidSnapshot("vendor/"+name+"/_source.json", "was changed outside trailguide project sync, such as by hand or in a merge, and only a record sync wrote can be trusted; restore one with git checkout -- "+record+", then run trailguide project sync, which migrates the project to .trailguide/lock.json; or "+retakeAdvice(name))
}

// recordChecksum returns the SHA-256 of record's fields other than its checksum, encoded as sync writes them.
func recordChecksum(record sourceRecord) string {
	record.Checksum = ""
	encoded, err := json.Marshal(record)
	if err != nil {
		return ""
	}
	return digest(encoded)
}

// encodeSnapshots prepares complete vendor bytes and the lockfile for parsed configuration, without writing files.
// Each source's lock entry lists its files by library path, and each file is stored where librarytree.SnapshotPath
// places it, so the snapshot mirrors the library's Trail Guide directory. Each snapshot must satisfy the same checks
// offline builds apply. A source's entry keeps the spelling of its ref in previous, the records the command read,
// when configuration names the same ref another way. Inputs stay unchanged; returned maps and byte slices belong to
// the caller. Errors return nil.
func encodeSnapshots(config rules.Configuration, snapshots map[string]snapshot, previous sourceRecords) (map[string][]byte, []byte, error) {
	output := map[string][]byte{}
	records := map[string]sourceRecord{}
	if len(snapshots) != len(config.Sources) {
		return nil, nil, invalidSnapshot(librarytree.Vendor, "source inventory differs from configuration; run trailguide project sync")
	}
	for _, source := range config.Sources {
		snapshot, ok := snapshots[source.Name]
		if !ok {
			return nil, nil, invalidSnapshot(librarytree.Vendor+"/"+source.Name, "missing source snapshot; run trailguide project sync")
		}
		selection, err := json.Marshal(snapshot.Selection)
		if err != nil {
			return nil, nil, fmt.Errorf("encode selection for %s: %v", source.Name, err)
		}
		kind := libraryKind
		if snapshot.NonLibrary {
			kind = nonLibraryKind
		}
		record := sourceRecord{FormatVersion: sourceRecordFormat, Kind: kind, Repository: snapshot.Repository, Ref: snapshot.Ref.String(), Release: snapshot.Release, Commit: snapshot.Commit, Selection: selection, RuleSelection: snapshot.RuleSelection, Groups: nonNil(snapshot.Groups), Rules: map[string]recordRule{}, Files: map[string]string{}}
		record.RetiredRules = nonNil(snapshot.RetiredRules)
		record.SkillInclude = nonNil(snapshot.SkillSelection)
		record.Executable = []string{}
		record.Skills, record.AvailableSkills = map[string]recordSkill{}, map[string]availableSkill{}
		for name, curated := range snapshot.Curated {
			if record.Curated == nil {
				record.Curated = map[string]recordCurated{}
			}
			kind := libraryKind
			if curated.NonLibrary {
				kind = nonLibraryKind
			}
			record.Curated[name] = recordCurated{Kind: kind, Repository: curated.Repository, Ref: curated.Ref.String(), Release: curated.Release, Commit: curated.Commit, Source: curated.Source, Skills: map[string]recordSkill{}, Files: map[string]string{}, Executable: []string{}}
		}
		for id, skill := range snapshot.Skills {
			entry := recordSkill{Level: nullableLevel(skill.Level), Included: skill.Included, UsedBy: nonNil(skill.UsedBy)}
			name, folder, curated := library.SplitCurated(id)
			if !curated {
				record.Skills[id] = entry
				continue
			}
			origin, ok := record.Curated[name]
			if !ok {
				return nil, nil, invalidSnapshot(librarytree.Vendor+"/"+source.Name, "the curated skill "+id+" has no recorded curated source")
			}
			entry.UsedBy = []string{}
			for _, user := range skill.UsedBy {
				if inside, found := strings.CutPrefix(user, name+":"); found {
					entry.UsedBy = append(entry.UsedBy, inside)
				} else {
					entry.UsedByLibrary = append(entry.UsedByLibrary, user)
				}
			}
			origin.Skills[folder] = entry
		}
		for id, skill := range snapshot.AvailableSkills {
			record.AvailableSkills[id] = availableSkill{Level: nullableLevel(skill.Level)}
		}
		for _, file := range snapshot.Executable {
			if name, relative, curated := curatedFile(file); curated && record.Curated[name].Files != nil {
				origin := record.Curated[name]
				origin.Executable = append(origin.Executable, relative)
				record.Curated[name] = origin
			} else {
				record.Executable = append(record.Executable, file)
			}
		}
		for id, rule := range snapshot.Rules {
			entry := recordRule{Version: rule.Version, Commit: rule.Commit, LegacyLayout: rule.LegacyLayout}
			if rule.Version != nil {
				entry.Release = &rule.Release
			}
			record.Rules[id] = entry
		}
		if err := rules.ValidatePaths(snapshot.Files, nil); err != nil {
			return nil, nil, err
		}
		placed := map[string]string{}
		for _, file := range slices.Sorted(maps.Keys(snapshot.Files)) {
			if first, _, _ := strings.Cut(file, "/"); strings.EqualFold(first, legacyRecord) {
				return nil, nil, invalidSnapshot(librarytree.Vendor+"/"+source.Name, "_source.json is reserved for the source record of Trail Guide 0.5")
			}
			stored := source.Name + "/" + librarytree.SnapshotPath(file)
			if other, taken := placed[stored]; taken {
				return nil, nil, invalidSnapshot(librarytree.Vendor+"/"+stored, "the library files "+other+" and "+file+" would both be stored here")
			}
			placed[stored] = file
			if name, relative, curated := curatedFile(file); curated && record.Curated[name].Files != nil {
				record.Curated[name].Files[relative] = digest(snapshot.Files[file])
			} else {
				record.Files[file] = digest(snapshot.Files[file])
			}
			output[stored] = bytes.Clone(snapshot.Files[file])
		}
		records[source.Name] = record
	}
	if err := rules.ValidatePaths(output, nil); err != nil {
		return nil, nil, err
	}
	lock, err := encodeLock(config, records, previous)
	if err != nil {
		return nil, nil, err
	}
	// Validate the same persisted representation the reader will consume.
	stored, err := lockRecords(config, lock, nil)
	if err != nil {
		return nil, nil, err
	}
	for _, source := range config.Sources {
		parsed := stored.records[source.Name]
		// A snapshot of a library release may also hold the ref that follows releases, as tests and older records do.
		if ref := snapshots[source.Name].Ref; !parsed.Ref.Equal(ref) && !(parsed.Ref.IsZero() && ref.Equal(source.Ref)) {
			return nil, nil, invalidSnapshot(entryLocation(source.Name)+".ref", "the source's snapshot records a ref the lock entry doesn't keep")
		}
		if err := matchSnapshotSource(source, parsed); err != nil {
			return nil, nil, err
		}
	}
	return output, lock, nil
}

// curatedFile returns the curated source's name and the path in its repository of file, a path in a library
// source's snapshot, when file is in the folder of the skills it curates from that source, and reports false
// otherwise.
func curatedFile(file string) (name, relative string, curated bool) {
	rest, ok := strings.CutPrefix(file, library.CuratedDirectory+"/")
	if !ok {
		return "", "", false
	}
	return strings.Cut(rest, "/")
}

// isCuratedSkill reports whether id names a skill a library curates.
func isCuratedSkill(id string) bool {
	_, _, curated := library.SplitCurated(id)
	return curated
}

// nonNil returns list, or an empty list for nil, so a record encodes an empty list rather than null.
func nonNil(list []string) []string {
	if list == nil {
		return []string{}
	}
	return list
}

// vendorExecutable returns, sorted, the executable files of the snapshots, by path in vendor/.
func vendorExecutable(snapshots map[string]snapshot) []string {
	executable := []string{}
	for name, snapshot := range snapshots {
		for _, file := range snapshot.Executable {
			executable = append(executable, name+"/"+file)
		}
	}
	slices.Sort(executable)
	return executable
}

// verifyExecutable fails, asking for sync, when the executable files in vendor, the vendor tree, differ from those
// the decoded snapshots record: a file's mode is part of its content.
func verifyExecutable(snapshots map[string]snapshot, vendor *filetxn.Tree) error {
	actual := []string{}
	if vendor != nil {
		actual = nonNil(vendor.Executable)
	}
	if expected := vendorExecutable(snapshots); !slices.Equal(actual, expected) {
		for _, file := range slices.Concat(actual, expected) {
			if !slices.Contains(actual, file) || !slices.Contains(expected, file) {
				return invalidSnapshot(librarytree.Vendor+"/"+file, "its executable bit differs from what the source record lists; run trailguide project sync")
			}
		}
	}
	return nil
}

// decodeSnapshots verifies every original byte against the project's records, stored, and each record's agreement
// with configuration before returning owned snapshots. Unexpected source files, lock entries of sources
// configuration no longer has, and incomplete records fail closed. It performs no filesystem or Git access. Rule and
// manifest validation remains the catalog loader's responsibility after this integrity check.
func decodeSnapshots(config rules.Configuration, stored sourceRecords, vendor map[string][]byte) (map[string]snapshot, error) {
	if err := rules.ValidatePaths(vendor, nil); err != nil {
		return nil, err
	}
	result := map[string]snapshot{}
	expected := map[string]bool{}
	configured := map[string]bool{}
	for _, source := range config.Sources {
		configured[source.Name] = true
		record, ok := stored.records[source.Name]
		if !ok {
			return nil, invalidSnapshot(entryLocation(source.Name), "missing lock entry; run trailguide project sync")
		}
		if err := matchSnapshotSource(source, record); err != nil {
			return nil, err
		}
		record.Files = map[string][]byte{}
		for _, file := range slices.Sorted(maps.Keys(record.digests)) {
			full := source.Name + "/" + librarytree.SnapshotPath(file)
			data, ok := vendor[full]
			if !ok || digest(data) != record.digests[file] {
				return nil, invalidSnapshot(librarytree.Vendor+"/"+full, "missing or modified imported content; run trailguide project sync")
			}
			record.Files[file] = bytes.Clone(data)
			expected[full] = true
		}
		if err := matchSkillUses(source, record.Snapshot); err != nil {
			return nil, err
		}
		result[source.Name] = record.Snapshot
	}
	for _, name := range slices.Sorted(maps.Keys(stored.refused)) {
		return nil, stored.refused[name]
	}
	for _, name := range slices.Sorted(maps.Keys(stored.records)) {
		if !configured[name] {
			return nil, invalidSnapshot(entryLocation(name), "lock entry of a source configuration no longer has; run trailguide project sync")
		}
	}
	for _, file := range slices.Sorted(maps.Keys(vendor)) {
		if !expected[file] {
			return nil, invalidSnapshot(librarytree.Vendor+"/"+file, "unexpected imported file or removed source; run trailguide project sync")
		}
	}
	return result, nil
}

// recordedSnapshots returns the snapshot, without files, of each configured source that stored records, so sync
// can restore its versions. A source without a record is absent from the result.
func recordedSnapshots(config rules.Configuration, stored sourceRecords) map[string]library.Snapshot {
	result := map[string]library.Snapshot{}
	for _, source := range config.Sources {
		if record, ok := stored.records[source.Name]; ok {
			result[source.Name] = record.Snapshot
		}
	}
	return result
}

// parsedRecord is a validated source record: its snapshot, without files, and its file digests by stored path.
type parsedRecord struct {
	library.Snapshot
	digests map[string]string
	// lockedRef is the ref a lock entry records, or zero for a per-source record from Trail Guide 0.5.
	lockedRef rules.GitRef
}

// parseSourceRecord is the one way to read a source record from disk. It validates a format 3 record's exact fields,
// value syntax, and complete digest inventory, and refuses, with changedOutsideSync, a record whose checksum doesn't
// match its fields, or that has none, so no caller can use a record changed outside sync. Relationships to
// configuration are matchSnapshotSource's. A record it can't read fails validation with advice to import the source
// again, and so does an older format, such as 2; a newer one fails with code unsupported-source-record, asking to
// upgrade Trail Guide.
func parseSourceRecord(data []byte, name string) (parsedRecord, error) {
	record, err := readSourceRecord(data, name)
	var invalid errs.ValidationError
	if errors.As(err, &invalid) && err == error(invalid) && !strings.Contains(invalid.ValidationProblem(), "trailguide project sync") {
		return parsedRecord{}, invalidSnapshot(invalid.ValidationLocation(), invalid.ValidationProblem()+"; "+reimport(name))
	}
	return record, err
}

// readSourceRecord is parseSourceRecord without the next step its validation errors share.
func readSourceRecord(data []byte, name string) (parsedRecord, error) {
	where := "vendor/" + name + "/_source.json"
	if err := rules.RequireResolvedMerge(data, where, "take one side, which sync wrote, such as with git checkout --ours or --theirs -- .trailguide/vendor/"+name+"/_source.json, and run trailguide project sync"); err != nil {
		return parsedRecord{}, err
	}
	var fields map[string]json.RawMessage
	if !utf8.Valid(data) || json.Unmarshal(data, &fields) != nil || fields == nil {
		return parsedRecord{}, invalidSnapshot(where, "expected a UTF-8 source record object")
	}
	var format int
	if json.Unmarshal(fields["formatVersion"], &format) == nil && format > sourceRecordFormat {
		// Deleting vendor/ and syncing would rewrite the project in an older format, so this isn't the advice.
		return parsedRecord{}, failure("unsupported-source-record", fmt.Sprintf("%s was written by a newer version of Trail Guide, in source record format %d, but this version reads only format %d; upgrade Trail Guide to use this project", where, format, sourceRecordFormat), nil)
	}
	if format != sourceRecordFormat {
		return parsedRecord{}, invalidSnapshot(where, unsupportedRecord)
	}
	allowed := []string{"formatVersion", "checksum", "kind", "repository", "ref", "release", "resolvedCommit", "groupSelection", "ruleSelection", "groups", "rules", "retiredRules", "skillInclude", "skills", "availableSkills", "files", "executable", "curated"}
	for _, key := range slices.Sorted(maps.Keys(fields)) {
		if !slices.Contains(allowed, key) || bytes.Equal(bytes.TrimSpace(fields[key]), []byte("null")) {
			return parsedRecord{}, invalidSnapshot(where+"."+key, "unknown or null source record field")
		}
	}
	for _, key := range []string{"kind", "repository", "resolvedCommit", "groupSelection", "groups", "rules", "retiredRules", "skillInclude", "skills", "availableSkills", "files", "executable"} {
		if _, ok := fields[key]; !ok {
			return parsedRecord{}, invalidSnapshot(where+"."+key, "missing source record field")
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var record sourceRecord
	if decoder.Decode(&record) != nil {
		return parsedRecord{}, invalidSnapshot(where, "invalid source record field values")
	}
	if record.Checksum != recordChecksum(record) {
		return parsedRecord{}, changedOutsideSync(name)
	}
	if raw, ok := fields["release"]; ok && record.Release < 1 {
		return parsedRecord{}, invalidSnapshot(where+".release", "expected a library release number, got "+string(raw))
	}
	return checkRecord(record, name, where)
}

// checkRecord validates a record's values and their relationships, other than to configuration, and returns it
// parsed, its curated sources folded into its own skills and files. name is the record's source, and where locates the
// record in diagnostics. A
// lockfile entry is checked as the record it converts to, so both layouts share one set of rules.
func checkRecord(record sourceRecord, name, where string) (parsedRecord, error) {
	result := parsedRecord{Snapshot: library.Snapshot{Repository: record.Repository, Release: record.Release, Commit: record.Commit, RuleSelection: []string{}, Rules: map[string]library.ImportedRule{}, NonLibrary: record.Kind == nonLibraryKind, Skills: map[string]library.ImportedSkill{}, AvailableSkills: map[string]library.ImportedSkill{}}, digests: record.Files}
	if record.Kind != libraryKind && record.Kind != nonLibraryKind {
		return parsedRecord{}, invalidSnapshot(where+".kind", "expected library or non-library")
	}
	if record.Files == nil {
		return parsedRecord{}, invalidSnapshot(where+".files", "missing file digests")
	}
	if err := foldCurated(&record, where); err != nil {
		return parsedRecord{}, err
	}
	if _, err := rules.ParseRepository(jsonText(record.Repository), where+".repository"); err != nil {
		return parsedRecord{}, err
	}
	if record.Ref != "" {
		var err error
		if result.Ref, err = rules.ParseGitRef(record.Ref, where+".ref"); err != nil {
			return parsedRecord{}, err
		}
	}
	if record.Release < 0 {
		return parsedRecord{}, invalidSnapshot(where+".release", "expected a library release number")
	}
	if !fullCommit(record.Commit) {
		return parsedRecord{}, invalidSnapshot(where+".commit", "expected a full commit SHA")
	}
	var err error
	if result.Selection, err = rules.ParseGroupSelection(record.Selection, where+".groupSelection"); err != nil {
		return parsedRecord{}, err
	}
	if result.RetiredRules, err = rules.ParseRuleList(jsonText(nonNil(record.RetiredRules)), where+".retiredRules"); err != nil {
		return parsedRecord{}, err
	}
	if record.RuleSelection != nil {
		if result.RuleSelection, err = rules.ParseRuleList(jsonText(record.RuleSelection), where+".ruleSelection"); err != nil {
			return parsedRecord{}, err
		}
	}
	groups, err := rules.ParseGroupSelection(jsonText(nonNil(record.Groups)), where+".groups")
	if err != nil {
		return parsedRecord{}, err
	}
	if groups.Pattern != "" {
		return parsedRecord{}, invalidSnapshot(where+".groups", "expected resolved group IDs, not a wildcard")
	}
	result.Groups = groups.Groups
	for id, rule := range record.Rules {
		location := where + ".rules." + id
		if err := librarytree.ValidateRuleID(id, location); err != nil {
			return parsedRecord{}, err
		}
		if (rule.Version == nil) != (rule.Release == nil) || (rule.Release != nil && *rule.Release < 1) || !fullCommit(rule.Commit) {
			return parsedRecord{}, invalidSnapshot(location, "expected a version, the library release that published it, and its commit, or a null version and release")
		}
		imported := library.ImportedRule{Version: rule.Version, Commit: rule.Commit, LegacyLayout: rule.LegacyLayout}
		if rule.Release != nil {
			imported.Release = *rule.Release
		}
		result.Rules[id] = imported
	}
	paths := map[string][]byte{}
	for file, hash := range record.Files {
		decoded, err := hex.DecodeString(hash)
		if err != nil || len(decoded) != sha256.Size || strings.ToLower(hash) != hash || file == "_source.json" {
			return parsedRecord{}, invalidSnapshot(where, "invalid or reserved file digest")
		}
		paths[file] = nil
	}
	if err := rules.ValidatePaths(paths, nil); err != nil {
		return parsedRecord{}, fmt.Errorf("%s: %w", where, err)
	}
	if result.Curated, err = readRecordCurated(record, where); err != nil {
		return parsedRecord{}, err
	}
	if err := readRecordSkills(record, where, paths, &result.Snapshot); err != nil {
		return parsedRecord{}, err
	}
	if result.NonLibrary {
		if result.Release != 0 || len(result.Groups) > 0 || len(result.Rules) > 0 || len(result.RetiredRules) > 0 || !result.Selection.IsEmpty() || len(result.RuleSelection) > 0 || len(result.Curated) > 0 {
			return parsedRecord{}, invalidSnapshot(where, "a record of a source that isn't a library has no library release, groups, or rules")
		}
		// Files outside the skills' folders are those the skills link to, which loadSnapshot checks once it has their
		// bytes.
		return result, nil
	}
	if _, ok := paths[librarytree.Manifest]; !ok {
		if _, legacy := paths[librarytree.LegacyManifest]; legacy {
			return parsedRecord{}, invalidSnapshot(where, "records a library in the layout from before this version of Trail Guide, with rule-library.yaml, which this version can't read. Once the library has moved its files into .trailguide/ and published a library release, "+reimport(name))
		}
		return parsedRecord{}, invalidSnapshot(where, "missing library manifest from inventory")
	}
	for _, group := range result.Groups {
		if _, ok := paths[group+"/_group.yaml"]; !ok {
			return parsedRecord{}, invalidSnapshot(where, "missing group metadata from inventory: "+group)
		}
	}
	return result, nil
}

// foldCurated moves what record's curated sources install into the record's own skills, files, and executable
// files, as a snapshot holds them: a curated skill's ID becomes its source's name, a colon, and its ID in the
// source's repository, and a file's path is in the source's folder of curated skills. It fails when the record's
// own fields hold a curated skill or file, or a library's skill records usedByLibrary, or a curated source records no
// skills or lists its executable files or a skill's usedByLibrary out of order.
func foldCurated(record *sourceRecord, where string) error {
	for id, skill := range record.Skills {
		if isCuratedSkill(id) {
			return invalidSnapshot(where+".skills."+id, "a curated skill is recorded in its curated source's skills")
		}
		if skill.UsedByLibrary != nil {
			return invalidSnapshot(where+".skills."+id+".usedByLibrary", "only a curated skill records the library's skills that use it")
		}
	}
	for _, file := range slices.Concat(slices.Collect(maps.Keys(record.Files)), record.Executable) {
		if _, _, curated := curatedFile(file); curated {
			return invalidSnapshot(where+".files", file+" is in the folder of curated skills, whose files are recorded in their curated source's files")
		}
	}
	for _, name := range slices.Sorted(maps.Keys(record.Curated)) {
		location := where + ".curated." + name
		entry := record.Curated[name]
		if len(entry.Skills) == 0 || entry.Files == nil || entry.Executable == nil || !slices.IsSorted(entry.Executable) || len(slices.Compact(slices.Clone(entry.Executable))) != len(entry.Executable) {
			return invalidSnapshot(location, "expected the skills the project installs from the curated source, their files, and their executable files in sorted order")
		}
		for id, skill := range entry.Skills {
			if isCuratedSkill(id) {
				return invalidSnapshot(location+".skills."+id, "expected the skill's ID in the curated repository")
			}
			if !slices.IsSorted(skill.UsedByLibrary) || slices.ContainsFunc(skill.UsedByLibrary, isCuratedSkill) {
				return invalidSnapshot(location+".skills."+id+".usedByLibrary", "expected the library's own skills in sorted order")
			}
			// The library's skills and the curated source's are one list once both have their IDs in the library.
			usedBy := append([]string{}, skill.UsedByLibrary...)
			for _, user := range skill.UsedBy {
				usedBy = append(usedBy, name+":"+user)
			}
			slices.Sort(usedBy)
			if skill.UsedBy == nil {
				usedBy = nil
			}
			record.Skills[name+":"+id] = recordSkill{Level: skill.Level, Included: skill.Included, UsedBy: usedBy}
		}
		prefix := library.CuratedDirectory + "/" + name + "/"
		for file, hash := range entry.Files {
			record.Files[prefix+file] = hash
		}
		for _, file := range entry.Executable {
			record.Executable = append(record.Executable, prefix+file)
		}
	}
	slices.Sort(record.Executable)
	return nil
}

// readRecordCurated validates the record's curated sources and returns them by name, or nil when there are none.
func readRecordCurated(record sourceRecord, where string) (map[string]library.CuratedSource, error) {
	if len(record.Curated) == 0 {
		return nil, nil
	}
	result := map[string]library.CuratedSource{}
	for _, name := range slices.Sorted(maps.Keys(record.Curated)) {
		location := where + ".curated." + name
		entry := record.Curated[name]
		address, _ := json.Marshal(entry.Repository)
		if _, err := rules.ParseRepository(address, location+".repository"); err != nil {
			return nil, err
		}
		if entry.Kind != libraryKind && entry.Kind != nonLibraryKind || !fullCommit(entry.Commit) || entry.Release < 0 || entry.Kind == nonLibraryKind && entry.Release != 0 {
			return nil, invalidSnapshot(location, "expected the kind, repository, and full commit SHA of the revision the curated skills come from")
		}
		curated := library.CuratedSource{Repository: entry.Repository, Release: entry.Release, Commit: entry.Commit, Source: entry.Source, NonLibrary: entry.Kind == nonLibraryKind}
		if entry.Ref != "" {
			var err error
			if curated.Ref, err = rules.ParseGitRef(entry.Ref, location+".ref"); err != nil {
				return nil, err
			}
		}
		result[name] = curated
	}
	return result, nil
}

// readRecordSkills validates the record's skill selection, skills, and executable files against its file inventory
// paths, and stores them in snapshot. A library's skills have the IDs of its skills/ folder and a level; another
// repository's have no level; a curated skill names a recorded curated source and has a level. Each installed
// skill needs its SKILL.md, and is one of the available skills, with the same level, except a curated skill
// installed only because other curated skills use it, which the library doesn't list.
func readRecordSkills(record sourceRecord, where string, paths map[string][]byte, snapshot *library.Snapshot) error {
	snapshot.SkillSelection = []string{}
	for i, id := range record.SkillInclude {
		if id != skills.Wildcard && id != skills.Recommended {
			if err := skills.ValidateID(id, where+".skillInclude"); err != nil {
				return err
			}
		}
		if i > 0 && record.SkillInclude[i-1] >= id {
			return invalidSnapshot(where+".skillInclude", "expected distinct selectors in sorted order")
		}
		snapshot.SkillSelection = append(snapshot.SkillSelection, id)
	}
	var err error
	available, installed := map[string]*string{}, map[string]*string{}
	for id, skill := range record.AvailableSkills {
		available[id] = skill.Level
	}
	for id, skill := range record.Skills {
		installed[id] = skill.Level
	}
	if snapshot.AvailableSkills, err = readRecordSkillLevels(available, where+".availableSkills", snapshot.NonLibrary, snapshot.Curated, false); err != nil {
		return err
	}
	if snapshot.Skills, err = readRecordSkillLevels(installed, where+".skills", snapshot.NonLibrary, snapshot.Curated, true); err != nil {
		return err
	}
	for _, id := range slices.Sorted(maps.Keys(snapshot.Skills)) {
		location := where + ".skills." + id
		available, ok := snapshot.AvailableSkills[id]
		if !ok && !(isCuratedSkill(id) && !record.Skills[id].Included) || ok && available.Level != snapshot.Skills[id].Level {
			return invalidSnapshot(location, "expected one of availableSkills, with the same level")
		}
		if _, ok := paths[library.SkillFolderIn(paths, id)+"/"+skills.File]; !ok {
			return invalidSnapshot(location, "missing the skill's SKILL.md from inventory")
		}
		usedBy := record.Skills[id].UsedBy
		for i, user := range usedBy {
			if _, recorded := record.Skills[user]; !recorded || user == id || i > 0 && usedBy[i-1] >= user {
				return invalidSnapshot(location+".usedBy", "expected distinct other recorded skills in sorted order")
			}
		}
		if usedBy == nil || !record.Skills[id].Included && len(usedBy) == 0 {
			return invalidSnapshot(location, "a skill that neither skills.include nor the library's required level selects is recorded only because other skills use it")
		}
		snapshot.Skills[id] = library.ImportedSkill{Level: snapshot.Skills[id].Level, Included: record.Skills[id].Included, UsedBy: slices.Clone(usedBy)}
	}
	snapshot.Executable = []string{}
	for i, file := range record.Executable {
		if _, ok := paths[file]; !ok || i > 0 && record.Executable[i-1] >= file || !isSkillFileIn(paths, snapshot.Skills, file) {
			return invalidSnapshot(where+".executable", "expected distinct skill files from the inventory in sorted order")
		}
		snapshot.Executable = append(snapshot.Executable, file)
	}
	return nil
}

// isSkillFileIn reports whether file is in the folder, among paths, of one of installed, as Snapshot.IsSkillFile
// does once the snapshot has its files.
func isSkillFileIn(paths map[string][]byte, installed map[string]library.ImportedSkill, file string) bool {
	for id := range installed {
		if strings.HasPrefix(file, library.SkillFolderIn(paths, id)+"/") {
			return true
		}
	}
	return false
}

// idFolder returns the folder skill id's ID names, in a snapshot whose skills sit at their IDs: the ID, or
// curated/<source>/<ID> for a curated skill.
func idFolder(id string) string {
	if source, skill, curated := library.SplitCurated(id); curated {
		return library.CuratedDirectory + "/" + source + "/" + skill
	}
	return id
}

// readRecordSkillLevels validates the levels of the skills recorded at location, by ID: a library's skills,
// skills/<name>, each with a level, or, when nonLibrary is set, skills from another repository with a null level,
// and a library's curated skills, each with a level and, when installed is set, of a source in curated; none in
// another's folder. Available skills needn't name a recorded source, since a library lists curated skills that a
// project may not install, and a curated source is recorded only when the project installs its skills. It returns the skills with only their
// levels.
func readRecordSkillLevels(records map[string]*string, location string, nonLibrary bool, curated map[string]library.CuratedSource, installed bool) (map[string]library.ImportedSkill, error) {
	result := map[string]library.ImportedSkill{}
	for _, id := range slices.Sorted(maps.Keys(records)) {
		where := location + "." + id
		level := records[id]
		if source, skill, isCurated := library.SplitCurated(id); isCurated {
			if err := skills.ValidateID(skill, where); err != nil {
				return nil, err
			}
			_, recorded := curated[source]
			if installed && !recorded || level == nil || !recordedLevel(*level) {
				return nil, invalidSnapshot(where, "expected a curated skill, of a recorded curated source when installed, with level required, recommended, offered, or dependency")
			}
		} else {
			if err := skills.ValidateID(id, where); err != nil {
				return nil, err
			}
			name, authored := strings.CutPrefix(id, skills.Directory+"/")
			if nonLibrary && level != nil || !nonLibrary && (level == nil || !recordedLevel(*level) || !authored || !skills.ValidName(name)) {
				return nil, invalidSnapshot(where, "expected a library's skill, skills/<name>, with level required, recommended, offered, or dependency, or a skill from another repository with a null level")
			}
		}
		for other := range records {
			if strings.HasPrefix(idFolder(id), idFolder(other)+"/") {
				return nil, invalidSnapshot(where, "a skill's folder can't be inside another skill's folder, "+other)
			}
		}
		imported := library.ImportedSkill{}
		if level != nil {
			imported.Level = *level
		}
		result[id] = imported
	}
	return result, nil
}

// recordedLevel reports whether level is one a source record gives a library's skill: one of skills.Levels, or
// skills.LevelDependency.
func recordedLevel(level string) bool {
	return slices.Contains(skills.Levels, level) || level == skills.LevelDependency
}

// matchSnapshotSource checks a record against its source's configuration without fetching Git, as documented in
// "What offline checks can verify": the recorded identity and selections, pinned versions, and ref.
func matchSnapshotSource(source rules.Source, record parsedRecord) error {
	where := entryLocation(source.Name)
	if source.Ref.IsZero() {
		release := record.Release
		if record.NonLibrary {
			release = 0
		}
		return failure("ref-required", rules.RefRequiredProblem(source.Name, release, true), nil)
	}
	// Only a ref edited to name another library release than the locked one waits for update; sync takes any other
	// edited ref.
	if number := source.Ref.Release(); source.Repository == record.Repository && !record.lockedRef.IsZero() && !source.Ref.Equal(record.lockedRef) && number != 0 && !record.NonLibrary && record.Release != 0 && number != record.Release {
		return invalidSnapshot(where+".ref", fmt.Sprintf("records ref %s, not %s, which sources.%s.ref names, and sync doesn't move a source; run trailguide project update %s to review the move and take it, or set sources.%s.ref back to %s", record.lockedRef, source.Ref, source.Name, source.Name, source.Name, record.lockedRef))
	}
	if source.Repository != record.Repository || !recordsRef(source, record) || !source.Groups.Equal(record.Selection) || !slices.Equal(source.Rules, record.RuleSelection) || !slices.Equal(source.Skills, record.SkillSelection) {
		return invalidSnapshot(where, "source identity or selection changed; run trailguide project sync")
	}
	diverging := source.SkillDivergences()
	for _, id := range slices.Sorted(maps.Keys(diverging)) {
		if record.AvailableSkills[id].Level == skills.LevelRequired {
			return &ValidationError{Location: diverging[id], Problem: rules.RequiredSkillProblem(source.Name, id, record.Release)}
		}
		if slices.Contains(source.LibraryRequires, id) {
			return &ValidationError{Location: diverging[id], Problem: rules.LibraryRequiresProblem(source.Name, id)}
		}
	}
	// The source imports each skill it forks that its repository still has, so updates can compare it with the fork,
	// but a library still lists a curated skill its curated repository deleted, so the record's forked skills aren't
	// compared with the selection.
	forked := func(id string) bool { _, ok := source.SkillForks[id]; return ok }
	included := []string{}
	for id, skill := range record.Skills {
		if skill.Included && !forked(id) {
			included = append(included, id)
		}
	}
	slices.Sort(included)
	selected := slices.DeleteFunc(slices.Concat(record.SelectedSkills(source.Skills, source.ExcludesLevel), source.LibraryRequires), forked)
	slices.Sort(selected)
	if !slices.Equal(slices.Compact(selected), included) {
		return invalidSnapshot(where, "recorded skills differ from what sources."+source.Name+".skills.include, skills.exclude, and skills.forks select with the library's required skills; run trailguide project sync")
	}
	for _, id := range slices.Sorted(maps.Keys(source.Pins)) {
		pin := source.Pins[id]
		// A pin of a rule the library retired keeps nothing, so any version passes; sync warns about it.
		rule, imported := record.Rules[id]
		if imported && (rule.Version == nil || *rule.Version != pin.Version) || !imported && !slices.Contains(record.RetiredRules, id) {
			return invalidSnapshot(where, "sources."+source.Name+".rules.pins."+id+" names a version the snapshot doesn't import; run trailguide project sync")
		}
	}
	// An exclusion matching neither an imported rule nor a retired one could be a typo that leaves the intended rule
	// active; only the library, which sync reads, can tell.
	for _, id := range slices.Sorted(maps.Keys(source.Exclude)) {
		matches := func(rule string) bool { return rules.SelectorMatches(id, rule) }
		if !slices.ContainsFunc(slices.Collect(maps.Keys(record.Rules)), matches) && !slices.ContainsFunc(record.RetiredRules, matches) {
			return invalidSnapshot("sources."+source.Name+".rules.exclude."+id, "names no rule the library supplies to this source; run trailguide project sync to check it against the library")
		}
	}
	for _, group := range source.Groups.Groups {
		if !slices.Contains(record.Groups, group) {
			return invalidSnapshot(where, "resolved groups differ from the explicit selection")
		}
	}
	if source.Groups.Pattern == "" && !slices.Equal(record.Groups, source.Groups.Groups) {
		return invalidSnapshot(where, "resolved groups differ from the explicit selection")
	}
	for _, group := range record.Groups {
		if !source.Groups.Includes(group) {
			return invalidSnapshot(where, "resolved group is outside the requested selection")
		}
	}
	if err := matchRevision(source, record); err != nil {
		return err
	}
	newest := 0
	commits := map[int]string{}
	for _, id := range slices.Sorted(maps.Keys(record.Rules)) {
		rule := record.Rules[id]
		if rule.Version == nil && record.Release != 0 {
			return invalidSnapshot(where+".rules."+id, "only a ref that isn't a library release can import a rule without a version")
		}
		if (rule.Version == nil || rule.Release == record.Release) && rule.Commit != record.Commit {
			return invalidSnapshot(where+".rules."+id, "the rule's commit differs from the resolved commit of its library release")
		}
		if commit, seen := commits[rule.Release]; rule.Version != nil && seen && commit != rule.Commit {
			return invalidSnapshot(where+".rules."+id, "rules from one library release record different commits")
		}
		commits[rule.Release] = rule.Commit
		newest = max(newest, rule.Release)
	}
	if source.Ref.FollowsReleases() && record.Release != 0 && record.Release < newest {
		return invalidSnapshot(where+".release", "expected the library release that supplies the shared files to be at least as new as every imported rule version's library release")
	}
	return nil
}

// matchSkillUses checks, without fetching Git, that snapshot records exactly the skills its included skills bring in
// and why: the skills they use, transitively, by the trailguide-skills-used of the vendored SKILL.md files and the source's
// skills.uses, a curated skill's from the repository it was curated from, and a library's skill's from the library
// or the skills it curates. A difference means configuration changed since sync, such as a new skills.uses entry, so it fails
// asking for sync, which checks the change against the source.
func matchSkillUses(source rules.Source, snapshot library.Snapshot) error {
	where := entryLocation(source.Name)
	uses := map[string][]string{}
	included := []string{}
	for _, id := range slices.Sorted(maps.Keys(snapshot.Skills)) {
		file := snapshot.SkillFolder(id) + "/" + skills.File
		metadata, err := skills.ReadMetadata(snapshot.Files[file], librarytree.Vendor+"/"+source.Name+"/"+file)
		if err != nil {
			return err
		}
		// A curated skill uses skills of the repository it was curated from, curated with it, but not the skills
		// that repository curates in turn, which aren't installed with it. A library's own skill names a curated
		// skill it uses by its ID in the library, which the snapshot records it under.
		if name, _, curated := library.SplitCurated(id); curated {
			uses[id] = []string{}
			for _, used := range metadata.SkillsUsed {
				if !strings.Contains(used, ":") {
					uses[id] = append(uses[id], name+":"+used)
				}
			}
		} else {
			uses[id] = slices.Concat(metadata.SkillsUsed, source.SkillUses[id])
		}
		if snapshot.Skills[id].Included {
			included = append(included, id)
		}
	}
	for _, id := range slices.Sorted(maps.Keys(source.SkillUses)) {
		if _, ok := snapshot.Skills[id]; !ok {
			return invalidSnapshot("sources."+source.Name+".skills.uses."+id, "names a skill the snapshot doesn't install; run trailguide project sync to check it against the source")
		}
	}
	installed, err := skills.ResolveUses(included, uses)
	if err != nil {
		return invalidSnapshot(where, "the recorded skills don't install every skill they use, by trailguide-skills-used and sources."+source.Name+".skills.uses; run trailguide project sync")
	}
	if len(installed) != len(snapshot.Skills) {
		return invalidSnapshot(where, "the recorded skills differ from those sources."+source.Name+".skills.include selects and the skills they use; run trailguide project sync")
	}
	for id, why := range installed {
		if !slices.Equal(why.UsedBy, snapshot.Skills[id].UsedBy) {
			return invalidSnapshot(where+".skills."+id, "the skills recorded as using it differ from trailguide-skills-used and sources."+source.Name+".skills.uses; run trailguide project sync")
		}
	}
	return nil
}

// recordsRef reports whether record holds the ref source asks for. A lock entry records the ref configuration named
// when sync or update wrote it. A per-source record of a library source whose ref chooses a library release,
// release/<number> or latest-release, is recorded without a ref, or, as sync recorded it before Trail Guide required a ref,
// with the same ref; matchRevision checks the release. Any other ref is recorded as configured.
func recordsRef(source rules.Source, record parsedRecord) bool {
	if !record.lockedRef.IsZero() {
		return source.Ref.Equal(record.lockedRef)
	}
	if source.Ref.FollowsReleases() && !record.NonLibrary && record.Release != 0 {
		return record.Ref.IsZero() || record.Ref.Equal(source.Ref)
	}
	return source.Ref.Equal(record.Ref)
}

// matchRevision checks the record's library release and commit against how the source chooses versions.
func matchRevision(source rules.Source, record parsedRecord) error {
	where := entryLocation(source.Name)
	ref := source.Ref
	if ref.FollowsReleases() && !record.NonLibrary && record.Release != 0 {
		if number := ref.Release(); number != 0 && record.Release != number {
			return invalidSnapshot(where+".release", fmt.Sprintf("records library release %d, not release/%d, which sources.%s.ref names; run trailguide project update %s to review the move and take it, or set sources.%s.ref back to release/%d", record.Release, number, source.Name, source.Name, source.Name, record.Release))
		}
		return nil
	}
	if ref.Kind() == rules.GitRefCommit {
		if record.Commit != ref.Canonical() {
			return invalidSnapshot(where, "resolved commit differs from requested commit")
		}
	}
	number, err := trailguide.ParseReleaseTag(strings.TrimPrefix(ref.Canonical(), "refs/tags/"))
	// A bare name shaped like a library release, such as release/5, can name a branch, whose record has no library
	// release.
	branch := ref.Bare() && record.Release == 0
	if ref.Kind() != rules.GitRefTag || err != nil || branch {
		if record.Release != 0 {
			return invalidSnapshot(where+".release", "a ref that isn't a library release tag records no library release")
		}
		return nil
	}
	if record.Release != number {
		return invalidSnapshot(where+".release", "expected the library release that sources."+source.Name+".ref names")
	}
	for _, id := range slices.Sorted(maps.Keys(record.Rules)) {
		if record.Rules[id].Release > number {
			return invalidSnapshot(where+".rules."+id, "a rule imported from "+source.Ref.String()+" can't come from a later library release")
		}
	}
	return nil
}

// jsonText returns value as JSON, for the parsers that read configuration's JSON form.
func jsonText(value any) json.RawMessage {
	text, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return text
}

// fullCommit accepts a lowercase 40-digit commit SHA.
func fullCommit(text string) bool {
	ref, err := rules.ParseGitRef(text, "commit")
	return err == nil && ref.Kind() == rules.GitRefCommit && ref.Canonical() == text
}

// digest hashes exact bytes, preserving binary data and line endings.
func digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

// invalidSnapshot reports caller-visible validation failures without logging source contents.
func invalidSnapshot(location, problem string) error {
	return &ValidationError{Location: location, Problem: problem}
}
