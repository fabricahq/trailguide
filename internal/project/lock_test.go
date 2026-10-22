// Exercise the lockfile through sync's and offline commands' boundary: migration from per-source records, the
// layouts readRecords refuses, and retaking a source.

package project

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/errs"
	"github.com/fabricahq/trailguide/internal/filetxn"
	"github.com/fabricahq/trailguide/internal/imports"
	"github.com/fabricahq/trailguide/internal/rules"
	"github.com/fabricahq/trailguide/trailguide"
)

// encodeStored returns what sync stores for snapshots: each vendored file by its path in vendor/, and the lockfile at
// lockFile, which no vendored path can take, since each is in a source's folder.
func encodeStored(config rules.Configuration, snapshots map[string]snapshot) (map[string][]byte, error) {
	vendor, lock, err := encodeSnapshots(config, snapshots, sourceRecords{})
	if err != nil {
		return nil, err
	}
	vendor[lockFile] = lock
	return vendor, nil
}

// decodeStored decodes what encodeStored returns, as an offline command reads it.
func decodeStored(config rules.Configuration, files map[string][]byte) (map[string]snapshot, error) {
	vendor := maps.Clone(files)
	lock, found := vendor[lockFile]
	delete(vendor, lockFile)
	if !found {
		lock = nil
	}
	stored, err := readRecords(config, lock, vendor, currentLayout, false, nil)
	if err != nil {
		return nil, err
	}
	return decodeSnapshots(config, stored, vendor)
}

// stateSnapshots decodes the snapshots of a project read with readProject, as offline commands do.
func stateSnapshots(state projectState) (map[string]snapshot, error) {
	stored, err := state.records()
	if err != nil {
		return nil, err
	}
	return decodeSnapshots(state.config, stored, treeFiles(state.vendor))
}

// lockEntry returns the entry of the source name in lock.
func lockEntry(t *testing.T, lock []byte, name string) trailguide.LockedSource {
	t.Helper()
	parsed, err := trailguide.ParseLock(lock)
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := parsed.Sources[name]
	if !ok {
		t.Fatalf("the lock has no entry for %s:\n%s", name, lock)
	}
	return entry
}

// editRecord returns files, as encodeStored returns them, with the lock entry of the source name converted to the
// record it holds, changed by edit, and converted back, its integrity computed again, as if sync had written it. It
// returns the error of an edit the lockfile can't hold.
func editRecord(t *testing.T, files map[string][]byte, name string, edit func(*sourceRecord)) (map[string][]byte, error) {
	t.Helper()
	parsed, err := trailguide.ParseLock(files[lockFile])
	if err != nil {
		t.Fatal(err)
	}
	record := entrySourceRecord(parsed.Sources[name])
	edit(&record)
	entry, err := lockedSource(record, parsed.Sources[name].Ref, entryLocation(name))
	if err != nil {
		return nil, err
	}
	parsed.Sources[name] = entry
	lock, err := parsed.Encode()
	if err != nil {
		return nil, err
	}
	edited := maps.Clone(files)
	edited[lockFile] = lock
	return edited, nil
}

// legacyStored returns what Trail Guide 0.5 stored for snapshots: each vendored file by its path in vendor/, and each
// source's record, in format 3, at <source>/_source.json, with no lockfile.
func legacyStored(t *testing.T, config rules.Configuration, snapshots map[string]snapshot) map[string][]byte {
	t.Helper()
	files, err := encodeStored(config, snapshots)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := trailguide.ParseLock(files[lockFile])
	if err != nil {
		t.Fatal(err)
	}
	delete(files, lockFile)
	for name, entry := range parsed.Sources {
		record := entrySourceRecord(entry)
		record.FormatVersion = sourceRecordFormat
		record.Ref = snapshots[name].Ref.String()
		if len(record.RuleSelection) == 0 {
			record.RuleSelection = nil
		}
		record.Checksum = recordChecksum(record)
		var encoded bytes.Buffer
		encoder := json.NewEncoder(&encoded)
		encoder.SetEscapeHTML(false)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(record); err != nil {
			t.Fatal(err)
		}
		files[name+"/"+legacyRecord] = encoded.Bytes()
	}
	return files
}

// decodeLegacy decodes what legacyStored returns, as sync reads it to migrate, checking each record against
// configuration as offline commands check lock entries.
func decodeLegacy(config rules.Configuration, files map[string][]byte) (map[string]snapshot, error) {
	stored, err := readRecords(config, nil, files, currentLayout, true, nil)
	if err != nil {
		return nil, err
	}
	vendor := maps.Clone(files)
	maps.DeleteFunc(vendor, func(name string, _ []byte) bool { return strings.HasSuffix(name, "/"+legacyRecord) })
	return decodeSnapshots(config, stored, vendor)
}

// rewriteEntry rewrites the lock entry of the source name in the project at root with edit applied, its integrity
// computed again, so the tampering is self-consistent and only deeper checks can catch it.
func rewriteEntry(t *testing.T, root *os.Root, name string, edit func(*sourceRecord)) {
	t.Helper()
	data, err := root.ReadFile(lockFile)
	if err != nil {
		t.Fatal(err)
	}
	files, err := editRecord(t, map[string][]byte{lockFile: data}, name, edit)
	if err != nil {
		t.Fatal(err)
	}
	writeFixture(t, root, lockFile, string(files[lockFile]))
}

// lockedEntry returns the parsed lock entry of the source name in the project of options.
func lockedEntry(t *testing.T, options Options, name string) trailguide.LockedSource {
	t.Helper()
	return lockEntry(t, []byte(readRepositoryFile(t, options, ".trailguide/"+lockFile)), name)
}

// entryFields returns the lock entry of the source name in the project of options, as JSON fields.
func entryFields(t *testing.T, options Options, name string) map[string]any {
	t.Helper()
	var lock struct {
		Sources map[string]map[string]any `json:"sources"`
	}
	if err := json.Unmarshal([]byte(readRepositoryFile(t, options, ".trailguide/"+lockFile)), &lock); err != nil {
		t.Fatal(err)
	}
	return lock.Sources[name]
}

// entryText returns the text of the lock entry of the source name in the project of options, as sync wrote it.
func entryText(t *testing.T, options Options, name string) string {
	t.Helper()
	lock := readRepositoryFile(t, options, ".trailguide/"+lockFile)
	start := strings.Index(lock, "\n    "+strconv.Quote(name)+": {")
	if start < 0 {
		t.Fatalf("the lock has no entry for %s:\n%s", name, lock)
	}
	end := strings.Index(lock[start:], "\n    }")
	return lock[start : start+end+6]
}

// toLegacy rewrites the synced project of options as Trail Guide 0.5 left it: no lockfile, and each source's record
// in format 3 at vendor/<source>/_source.json. It returns the lockfile it removed.
func toLegacy(t *testing.T, options Options) []byte {
	t.Helper()
	root, err := openProject(context.Background(), options, false)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	lock, err := root.ReadFile(lockFile)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := trailguide.ParseLock(lock)
	if err != nil {
		t.Fatal(err)
	}
	for name, entry := range parsed.Sources {
		record := entrySourceRecord(entry)
		record.FormatVersion = sourceRecordFormat
		if ref, err := rules.ParseGitRef(entry.Ref, "ref"); err != nil || entry.Kind == trailguide.KindLibrary && entry.Release != 0 && ref.FollowsReleases() {
			record.Ref = ""
		}
		if len(record.RuleSelection) == 0 {
			record.RuleSelection = nil
		}
		record.Checksum = recordChecksum(record)
		encoded, err := json.MarshalIndent(record, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		writeFixture(t, root, "_managed/vendor/"+name+"/"+legacyRecord, string(encoded)+"\n")
	}
	if err := root.Remove(lockFile); err != nil {
		t.Fatal(err)
	}
	toLegacyLayout(t, root)
	return lock
}

// toLegacyLayout moves the managed trees of the project at root to where Trail Guide 0.6 and earlier kept them.
func toLegacyLayout(t *testing.T, root *os.Root) {
	t.Helper()
	for from, to := range map[string]string{"_managed/vendor": "vendor", "_managed/generated": "generated"} {
		if err := root.Rename(from, to); err != nil {
			t.Fatal(err)
		}
	}
	if err := root.Remove("_managed"); err != nil {
		t.Fatal(err)
	}
}

// legacyProject returns a project of team at latest-release, synced at library release 1 and left as Trail Guide
// 0.5 wrote it, after the library published release 2, so a migration that moved the source would show. It also
// returns the lockfile sync wrote at release 1.
func legacyProject(t *testing.T) (Options, imports.Options, []byte) {
	t.Helper()
	f, options, git := syncProject(t)
	configure(t, options, f, `"rules":{"include":["techs/go/*"]},"ref":"latest-release"`)
	if _, err := Sync(context.Background(), options, git); err != nil {
		t.Fatal(err)
	}
	lock := toLegacy(t, options)
	secondRelease(t, f)
	return options, git, lock
}

// TestSync_MigratesPerSourceRecordsWithoutMovingASource: every other command asks to migrate a project Trail Guide
// 0.5 synced, writing nothing, and sync migrates it in one transaction: it writes the lockfile a sync at the same
// revision writes, removes the records, keeps every vendored file, and moves no source, though a newer release
// exists. The migrated project is current offline.
func TestSync_MigratesPerSourceRecordsWithoutMovingASource(t *testing.T) {
	ctx := context.Background()
	options, git, lock := legacyProject(t)
	before := projectTree(t, options)
	_, buildErr := Build(ctx, options)
	_, checkErr := Check(ctx, options)
	_, updateErr := PlanUpdate(ctx, options, git, nil, nil)
	for name, err := range map[string]error{"build": buildErr, "check": checkErr, "update": updateErr} {
		var failure *filetxn.Error
		if !errors.As(err, &failure) || failure.Code != "lock-migration-needed" || !strings.Contains(err.Error(), "Run trailguide project sync once to migrate it") {
			t.Errorf("%s: %v", name, err)
		}
	}
	if projectTree(t, options).Digest() != before.Digest() {
		t.Fatal("a command that asked to migrate changed the project")
	}
	changes, err := Sync(ctx, options, git)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(changes.Added, lockFile) || !slices.Contains(changes.Removed, "vendor/team/_source.json") || !slices.Contains(changes.Added, "_managed/generated/RULES.md") || !slices.Contains(changes.Removed, "generated/RULES.md") {
		t.Errorf("changes = %+v; want the lockfile added, the record removed, and the managed trees moved", changes)
	}
	after := projectTree(t, options)
	if !bytes.Equal(after.Files[lockFile], lock) {
		t.Fatalf("migration wrote:\n%s\nwant what sync wrote at the same revision:\n%s", after.Files[lockFile], lock)
	}
	for name, data := range before.Files {
		moved := "_managed/" + name
		if strings.HasPrefix(name, "vendor/") && !strings.HasSuffix(name, "/"+legacyRecord) && !bytes.Equal(after.Files[moved], data) {
			t.Errorf("migration changed %s", name)
		}
		if strings.HasSuffix(name, "/"+legacyRecord) && after.Files[moved] != nil {
			t.Errorf("migration kept %s", name)
		}
	}
	for _, name := range []string{"vendor", "generated"} {
		if _, err := os.Stat(filepath.Join(options.Directory, ".trailguide", name)); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf(".trailguide/%s: %v, want it moved under _managed/", name, err)
		}
	}
	requireCurrent(t, options)
}

// TestSync_RefusesToMigrateFromAnIncompleteLayout writes nothing for every state a finished migration can't start
// from, naming the way out, and --retake resolves a source whose record is missing again at its ref.
func TestSync_RefusesToMigrateFromAnIncompleteLayout(t *testing.T) {
	ctx := context.Background()
	for _, test := range []struct {
		name, code string
		alter      func(t *testing.T, root *os.Root, lock []byte)
		retake     bool
	}{
		// With no record left, the project can't tell a lost record from a lost lockfile.
		{"missing record", "lock-missing", func(t *testing.T, root *os.Root, _ []byte) {
			if err := root.Remove("vendor/team/" + legacyRecord); err != nil {
				t.Fatal(err)
			}
		}, true},
		{"vendor folder of no source", "lock-migration-blocked", func(t *testing.T, root *os.Root, _ []byte) {
			writeFixture(t, root, "vendor/gone/rules/techs/go/errors.md", projectRule)
		}, false},
		{"changed vendored file", "", func(t *testing.T, root *os.Root, _ []byte) {
			writeFixture(t, root, "vendor/team/LICENSE", "Changed terms\n")
		}, true},
		{"mixed layout", "mixed-layout", func(t *testing.T, root *os.Root, lock []byte) {
			writeFixture(t, root, lockFile, string(lock))
		}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			options, git, lock := legacyProject(t)
			root, err := openProject(ctx, options, false)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			test.alter(t, root, lock)
			before := projectTree(t, options)
			_, err = Sync(ctx, options, git)
			var failure *filetxn.Error
			var invalid errs.ValidationError
			if test.code != "" && (!errors.As(err, &failure) || failure.Code != test.code) || test.code == "" && !errors.As(err, &invalid) {
				t.Fatalf("got %v; want %q", err, test.code)
			}
			if test.retake && !strings.Contains(err.Error(), "trailguide project sync --retake team") {
				t.Errorf("%v doesn't name --retake", err)
			}
			if projectTree(t, options).Digest() != before.Digest() {
				t.Fatal("a refused migration changed the project")
			}
			if test.retake {
				if _, err := Sync(ctx, options, git, "team"); err != nil {
					t.Fatal(err)
				}
				if release := lockEntry(t, projectTree(t, options).Files[lockFile], "team").Release; release != 2 {
					t.Errorf("retaking team at latest-release took release %d, not the newest, 2", release)
				}
				requireCurrent(t, options)
			}
		})
	}
}

// TestProject_RefusesAMissingLockfile: a project whose vendor/ has files but whose lockfile is gone fails in every
// command, never resolving its sources again, and sync --retake for every source is the explicit way to.
func TestProject_RefusesAMissingLockfile(t *testing.T) {
	ctx := context.Background()
	options, git, _ := legacyProject(t)
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	root, err := openProject(ctx, options, false)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := root.Remove(lockFile); err != nil {
		t.Fatal(err)
	}
	before := projectTree(t, options)
	_, syncErr := Sync(ctx, options, git)
	_, checkErr := Check(ctx, options)
	for name, err := range map[string]error{"sync": syncErr, "check": checkErr} {
		var failure *filetxn.Error
		if !errors.As(err, &failure) || failure.Code != "lock-missing" || !strings.Contains(err.Error(), "git checkout -- .trailguide/lock.json") {
			t.Errorf("%s: %v", name, err)
		}
	}
	if projectTree(t, options).Digest() != before.Digest() {
		t.Fatal("a refusal changed the project")
	}
	// With no source configured, sync can't tell what the vendor folder holds either, so it refuses rather than
	// discarding it.
	config := readRepositoryFile(t, options, ".trailguide/config.yaml")
	writeFixture(t, root, configurationFile, "schemaVersion: 2\nsources: {}\n")
	if _, err := Sync(ctx, options, git); !strings.Contains(fmt.Sprint(err), "delete .trailguide/_managed/vendor/") {
		t.Fatalf("sync with no source: %v", err)
	}
	if _, err := root.Stat("_managed/vendor/team"); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, root, configurationFile, config)
	if _, err := Sync(ctx, options, git, "team"); err != nil {
		t.Fatal(err)
	}
	requireCurrent(t, options)
}

// lockedProject returns a project synced at library release 1 with ref latest-release, after the library publishes
// release 2, so any sync that resolves the source again moves it.
func lockedProject(t *testing.T) (Options, imports.Options) {
	t.Helper()
	f, options, git := syncProject(t)
	configure(t, options, f, `"rules":{"include":["techs/go/*"]},"ref":"latest-release"`)
	if _, err := Sync(context.Background(), options, git); err != nil {
		t.Fatal(err)
	}
	secondRelease(t, f)
	return options, git
}

// requireRefusal fails the test unless every command in errs failed with code, and the project is still before.
func requireRefusal(t *testing.T, options Options, before *filetxn.Tree, code string, errs map[string]error) {
	t.Helper()
	for name, err := range errs {
		var failure *filetxn.Error
		if !errors.As(err, &failure) || failure.Code != code {
			t.Errorf("%s: %v; want %s", name, err, code)
		}
	}
	if projectTree(t, options).Digest() != before.Digest() {
		t.Fatal("a refusal changed the project")
	}
}

// TestSync_RefusesALockEntryRemovedOutsideTrailGuide: a configured source whose vendor folder has files but whose
// lock entry is gone, such as after a merge resolution dropped it, isn't resolved again as a new source, which would
// move it; sync and check refuse it, writing nothing, and --retake resolves it again.
func TestSync_RefusesALockEntryRemovedOutsideTrailGuide(t *testing.T) {
	ctx := context.Background()
	options, git := lockedProject(t)
	root, err := openProject(ctx, options, false)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	writeFixture(t, root, lockFile, `{"lockVersion": 1, "sources": {}}`+"\n")
	before := projectTree(t, options)
	_, syncErr := Sync(ctx, options, git)
	_, checkErr := Check(ctx, options)
	requireRefusal(t, options, before, "lock-entry-missing", map[string]error{"sync": syncErr, "check": checkErr})
	if !strings.Contains(syncErr.Error(), "sync --retake team") {
		t.Fatalf("sync: %v", syncErr)
	}
	if _, err := Sync(ctx, options, git, "team"); err != nil {
		t.Fatal(err)
	}
	if entry := lockedEntry(t, options, "team"); entry.Release != 2 {
		t.Fatalf("retaken at release %d; want 2", entry.Release)
	}
	requireCurrent(t, options)
}

// TestSync_RetakesEachConflictedSourceThroughConflictMarkers: a lockfile with merge conflict markers fails every
// command, naming the conflicted source, and sync with --retake for every conflicted source resolves each again at
// its ref, keeping the entries both sides agree on, and writes a lockfile without markers.
func TestSync_RetakesEachConflictedSourceThroughConflictMarkers(t *testing.T) {
	ctx := context.Background()
	options, git := lockedProject(t)
	root, err := openProject(ctx, options, false)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	ours := readRepositoryFile(t, options, ".trailguide/lock.json")
	start := strings.Index(ours, `    "team": {`)
	end := start + strings.Index(ours[start:], "\n    }") + len("\n    }")
	block := ours[start:end]
	theirs := strings.Replace(block, `"ref": "latest-release"`, `"ref": "release/2"`, 1)
	writeFixture(t, root, lockFile, ours[:start]+"<<<<<<< HEAD\n"+block+"\n=======\n"+theirs+"\n>>>>>>> theirs"+ours[end:])
	before := projectTree(t, options)
	_, syncErr := Sync(ctx, options, git)
	_, checkErr := Check(ctx, options)
	requireRefusal(t, options, before, "lock-conflict", map[string]error{"sync": syncErr, "check": checkErr})
	if !strings.Contains(syncErr.Error(), "team: ours ref latest-release") {
		t.Fatalf("sync: %v", syncErr)
	}
	if _, err := Sync(ctx, options, git, "team"); err != nil {
		t.Fatal(err)
	}
	if text := readRepositoryFile(t, options, ".trailguide/lock.json"); strings.Contains(text, "<<<<<<<") {
		t.Fatalf("lock.json kept conflict markers:\n%s", text)
	}
	requireCurrent(t, options)
}

// TestSync_RetakingEverySourceRecoversAnUnreadableLockfile: a lockfile that doesn't read at all fails every command,
// writing nothing, unless sync resolves every configured source again.
func TestSync_RetakingEverySourceRecoversAnUnreadableLockfile(t *testing.T) {
	ctx := context.Background()
	options, git := lockedProject(t)
	root, err := openProject(ctx, options, false)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	writeFixture(t, root, lockFile, `{"lockVersion": 1, "sources": {`)
	before := projectTree(t, options)
	_, syncErr := Sync(ctx, options, git)
	_, checkErr := Check(ctx, options)
	for name, err := range map[string]error{"sync": syncErr, "check": checkErr} {
		if err == nil || !strings.Contains(err.Error(), "--retake for every source") {
			t.Errorf("%s: %v", name, err)
		}
	}
	if projectTree(t, options).Digest() != before.Digest() {
		t.Fatal("a refusal changed the project")
	}
	if _, err := Sync(ctx, options, git, "team"); err != nil {
		t.Fatal(err)
	}
	requireCurrent(t, options)
}
