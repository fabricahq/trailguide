// Verify project add source records the documented selection and revision fields without fetching.

package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"go.yaml.in/yaml/v4"
)

// TestSourceSelectionFlags records rule selectors, skill selectors, and an optional exact ref, writing them as
// rules.include and skills.include, and rejects invalid selections without writes.
func TestSourceSelectionFlags(t *testing.T) {
	binary := buildCLI(t)
	repository := "https://example.invalid/rules.git"
	include := func(selectors ...any) map[string]any { return map[string]any{"include": selectors} }
	for _, tc := range []struct {
		name string
		args []string
		code int
		want map[string]any
	}{
		{"groups without ref", []string{"--rules", "techs/go/*"}, 0, map[string]any{"repository": repository, "rules": include("techs/go/*")}},
		{"tag ref", []string{"--ref", "release/5", "--rules", "techs/go/*"}, 0, map[string]any{"repository": repository, "rules": include("techs/go/*"), "ref": "release/5"}},
		{"rules only", []string{"--rules", "techs/go/wrap-errors", "--rules", "practices/testing/verify-retry-limits"}, 0,
			map[string]any{"repository": repository, "rules": include("practices/testing/verify-retry-limits", "techs/go/wrap-errors")}},
		{"groups and rules", []string{"--rules", "*", "--rules", "techs/go/wrap-errors"}, 0, map[string]any{"repository": repository, "rules": include("*", "techs/go/wrap-errors")}},
		{"wildcard and groups", []string{"--rules", "techs/*", "--rules", "practices/testing/*"}, 0, map[string]any{"repository": repository, "rules": include("techs/*", "practices/testing/*")}},
		{"skills only", []string{"--skills", "tools/review", "--skills", "skills/build-feature"}, 0, map[string]any{"repository": repository, "skills": include("skills/build-feature", "tools/review")}},
		{"every skill from a ref", []string{"--ref", "v1.0.0", "--skills", "*"}, 0, map[string]any{"repository": repository, "ref": "v1.0.0", "skills": include("*")}},
		{"rules and skills", []string{"--rules", "techs/go/*", "--skills", "skills/build-feature"}, 0, map[string]any{"repository": repository, "rules": include("techs/go/*"), "skills": include("skills/build-feature")}},
		{"recommended skills", []string{"--skills", "recommended", "--skills", "skills/deploy"}, 0, map[string]any{"repository": repository, "skills": include("recommended", "skills/deploy")}},
		{"required skills only", nil, 0, map[string]any{"repository": repository}},
		{"recommended prefix", []string{"--skills", "recommended/*"}, 1, nil},
		{"skill prefix", []string{"--skills", "skills/*"}, 1, nil},
		{"wildcard and a skill", []string{"--skills", "*", "--skills", "skills/build-feature"}, 1, nil},
		{"version range", []string{"--ref", ">= 1.2.0, < 2.0.0", "--rules", "techs/go/*"}, 2, nil},
		{"every skill from a branch", []string{"--ref", "refs/heads/main", "--skills", "*"}, 0, map[string]any{"repository": repository, "ref": "refs/heads/main", "skills": include("*")}},
		{"remote-tracking ref", []string{"--ref", "refs/remotes/origin/main", "--rules", "techs/go/*"}, 2, nil},
		{"group as rule", []string{"--rules", "techs/go"}, 1, nil},
		{"duplicate rules", []string{"--rules", "techs/go/a", "--rules", "techs/go/a"}, 1, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if out, diagnostic, code := runCLI(t, binary, dir, "project", "init"); code != 0 {
				t.Fatal(code, out, diagnostic)
			}
			before := projectFileContents(t, dir)
			args := append([]string{"project", "add", "source", "team", "--repository", repository, "--non-interactive", "--json"}, tc.args...)
			out, diagnostic, code := runCLI(t, binary, dir, args...)
			var result response
			if diagnostic != "" || json.Unmarshal([]byte(out), &result) != nil || code != tc.code {
				t.Fatal(code, out, diagnostic)
			}
			if tc.want == nil {
				if result.OK || result.Error == nil || !reflect.DeepEqual(before, projectFileContents(t, dir)) {
					t.Fatal("invalid selection must fail without writes", out)
				}
				return
			}
			data, err := os.ReadFile(filepath.Join(dir, ".trailguide/config.yaml"))
			if err != nil {
				t.Fatal(err)
			}
			var config struct{ Sources map[string]map[string]any }
			if err := yaml.Unmarshal(data, &config); err != nil || !reflect.DeepEqual(config.Sources["team"], tc.want) {
				t.Fatal(string(data), err)
			}
			if _, err := os.Stat(filepath.Join(dir, ".trailguide/vendor")); !os.IsNotExist(err) {
				t.Fatal("add source fetched files", err)
			}
		})
	}
}
