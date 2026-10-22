// Verify the authored library and group YAML files through the native CLI.

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestYAMLLibraryMetadataLifecycle(t *testing.T) {
	binary := buildCLI(t)
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		out, diagnostic, code := runCLI(t, binary, dir, args...)
		if code != 0 {
			t.Fatal(args, code, out, diagnostic)
		}
	}
	run("library", "init")
	run("library", "add", "group", "practices/testing", "--name", "Testing", "--description", "Test behavior.", "--when-to-read", "When writing tests.")
	for name, prefix := range map[string]string{"config.yaml": "schemaVersion: 2\nsources: {}\nlibrary:\n  formatVersion: 1\n", "practices/testing/_group.yaml": "name: Testing\n"} {
		data, err := os.ReadFile(libraryFile(dir, name))
		if err != nil || !strings.HasPrefix(string(data), prefix) {
			t.Fatal(name, string(data), err)
		}
	}
	// Exercise human-authored comments and folded text, not only the CLI's serializer.
	metadata := "# Group guidance\nname: Testing\ndescription: Test behavior.\nwhenToRead: >\n  When writing\n  or reviewing tests.\n"
	name := libraryFile(dir, "practices/testing/_group.yaml")
	if err := os.WriteFile(name, []byte(metadata), 0600); err != nil {
		t.Fatal(err)
	}
	run("library", "check")
	run("library", "init")
	data, err := os.ReadFile(name)
	if err != nil || string(data) != metadata {
		t.Fatal("authored YAML changed", err)
	}
	for _, old := range []string{"rule-library.yaml", "rule-library.json", "practices", libraryFile(".", "practices/testing/_group.json")} {
		if _, err := os.Stat(filepath.Join(dir, old)); !os.IsNotExist(err) {
			t.Fatal("legacy metadata created", old, err)
		}
	}
}
