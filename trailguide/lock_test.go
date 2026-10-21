// Check the lockfile's format, integrity, and merge-conflict report through the public parser and encoder.

package trailguide_test

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/trailguide"
)

// Commits of the fixture's library releases.
var (
	lockOne = strings.Repeat("1", 40)
	lockTwo = strings.Repeat("2", 40)
)

// lockFixture returns a lock with a library on release 2 whose rules come from releases 1 and 2, curating a skill,
// and a repository that isn't a library.
func lockFixture() trailguide.Lock {
	one, two := 1, 2
	offered := "offered"
	v1, v2 := trailguide.RuleVersion{Major: 1}, trailguide.RuleVersion{Major: 1, Minor: 1}
	return trailguide.Lock{Sources: map[string]trailguide.LockedSource{
		"fabrica": {
			Kind: trailguide.KindLibrary, Repository: "https://github.com/fabricahq/public-library.git", Ref: "latest-release", Release: 2, Commit: lockTwo,
			ReleaseCommits: map[int]string{1: lockOne, 2: lockTwo},
			Selectors:      trailguide.LockSelectors{Groups: json.RawMessage(`["techs/go"]`), Rules: []string{}, Skills: []string{"skills/review"}},
			Groups:         []string{"techs/go"},
			Rules:          map[string]trailguide.LockedRule{"techs/go/errors": {Version: &v2, Release: &two}, "techs/go/naming": {Version: &v1, Release: &one, LegacyLayout: true}},
			RetiredRules:   []string{},
			Skills:         map[string]trailguide.LockedSkill{"skills/review": {Level: &offered, Included: true, UsedBy: []string{}}},
			AvailableSkills: map[string]trailguide.LockedAvailableSkill{
				"skills/review": {Level: &offered}, "pocock:skills/grill": {Level: &offered},
			},
			Files:      map[string]string{"config.yaml": strings.Repeat("a", 64), "techs/go/errors.md": strings.Repeat("b", 64)},
			Executable: []string{},
			Curated: map[string]trailguide.LockedCurated{"pocock": {
				Kind: trailguide.KindNonLibrary, Repository: "https://github.com/mattpocock/skills.git", Ref: "main", Commit: lockOne,
				Skills:     map[string]trailguide.LockedSkill{"skills/grill": {Level: &offered, Included: false, UsedBy: []string{}, UsedByLibrary: []string{"skills/review"}}},
				Files:      map[string]string{"skills/grill/SKILL.md": strings.Repeat("c", 64)},
				Executable: []string{},
			}},
		},
		"tools": {
			Kind: trailguide.KindNonLibrary, Repository: "https://github.com/acme/tools.git", Ref: "main", Commit: lockOne,
			Selectors:       trailguide.LockSelectors{Skills: []string{"*"}},
			Skills:          map[string]trailguide.LockedSkill{"tools/plan": {Included: true, UsedBy: []string{}}},
			AvailableSkills: map[string]trailguide.LockedAvailableSkill{"tools/plan": {}},
			Files:           map[string]string{"tools/plan/SKILL.md": strings.Repeat("d", 64)},
			Executable:      []string{},
		},
	}}
}

// encodeFixture returns the fixture's lockfile text.
func encodeFixture(t *testing.T) string {
	t.Helper()
	data, err := lockFixture().Encode()
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// TestLock_RoundTripsAndComputesIntegrity parses what Encode writes back to the same entries, each with the integrity
// EntryIntegrity computes, and encodes it again byte for byte.
func TestLock_RoundTripsAndComputesIntegrity(t *testing.T) {
	text := encodeFixture(t)
	parsed, err := trailguide.ParseLock([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	want := lockFixture()
	for name, entry := range want.Sources {
		if entry.Integrity, err = trailguide.EntryIntegrity(name, entry); err != nil || !strings.HasPrefix(entry.Integrity, "sha256-") {
			t.Fatalf("%s: %q, %v", name, entry.Integrity, err)
		}
		want.Sources[name] = entry
	}
	if !reflect.DeepEqual(parsed, want) {
		t.Fatalf("round trip:\n%+v\nwant\n%+v", parsed, want)
	}
	again, err := parsed.Encode()
	if err != nil || string(again) != text {
		t.Fatalf("encoding again: %v\n%s", err, again)
	}
	for _, line := range []string{
		`  "lockVersion": 1,`,
		`      "ref": "latest-release",`,
		`        "techs/go/naming": {"legacyLayout": true, "release": 1, "version": "1.0.0"}`,
		`        "skills/review": {"included": true, "level": "offered", "usedBy": []}`,
		`        "1": "` + lockOne + `",`,
	} {
		if !strings.Contains(text, line+"\n") {
			t.Errorf("the lock lacks the line\n%s\nin\n%s", line, text)
		}
	}
}

// TestLock_IntegrityIsTheCanonicalFormsDigest computes integrity as documented: the SHA-256 of the RFC 8785 form of
// the entry, without integrity, and the source's name, so an entry moved to another name fails too.
func TestLock_IntegrityIsTheCanonicalFormsDigest(t *testing.T) {
	entry := trailguide.LockedSource{Kind: trailguide.KindNonLibrary, Repository: "https://example.com/a b<c>.git", Ref: "main", Commit: lockOne,
		Selectors: trailguide.LockSelectors{Skills: []string{"\u0007"}}, Skills: map[string]trailguide.LockedSkill{}, AvailableSkills: map[string]trailguide.LockedAvailableSkill{"ﬁ": {}, "\U0001F600": {}}, Files: map[string]string{}, Executable: []string{}}
	// Keys sort by UTF-16 code units, so the emoji's surrogate pair sorts before U+FB01, though its UTF-8 bytes sort
	// after; only control characters are escaped.
	canonical := `{"entry":{"availableSkills":{"` + "\U0001F600" + `":{"level":null},"` + "ﬁ" + `":{"level":null}},"commit":"` + lockOne + `","executable":[],"files":{},"kind":"non-library","ref":"main","repository":"https://example.com/a` + " " + `b<c>.git","selectors":{"skills":["\u0007"]},"skills":{}},"name":"tools"}`
	sum := sha256.Sum256([]byte(canonical))
	want := "sha256-" + base64.StdEncoding.EncodeToString(sum[:])
	if got, err := trailguide.EntryIntegrity("tools", entry); err != nil || got != want {
		t.Fatalf("got %s, %v; want %s", got, err, want)
	}
	if other, _ := trailguide.EntryIntegrity("other", entry); other == want {
		t.Fatal("integrity doesn't cover the source's name")
	}
}

// TestLock_RefusesAnEntryChangedOutsideTrailGuide reports, at the entry's integrity, any change to an entry's
// fields, added or removed ones included, and reformatting changes nothing.
func TestLock_RefusesAnEntryChangedOutsideTrailGuide(t *testing.T) {
	text := encodeFixture(t)
	compact := map[string]any{}
	if err := json.Unmarshal([]byte(text), &compact); err != nil {
		t.Fatal(err)
	}
	reformatted, _ := json.Marshal(compact)
	if _, err := trailguide.ParseLock(reformatted); err != nil {
		t.Fatalf("reformatting: %v", err)
	}
	for name, edit := range map[string]func(string) string{
		"changed commit": func(text string) string {
			return strings.Replace(text, `"commit": "`+lockOne, `"commit": "`+lockTwo, 1)
		},
		"added field": func(text string) string {
			return strings.Replace(text, `"kind": "non-library",`, `"kind": "non-library", "pins": {},`, 1)
		},
		"moved entry": func(text string) string { return strings.Replace(text, `"tools": {`, `"utils": {`, 1) },
		"removed integrity": func(text string) string {
			start := strings.Index(text, `      "integrity": `)
			return text[:start] + text[strings.Index(text[start:], "\n")+start+1:]
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, refused, err := trailguide.ParseLockEntries([]byte(edit(text)))
			if err != nil || len(refused) != 1 {
				t.Fatalf("got %v, %v; want one refused entry", refused, err)
			}
			for source, err := range refused {
				var invalid *trailguide.ValidationError
				if !errors.As(err, &invalid) || invalid.Location != "lock.json.sources."+source+".integrity" || !strings.Contains(invalid.Problem, "changed outside Trail Guide") {
					t.Fatalf("%s: %v", source, err)
				}
			}
			if _, err := trailguide.ParseLock([]byte(edit(text))); err == nil {
				t.Fatal("ParseLock accepted the changed entry")
			}
		})
	}
}

// TestLock_RefusesInconsistentEntries refuses to encode an entry whose releaseCommits doesn't match its rules, or
// whose commit, file digest, or kind is malformed.
func TestLock_RefusesInconsistentEntries(t *testing.T) {
	for name, test := range map[string]struct {
		edit func(*trailguide.LockedSource)
		want string
	}{
		"rule from an unlisted release": {func(s *trailguide.LockedSource) { delete(s.ReleaseCommits, 1) }, "comes from library release 1, which releaseCommits doesn't list"},
		"release that no rule uses":     {func(s *trailguide.LockedSource) { s.ReleaseCommits[3] = lockOne }, "lists library release 3, which no rule comes from"},
		"release with another commit":   {func(s *trailguide.LockedSource) { s.ReleaseCommits[2] = lockOne }, "expected library release 2, the entry's release, to map to its commit"},
		"short commit":                  {func(s *trailguide.LockedSource) { s.Commit = "abc123" }, "expected a full lowercase commit SHA"},
		"bad file digest":               {func(s *trailguide.LockedSource) { s.Files["config.yaml"] = "ABC" }, "expected a lowercase hexadecimal SHA-256"},
		"unknown kind":                  {func(s *trailguide.LockedSource) { s.Kind = "plugin" }, "expected library or non-library"},
	} {
		t.Run(name, func(t *testing.T) {
			lock := lockFixture()
			entry := lock.Sources["fabrica"]
			test.edit(&entry)
			lock.Sources["fabrica"] = entry
			if _, err := lock.Encode(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("got %v; want %q", err, test.want)
			}
		})
	}
}

// TestLock_RefusesDuplicateKeysAndNewerFormats refuses a key twice at any depth, as a merge that kept both sides
// leaves it, and asks to upgrade for a newer lockVersion.
func TestLock_RefusesDuplicateKeysAndNewerFormats(t *testing.T) {
	text := encodeFixture(t)
	duplicated := strings.Replace(text, `      "kind": "non-library",`, `      "kind": "non-library",`+"\n"+`      "kind": "non-library",`, 1)
	if _, err := trailguide.ParseLock([]byte(duplicated)); err == nil || !strings.Contains(err.Error(), "appears twice") {
		t.Fatalf("duplicate key: %v", err)
	}
	newer := strings.Replace(text, `"lockVersion": 1`, `"lockVersion": 2`, 1)
	var unsupported *trailguide.UnsupportedLockError
	if _, err := trailguide.ParseLock([]byte(newer)); !errors.As(err, &unsupported) || unsupported.Version != 2 || !strings.Contains(err.Error(), "upgrade Trail Guide") {
		t.Fatalf("newer format: %v", err)
	}
	if _, err := trailguide.ParseLock([]byte(strings.Replace(text, `"lockVersion": 1`, `"lockVersion": 0`, 1))); err == nil {
		t.Fatal("accepted lockVersion 0")
	}
}

// TestLock_ReportsEachConflictedSource lists, by name, each source whose entry differs between the sides of a merge
// conflict, with what each side locked, and leaves out a source both sides agree on, with or without a diff3 base.
func TestLock_ReportsEachConflictedSource(t *testing.T) {
	ours := encodeFixture(t)
	lock := lockFixture()
	entry := lock.Sources["tools"]
	entry.Commit = lockTwo
	lock.Sources["tools"] = entry
	data, err := lock.Encode()
	if err != nil {
		t.Fatal(err)
	}
	theirs := string(data)
	start := strings.Index(ours, `    "tools": {`)
	block := func(text string) string { return text[start : start+strings.Index(text[start:], "\n    }")+6] }
	for name, base := range map[string]string{"two sides": "", "diff3": "||||||| base\n" + block(ours) + "\n"} {
		t.Run(name, func(t *testing.T) {
			conflicted := ours[:start] + "<<<<<<< HEAD\n" + block(ours) + "\n" + base + "=======\n" + block(theirs) + "\n>>>>>>> theirs\n" + ours[start+len(block(ours))+1:]
			_, err := trailguide.ParseLock([]byte(conflicted))
			var conflict *trailguide.LockConflictError
			if !errors.As(err, &conflict) || len(conflict.Sources) != 1 || conflict.Sources[0].Name != "tools" {
				t.Fatalf("got %v", err)
			}
			source := conflict.Sources[0]
			if source.Ours.Commit != lockOne || source.Theirs.Commit != lockTwo || source.Ours.Ref != "main" {
				t.Fatalf("sides %+v, %+v", source.Ours, source.Theirs)
			}
			if !strings.Contains(err.Error(), "tools: ours ref main, commit "+lockOne+", theirs ref main, commit "+lockTwo) || !strings.Contains(err.Error(), "never lines from both") {
				t.Fatalf("message: %v", err)
			}
		})
	}
}

// TestLock_ParseLockEntriesKeepsTheOthers accepts every entry but one changed outside Trail Guide.
func TestLock_ParseLockEntriesKeepsTheOthers(t *testing.T) {
	text := encodeFixture(t)
	start := strings.Index(text, `    "tools": {`)
	text = text[:start] + strings.Replace(text[start:], `"commit": "`+lockOne, `"commit": "`+lockTwo, 1)
	lock, refused, err := trailguide.ParseLockEntries([]byte(text))
	if err != nil || len(refused) != 1 || refused["tools"] == nil || len(lock.Sources) != 1 || lock.Sources["fabrica"].Commit != lockTwo {
		t.Fatalf("got %+v, %v, %v", lock, refused, err)
	}
}
