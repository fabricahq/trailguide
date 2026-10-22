// Parse, verify, and write a project's lockfile, .trailguide/lock.json, without accessing Git.

package trailguide

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/fabricahq/trailguide/internal/skills"
)

// LockFile is the lockfile's path in a project's Trail Guide directory.
const LockFile = "lock.json"

// LockFormat is the lockfile format this version of Trail Guide writes and reads, recorded in its lockVersion.
const LockFormat = 1

// lockNotice is the notice Trail Guide writes at the top of every lockfile.
const lockNotice = "Written by trailguide project sync and update. Do not edit by hand."

// Kinds of locked source: a library, whose repository has a library manifest at the locked commit, or another
// repository, which supplies only skills.
const (
	KindLibrary    = "library"
	KindNonLibrary = "non-library"
)

// Lock is a project's lockfile: what each source resolved to when sync or update last took it. Sources is keyed by
// source name and is never nil in a parsed lock.
type Lock struct {
	Sources map[string]LockedSource
}

// LockedSource is one source's entry in a lockfile. Ref is the ref configuration named when the entry was written, and
// Commit the full commit SHA it resolved to. Release is the library release the source is on, or 0 when its ref
// follows a branch or commit, or it isn't a library. ReleaseCommits maps each library release the entry's rules come
// from, and Release, to that release's commit. Fields that don't apply to a kind are empty.
type LockedSource struct {
	// Integrity is "sha256-" and the base64 SHA-256 of the entry's canonical form. Parse checks it; Encode computes it,
	// ignoring this field.
	Integrity       string
	Kind            string
	Repository      string
	Ref             string
	Release         int
	Commit          string
	ReleaseCommits  map[int]string
	Selectors       LockSelectors
	Groups          []string
	Rules           map[string]LockedRule
	RetiredRules    []string
	Skills          map[string]LockedSkill
	AvailableSkills map[string]LockedAvailableSkill
	// Files maps each stored file's path in the source's snapshot to its lowercase hexadecimal SHA-256.
	Files      map[string]string
	Executable []string
	Curated    map[string]LockedCurated
}

// LockSelectors are a source's rule and skill selectors when its entry was written. Groups is the group selection as
// configuration resolves it, in its JSON form; Groups and Rules are empty for a source that isn't a library.
type LockSelectors struct {
	Groups json.RawMessage
	Rules  []string
	Skills []string
}

// LockedRule is one rule's version and the library release that published it; both are nil for a rule file with
// unreleased changes. Its commit is the entry's ReleaseCommits[Release], or the entry's Commit when unreleased.
type LockedRule struct {
	Version      *RuleVersion
	Release      *int
	LegacyLayout bool
}

// LockedSkill is one installed skill. Level is nil for a skill from a repository that isn't a library. UsedByLibrary
// is set only for a curated skill, listing the library's own skills that use it.
type LockedSkill struct {
	Level         *string
	Included      bool
	UsedBy        []string
	UsedByLibrary []string
}

// LockedAvailableSkill is a skill the source offers at the locked commit, installed or not.
type LockedAvailableSkill struct {
	Level *string
}

// LockedCurated is the revision of a source a library curates skills from, and what the project installs from it.
type LockedCurated struct {
	Kind       string
	Repository string
	Ref        string
	Release    int
	Commit     string
	Source     string
	Skills     map[string]LockedSkill
	Files      map[string]string
	Executable []string
}

// UnsupportedLockError reports a lockfile whose lockVersion is newer than LockFormat: a newer Trail Guide wrote it.
type UnsupportedLockError struct {
	Version int
}

func (e *UnsupportedLockError) Error() string {
	return fmt.Sprintf("%s was written by a newer version of Trail Guide, in lock format %d, but this version reads only format %d; upgrade Trail Guide to use this project", LockFile, e.Version, LockFormat)
}

// LockConflictError reports a lockfile with unresolved merge conflicts. Sources lists, by name, each source whose
// entry differs between the two sides, with what each side locked, so the conflict can be resolved one entry at a time.
// When both sides read as JSON, Ours is the lockfile with every conflict resolved to the first side, so a caller that
// resolves each conflicted source again can still use the entries both sides agree on; otherwise it is nil.
type LockConflictError struct {
	Sources []ConflictedSource
	Ours    []byte
}

// ConflictedSource is one source whose entry a merge conflict splits. Ours and Theirs are nil when that side has no
// readable entry for the source.
type ConflictedSource struct {
	Name         string
	Ours, Theirs *LockedRevision
}

// LockedRevision is what one side of a conflict locked for a source.
type LockedRevision struct {
	Ref     string
	Release int
	Commit  string
}

func (e *LockConflictError) Error() string {
	var text strings.Builder
	text.WriteString(LockFile + " has unresolved merge conflicts. Resolve each conflicted source by keeping one side's whole entry, never lines from both, and leave other sources as Git merged them; then run trailguide project sync, and trailguide project update SOURCE to take the other side's change too. Or leave the markers and run trailguide project sync with --retake for each conflicted source, which resolves it again at its ref.")
	for _, source := range e.Sources {
		fmt.Fprintf(&text, " %s: ours %s, theirs %s.", source.Name, source.Ours.describe(), source.Theirs.describe())
	}
	return text.String()
}

// describe returns what r locked, or that the side has no entry.
func (r *LockedRevision) describe() string {
	if r == nil {
		return "has no entry"
	}
	text := "ref " + r.Ref
	if r.Release != 0 {
		text += ", release " + strconv.Itoa(r.Release)
	}
	return text + ", commit " + r.Commit
}

// ParseLock reads and verifies a lockfile: it rejects merge conflict markers (as a *LockConflictError), invalid UTF-8,
// duplicate keys at any depth, unknown fields, fields missing or present where the kind of source says they must not
// be, an entry whose integrity doesn't match its canonical form, and a releaseCommits map that doesn't account for
// every rule's release. A newer lockVersion fails with *UnsupportedLockError. It doesn't check entries against
// configuration or files. An entry's problem fails with the *ValidationError of the first such source by name.
func ParseLock(data []byte) (Lock, error) {
	lock, refused, err := ParseLockEntries(data)
	if err != nil {
		return Lock{}, err
	}
	if len(refused) > 0 {
		return Lock{}, refused[slices.Sorted(maps.Keys(refused))[0]]
	}
	return lock, nil
}

// ParseLockEntries is ParseLock, except that it verifies each source's entry on its own: Sources holds the entries it
// accepts, and refused, by source name, the error of each entry it doesn't, so a caller can still use the others,
// such as to resolve a refused source again. It fails only for a problem with the whole file.
func ParseLockEntries(data []byte) (Lock, map[string]error, error) {
	if conflicted := conflictedLines(data); conflicted {
		return Lock{}, nil, lockConflict(data)
	}
	value, err := decodeStrict(data, LockFile)
	if err != nil {
		return Lock{}, nil, err
	}
	top, ok := value.(map[string]any)
	if !ok {
		return Lock{}, nil, invalid(LockFile, "expected a JSON object")
	}
	version, ok := top["lockVersion"].(json.Number)
	if !ok {
		return Lock{}, nil, invalid(LockFile+".lockVersion", "expected the lock format number")
	}
	if number, err := strconv.Atoi(version.String()); err != nil || number != LockFormat {
		if err == nil && number > LockFormat {
			return Lock{}, nil, &UnsupportedLockError{Version: number}
		}
		return Lock{}, nil, invalid(LockFile+".lockVersion", fmt.Sprintf("expected %d", LockFormat))
	}
	if err := lockKnownFields(top, []string{"lockVersion", "notice", "sources"}, []string{"lockVersion", "sources"}, LockFile); err != nil {
		return Lock{}, nil, err
	}
	if notice, present := top["notice"]; present {
		if _, ok := notice.(string); !ok {
			return Lock{}, nil, invalid(LockFile+".notice", "expected text")
		}
	}
	sources, ok := top["sources"].(map[string]any)
	if !ok {
		return Lock{}, nil, invalid(LockFile+".sources", "expected an object of sources by name")
	}
	lock := Lock{Sources: map[string]LockedSource{}}
	refused := map[string]error{}
	for _, name := range slices.Sorted(maps.Keys(sources)) {
		entry, err := parseLockedSource(name, sources[name])
		if err != nil {
			refused[name] = err
			continue
		}
		lock.Sources[name] = entry
	}
	return lock, refused, nil
}

// Encode returns the lockfile text for l, computing each entry's integrity: keys sorted, two-space indentation, and
// each rule, skill, and file on its own line, so each source's entry is one contiguous block in a diff. It fails when
// an entry is one ParseLock would refuse.
func (l Lock) Encode() ([]byte, error) {
	sources := map[string]any{}
	for name, source := range l.Sources {
		tree, err := lockedSourceTree(source)
		if err != nil {
			return nil, err
		}
		tree["integrity"] = entryIntegrity(name, tree)
		sources[name] = tree
	}
	var text bytes.Buffer
	writePretty(&text, map[string]any{"lockVersion": json.Number(strconv.Itoa(LockFormat)), "notice": lockNotice, "sources": sources}, "", nil)
	text.WriteByte('\n')
	if _, err := ParseLock(text.Bytes()); err != nil {
		return nil, err
	}
	return text.Bytes(), nil
}

// EntryIntegrity returns the integrity of source as the source named name: "sha256-" and the standard base64
// encoding of the SHA-256 of the canonical JSON (RFC 8785) of {"entry": <the entry without integrity>, "name": name}.
func EntryIntegrity(name string, source LockedSource) (string, error) {
	tree, err := lockedSourceTree(source)
	if err != nil {
		return "", err
	}
	return entryIntegrity(name, tree), nil
}

// entryIntegrity is EntryIntegrity over an entry's JSON tree, ignoring any integrity field it holds.
func entryIntegrity(name string, entry map[string]any) string {
	without := maps.Clone(entry)
	delete(without, "integrity")
	var canonical bytes.Buffer
	writeCanonical(&canonical, map[string]any{"entry": without, "name": name})
	sum := sha256.Sum256(canonical.Bytes())
	return "sha256-" + base64.StdEncoding.EncodeToString(sum[:])
}

// The fields every entry has, and those only a library's has. A library's entry may also have release, and any entry
// curated.
var (
	lockCommonFields  = []string{"integrity", "kind", "repository", "ref", "commit", "selectors", "skills", "availableSkills", "files", "executable"}
	lockLibraryFields = []string{"releaseCommits", "groups", "rules", "retiredRules"}
)

// parseLockedSource verifies one source's entry, as ParseLock documents.
func parseLockedSource(name string, value any) (LockedSource, error) {
	where := LockFile + ".sources." + name
	if !skills.ValidSourceName(name) {
		return LockedSource{}, invalid(where, "invalid source name")
	}
	entry, ok := value.(map[string]any)
	if !ok {
		return LockedSource{}, invalid(where, "expected an object")
	}
	// Any change outside Trail Guide, an added or removed field included, fails here first, with one explanation.
	integrity, _ := entry["integrity"].(string)
	if expected := entryIntegrity(name, entry); integrity != expected {
		return LockedSource{}, invalid(where+".integrity", "doesn't match the entry, so it was changed outside Trail Guide, such as by hand or in a merge that combined both sides; restore the entry Trail Guide wrote, such as from Git, and run trailguide project sync")
	}
	kind, _ := entry["kind"].(string)
	required := lockCommonFields
	switch kind {
	case KindLibrary:
		required = slices.Concat(lockCommonFields, lockLibraryFields)
	case KindNonLibrary:
	default:
		return LockedSource{}, invalid(where+".kind", "expected library or non-library")
	}
	allowed := slices.Concat(required, []string{"curated"})
	if kind == KindLibrary {
		allowed = append(allowed, "release")
	}
	if err := lockKnownFields(entry, allowed, required, where); err != nil {
		return LockedSource{}, err
	}
	source := LockedSource{Integrity: integrity, Kind: kind}
	var err error
	if source.Repository, err = textField(entry, "repository", where); err != nil {
		return LockedSource{}, err
	}
	if source.Ref, err = textField(entry, "ref", where); err != nil {
		return LockedSource{}, err
	}
	if source.Commit, err = commitField(entry, "commit", where); err != nil {
		return LockedSource{}, err
	}
	if _, present := entry["release"]; present {
		if source.Release, err = releaseField(entry["release"], where+".release"); err != nil {
			return LockedSource{}, err
		}
	}
	if source.Selectors, err = parseSelectors(entry["selectors"], kind, where+".selectors"); err != nil {
		return LockedSource{}, err
	}
	if source.Skills, err = parseLockedSkills(entry["skills"], where+".skills", false); err != nil {
		return LockedSource{}, err
	}
	if source.AvailableSkills, err = parseAvailableSkills(entry["availableSkills"], where+".availableSkills"); err != nil {
		return LockedSource{}, err
	}
	if source.Files, err = parseDigests(entry["files"], where+".files"); err != nil {
		return LockedSource{}, err
	}
	if source.Executable, err = textList(entry["executable"], where+".executable"); err != nil {
		return LockedSource{}, err
	}
	if raw, present := entry["curated"]; present {
		if source.Curated, err = parseCurated(raw, where+".curated"); err != nil {
			return LockedSource{}, err
		}
	}
	if kind == KindNonLibrary {
		return source, nil
	}
	if source.Groups, err = textList(entry["groups"], where+".groups"); err != nil {
		return LockedSource{}, err
	}
	if source.RetiredRules, err = textList(entry["retiredRules"], where+".retiredRules"); err != nil {
		return LockedSource{}, err
	}
	if source.Rules, err = parseLockedRules(entry["rules"], where+".rules"); err != nil {
		return LockedSource{}, err
	}
	if source.ReleaseCommits, err = parseReleaseCommits(entry["releaseCommits"], where+".releaseCommits"); err != nil {
		return LockedSource{}, err
	}
	if err := checkReleaseCommits(source, where); err != nil {
		return LockedSource{}, err
	}
	return source, nil
}

// checkReleaseCommits requires source's releaseCommits to hold exactly the releases its rules come from and its own
// release, which must map to its commit.
func checkReleaseCommits(source LockedSource, where string) error {
	needed := map[int]bool{}
	for id, rule := range source.Rules {
		if rule.Release != nil {
			if _, ok := source.ReleaseCommits[*rule.Release]; !ok {
				return invalid(where+".rules."+id, fmt.Sprintf("comes from library release %d, which releaseCommits doesn't list", *rule.Release))
			}
			needed[*rule.Release] = true
		}
	}
	if source.Release != 0 {
		if source.ReleaseCommits[source.Release] != source.Commit {
			return invalid(where+".releaseCommits", fmt.Sprintf("expected library release %d, the entry's release, to map to its commit", source.Release))
		}
		needed[source.Release] = true
	}
	for release := range source.ReleaseCommits {
		if !needed[release] {
			return invalid(where+".releaseCommits", fmt.Sprintf("lists library release %d, which no rule comes from", release))
		}
	}
	return nil
}

// lockedSourceTree returns the JSON tree Encode writes for source, without integrity: exactly the fields its kind
// has, required ones even when empty.
func lockedSourceTree(source LockedSource) (map[string]any, error) {
	tree := map[string]any{
		"kind":            source.Kind,
		"repository":      source.Repository,
		"ref":             source.Ref,
		"commit":          source.Commit,
		"skills":          skillsTree(source.Skills),
		"availableSkills": availableTree(source.AvailableSkills),
		"files":           digestsTree(source.Files),
		"executable":      listTree(source.Executable),
	}
	selectors := map[string]any{"skills": listTree(source.Selectors.Skills)}
	switch source.Kind {
	case KindLibrary:
		groups, err := rawTree(source.Selectors.Groups)
		if err != nil {
			return nil, fmt.Errorf("encode the group selection: %w", err)
		}
		selectors["groups"] = groups
		selectors["rules"] = listTree(source.Selectors.Rules)
		tree["groups"] = listTree(source.Groups)
		tree["retiredRules"] = listTree(source.RetiredRules)
		rules := map[string]any{}
		for id, rule := range source.Rules {
			value := map[string]any{"version": nil, "release": nil}
			if rule.Version != nil {
				value["version"] = rule.Version.String()
			}
			if rule.Release != nil {
				value["release"] = json.Number(strconv.Itoa(*rule.Release))
			}
			if rule.LegacyLayout {
				value["legacyLayout"] = true
			}
			rules[id] = value
		}
		tree["rules"] = rules
		commits := map[string]any{}
		for release, commit := range source.ReleaseCommits {
			commits[strconv.Itoa(release)] = commit
		}
		tree["releaseCommits"] = commits
		if source.Release != 0 {
			tree["release"] = json.Number(strconv.Itoa(source.Release))
		}
	case KindNonLibrary:
	default:
		return nil, invalid("kind", "expected library or non-library")
	}
	tree["selectors"] = selectors
	if len(source.Curated) > 0 {
		curated := map[string]any{}
		for name, entry := range source.Curated {
			value := map[string]any{"kind": entry.Kind, "repository": entry.Repository, "commit": entry.Commit, "skills": skillsTree(entry.Skills), "files": digestsTree(entry.Files), "executable": listTree(entry.Executable)}
			if entry.Ref != "" {
				value["ref"] = entry.Ref
			}
			if entry.Release != 0 {
				value["release"] = json.Number(strconv.Itoa(entry.Release))
			}
			if entry.Source != "" {
				value["source"] = entry.Source
			}
			curated[name] = value
		}
		tree["curated"] = curated
	}
	return tree, nil
}

// skillsTree returns the JSON tree of skills.
func skillsTree(skills map[string]LockedSkill) map[string]any {
	tree := map[string]any{}
	for id, skill := range skills {
		value := map[string]any{"level": nil, "included": skill.Included, "usedBy": listTree(skill.UsedBy)}
		if skill.Level != nil {
			value["level"] = *skill.Level
		}
		if len(skill.UsedByLibrary) > 0 {
			value["usedByLibrary"] = listTree(skill.UsedByLibrary)
		}
		tree[id] = value
	}
	return tree
}

// availableTree returns the JSON tree of available skills.
func availableTree(available map[string]LockedAvailableSkill) map[string]any {
	tree := map[string]any{}
	for id, skill := range available {
		value := map[string]any{"level": nil}
		if skill.Level != nil {
			value["level"] = *skill.Level
		}
		tree[id] = value
	}
	return tree
}

// digestsTree returns the JSON tree of file digests.
func digestsTree(files map[string]string) map[string]any {
	tree := map[string]any{}
	for file, hash := range files {
		tree[file] = hash
	}
	return tree
}

// listTree returns list as a JSON array, empty rather than null.
func listTree(list []string) []any {
	tree := []any{}
	for _, item := range list {
		tree = append(tree, item)
	}
	return tree
}

// rawTree decodes raw JSON, or an empty list for none, into a tree.
func rawTree(raw json.RawMessage) (any, error) {
	if len(raw) == 0 {
		return []any{}, nil
	}
	return decodeStrict(raw, "groups")
}

// parseSelectors reads an entry's selectors: skills always, and groups and rules for a library.
func parseSelectors(value any, kind, where string) (LockSelectors, error) {
	object, ok := value.(map[string]any)
	if !ok {
		return LockSelectors{}, invalid(where, "expected an object")
	}
	required := []string{"skills"}
	if kind == KindLibrary {
		required = []string{"groups", "rules", "skills"}
	}
	if err := lockKnownFields(object, required, required, where); err != nil {
		return LockSelectors{}, err
	}
	var selectors LockSelectors
	var err error
	if selectors.Skills, err = textList(object["skills"], where+".skills"); err != nil {
		return LockSelectors{}, err
	}
	if kind != KindLibrary {
		return selectors, nil
	}
	if selectors.Rules, err = textList(object["rules"], where+".rules"); err != nil {
		return LockSelectors{}, err
	}
	var canonical bytes.Buffer
	writeCanonical(&canonical, object["groups"])
	selectors.Groups = canonical.Bytes()
	return selectors, nil
}

// parseLockedRules reads an entry's rules: each a version and release, both null or both set, and an optional
// legacyLayout that is true when present.
func parseLockedRules(value any, where string) (map[string]LockedRule, error) {
	object, ok := value.(map[string]any)
	if !ok {
		return nil, invalid(where, "expected an object of rules by ID")
	}
	result := map[string]LockedRule{}
	for _, id := range slices.Sorted(maps.Keys(object)) {
		location := where + "." + id
		if err := ValidateRuleID(id, location); err != nil {
			return nil, err
		}
		rule, ok := object[id].(map[string]any)
		if !ok {
			return nil, invalid(location, "expected an object")
		}
		if err := lockKnownFields(rule, []string{"version", "release", "legacyLayout"}, []string{"version", "release"}, location); err != nil {
			return nil, err
		}
		var locked LockedRule
		if rule["version"] != nil {
			text, ok := rule["version"].(string)
			if !ok {
				return nil, invalid(location+".version", "expected a rule version or null")
			}
			version, err := ParseRuleVersion(text, location+".version")
			if err != nil {
				return nil, err
			}
			locked.Version = &version
		}
		if rule["release"] != nil {
			release, err := releaseField(rule["release"], location+".release")
			if err != nil {
				return nil, err
			}
			locked.Release = &release
		}
		if (locked.Version == nil) != (locked.Release == nil) {
			return nil, invalid(location, "expected a version and the library release that published it, or null for both")
		}
		if raw, present := rule["legacyLayout"]; present {
			if raw != true {
				return nil, invalid(location+".legacyLayout", "expected true, or no field")
			}
			locked.LegacyLayout = true
		}
		result[id] = locked
	}
	return result, nil
}

// parseLockedSkills reads installed skills; usedByLibrary is allowed, and non-empty, only when curated is set.
func parseLockedSkills(value any, where string, curated bool) (map[string]LockedSkill, error) {
	object, ok := value.(map[string]any)
	if !ok {
		return nil, invalid(where, "expected an object of skills by ID")
	}
	allowed := []string{"level", "included", "usedBy"}
	if curated {
		allowed = append(allowed, "usedByLibrary")
	}
	result := map[string]LockedSkill{}
	for _, id := range slices.Sorted(maps.Keys(object)) {
		location := where + "." + id
		skill, ok := object[id].(map[string]any)
		if !ok {
			return nil, invalid(location, "expected an object")
		}
		if err := lockKnownFields(skill, allowed, []string{"level", "included", "usedBy"}, location); err != nil {
			return nil, err
		}
		var locked LockedSkill
		var err error
		if locked.Level, err = levelField(skill["level"], location+".level"); err != nil {
			return nil, err
		}
		if locked.Included, ok = skill["included"].(bool); !ok {
			return nil, invalid(location+".included", "expected true or false")
		}
		if locked.UsedBy, err = textList(skill["usedBy"], location+".usedBy"); err != nil {
			return nil, err
		}
		if raw, present := skill["usedByLibrary"]; present {
			if locked.UsedByLibrary, err = textList(raw, location+".usedByLibrary"); err != nil {
				return nil, err
			}
			if len(locked.UsedByLibrary) == 0 {
				return nil, invalid(location+".usedByLibrary", "expected skills, or no field")
			}
		}
		result[id] = locked
	}
	return result, nil
}

// parseAvailableSkills reads the skills a source offers.
func parseAvailableSkills(value any, where string) (map[string]LockedAvailableSkill, error) {
	object, ok := value.(map[string]any)
	if !ok {
		return nil, invalid(where, "expected an object of skills by ID")
	}
	result := map[string]LockedAvailableSkill{}
	for _, id := range slices.Sorted(maps.Keys(object)) {
		location := where + "." + id
		skill, ok := object[id].(map[string]any)
		if !ok {
			return nil, invalid(location, "expected an object")
		}
		if err := lockKnownFields(skill, []string{"level"}, []string{"level"}, location); err != nil {
			return nil, err
		}
		level, err := levelField(skill["level"], location+".level")
		if err != nil {
			return nil, err
		}
		result[id] = LockedAvailableSkill{Level: level}
	}
	return result, nil
}

// parseCurated reads a library's curated sources, which must be present only when there is one.
func parseCurated(value any, where string) (map[string]LockedCurated, error) {
	object, ok := value.(map[string]any)
	if !ok || len(object) == 0 {
		return nil, invalid(where, "expected the curated sources by name, or no field when there are none")
	}
	result := map[string]LockedCurated{}
	for _, name := range slices.Sorted(maps.Keys(object)) {
		location := where + "." + name
		entry, ok := object[name].(map[string]any)
		if !ok {
			return nil, invalid(location, "expected an object")
		}
		required := []string{"kind", "repository", "commit", "skills", "files", "executable"}
		if err := lockKnownFields(entry, slices.Concat(required, []string{"ref", "release", "source"}), required, location); err != nil {
			return nil, err
		}
		var curated LockedCurated
		var err error
		if curated.Kind, err = textField(entry, "kind", location); err != nil {
			return nil, err
		}
		if curated.Kind != KindLibrary && curated.Kind != KindNonLibrary {
			return nil, invalid(location+".kind", "expected library or non-library")
		}
		if curated.Repository, err = textField(entry, "repository", location); err != nil {
			return nil, err
		}
		if curated.Commit, err = commitField(entry, "commit", location); err != nil {
			return nil, err
		}
		if _, present := entry["ref"]; present {
			if curated.Ref, err = textField(entry, "ref", location); err != nil {
				return nil, err
			}
		}
		if _, present := entry["release"]; present {
			if curated.Release, err = releaseField(entry["release"], location+".release"); err != nil {
				return nil, err
			}
		}
		if _, present := entry["source"]; present {
			if curated.Source, err = textField(entry, "source", location); err != nil {
				return nil, err
			}
		}
		if curated.Skills, err = parseLockedSkills(entry["skills"], location+".skills", true); err != nil {
			return nil, err
		}
		if curated.Files, err = parseDigests(entry["files"], location+".files"); err != nil {
			return nil, err
		}
		if curated.Executable, err = textList(entry["executable"], location+".executable"); err != nil {
			return nil, err
		}
		result[name] = curated
	}
	return result, nil
}

// parseReleaseCommits reads a map of library release numbers to full commit SHAs.
func parseReleaseCommits(value any, where string) (map[int]string, error) {
	object, ok := value.(map[string]any)
	if !ok {
		return nil, invalid(where, "expected an object of commits by library release")
	}
	result := map[int]string{}
	for _, key := range slices.Sorted(maps.Keys(object)) {
		release, err := strconv.Atoi(key)
		if err != nil || release < 1 || strconv.Itoa(release) != key {
			return nil, invalid(where+"."+key, "expected a library release number")
		}
		commit, err := commitField(object, key, where)
		if err != nil {
			return nil, err
		}
		result[release] = commit
	}
	return result, nil
}

// parseDigests reads a map of file paths to lowercase hexadecimal SHA-256 digests.
func parseDigests(value any, where string) (map[string]string, error) {
	object, ok := value.(map[string]any)
	if !ok {
		return nil, invalid(where, "expected an object of digests by path")
	}
	result := map[string]string{}
	for file, raw := range object {
		hash, ok := raw.(string)
		decoded, err := hex.DecodeString(hash)
		if !ok || err != nil || len(decoded) != sha256.Size || strings.ToLower(hash) != hash || file == "" {
			return nil, invalid(where+"."+file, "expected a lowercase hexadecimal SHA-256")
		}
		result[file] = hash
	}
	return result, nil
}

// knownFields fails when object has a field outside allowed or lacks one of required.
func lockKnownFields(object map[string]any, allowed, required []string, where string) error {
	for _, key := range slices.Sorted(maps.Keys(object)) {
		if !slices.Contains(allowed, key) {
			return invalid(where+"."+key, "unknown field, or one this kind of entry doesn't have")
		}
	}
	for _, key := range required {
		if _, ok := object[key]; !ok {
			return invalid(where+"."+key, "missing field")
		}
	}
	return nil
}

// textField returns object[key] as non-empty text.
func textField(object map[string]any, key, where string) (string, error) {
	text, ok := object[key].(string)
	if !ok || text == "" {
		return "", invalid(where+"."+key, "expected text")
	}
	return text, nil
}

// commitField returns object[key] as a full lowercase commit SHA.
func commitField(object map[string]any, key, where string) (string, error) {
	text, ok := object[key].(string)
	decoded, err := hex.DecodeString(text)
	if !ok || err != nil || len(decoded) != 20 || strings.ToLower(text) != text {
		return "", invalid(where+"."+key, "expected a full lowercase commit SHA")
	}
	return text, nil
}

// releaseField returns value as a library release number.
func releaseField(value any, where string) (int, error) {
	number, ok := value.(json.Number)
	release, err := strconv.Atoi(number.String())
	if !ok || err != nil || release < 1 || strconv.Itoa(release) != number.String() {
		return 0, invalid(where, "expected a library release number")
	}
	return release, nil
}

// levelField returns value as a skill level, or nil for null.
func levelField(value any, where string) (*string, error) {
	if value == nil {
		return nil, nil
	}
	level, ok := value.(string)
	if !ok || level == "" {
		return nil, invalid(where, "expected a skill level or null")
	}
	return &level, nil
}

// textList returns value as a list of non-empty text.
func textList(value any, where string) ([]string, error) {
	items, ok := value.([]any)
	if !ok {
		return nil, invalid(where, "expected a list")
	}
	result := []string{}
	for _, item := range items {
		text, ok := item.(string)
		if !ok || text == "" {
			return nil, invalid(where, "expected a list of text")
		}
		result = append(result, text)
	}
	return result, nil
}

// decodeStrict decodes JSON text into a tree of map[string]any, []any, string, json.Number, bool, and nil, failing on
// invalid UTF-8, trailing data, and duplicate keys at any depth.
func decodeStrict(data []byte, where string) (any, error) {
	if !utf8.Valid(data) {
		return nil, invalid(where, "expected UTF-8 JSON")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	value, err := decodeValue(decoder, where)
	if err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, invalid(where, "expected one JSON value")
	}
	return value, nil
}

// decodeValue decodes the next value from decoder, as decodeStrict documents.
func decodeValue(decoder *json.Decoder, where string) (any, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, invalid(where, "invalid JSON: "+err.Error())
	}
	switch token {
	case json.Delim('{'):
		object := map[string]any{}
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return nil, invalid(where, "invalid JSON: "+err.Error())
			}
			name := key.(string)
			if _, seen := object[name]; seen {
				return nil, invalid(where+"."+name, "appears twice, usually from a merge that kept both sides; keep one side's whole entry")
			}
			if object[name], err = decodeValue(decoder, where+"."+name); err != nil {
				return nil, err
			}
		}
		if _, err := decoder.Token(); err != nil {
			return nil, invalid(where, "invalid JSON: "+err.Error())
		}
		return object, nil
	case json.Delim('['):
		list := []any{}
		for decoder.More() {
			item, err := decodeValue(decoder, where)
			if err != nil {
				return nil, err
			}
			list = append(list, item)
		}
		if _, err := decoder.Token(); err != nil {
			return nil, invalid(where, "invalid JSON: "+err.Error())
		}
		return list, nil
	}
	return token, nil
}

// writeCanonical writes value in the JSON Canonicalization Scheme (RFC 8785) for the values decodeStrict produces:
// object keys sorted by UTF-16 code units, no whitespace, and strings escaped as ECMAScript's JSON.stringify does.
// Numbers are integers, written as their decimal digits.
func writeCanonical(out *bytes.Buffer, value any) {
	switch value := value.(type) {
	case map[string]any:
		out.WriteByte('{')
		for i, key := range sortedKeys(value) {
			if i > 0 {
				out.WriteByte(',')
			}
			writeString(out, key)
			out.WriteByte(':')
			writeCanonical(out, value[key])
		}
		out.WriteByte('}')
	case []any:
		out.WriteByte('[')
		for i, item := range value {
			if i > 0 {
				out.WriteByte(',')
			}
			writeCanonical(out, item)
		}
		out.WriteByte(']')
	default:
		writeScalar(out, value)
	}
}

// compactKeys are the keys whose object values writePretty writes one per line: each rule, skill, and available skill.
var compactKeys = []string{"rules", "skills", "availableSkills"}

// writePretty writes value for people to read: sorted keys, two-space indentation, and each item of a list, each
// file digest, and each rule and skill on its own line. parent is the key that holds value.
func writePretty(out *bytes.Buffer, value any, indent string, parent *string) {
	switch value := value.(type) {
	case map[string]any:
		if len(value) == 0 {
			out.WriteString("{}")
			return
		}
		out.WriteString("{\n")
		inner := indent + "  "
		for i, key := range sortedKeys(value) {
			out.WriteString(inner)
			writeString(out, key)
			out.WriteString(": ")
			if parent != nil && slices.Contains(compactKeys, *parent) {
				writeInline(out, value[key])
			} else {
				writePretty(out, value[key], inner, &key)
			}
			if i < len(value)-1 {
				out.WriteByte(',')
			}
			out.WriteByte('\n')
		}
		out.WriteString(indent + "}")
	case []any:
		if len(value) == 0 {
			out.WriteString("[]")
			return
		}
		out.WriteString("[\n")
		for i, item := range value {
			out.WriteString(indent + "  ")
			writeInline(out, item)
			if i < len(value)-1 {
				out.WriteByte(',')
			}
			out.WriteByte('\n')
		}
		out.WriteString(indent + "]")
	default:
		writeScalar(out, value)
	}
}

// writeInline writes value on one line, with a space after each colon and comma.
func writeInline(out *bytes.Buffer, value any) {
	switch value := value.(type) {
	case map[string]any:
		out.WriteByte('{')
		for i, key := range sortedKeys(value) {
			if i > 0 {
				out.WriteString(", ")
			}
			writeString(out, key)
			out.WriteString(": ")
			writeInline(out, value[key])
		}
		out.WriteByte('}')
	case []any:
		out.WriteByte('[')
		for i, item := range value {
			if i > 0 {
				out.WriteString(", ")
			}
			writeInline(out, item)
		}
		out.WriteByte(']')
	default:
		writeScalar(out, value)
	}
}

// writeScalar writes a string, number, boolean, or null.
func writeScalar(out *bytes.Buffer, value any) {
	switch value := value.(type) {
	case string:
		writeString(out, value)
	case json.Number:
		out.WriteString(value.String())
	case bool:
		out.WriteString(strconv.FormatBool(value))
	case nil:
		out.WriteString("null")
	default:
		panic(fmt.Sprintf("unexpected JSON value %T", value))
	}
}

// writeString writes text as a JSON string, escaping only what ECMAScript's JSON.stringify escapes.
func writeString(out *bytes.Buffer, text string) {
	out.WriteByte('"')
	for _, r := range text {
		switch r {
		case '"':
			out.WriteString(`\"`)
		case '\\':
			out.WriteString(`\\`)
		case '\b':
			out.WriteString(`\b`)
		case '\f':
			out.WriteString(`\f`)
		case '\n':
			out.WriteString(`\n`)
		case '\r':
			out.WriteString(`\r`)
		case '\t':
			out.WriteString(`\t`)
		default:
			if r < 0x20 {
				fmt.Fprintf(out, `\u%04x`, r)
			} else {
				out.WriteRune(r)
			}
		}
	}
	out.WriteByte('"')
}

// sortedKeys returns object's keys sorted by their UTF-16 code units, as RFC 8785 orders them.
func sortedKeys(object map[string]any) []string {
	keys := slices.Collect(maps.Keys(object))
	slices.SortFunc(keys, func(a, b string) int { return slices.Compare(utf16.Encode([]rune(a)), utf16.Encode([]rune(b))) })
	return keys
}

// conflictedLines reports whether data has a line that starts with a Git merge conflict marker.
func conflictedLines(data []byte) bool {
	for line := range bytes.Lines(data) {
		if _, ok := conflictMarker(line); ok {
			return true
		}
	}
	return false
}

// conflictMarker returns which Git conflict marker line starts, '<', '|', '=', or '>', reporting whether it does.
func conflictMarker(line []byte) (byte, bool) {
	text := strings.TrimRight(string(line), "\r\n")
	for _, marker := range []string{"<<<<<<<", "|||||||", "=======", ">>>>>>>"} {
		if text == marker || strings.HasPrefix(text, marker+" ") {
			return marker[0], true
		}
	}
	return 0, false
}

// lockConflict returns the conflict error for data: the sources whose entries differ between the two sides of its
// merge conflicts, by name, with what each side locked, and the first side when both sides read as JSON. A side that
// doesn't parse as JSON names no sources.
func lockConflict(data []byte) *LockConflictError {
	ours, theirs := conflictSides(data)
	oursSources, oursRead := sideSources(ours)
	theirsSources, theirsRead := sideSources(theirs)
	names := slices.Sorted(maps.Keys(oursSources))
	for name := range theirsSources {
		if _, ok := oursSources[name]; !ok {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	result := &LockConflictError{Sources: []ConflictedSource{}}
	for _, name := range names {
		a, b := oursSources[name], theirsSources[name]
		if a.text != b.text {
			result.Sources = append(result.Sources, ConflictedSource{Name: name, Ours: a.revision, Theirs: b.revision})
		}
	}
	if oursRead && theirsRead {
		result.Ours = ours
	}
	return result
}

// conflictSides returns data with every conflict resolved to the first side, ours, and to the second, theirs,
// dropping a diff3 base section.
func conflictSides(data []byte) (ours, theirs []byte) {
	var a, b bytes.Buffer
	side := byte(0)
	for line := range bytes.Lines(data) {
		if marker, ok := conflictMarker(line); ok {
			side = marker
			if marker == '>' {
				side = 0
			}
			continue
		}
		switch side {
		case 0:
			a.Write(line)
			b.Write(line)
		case '<':
			a.Write(line)
		case '=':
			b.Write(line)
		}
	}
	return a.Bytes(), b.Bytes()
}

// sideSource is one side's entry for a source: its canonical text, to compare, and what it locked.
type sideSource struct {
	text     string
	revision *LockedRevision
}

// sideSources reads one side of a conflicted lock without verifying it, returning each source's entry and whether
// the side reads as a JSON object of sources.
func sideSources(data []byte) (map[string]sideSource, bool) {
	value, err := decodeStrict(data, LockFile)
	result := map[string]sideSource{}
	top, _ := value.(map[string]any)
	sources, ok := top["sources"].(map[string]any)
	if err != nil || !ok {
		return result, false
	}
	for name, raw := range sources {
		var canonical bytes.Buffer
		writeCanonical(&canonical, raw)
		entry, _ := raw.(map[string]any)
		revision := &LockedRevision{}
		revision.Ref, _ = entry["ref"].(string)
		revision.Commit, _ = entry["commit"].(string)
		if number, ok := entry["release"].(json.Number); ok {
			revision.Release, _ = strconv.Atoi(number.String())
		}
		result[name] = sideSource{text: canonical.String(), revision: revision}
	}
	return result, true
}
