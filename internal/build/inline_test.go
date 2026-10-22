// Verify whole-group delivery boundaries and link isolation through the complete output API.

package build

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/parser"

	"github.com/fabricahq/trailguide/trailguide"
)

// TestPrepareInlineBoundaries includes whole UTF-8 groups only when the inline byte limit and index line limit allow them.
func TestPrepareInlineBoundaries(t *testing.T) {
	config, libraries := fixture(t, `[]`)
	resolved, err := resolve(config, libraries, nil)
	if err != nil {
		t.Fatal(err)
	}
	resolved.Groups[0].Rules[0].Rule.Document += "\nUnicode: 日本語\n"
	options := Options{IndexMaxLines: defaultIndexMaxLines}
	whole, err := prepare(resolved, options)
	if err != nil {
		t.Fatal(err)
	}
	file := "groups/techs/go.md"
	size := len(whole.Files[file])
	lines := strings.Count(string(whole.Files[file]), "\n")
	if !strings.Contains(string(whole.Files[file]), "Return errors to the caller.") {
		t.Fatal("default did not inline short group")
	}
	for _, test := range []struct {
		name          string
		index, inline int
		full          bool
	}{
		{"exact inline fit", 750, size, true}, {"one byte over inline", 750, size - 1, false},
		{"exact index fit", lines, 8000, true}, {"one line over index", lines - 1, 8000, false},
		{"disabled", 750, 0, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			options.IndexMaxLines, options.GroupInlineMaxBytes = test.index, &test.inline
			got, err := prepare(resolved, options)
			if err != nil {
				t.Fatal(err)
			}
			page := string(got.Files[file])
			if !strings.Contains(page, "Go guidance.\n\n**When to read this group:**") {
				t.Fatal("group description missing from inline or summary mode")
			}
			if strings.Contains(page, "Return errors to the caller.") != test.full {
				t.Fatalf("wrong delivery mode: %s", page)
			}
			for path, content := range got.Files {
				if (path == "RULES.md" || strings.HasPrefix(path, "groups/techs/")) && strings.Count(string(content), "\n") > test.index {
					t.Fatalf("oversized page %s", path)
				}
			}
			if !reflect.DeepEqual(got.Files["rules/team/techs/go/errors.md"], whole.Files["rules/team/techs/go/errors.md"]) {
				t.Fatal("delivery changed standalone rule")
			}
		})
	}
	negative := -1
	options.GroupInlineMaxBytes = &negative
	if got, err := prepare(resolved, options); err == nil || got.Files != nil {
		t.Fatal("accepted negative budget or returned partial output")
	}
}

// TestPrepareInlineLinks keeps reused labels, fragments, headings, and attachments correct across two complete rules.
func TestPrepareInlineLinks(t *testing.T) {
	for _, links := range []string{
		"[full][asset] [asset][] [asset] ![asset] [![asset]][asset]",
		"## [asset]\n\n> [asset][]\n\n[full][ASSET]",
		"[full][multi\n line] [multi\nline][] ![multi\n line]",
	} {
		t.Run(links, func(t *testing.T) {
			config, libraries := fixture(t, `[]`)
			lib := libraries["team"]
			lib.Catalog.Groups[0].Rules = nil
			for i := range 2 {
				label := "asset"
				if strings.Contains(links, "multi") {
					label = "multi line"
				}
				body := fmt.Sprintf("\n%s\n\n[fragment](#details)\n\n## Details\n\nBody %d\n\n[%s]: /assets/%d.txt\n\n`[asset]`\n", links, i, label, i)
				rule, err := trailguide.ParseRule(document+body, fmt.Sprintf("techs/go/rule-%d.md", i), "team")
				if err != nil {
					t.Fatal(err)
				}
				lib.Catalog.Groups[0].Rules = append(lib.Catalog.Groups[0].Rules, rule)
				lib.Catalog.SupportingFiles[fmt.Sprintf("assets/%d.txt", i)] = []byte("attachment")
			}
			libraries["team"] = versioned(lib.Catalog)
			resolved, err := resolve(config, libraries, nil)
			if err != nil {
				t.Fatal(err)
			}
			before, _ := renderRules(resolved)
			got, err := prepare(resolved, Options{IndexMaxLines: 16000})
			if err != nil {
				t.Fatal(err)
			}
			page := got.Files["groups/techs/go.md"]
			targets := map[string]int{}
			root := parser.New().Parse(page)
			_ = ast.Walk(root, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
				if entering {
					switch n := node.(type) {
					case *ast.Link:
						targets[n.Destination.Value(page)]++
					case *ast.Image:
						targets[n.Destination.Value(page)]++
					}
				}
				return ast.WalkContinue, nil
			})
			for i := range 2 {
				target := fmt.Sprintf("../../../vendor/team/assets/%d.txt", i)
				wantCount := 3
				if strings.HasPrefix(links, "[full][asset]") {
					wantCount = 6
				}
				if targets[target] != wantCount {
					t.Fatalf("references captured or lost: %v\n%s", targets, page)
				}
				fragment := fmt.Sprintf("../../rules/team/techs/go/rule-%d.md#details", i)
				if targets[fragment] != 1 {
					t.Fatalf("fragment not directed to own standalone rule: %v", targets)
				}
			}
			if strings.Count(string(page), "### Return errors") != 2 || strings.Count(string(page), "#### Guidance") != 2 || strings.Count(string(page), "##### Details") != 2 {
				t.Fatalf("incorrect inline headings: %s", page)
			}
			if strings.Count(string(page), "`[asset]`") != 2 {
				t.Fatal("rewrote code")
			}
			after, _ := renderRules(resolved)
			if !reflect.DeepEqual(before, after) {
				t.Fatal("mutated source rules")
			}
		})
	}
}

// TestPrepareInlineFallbackNeverHidesInvalidRules validates later rules even after an earlier body exceeds the inline budget.
func TestPrepareInlineFallbackNeverHidesInvalidRules(t *testing.T) {
	config, libraries := fixture(t, `[]`)
	resolved, err := resolve(config, libraries, nil)
	if err != nil {
		t.Fatal(err)
	}
	resolved.Groups[0].Rules[0].Rule.Document += strings.Repeat("Long body. ", 1000)
	second := resolved.Groups[0].Rules[0]
	second.Rule.ID = "team:techs/go/later"
	second.Rule.Document = document + "\n[missing](/assets/missing.txt)"
	resolved.Groups[0].Rules = append(resolved.Groups[0].Rules, second)
	if got, err := prepare(resolved, Options{IndexMaxLines: defaultIndexMaxLines}); err == nil || got.Files != nil {
		t.Fatal("inline fallback concealed invalid later rule")
	}
}

// TestInlineMultilineHeadingReference preserves link destinations when a reference label crosses heading lines.
func TestInlineMultilineHeadingReference(t *testing.T) {
	for _, heading := range []string{"[full][multi\nline]\n======", "> [full][multi\n> line]\n> ======", "![full][multi\nline]\n======", "> ![full][multi\n> line]\n> ======"} {
		config, libraries := fixture(t, `[]`)
		resolved, err := resolve(config, libraries, nil)
		if err != nil {
			t.Fatal(err)
		}
		resolved.Groups[0].Rules[0].Rule.Document += "\n" + heading + "\n\n[multi line]: https://example.com\n"
		got, err := prepare(resolved, Options{IndexMaxLines: defaultIndexMaxLines})
		if err != nil {
			t.Fatal(err)
		}
		page := got.Files["groups/techs/go.md"]
		found := false
		_ = ast.Walk(parser.New().Parse(page), func(node ast.Node, entering bool) (ast.WalkStatus, error) {
			if entering {
				switch link := node.(type) {
				case *ast.Link:
					found = found || link.Destination.Value(page) == "https://example.com"
				case *ast.Image:
					found = found || link.Destination.Value(page) == "https://example.com"
				}
			}
			return ast.WalkContinue, nil
		})
		if !found {
			t.Fatalf("heading lost reference link: %s", page)
		}
	}
}
