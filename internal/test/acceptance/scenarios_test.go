// Package acceptance exercises complete CLI lifecycles in disposable projects.
package acceptance

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/fabricahq/trailguide/internal/filetxn"
	"github.com/fabricahq/trailguide/internal/test/gitfixture"
)

// Step records an actual command or a deliberate fixture edit and its observable outcome.
type Step struct {
	Label     string   `json:"label"`
	Arguments []string `json:"arguments,omitempty"`
	Stdout    string   `json:"stdout,omitempty"`
	Stderr    string   `json:"stderr,omitempty"`
	ExitCode  int      `json:"exitCode"`
}

// Report retains commands, verified invariants, and exact project bytes for inspection.
type Report struct {
	Scenario string            `json:"scenario"`
	Steps    []Step            `json:"steps"`
	Verified []string          `json:"verified"`
	Files    map[string][]byte `json:"files"`
}

// timing describes why and when a step ended: the scenario context's error, the signal that ended the process when
// one did, and how long the step and the scenario had run.
func timing(ctx context.Context, step, scenario time.Time, signal string) string {
	text := fmt.Sprintf("context: %v; step took %s; scenario ran %s", ctx.Err(), time.Since(step).Round(time.Millisecond), time.Since(scenario).Round(time.Millisecond))
	if deadline, ok := ctx.Deadline(); ok {
		text += fmt.Sprintf("; deadline %s after the scenario started", deadline.Sub(scenario).Round(time.Second))
	}
	if signal != "" {
		text += "; ended by signal " + signal
	}
	return text
}

// Run exercises a real native CLI from library authoring through Git sync and offline checks.
// All writes and Git history belong to disposable directories; no external network or global install is used.
func Run(ctx context.Context, binary, scenario string) (report Report, err error) {
	report = Report{Scenario: scenario, Steps: []Step{}, Verified: []string{}}
	started := time.Now()
	if !slices.Contains([]string{"lifecycle", "stale-output", "versions", "changed-vendor", "failed-sync"}, scenario) {
		return report, fmt.Errorf("unknown acceptance scenario")
	}
	directory, err := os.MkdirTemp("", "trailguide-acceptance-")
	if err != nil {
		return report, err
	}
	defer func() { err = errors.Join(err, os.RemoveAll(directory)) }()
	libraryDir := filepath.Join(directory, "library")
	consumer := filepath.Join(directory, "consumer")
	for _, dir := range []string{libraryDir, consumer} {
		if err := os.Mkdir(dir, 0700); err != nil {
			return report, err
		}
	}
	// Keep the observed project even when a command or invariant fails, before temporary cleanup.
	defer func() {
		captureCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		final, captureErr := readTree(captureCtx, consumer)
		if captureErr != nil {
			err = errors.Join(err, fmt.Errorf("capture acceptance project files: %w", captureErr))
			return
		}
		report.Files = final.Files
	}()
	offline := []string{"PATH=" + filepath.Join(directory, "no-runtime")}
	// invoke captures real exit status and fails the pilot when it differs from the stated scenario.
	invoke := func(label, dir string, env []string, want int, args ...string) error {
		var original *filetxn.Tree
		var err error
		if len(args) > 1 && args[0] == "project" && args[1] == "check" {
			args = append(args, "--json")
			original, err = readTree(ctx, dir)
			if err != nil {
				return err
			}
		}
		command := exec.CommandContext(ctx, binary, args...)
		command.Dir = dir
		command.Env = env
		var out, diagnostic bytes.Buffer
		command.Stdout = &out
		command.Stderr = &diagnostic
		stepStarted := time.Now()
		runErr := command.Run()
		code := 0
		signal := ""
		if runErr != nil {
			var exit *exec.ExitError
			if !errors.As(runErr, &exit) {
				return fmt.Errorf("%s: %w (%s)", label, runErr, timing(ctx, stepStarted, started, ""))
			}
			code = exit.ExitCode()
			if status, ok := exit.Sys().(syscall.WaitStatus); ok && status.Signaled() {
				signal = status.Signal().String()
			}
		}
		report.Steps = append(report.Steps, Step{label, args, out.String(), diagnostic.String(), code})
		if code != want {
			// An exit of -1 means a signal ended the process, such as the scenario's deadline killing it.
			return fmt.Errorf("%s: exit %d, expected %d (%s): %s", label, code, want, timing(ctx, stepStarted, started, signal), diagnostic.String())
		}
		if want != 0 && strings.TrimSpace(diagnostic.String()) == "" && !(len(args) > 1 && args[0] == "project" && args[1] == "check" && reportsCheckProblems(out.Bytes())) {
			return fmt.Errorf("%s: refusal returned no diagnostic", label)
		}
		if original != nil {
			after, err := readTree(ctx, dir)
			if err != nil {
				return err
			}
			if !equalTrees(original, after) {
				return fmt.Errorf("%s: read-only check changed project", label)
			}
		}
		return ctx.Err()
	}
	terms := []byte("Original library terms.\r\nPreserve these bytes.\r\n")
	notice := []byte("Original notice.\r\n")
	for name, data := range map[string][]byte{"terms.txt": terms, "notice.txt": notice, "body.md": []byte("Return every failure.\n\n![diagram](assets/errors/diagram.bin)\n"), "naming.md": []byte("Name errors after the failed operation.\n")} {
		if err := os.WriteFile(filepath.Join(libraryDir, name), data, 0600); err != nil {
			return report, err
		}
	}
	commands := [][]string{
		{"library", "init", "--spdx", "MIT", "--license-file", "terms.txt", "--notice-file", "notice.txt"},
		{"library", "add", "group", "techs/go", "--name", "Go", "--description", "Shared Go guidance.", "--when-to-read", "When editing Go."},
		{"library", "add", "rule", "techs/go/errors", "--title", "Return errors", "--impact", "HIGH", "--impact-description", "Preserve failures.", "--when-to-read", "When calling functions.", "--body-file", "body.md"},
		{"library", "add", "rule", "techs/go/naming", "--title", "Name errors", "--impact", "LOW", "--impact-description", "Find failures.", "--when-to-read", "When creating errors.", "--body-file", "naming.md"},
	}
	for _, args := range commands {
		if err := invoke("Author library", libraryDir, offline, 0, args...); err != nil {
			return report, err
		}
	}
	asset := []byte{0, 255, 13, 10, 42}
	assetPath := filepath.Join(libraryDir, ".trailguide/rules/techs/go/assets/errors")
	if err := os.MkdirAll(assetPath, 0700); err != nil {
		return report, err
	}
	if err := os.WriteFile(filepath.Join(assetPath, "diagram.bin"), asset, 0600); err != nil {
		return report, err
	}
	if err := invoke("Validate complete library", libraryDir, offline, 0, "library", "check"); err != nil {
		return report, err
	}
	libraryTree, err := readTree(ctx, libraryDir)
	if err != nil {
		return report, err
	}
	fixture, err := gitfixture.New(ctx, libraryTree.Files)
	if err != nil {
		return report, err
	}
	defer func() { err = errors.Join(err, fixture.Close()) }()
	gitBin := filepath.Join(directory, "git-only")
	if err := os.Mkdir(gitBin, 0700); err != nil {
		return report, err
	}
	if err := os.Symlink(fixture.GitPath, filepath.Join(gitBin, "git")); err != nil {
		return report, err
	}
	online := []string{"PATH=" + gitBin}
	for _, value := range fixture.Environment {
		key, _, _ := strings.Cut(value, "=")
		if strings.HasPrefix(key, "GIT_") || key == "HOME" || key == "XDG_CONFIG_HOME" {
			online = append(online, value)
		}
	}
	author, err := fixture.Clone(ctx)
	if err != nil {
		return report, err
	}
	report.Steps = append(report.Steps, Step{Label: "Fixture edit: commit the library to its repository and clone it as the author's checkout"})
	if err := invoke("Publish the first library release", author, online, 0, "library", "release"); err != nil {
		return report, err
	}
	if err := contains("first library release output", report.Steps[len(report.Steps)-1].Stdout, "Published library release 1.", "Tag:                 release/1 (created and pushed to origin)", "  techs/go/errors  new  1.0.0\n", "  techs/go/naming  new  1.0.0\n"); err != nil {
		return report, err
	}
	report.Verified = append(report.Verified, "trailguide library release publishes the first library release, giving every rule 1.0.0")
	if err := invoke("Configure consumer", consumer, offline, 0, "project", "init"); err != nil {
		return report, err
	}
	// Adding a library source reads its newest library release, which becomes the source's ref.
	if err := invoke("Add the library at its newest release", consumer, online, 0, "project", "add", "source", "team", "--repository", fixture.Repository, "--rules", "techs/go/*"); err != nil {
		return report, err
	}
	if err := contains("add source output", report.Steps[len(report.Steps)-1].Stdout, "Set sources.team.ref to release/1, the library's newest release."); err != nil {
		return report, err
	}
	if err := invoke("Sync from real Git with no Node/Bun", consumer, online, 0, "project", "sync"); err != nil {
		return report, err
	}
	if err := invoke("Check the first sync offline", consumer, offline, 0, "project", "check"); err != nil {
		return report, err
	}
	files, err := readTree(ctx, consumer)
	if err != nil {
		return report, err
	}
	for name, want := range map[string][]byte{".trailguide/_managed/vendor/team/LICENSE.md": terms, ".trailguide/_managed/vendor/team/NOTICE.md": notice, ".trailguide/_managed/vendor/team/rules/techs/go/assets/errors/diagram.bin": asset} {
		if !bytes.Equal(files.Files[name], want) {
			return report, fmt.Errorf("original bytes not preserved: %s", name)
		}
	}
	lock := string(files.Files[".trailguide/lock.json"])
	if !strings.Contains(lock, fixture.LatestCommit) || !strings.Contains(lock, `"release": 1,`) || !strings.Contains(lock, `"version": "1.0.0"`) {
		return report, fmt.Errorf("library release or rule versions missing from lock.json")
	}
	if !strings.Contains(string(files.Files[".trailguide/_managed/generated/rules/team/techs/go/errors.md"]), "Version: 1.0.0") {
		return report, fmt.Errorf("rule version missing from generated guidance")
	}
	report.Verified = append(report.Verified, "Library release release/1 imported at its commit, with each rule's version in lock.json and guidance", "License, notice, and binary asset bytes preserved exactly")
	if err := invoke("Local group overrides imported guidance", consumer, offline, 0, "project", "add", "group", "techs/go", "--name", "Project Go", "--description", "Project-specific Go guidance.", "--when-to-read", "When changing this project."); err != nil {
		return report, err
	}
	if err := invoke("Build offline without Git, Node, or Bun", consumer, offline, 0, "project", "build"); err != nil {
		return report, err
	}
	if err := invoke("Check offline", consumer, offline, 0, "project", "check"); err != nil {
		return report, err
	}
	before, err := readTree(ctx, consumer)
	if err != nil {
		return report, err
	}
	if !strings.Contains(string(before.Files[".trailguide/_managed/generated/RULES.md"]), "Project-specific Go guidance.") {
		return report, fmt.Errorf("local group guidance missing")
	}
	if err := invoke("Repeat build", consumer, offline, 0, "project", "build"); err != nil {
		return report, err
	}
	after, err := readTree(ctx, consumer)
	if err != nil {
		return report, err
	}
	if !equalTrees(before, after) {
		return report, fmt.Errorf("repeat build changed bytes")
	}
	report.Verified = append(report.Verified, "Local group guidance wins", "Offline build/check work with no runtimes on PATH", "Repeated build is byte-for-byte stable")
	switch scenario {
	case "stale-output":
		name := ".trailguide/_managed/generated/RULES.md"
		if err := os.WriteFile(filepath.Join(consumer, name), []byte("Stale output"), 0600); err != nil {
			return report, err
		}
		report.Steps = append(report.Steps, Step{Label: "Fixture edit: replace generated RULES.md with stale text"})
		if err := invoke("Read-only check detects stale output", consumer, offline, 1, "project", "check"); err != nil {
			return report, err
		}
		if err := invoke("Rebuild repairs generated output", consumer, offline, 0, "project", "build"); err != nil {
			return report, err
		}
		if err := invoke("Final offline check", consumer, offline, 0, "project", "check"); err != nil {
			return report, err
		}
		report.Verified = append(report.Verified, "Check reports stale output without changing it; rebuild repairs it")
	case "changed-vendor":
		name := filepath.Join(consumer, ".trailguide/_managed/vendor/team/LICENSE.md")
		if err := os.WriteFile(name, []byte("Manual edit"), 0600); err != nil {
			return report, err
		}
		report.Steps = append(report.Steps, Step{Label: "Fixture edit: manually change vendored LICENSE.md"})
		before, err = readTree(ctx, consumer)
		if err != nil {
			return report, err
		}
		if err := invoke("Refuse changed snapshot", consumer, offline, 1, "project", "build"); err != nil {
			return report, err
		}
		after, err = readTree(ctx, consumer)
		if err != nil {
			return report, err
		}
		if !equalTrees(before, after) {
			return report, fmt.Errorf("failed build changed files")
		}
		report.Verified = append(report.Verified, "Changed vendor bytes produce an error and preserve all project files")
	case "lifecycle":
		if err := lifecycleScenario(ctx, &report, invoke, fixture, author, directory, consumer, online, offline); err != nil {
			return report, err
		}
	case "versions", "failed-sync":
		document := libraryTree.Files[".trailguide/rules/techs/go/errors.md"]
		if scenario == "versions" {
			document = bytes.ReplaceAll(document, []byte("Return every failure."), []byte("Return updated failures."))
		} else {
			document = []byte("Invalid rule without metadata")
		}
		if _, err := fixture.Commit(ctx, fixture.Worktree(), "Change the errors rule", gitfixture.Library(map[string][]byte{"techs/go/errors.md": document})); err != nil {
			return report, err
		}
		secondRelease := "formatVersion: 1\nrelease: 2\nrules:\n  techs/go/errors: 1.1.0\n  techs/go/naming: 1.0.0\nchanges:\n  techs/go/errors: {change: minor, from: 1.0.0, summaries: [Add updated wording.]}\n"
		if err := fixture.Release(ctx, 2, secondRelease); err != nil {
			return report, err
		}
		report.Steps = append(report.Steps, Step{Label: "Fixture edit: publish library release release/2 with techs/go/errors at 1.1.0 and " + scenario + " content"})
		before, err = readTree(ctx, consumer)
		if err != nil {
			return report, err
		}
		if err := invoke("Sync after a new library release", consumer, online, 0, "project", "sync"); err != nil {
			return report, err
		}
		after, err = readTree(ctx, consumer)
		if err != nil {
			return report, err
		}
		if !equalTrees(before, after) {
			return report, fmt.Errorf("sync adopted a newer version on its own")
		}
		report.Verified = append(report.Verified, "Sync restores the recorded versions after a new library release")
		configure := func(label, ref, fields string) error {
			config := "schemaVersion: 2\nsources:\n  team:\n    repository: " + fixture.Repository + "\n    ref: " + ref + "\n    rules:\n      include:\n        - techs/go/*\n" + fields
			if err := os.WriteFile(filepath.Join(consumer, ".trailguide/config.yaml"), []byte(config), 0600); err != nil {
				return err
			}
			report.Steps = append(report.Steps, Step{Label: "Configuration edit: " + label})
			return nil
		}
		if scenario == "failed-sync" {
			if err := configure("move ref to release/2", "release/2", ""); err != nil {
				return report, err
			}
			before, err = readTree(ctx, consumer)
			if err != nil {
				return report, err
			}
			if err := invoke("Sync a release with an invalid rule", consumer, online, 1, "project", "sync"); err != nil {
				return report, err
			}
			after, err = readTree(ctx, consumer)
			if err != nil {
				return report, err
			}
			if !equalTrees(before, after) {
				return report, fmt.Errorf("failed sync changed project")
			}
			report.Verified = append(report.Verified, "Failed import preserves vendor and generated bytes")
			return report, nil
		}
		if err := configure("move ref to release/2 and pin techs/go/errors to 1.0.0", "release/2", "      pins:\n        - id: techs/go/errors\n          version: \"1.0.0\"\n          reason: Keep the original wording.\n"); err != nil {
			return report, err
		}
		if err := invoke("Sync the edited ref", consumer, online, 1, "project", "sync"); err != nil {
			return report, err
		}
		if !strings.Contains(report.Steps[len(report.Steps)-1].Stderr, "sources.team.ref names release/2, but the project records library release 1, and sync doesn't move a source to another release. Run trailguide project update team") {
			return report, fmt.Errorf("sync didn't refuse the edited ref:\n%s", report.Steps[len(report.Steps)-1].Stderr)
		}
		if err := invoke("Update to release/2 with the pin", consumer, online, 0, "project", "update", "--yes"); err != nil {
			return report, err
		}
		after, err = readTree(ctx, consumer)
		if err != nil {
			return report, err
		}
		if !bytes.Equal(after.Files[".trailguide/_managed/vendor/team/rules/techs/go/errors.md"], libraryTree.Files[".trailguide/rules/techs/go/errors.md"]) || !bytes.Equal(after.Files[".trailguide/_managed/vendor/team/rules/techs/go/naming.md"], libraryTree.Files[".trailguide/rules/techs/go/naming.md"]) {
			return report, fmt.Errorf("update did not keep the pinned rule at 1.0.0 beside the release's other rule")
		}
		if !strings.Contains(string(after.Files[".trailguide/_managed/generated/rules/team/techs/go/errors.md"]), "Version: 1.0.0") || !strings.Contains(string(after.Files[".trailguide/lock.json"]), `"release": 2,`) {
			return report, fmt.Errorf("the pinned version or the new release is missing")
		}
		if err := invoke("Check the pinned version offline", consumer, offline, 0, "project", "check"); err != nil {
			return report, err
		}
		report.Verified = append(report.Verified, "Sync refuses a ref edited to name a newer library release, and update takes it, while a pin holds one rule at its version, and checks offline")
		if err := configure("remove the pin", "release/2", ""); err != nil {
			return report, err
		}
		if err := invoke("Sync keeps the formerly pinned rule", consumer, online, 0, "project", "sync"); err != nil {
			return report, err
		}
		after, err = readTree(ctx, consumer)
		if err != nil {
			return report, err
		}
		if !bytes.Equal(after.Files[".trailguide/_managed/vendor/team/rules/techs/go/errors.md"], libraryTree.Files[".trailguide/rules/techs/go/errors.md"]) {
			return report, fmt.Errorf("sync moved a rule the record locks")
		}
		if err := invoke("Update moves the formerly pinned rule", consumer, online, 0, "project", "update", "--yes"); err != nil {
			return report, err
		}
		after, err = readTree(ctx, consumer)
		if err != nil {
			return report, err
		}
		if !bytes.Equal(after.Files[".trailguide/_managed/vendor/team/rules/techs/go/errors.md"], document) || !strings.Contains(string(after.Files[".trailguide/_managed/generated/rules/team/techs/go/errors.md"]), "Version: 1.1.0") {
			return report, fmt.Errorf("update did not move the formerly pinned rule to 1.1.0")
		}
		report.Verified = append(report.Verified, "Removing a pin leaves the rule at its recorded version, and update moves it to the version its release publishes")
		unreleased, err := fixture.Commit(ctx, fixture.Worktree(), "Unreleased change", gitfixture.Library(map[string][]byte{"techs/go/naming.md": bytes.ReplaceAll(libraryTree.Files[".trailguide/rules/techs/go/naming.md"], []byte("failed operation"), []byte("failed operation and its input"))}))
		if err != nil {
			return report, err
		}
		if err := configure("import the unreleased commit with ref", unreleased, ""); err != nil {
			return report, err
		}
		if err := invoke("Sync an unreleased commit", consumer, online, 0, "project", "sync"); err != nil {
			return report, err
		}
		output := report.Steps[len(report.Steps)-1].Stdout
		if !strings.Contains(output, "Warning: Source team imports "+unreleased) || !strings.Contains(output, "Unreleased rules: techs/go/naming.") {
			return report, fmt.Errorf("sync did not warn about unreleased rules: %s", output)
		}
		if err := invoke("Check the unreleased import offline", consumer, offline, 0, "project", "check"); err != nil {
			return report, err
		}
		report.Verified = append(report.Verified, "An unreleased ref warns and still checks offline")
	}
	return report, nil
}

// readTree captures file bytes and empty directories through the confined project reader.
func readTree(ctx context.Context, directory string) (*filetxn.Tree, error) {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	tree, err := filetxn.ReadTree(ctx, root, ".")
	if err != nil {
		return nil, err
	}
	return tree, nil
}

// equalTrees compares original bytes and directory inventory, including empty directories.
func equalTrees(before, after *filetxn.Tree) bool {
	return maps.EqualFunc(before.Files, after.Files, bytes.Equal) && slices.Equal(before.Directories, after.Directories)
}

// reportsCheckProblems recognizes a read-only check report whose problems explain the nonzero exit.
func reportsCheckProblems(data []byte) bool {
	var result struct {
		OK    bool
		Value struct {
			Status   string
			Problems []struct {
				Kind, Path string
				NextSteps  []struct {
					Instruction string
					Commands    []string
				}
			}
		}
		Error struct{ Kind string }
	}
	if json.Unmarshal(data, &result) != nil || result.OK || result.Error.Kind != "out-of-date" || result.Value.Status != "out-of-date" || len(result.Value.Problems) == 0 {
		return false
	}
	for _, problem := range result.Value.Problems {
		if len(problem.NextSteps) == 0 || len(problem.NextSteps[0].Commands) == 0 {
			return false
		}
	}
	return true
}
