// Check JSON output conventions that hold across commands: lists are always present, and enumerated values are
// kebab-case.

package cli

import (
	"context"
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// jsonNulls returns the path of every null value in a JSON document, such as value.warnings.
func jsonNulls(value any, path string) []string {
	switch typed := value.(type) {
	case nil:
		return []string{path}
	case map[string]any:
		var nulls []string
		for key, item := range typed {
			nulls = append(nulls, jsonNulls(item, path+"."+key)...)
		}
		return nulls
	case []any:
		var nulls []string
		for _, item := range typed {
			nulls = append(nulls, jsonNulls(item, path+"[]")...)
		}
		return nulls
	}
	return nil
}

// TestJSONOutput_ListsAreAlwaysPresent runs project and library commands with --json and requires every list
// field, including warnings, to be present and never null when there's nothing to report.
func TestJSONOutput_ListsAreAlwaysPresent(t *testing.T) {
	project, library := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "body.md"), []byte("Return failures to callers.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		directory string
		args      []string
		lists     []string
	}{
		{project, []string{"project", "init"}, []string{"added", "changed", "warnings", "nextSteps"}},
		{project, []string{"project", "add", "group", "techs/go", "--name", "Go", "--description", "Go guidance.", "--when-to-read", "When editing Go."}, []string{"added", "changed", "warnings", "nextSteps"}},
		{project, []string{"project", "add", "rule", "techs/go/errors", "--title", "Return errors", "--impact", "HIGH", "--impact-description", "Preserve failures.", "--when-to-read", "When calling functions.", "--body-file", "body.md"}, []string{"added", "changed", "warnings", "nextSteps"}},
		{project, []string{"project", "build"}, []string{"added", "changed", "removed", "warnings"}},
		{project, []string{"project", "sync"}, []string{"added", "changed", "removed", "warnings"}},
		{project, []string{"project", "check"}, []string{"problems"}},
		{project, []string{"project", "list", "rules"}, []string{"rules"}},
		{library, []string{"library", "init"}, []string{"added", "changed", "warnings", "nextSteps"}},
		{library, []string{"library", "add", "group", "techs/go", "--name", "Go", "--description", "Go guidance.", "--when-to-read", "When editing Go."}, []string{"added", "changed", "warnings", "nextSteps"}},
		{library, []string{"library", "check"}, []string{"warnings"}},
	} {
		out, diagnostic, code := invokeReport(t, context.Background(), test.directory, append(test.args, "--json")...)
		var response struct {
			OK    bool
			Value map[string]any
		}
		if err := json.Unmarshal([]byte(out), &response); err != nil || code != 0 || diagnostic != "" || !response.OK {
			t.Fatalf("%v: exit %d, %v:\n%s%s", test.args, code, err, out, diagnostic)
		}
		var document any
		if err := json.Unmarshal([]byte(out), &document); err != nil {
			t.Fatal(err)
		}
		if nulls := jsonNulls(document, ""); len(nulls) > 0 {
			t.Errorf("%v: null values at %s", test.args, strings.Join(nulls, ", "))
		}
		for _, name := range test.lists {
			if _, isList := response.Value[name].([]any); !isList {
				t.Errorf("%v: value.%s is %#v, want a list", test.args, name, response.Value[name])
			}
		}
		if steps, ok := response.Value["nextSteps"].([]any); ok {
			for _, step := range steps {
				if _, isList := step.(map[string]any)["commands"].([]any); !isList {
					t.Errorf("%v: next step %v has no commands list", test.args, step)
				}
			}
		}
	}
}

// kebabCase matches an enumerated JSON value such as out-of-date or stale-contents.
var kebabCase = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// enumValues returns the value of every enumerated field in a JSON document, by path.
func enumValues(value any, path string) map[string]string {
	values := map[string]string{}
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			if text, ok := item.(string); ok && slices.Contains([]string{"status", "kind", "change", "decision", "code"}, key) {
				values[path+"."+key] = text
			}
			maps.Copy(values, enumValues(item, path+"."+key))
		}
	case []any:
		for _, item := range typed {
			maps.Copy(values, enumValues(item, path+"[]"))
		}
	}
	return values
}

// TestJSONOutput_EnumeratedValuesAreKebabCase reports a stale project's status, problem kinds, and error kind in
// kebab-case, as error codes are.
func TestJSONOutput_EnumeratedValuesAreKebabCase(t *testing.T) {
	directory := t.TempDir()
	for _, args := range [][]string{{"project", "init"}, {"project", "add", "group", "techs/go", "--name", "Go", "--description", "Go guidance.", "--when-to-read", "When editing Go."}, {"project", "build"}} {
		if out, diagnostic, code := invokeReport(t, context.Background(), directory, args...); code != 0 {
			t.Fatal(args, code, out, diagnostic)
		}
	}
	if err := os.WriteFile(filepath.Join(directory, ".trailguide", "generated", "RULES.md"), []byte("Stale.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	out, _, code := invokeReport(t, context.Background(), directory, "project", "check", "--json")
	var document any
	if err := json.Unmarshal([]byte(out), &document); err != nil || code != 1 {
		t.Fatal(code, err, out)
	}
	values := enumValues(document, "")
	if values[".value.status"] != "out-of-date" || values[".error.kind"] != "out-of-date" || values[".value.problems[].kind"] != "stale-contents" {
		t.Fatalf("values %v", values)
	}
	for path, value := range values {
		if !kebabCase.MatchString(value) {
			t.Errorf("%s is %q, not kebab-case", path, value)
		}
	}
}
