// Check that init leaves a project in the layout Trail Guide 0.7 and earlier wrote for sync to move.

package cli

import (
	"encoding/json"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"testing"
)

// TestProjectInit_RefusesTheLegacyLayout runs init again in a project whose generated guidance is still in
// .trailguide/generated/, as an upgrade leaves it. Pointing AGENTS.md and the guide to _managed/generated/ before sync
// moves the files would leave agents without their rules, so init fails with layout-migration-needed, changing nothing,
// and sync then moves the project.
func TestProjectInit_RefusesTheLegacyLayout(t *testing.T) {
	binary := buildCLI(t)
	directory := t.TempDir()
	for _, args := range [][]string{{"project", "init"}, {"project", "build"}} {
		if out, diagnostic, code := runCLI(t, binary, directory, args...); code != 0 {
			t.Fatalf("%v: exit %d\n%s%s", args, code, out, diagnostic)
		}
	}
	trailguide := filepath.Join(directory, ".trailguide")
	if err := os.Rename(filepath.Join(trailguide, "_managed", "generated"), filepath.Join(trailguide, "generated")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(trailguide, "_managed")); err != nil {
		t.Fatal(err)
	}
	before := readFiles(t, directory)
	out, diagnostic, code := runCLI(t, binary, directory, "--json", "project", "init")
	var result struct {
		OK    bool
		Error struct{ Code string }
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil || code != 1 || result.OK || result.Error.Code != "layout-migration-needed" {
		t.Fatalf("exit %d, %v:\n%s%s", code, err, out, diagnostic)
	}
	if after := readFiles(t, directory); !maps.Equal(before, after) {
		t.Fatal("init changed the project before sync moved it")
	}
	if out, diagnostic, code := runCLI(t, binary, directory, "project", "sync"); code != 0 {
		t.Fatalf("sync: exit %d\n%s%s", code, out, diagnostic)
	}
	if _, err := os.Stat(filepath.Join(trailguide, "_managed", "generated", "RULES.md")); err != nil {
		t.Fatalf("sync didn't move the rules: %v", err)
	}
}

// readFiles returns every regular file under directory by its path there.
func readFiles(t *testing.T, directory string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(directory, func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, err := os.ReadFile(name)
		files[name] = string(data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}
