// Serialize project writers and recover interrupted managed-output replacements.

package filetxn

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"syscall"

	"github.com/fabricahq/trailguide/internal/rules"
)

const transactionName = ".trailguide-transaction"

// backupPrefix begins the name of each backup of a target's previous output in the transaction directory.
const backupPrefix = "old-"

// maxJournalEntries is how many targets a journal of each format version can hold: format 2 holds the two managed
// trees, format 3 adds a project guide, format 4 adds the configuration, format 5 adds local group metadata, format 6
// adds local rules and their asset directories, which it can remove, format 7 adds skill folders outside the
// Trail Guide directory, which it can remove too, format 8 adds the repository's AGENTS.md, format 9 adds authored
// skill folders, and format 10 adds the lockfile.
var maxJournalEntries = map[int]int{1: 2, 2: 2, 3: 3, 4: 4, 5: 4 + maxLocalGroupMetadata, 6: 4 + maxLocalGroupMetadata + 2*maxLocalRules, 7: 4 + maxLocalGroupMetadata + 2*maxLocalRules + maxSkillFolders, 8: 5 + maxLocalGroupMetadata + 2*maxLocalRules + maxSkillFolders, 9: 5 + maxLocalGroupMetadata + 2*maxLocalRules + maxSkillFolders + maxAuthoredSkills, 10: 6 + maxLocalGroupMetadata + 2*maxLocalRules + maxSkillFolders + maxAuthoredSkills}

// maxJournalFormat is the newest journal format; Apply writes the oldest one that can hold its targets.
const maxJournalFormat = 10

const lockName = ".trailguide-lock"
const cleanupName = ".trailguide-cleanup"

// legacyLockName is the lock Code Rules took before it was renamed Trail Guide. A writer of a project still in
// legacyDirectory takes it too, before its own, so a Code Rules process can't write the project at the same time.
const legacyLockName = ".code-rules-lock"

// legacyDirectory is the project directory Code Rules used before it was renamed Trail Guide.
const legacyDirectory = ".code-rules"

// legacyNames are the lock, transaction, and cleanup names Code Rules used before it was renamed Trail Guide. Their
// presence means a Code Rules process is writing the project, or left an interrupted write that only it recovers;
// legacyLockName can also be held by a Trail Guide writer of a project in legacyDirectory.
var legacyNames = []string{legacyLockName, ".code-rules-transaction", ".code-rules-cleanup"}

// requireNoLegacyWriter fails with busy when root holds one of names, which are some of legacyNames.
func requireNoLegacyWriter(root *os.Root, names []string) error {
	for _, name := range names {
		present, err := exists(root, name)
		if err != nil {
			return err
		}
		if present {
			return failure("busy", name+": another process is writing this project, or was interrupted while writing it. Wait for it to exit; if none is running, run the same command again with the version that was interrupted, code-rules or trailguide, to recover its changes, then run this command again", nil)
		}
	}
	return nil
}

// isLegacyRoot reports whether root is a project directory still named legacyDirectory.
func isLegacyRoot(root *os.Root) bool {
	return filepath.Base(root.Name()) == legacyDirectory
}

// Target is a managed directory, project guide, the project configuration, an authored group's metadata, an
// authored rule or its asset directory, which only update replaces, when it replaces a fork, or an installed skill
// folder.
type Target string

const (
	Vendor    Target = "vendor"
	Generated Target = "generated"
)

// Writer holds exclusive project ownership only during WithWriter's callback.
// Callers must not retain it or call Apply concurrently.
type Writer struct {
	ctx  context.Context
	root *os.Root
	// rename and mkdir are the filesystem boundaries used by tests to stop a child at an exact rename or directory.
	rename func(string, string) error
	mkdir  func(string) error
	// renameRepository, when set, replaces renames in the repository root, which skill folders and AGENTS.md use,
	// for tests.
	renameRepository func(root *os.Root, from, to string) error
	active           bool
	// recovered reports that WithWriter first recovered an interrupted earlier operation, changing project files.
	recovered bool
}

// Recovered reports whether, before handing over the writer, WithWriter recovered an interrupted earlier operation,
// restoring or finishing its files, so the project changed even if the caller's own operation writes nothing.
func (w *Writer) Recovered() bool { return w.recovered }

// lockOwner records host, process, and invocation identity. Ambiguous or remote owners are never reclaimed.
type lockOwner struct {
	Host  string `json:"host"`
	PID   int    `json:"pid"`
	Token string `json:"token"`
}

// journalEntry identifies before/after output; Existed distinguishes absent from empty targets.
type journalEntry struct {
	Name    Target `json:"name"`
	Before  string `json:"before"`
	After   string `json:"after"`
	Existed bool   `json:"existed"`
}
type journalRecord struct {
	FormatVersion int            `json:"formatVersion"`
	Entries       []journalEntry `json:"entries"`
}

// RequireIdle refuses read-only operations while writing or recovery could expose mixed output.
// It never creates a lock or performs recovery.
func RequireIdle(root *os.Root) error {
	if err := requireNoLegacyWriter(root, legacyNames); err != nil {
		return err
	}
	for _, name := range []string{lockName, transactionName, cleanupName} {
		present, err := exists(root, name)
		if err != nil {
			return err
		}
		if present {
			return failure("busy", "a project writer or pending recovery exists; retry build or sync after the writer exits", nil)
		}
	}
	return nil
}

// WithWriter acquires ownership, recovers an interrupted write, and releases its own lock.
// Only a provably dead process on this host can be reclaimed. A failed recovery retains backups.
func WithWriter(ctx context.Context, root *os.Root, operation func(*Writer) error) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if root == nil || operation == nil {
		return failure("invalid-operation", "expected an open project root and writer operation", nil)
	}
	unlocked := legacyNames
	if isLegacyRoot(root) {
		// Code Rules honors only its own lock, so a project it can still write is locked under that name first.
		if err := acquireLock(ctx, root, legacyLockName); err != nil {
			return err
		}
		defer func() {
			if removeErr := root.RemoveAll(legacyLockName); removeErr != nil {
				err = errors.Join(err, removeErr)
			}
		}()
		unlocked = legacyNames[1:]
	}
	if err := requireNoLegacyWriter(root, unlocked); err != nil {
		return err
	}
	if err := acquireLock(ctx, root, lockName); err != nil {
		return err
	}
	// Joining only a failed removal keeps err itself, so callers still see its identity, such as a validation error.
	defer func() {
		if removeErr := root.RemoveAll(lockName); removeErr != nil {
			err = errors.Join(err, removeErr)
		}
	}()
	recovered := pendingTransaction(root)
	if err := recoverChanges(root); err != nil {
		return err
	}
	w := &Writer{ctx: ctx, root: root, active: true, recovered: recovered, rename: func(from, to string) error { return renameManaged(root, from, to) }, mkdir: func(name string) error { return root.Mkdir(name, 0755) }}
	defer func() { w.active = false }()
	return operation(w)
}

// acquireLock exclusively creates an owner record in lockName or serializes reclamation of a dead local owner.
func acquireLock(ctx context.Context, root *os.Root, lockName string) error {
	host, err := os.Hostname()
	if err != nil {
		return err
	}
	for attempts := 0; attempts < 3; attempts++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := root.Mkdir(lockName, 0700)
		if err == nil {
			token := make([]byte, 16)
			if _, err := rand.Read(token); err != nil {
				return errors.Join(err, root.RemoveAll(lockName))
			}
			if err := durableJSON(root, path.Join(lockName, "owner.json"), lockOwner{host, os.Getpid(), hex.EncodeToString(token)}); err != nil {
				return errors.Join(err, root.RemoveAll(lockName))
			}
			return nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("create project lock: %w", err)
		}
		tree, err := ReadTree(ctx, root, lockName)
		if err != nil {
			return err
		}
		var owner lockOwner
		if tree == nil || json.Unmarshal(tree.Files["owner.json"], &owner) != nil || owner.Host != host || owner.PID <= 0 || owner.Token == "" {
			return failure("busy", "project lock ownership is incomplete or belongs to another host; verify the owner before removing the lock", nil)
		}
		// ESRCH is the only evidence that permits reclamation; EPERM and all uncertainty fail closed.
		if err := syscall.Kill(owner.PID, 0); !errors.Is(err, syscall.ESRCH) {
			return failure("busy", "another writer is using this project or ownership cannot be verified", err)
		}
		claim := path.Join(lockName, "recovery-claim")
		if err := root.Mkdir(claim, 0700); err != nil {
			return failure("busy", "another process is recovering the project lock", err)
		}
		data, err := ReadFile(ctx, root, path.Join(lockName, "owner.json"))
		var current lockOwner
		if err != nil || json.Unmarshal(data, &current) != nil || current != owner {
			return failure("busy", "lock ownership changed during recovery; preserve its recovery claim", err)
		}
		if err := root.RemoveAll(lockName); err != nil {
			return err
		}
	}
	return failure("busy", "project lock contention; retry later", nil)
}

// Apply stages complete managed targets and rechecks caller inputs before replacing live output.
// It rolls back on failure when no later edits would be lost, and recovery restores or finishes every target in
// the transaction together. Empty output is a no-op. A nil file map removes a local rule's asset directory or a skill
// folder, and does nothing when it is absent. It changes nothing outside its transaction directory until
// assertUnchanged has passed and the journal is written; only then does it create the missing parent directories of
// local rule targets and skill folders. A rollback leaves those directories in place, empty, since nothing proves who
// made a directory at that path. Once it has removed a skill folder, it also removes the skill target's folder, such
// as .claude/skills, when that leaves it empty. Every file it writes is not executable; see ApplyModes.
// This promises recoverability, not simultaneous visibility of two renames or power-loss durability.
func (w *Writer) Apply(output map[Target]map[string][]byte, assertUnchanged func() error) error {
	return w.ApplyModes(output, nil, assertUnchanged)
}

// ApplyModes is Apply, except that the files executable lists for a tree target, by path in the target, are
// executable. Each must be a file output gives that target.
func (w *Writer) ApplyModes(output map[Target]map[string][]byte, executable map[Target][]string, assertUnchanged func() error) (err error) {
	if w == nil || !w.active {
		return failure("invalid-operation", "writer is outside its ownership callback", nil)
	}
	if err := w.ctx.Err(); err != nil {
		return err
	}
	if len(output) == 0 {
		return nil
	}
	if _, first := output[GuideReadme]; first {
		if _, second := output[GuideStandalone]; second {
			return failure("invalid-target", "only one project guide may be replaced", nil)
		}
	}
	places := &roots{project: w.root}
	defer places.close()
	order := []Target{Vendor, Generated, GuideReadme, GuideStandalone, Config, Lock, AgentsFile}
	metadata, localRules, authoredSkills, skillFolders := 0, 0, 0, 0
	for target, files := range output {
		if !validTarget(target) {
			return failure("invalid-target", fmt.Sprintf("%q: only managed trees, project guides, the configuration, the lockfile, AGENTS.md, local group metadata, local rules and their asset directories, authored skills, and skill folders may be replaced", target), nil)
		}
		if files == nil && !removable(target) {
			return failure("invalid-target", fmt.Sprintf("%q: only a local rule's asset directory or a skill folder may be removed", target), nil)
		}
		_, skill := skillFolderTarget(target)
		switch {
		case localGroupMetadataTarget(target):
			metadata++
		case localRuleTarget(target):
			localRules++
		case authoredSkillTarget(target):
			authoredSkills++
		case skill:
			skillFolders++
		}
		if localTarget(target) || skill {
			order = append(order, target)
			p, err := places.place(target)
			if err != nil {
				return err
			}
			if err := requireRealParents(p.root, p.live); err != nil {
				return err
			}
		}
		if fileTarget(target) && !localRuleTarget(target) {
			p, err := places.place(target)
			if err != nil {
				return err
			}
			if err := rejectAlias(p.root, p.live); err != nil {
				return err
			}
		}
		// AGENTS.md's one file is named for the target, outside the Trail Guide directory, which writeTarget checks.
		if err := rules.ValidatePaths(files, nil); err != nil && target != AgentsFile {
			return err
		}
	}
	for target, list := range executable {
		files, ok := output[target]
		for _, file := range list {
			if _, listed := files[file]; !ok || fileTarget(target) || !listed {
				return failure("invalid-target", fmt.Sprintf("%q: %s is executable but isn't a file of the target's output", target, file), nil)
			}
		}
	}
	// Local targets and skill folders follow the fixed targets in path order, so journals are deterministic.
	fixed := 7
	if metadata > maxLocalGroupMetadata || localRules > maxLocalRules || len(order)-fixed-metadata-localRules-authoredSkills-skillFolders > maxLocalRules || authoredSkills > maxAuthoredSkills || skillFolders > maxSkillFolders {
		return failure("invalid-target", fmt.Sprintf("one transaction can hold at most %d local group metadata files, %d local rules with their asset directories, %d authored skills, and %d skill folders", maxLocalGroupMetadata, maxLocalRules, maxAuthoredSkills, maxSkillFolders), nil)
	}
	slices.Sort(order[fixed:])
	if err := w.root.Mkdir(transactionName, 0700); err != nil {
		return err
	}
	prepared := false
	// parents lists, outermost first, the missing parent directories of local rule targets and skill folders to
	// create once the journal is written. Nothing removes them again, so a rollback leaves them, empty.
	parents := []parent{}
	defer func() {
		if err == nil {
			return
		}
		var recoveryErr error
		if prepared {
			recoveryErr = recoverChanges(w.root)
		} else {
			recoveryErr = w.root.RemoveAll(transactionName)
		}
		if recoveryErr != nil {
			err = failure("recovery-required", "update failed and rollback could not finish; preserve .trailguide-transaction for recovery", errors.Join(err, recoveryErr))
		}
	}()
	entries := []journalEntry{}
	for _, target := range order {
		files, ok := output[target]
		if !ok {
			continue
		}
		p, err := places.place(target)
		if err != nil {
			return err
		}
		before, err := readTarget(w.ctx, p.root, target, p.live)
		if err != nil {
			return err
		}
		if files == nil {
			if before != nil {
				entries = append(entries, journalEntry{target, treeDigest(before), treeDigest(nil), true})
			}
			continue
		}
		if _, skill := skillFolderTarget(target); skill || localRuleTarget(target) || localRuleAssetsTarget(target) || authoredSkillTarget(target) {
			missing, err := missingParents(p.root, path.Dir(p.live))
			if err != nil {
				return err
			}
			for _, directory := range missing {
				if !slices.Contains(parents, parent{p.root, directory}) {
					parents = append(parents, parent{p.root, directory})
				}
			}
			// A target whose parent is still missing can't collide with an existing spelling.
			if len(missing) == 0 {
				if err := rejectAlias(p.root, p.live); err != nil {
					return err
				}
			}
		}
		staged := p.entry("new-", target)
		if err := writeTarget(w.ctx, p.root, target, staged, files, executable[target]); err != nil {
			return err
		}
		if fileTarget(target) {
			// The transaction directory is the same in either root, so the probe uses the Trail Guide directory's.
			if err := w.probeFileRename(target, path.Join(transactionName, "new-"+entryName(target))); err != nil {
				return err
			}
		}
		after, err := readTarget(w.ctx, p.root, target, staged)
		if err != nil {
			return err
		}
		entries = append(entries, journalEntry{target, treeDigest(before), treeDigest(after), before != nil})
	}
	// Only removals of absent directories leave nothing to replace.
	if len(entries) == 0 {
		return w.root.RemoveAll(transactionName)
	}
	if assertUnchanged != nil {
		if err := assertUnchanged(); err != nil {
			return err
		}
	}
	// Recheck targets too, even when the caller only checks authored inputs.
	for _, entry := range entries {
		p, err := places.place(entry.Name)
		if err != nil {
			return err
		}
		current, err := readTarget(w.ctx, p.root, entry.Name, p.live)
		if err != nil {
			return err
		}
		if treeDigest(current) != entry.Before {
			return failure("concurrent-change", string(entry.Name)+": output changed during staging", nil)
		}
	}
	formatVersion := 2
	for _, entry := range entries {
		_, skill := skillFolderTarget(entry.Name)
		switch {
		case guideTarget(entry.Name):
			formatVersion = max(formatVersion, 3)
		case entry.Name == Config:
			formatVersion = max(formatVersion, 4)
		case localGroupMetadataTarget(entry.Name):
			formatVersion = max(formatVersion, 5)
		case localRuleTarget(entry.Name), localRuleAssetsTarget(entry.Name):
			formatVersion = max(formatVersion, 6)
		case skill:
			formatVersion = max(formatVersion, 7)
		case entry.Name == AgentsFile:
			formatVersion = max(formatVersion, 8)
		case authoredSkillTarget(entry.Name):
			formatVersion = max(formatVersion, 9)
		case entry.Name == Lock:
			formatVersion = 10
		}
	}
	if err := durableJSON(w.root, path.Join(transactionName, "journal.json"), journalRecord{formatVersion, entries}); err != nil {
		return err
	}
	prepared = true
	for _, directory := range parents {
		if err := w.createParent(directory); err != nil {
			return err
		}
	}
	for _, entry := range entries {
		if err := w.ctx.Err(); err != nil {
			return err
		}
		p, err := places.place(entry.Name)
		if err != nil {
			return err
		}
		if entry.Existed {
			backupPath := p.entry(backupPrefix, entry.Name)
			if err := w.renameIn(p, p.live, backupPath); err != nil {
				return err
			}
			// Inspect the tree actually displaced, including edits made after the earlier check.
			backup, err := readTarget(w.ctx, p.root, entry.Name, backupPath)
			if err != nil {
				return err
			}
			if treeDigest(backup) != entry.Before {
				return failure("concurrent-change", string(entry.Name)+": output changed before replacement; preserve its backup", nil)
			}
		}
		if removes(entry) {
			continue
		}
		if err := w.renameIn(p, p.entry("new-", entry.Name), p.live); err != nil {
			return err
		}
	}
	if err := w.ctx.Err(); err != nil {
		return err
	}
	if err := durableJSON(w.root, path.Join(transactionName, "commit-ready.json"), true); err != nil {
		return err
	}
	if err := w.rename(path.Join(transactionName, "commit-ready.json"), path.Join(transactionName, "committed.json")); err != nil {
		return err
	}
	// A durable commit marker makes later cleanup failures cleanup-only, never rollback.
	_ = finishCommitted(w.root)
	for _, entry := range entries {
		if _, skill := skillFolderTarget(entry.Name); skill && removes(entry) {
			p, err := places.place(entry.Name)
			if err == nil {
				// Removing a directory that still has entries fails, which leaves it as it is.
				_ = p.root.Remove(path.Dir(p.live))
			}
		}
	}
	return nil
}

// parent is a missing parent directory, by its path in root, of a target that Apply creates.
type parent struct {
	root      *os.Root
	directory string
}

// renameIn renames from to to, both paths in p.root, through the writer's rename boundary in the Trail Guide
// directory.
func (w *Writer) renameIn(p place, from, to string) error {
	if p.root == w.root {
		return w.rename(from, to)
	}
	if w.renameRepository != nil {
		return w.renameRepository(p.root, from, to)
	}
	return renameInRepository(p, from, to)
}

// renameInRepository renames from to to in the repository root, never replacing a file that appeared at a file
// target's path.
func renameInRepository(p place, from, to string) error {
	if p.file {
		return renameWithout(p.root, from, to, renameExclusive)
	}
	return p.root.Rename(from, to)
}

// missingParents returns, outermost first, the directories from directory up to the first existing one that don't
// exist yet. An existing one must be a directory, not a symbolic link.
func missingParents(root *os.Root, directory string) ([]string, error) {
	missing := []string{}
	for ; directory != "." && directory != ""; directory = path.Dir(directory) {
		info, err := root.Lstat(directory)
		if errors.Is(err, fs.ErrNotExist) {
			missing = append(missing, directory)
			continue
		}
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			return nil, failure("unsafe-path", directory+": expected a directory without symlinks", nil)
		}
		break
	}
	slices.Reverse(missing)
	return missing, nil
}

// createParent creates missing, a missing parent of a local rule target or a skill folder, refusing a name that
// collides with an existing spelling. A directory that appeared meanwhile is used as it is, since nothing removes
// parents; anything else there, such as a file or a symbolic link, is refused.
func (w *Writer) createParent(missing parent) error {
	root, directory := missing.root, missing.directory
	if err := rejectAlias(root, directory); err != nil {
		return err
	}
	var err error
	if root == w.root {
		err = w.mkdir(directory)
	} else {
		err = root.Mkdir(directory, 0755)
	}
	if !errors.Is(err, fs.ErrExist) {
		return err
	}
	info, err := root.Lstat(directory)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return failure("unsafe-path", directory+": expected a directory without symlinks", nil)
	}
	return nil
}

// removes reports whether entry removes its target rather than replacing it, which only a local rule's asset
// directory or a skill folder can.
func removes(entry journalEntry) bool { return entry.After == treeDigest(nil) }

// recoverChanges validates every restoration before mutating any target and ignores caller cancellation.
func recoverChanges(root *os.Root) error {
	return recoverWithRename(root, func(from, to string) error { return renameManaged(root, from, to) })
}

// pendingTransaction reports whether an interrupted operation left a transaction for recovery to restore or finish.
func pendingTransaction(root *os.Root) bool {
	_, err := root.Lstat(transactionName)
	return err == nil
}

// recoverWithRename owns recovery renames; tests use this boundary to reproduce late edits and interruptions.
func recoverWithRename(root *os.Root, rename func(string, string) error) error {
	ctx := context.Background()
	places := &roots{project: root}
	defer places.close()
	// renameIn renames within p.root, through rename in the Trail Guide directory.
	renameIn := func(p place, from, to string) error {
		if p.root == root {
			return rename(from, to)
		}
		return renameInRepository(p, from, to)
	}
	garbage, err := readTree(ctx, root, cleanupName, recoveryLimits)
	if err != nil {
		return err
	}
	if garbage != nil {
		if err := root.RemoveAll(cleanupName); err != nil {
			return err
		}
	}
	tree, err := readTree(ctx, root, transactionName, recoveryLimits)
	if err != nil {
		return err
	}
	if tree == nil {
		return nil
	}
	data, hasJournal := tree.Files["journal.json"]
	if !hasJournal {
		for _, dir := range tree.Directories {
			if strings.HasPrefix(dir, backupPrefix) {
				return failure("recovery-required", "missing journal with retained output; preserve transaction files for manual recovery", nil)
			}
		}
		for name := range tree.Files {
			if strings.HasPrefix(name, backupPrefix) && !strings.Contains(name, "/") {
				return failure("recovery-required", "missing journal with retained backup; preserve transaction files", nil)
			}
		}
		if _, ok := tree.Files["committed.json"]; ok {
			return failure("recovery-required", "missing journal with commit marker; preserve transaction files", nil)
		}
		return root.RemoveAll(transactionName)
	}
	var raw struct {
		Entries []map[string]json.RawMessage `json:"entries"`
	}
	if json.Unmarshal(data, &raw) != nil {
		return failure("recovery-required", "invalid transaction journal", nil)
	}
	for _, entry := range raw.Entries {
		for _, key := range []string{"name", "before", "after", "existed"} {
			value, ok := entry[key]
			if !ok || string(value) == "null" {
				return failure("recovery-required", "incomplete transaction entry; preserve journal", nil)
			}
		}
	}
	var record journalRecord
	if json.Unmarshal(data, &record) != nil || record.FormatVersion < 1 || record.FormatVersion > maxJournalFormat || len(record.Entries) == 0 || len(record.Entries) > maxJournalEntries[record.FormatVersion] {
		return failure("recovery-required", "invalid transaction journal; preserve it for manual recovery", nil)
	}
	seen := map[Target]bool{}
	for _, entry := range record.Entries {
		localRule := localRuleTarget(entry.Name) || localRuleAssetsTarget(entry.Name)
		_, skill := skillFolderTarget(entry.Name)
		if (!validTarget(entry.Name) || (guideTarget(entry.Name) && record.FormatVersion < 3) || (entry.Name == Config && record.FormatVersion < 4) || (localGroupMetadataTarget(entry.Name) && record.FormatVersion < 5) || (localRule && record.FormatVersion < 6) || (skill && record.FormatVersion < 7) || (entry.Name == AgentsFile && record.FormatVersion < 8) || (authoredSkillTarget(entry.Name) && record.FormatVersion < 9) || (entry.Name == Lock && record.FormatVersion < 10)) || seen[entry.Name] || !validDigest(entry.Before) || !validDigest(entry.After) || entry.Existed == (entry.Before == treeDigest(nil)) || (removes(entry) && (!removable(entry.Name) || !entry.Existed)) {
			return failure("recovery-required", "invalid transaction entry; preserve it for manual recovery", nil)
		}
		seen[entry.Name] = true
		p, err := places.place(entry.Name)
		if err != nil {
			return err
		}
		// A symbolic link in a parent could make recovery inspect, discard, or replace another file.
		if err := requireRealParents(p.root, p.live); err != nil {
			return failure("recovery-required", string(entry.Name)+": a parent directory changed into a symbolic link or file after interruption; preserve the transaction and restore the directory", err)
		}
	}
	if seen[GuideReadme] && seen[GuideStandalone] {
		return failure("recovery-required", "multiple project guides in transaction; preserve journal", nil)
	}
	marker, committed := tree.Files["committed.json"]
	if committed && string(marker) != "true\n" {
		return failure("recovery-required", "invalid commit marker; preserve transaction files", nil)
	}
	for _, entry := range record.Entries {
		p, err := places.place(entry.Name)
		if err != nil {
			return err
		}
		current, err := readTarget(ctx, p.root, entry.Name, p.live)
		if err != nil {
			return err
		}
		backup, err := readTarget(ctx, p.root, entry.Name, p.entry(backupPrefix, entry.Name))
		if err != nil {
			return err
		}
		if backup != nil && !entry.Existed {
			return failure("recovery-required", "unexpected backup for an originally absent target; preserve transaction files", nil)
		}
		if committed {
			continue
		}
		discarded, err := readTarget(ctx, p.root, entry.Name, p.entry("discarded-", entry.Name))
		if err != nil {
			return err
		}
		if discarded != nil && treeDigest(discarded) != entry.After {
			return failure("recovery-required", string(entry.Name)+": displaced output changed; preserve transaction files", nil)
		}
		if backup != nil && treeDigest(backup) != entry.Before {
			return failure("recovery-required", string(entry.Name)+": backup changed; manual recovery required", nil)
		}
		currentHash := treeDigest(current)
		safe := currentHash == entry.Before || (!entry.Existed && currentHash == entry.After)
		if backup != nil {
			safe = current == nil || currentHash == entry.After
		}
		if !safe {
			return failure("recovery-required", string(entry.Name)+": changed after interruption; preserve current files and transaction backups", nil)
		}
	}
	if committed {
		return finishCommitted(root)
	}
	// Older TypeScript recovery cannot understand quarantined output. Upgrade before
	// introducing it so that either runtime refuses to discard unknown state.
	if record.FormatVersion == 1 {
		record.FormatVersion = 2
		next := path.Join(transactionName, "journal-next.json")
		if err := root.Remove(next); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if err := durableJSON(root, next, record); err != nil {
			return err
		}
		if err := root.Rename(next, path.Join(transactionName, "journal.json")); err != nil {
			return err
		}
	}
	for _, entry := range slices.Backward(record.Entries) {
		p, err := places.place(entry.Name)
		if err != nil {
			return err
		}
		backup := p.entry(backupPrefix, entry.Name)
		present, err := exists(p.root, backup)
		if err != nil {
			return err
		}
		if present || !entry.Existed {
			// Move live output into our transaction before inspecting or deleting it.
			// A path-based edit before this rename is retained on mismatch, never removed.
			current, err := exists(p.root, p.live)
			if err != nil {
				return err
			}
			if current {
				discardedPath := p.entry("discarded-", entry.Name)
				already, err := exists(p.root, discardedPath)
				if err != nil {
					return err
				}
				if already {
					return failure("recovery-required", "both live and displaced output exist; preserve transaction files", nil)
				}
				if err := renameIn(p, p.live, discardedPath); err != nil {
					return err
				}
				discarded, err := readTarget(ctx, p.root, entry.Name, discardedPath)
				if err != nil {
					return err
				}
				if treeDigest(discarded) != entry.After {
					return failure("recovery-required", string(entry.Name)+": output changed during recovery; preserve displaced files and backup", nil)
				}
			}
		}
		if present {
			if err := renameIn(p, backup, p.live); err != nil {
				return err
			}
		}
	}
	// Parent directories the transaction created stay, even when empty: a path proves nothing about who made the
	// directory there now, and an empty directory under rules/ or in a skill target is harmless.
	return root.RemoveAll(transactionName)
}

// finishCommitted retires the journal before deleting backups, preventing interrupted cleanup rollback.
func finishCommitted(root *os.Root) error {
	if err := root.Rename(transactionName, cleanupName); err != nil {
		return err
	}
	return root.RemoveAll(cleanupName)
}

// durableJSON exclusively creates a synced journal or lock record with a trailing newline.
func durableJSON(root *os.Root, name string, value any) error {
	f, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	encodeErr := json.NewEncoder(f).Encode(value)
	var syncErr error
	if encodeErr == nil {
		syncErr = f.Sync()
	}
	return errors.Join(encodeErr, syncErr, f.Close())
}

// exists distinguishes absence from permission failures and includes symlinks as present.
func exists(root *os.Root, name string) (bool, error) {
	if root == nil {
		return false, failure("unsafe-path", "expected an open project root", nil)
	}
	_, err := root.Lstat(name)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

// validDigest accepts only the SHA-256 representation written into version-1 journals.
func validDigest(value string) bool {
	data, err := hex.DecodeString(value)
	if err != nil || len(data) != 32 {
		return false
	}
	return hex.EncodeToString(data) == value
}
