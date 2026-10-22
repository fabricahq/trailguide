// Validate rendered guidance, relocation, source preservation, and unsafe link failures.

package build

import (
	"errors"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/errs"
	"github.com/fabricahq/trailguide/trailguide"
)

// renderFixture resolves a local rule with a caller-supplied body and optional supporting files.
func renderFixture(t *testing.T, body string, support map[string][]byte) (map[string]string, error) {
	t.Helper()
	config, libraries := fixture(t, `[]`)
	files := map[string][]byte{"techs/go/local.md": []byte(strings.Split(document, "---\n#")[0] + "---\n" + body)}
	for file, data := range support {
		files[file] = data
	}
	resolved, err := resolve(config, libraries, files)
	if err != nil {
		return nil, err
	}
	return renderRules(resolved)
}

// TestRenderRules retains non-rewritten bytes and relocates parsed links without changing code examples.
func TestRenderRules(t *testing.T) {
	body := "# Return errors\n\nA  sentence.\r\n\n[asset](assets/local/a%20b.png#part)\n\n[self]() [ref][r]\n\n[r]: assets/local/a%20b.png\n\n`[code](missing)`\n\n## More\n\nKeep trailing spaces.  \nNext line.\n"
	output, err := renderFixture(t, body, map[string][]byte{"techs/go/assets/local/a b.png": {0, 255}})
	if err != nil {
		t.Fatal(err)
	}
	got := output["rules/local/techs/go/local.md"]
	for _, want := range []string{"A  sentence.\r\n", "../../../../../../rules/techs/go/assets/local/a%20b.png#part", "[self](../../../../../../rules/techs/go/local.md)", "`[code](missing)`", "Keep trailing spaces.  \nNext line.", "### Source metadata", "**Rule source:**"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
	if strings.Count(got, "# Return errors") != 1 {
		t.Fatal("retained redundant heading")
	}
}

// TestRenderLinksSharedAuthoredAssets links a rule in rules/ to a shared asset in assets/ through its place in the
// Trail Guide directory, three levels up.
func TestRenderLinksSharedAuthoredAssets(t *testing.T) {
	output, err := renderFixture(t, "[diagram](../../../assets/diagram.svg)", map[string][]byte{"assets/diagram.svg": []byte("<svg/>")})
	if err != nil {
		t.Fatal(err)
	}
	if got := output["rules/local/techs/go/local.md"]; !strings.Contains(got, "[diagram](../../../../../../assets/diagram.svg)") {
		t.Fatal(got)
	}
}

// TestRenderRejectsLinksOutsideAuthoredFolders refuses a link from an authored rule to anything outside rules/ and
// assets/, including a shared asset linked from the rule's place in the library layout, one level short.
func TestRenderRejectsLinksOutsideAuthoredFolders(t *testing.T) {
	for _, test := range []struct{ body, target string }{
		{"[diagram](../../assets/diagram.svg)", "rules/assets/diagram.svg"},
		{"[config](../../../config.yaml)", "config.yaml"},
		{"[vendored](../../../vendor/team/rules/techs/go/errors.md)", "vendor/team/rules/techs/go/errors.md"},
	} {
		t.Run(test.target, func(t *testing.T) {
			output, err := renderFixture(t, test.body, map[string][]byte{"assets/diagram.svg": []byte("<svg/>")})
			var validation errs.ValidationError
			if !errors.As(err, &validation) || validation.ValidationLocation() != "rules/techs/go/local.md" || validation.ValidationProblem() != "links to "+test.target+", which is outside .trailguide/rules/ and .trailguide/assets/; link to this rule's own assets directory, or to a shared asset in .trailguide/assets/" || output != nil {
				t.Fatalf("got %v, %v", output, err)
			}
		})
	}
}

// TestRenderSetext preserves the paragraph immediately after an underline heading.
func TestRenderSetext(t *testing.T) {
	output, err := renderFixture(t, "Different title\n===\nParagraph after heading.\n", nil)
	if err != nil {
		t.Fatal(err)
	}
	got := output["rules/local/techs/go/local.md"]
	if !strings.Contains(got, "### Different title\nParagraph after heading.") {
		t.Fatal(got)
	}
}

// TestRenderRejectsUnsafeReferences refuses missing local links and relative HTML attributes.
func TestRenderRejectsUnsafeReferences(t *testing.T) {
	for _, body := range []string{"[missing](missing.md)", "[escape](../../../outside)", "<img src='assets/local/a.png'>", "<a href=missing.md>x</a>"} {
		output, err := renderFixture(t, body, nil)
		if err == nil || output != nil {
			t.Fatalf("accepted %q: %v", body, output)
		}
	}
}

// TestRenderRejectsRuleLinks refuses retained, excluded, and unretained targets without remote fallback.
func TestRenderRejectsRuleLinks(t *testing.T) {
	for _, state := range []string{"selected", "excluded", "unselected"} {
		t.Run(state, func(t *testing.T) {
			exclude := `[]`
			if state == "excluded" {
				exclude = `[{"id":"techs/go/other","reason":"Project policy"}]`
			}
			config, libraries := fixture(t, exclude)
			supplied := libraries["team"]
			target := "techs/go/other.md"
			if state == "unselected" {
				target = "techs/rust/other.md"
			} else {
				other, err := trailguide.ParseRule(document, target, "team")
				if err != nil {
					t.Fatal(err)
				}
				supplied.Catalog.Groups[0].Rules = append(supplied.Catalog.Groups[0].Rules, other)
			}
			supplied.Catalog.Groups[0].Rules[0].Document = document + "\n[other](/rules/" + target + "#details)\n"
			libraries["team"] = versioned(supplied.Catalog)
			resolved, err := resolve(config, libraries, nil)
			if err != nil {
				t.Fatal(err)
			}
			output, err := renderRules(resolved)
			if err == nil || !strings.Contains(err.Error(), "links to other rule documents are not allowed") || output != nil {
				t.Fatalf("expected rule-link error without partial output: %+v, %v", output, err)
			}
		})
	}
}

// TestRenderPreservesEntityDestinations prevents a second Markdown decode from changing external URL semantics.
func TestRenderPreservesEntityDestinations(t *testing.T) {
	output, err := renderFixture(t, "[x](https://example.com/?q=&amp;copy;)\n", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output["rules/local/techs/go/local.md"], "?q=&amp;copy;") {
		t.Fatal(output)
	}
}

// TestRenderNestsEmptyHeading preserves empty heading structure below the generated groupGuidance section.
func TestRenderNestsEmptyHeading(t *testing.T) {
	output, err := renderFixture(t, "#\nAfter.\n", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output["rules/local/techs/go/local.md"], "###\nAfter.") {
		t.Fatal(output)
	}
}

// TestRenderNestedHeadings nests container headings without consuming following guidance.
func TestRenderNestedHeadings(t *testing.T) {
	for _, body := range []string{"> # Caveat\n> Keep this.\n", "- # Caveat\n\n  Keep this.\n", "> Multi\n> line\n> ===\n> Keep this.\n"} {
		output, err := renderFixture(t, body, nil)
		if err != nil {
			t.Fatal(err)
		}
		got := output["rules/local/techs/go/local.md"]
		if !strings.Contains(got, "### ") || !strings.Contains(got, "Keep this.") {
			t.Fatal(got)
		}
	}
}

// TestRenderIgnoresInertHTML distinguishes comments and data attributes from actual resource URLs.
func TestRenderIgnoresInertHTML(t *testing.T) {
	for _, body := range []string{`Text <script>const example='<img src="draft.md">';</script>`, `Text <textarea><img src="draft.md"></textarea>`, `<!-- href="draft.md" -->`, `<span data-href="draft.md">Text</span>`, `<script>const example='src="draft.md"';</script>`} {
		if _, err := renderFixture(t, body, nil); err != nil {
			t.Fatalf("%q: %v", body, err)
		}
	}
}

// TestRenderRejectsFileDirectoryConflict refuses an output path that must be both file and directory.
func TestRenderRejectsFileDirectoryConflict(t *testing.T) {
	config, libraries := fixture(t, `[]`)
	resolved, err := resolve(config, libraries, map[string][]byte{"techs/go/a.md": []byte(document), "techs/go/a.md/b.md": []byte(document)})
	if err != nil {
		t.Fatal(err)
	}
	if output, err := renderRules(resolved); err == nil || output != nil {
		t.Fatal("accepted file/directory conflict")
	}
}

// TestRenderReferenceImages keeps shared image definitions usable as both images and file links.
func TestRenderReferenceImages(t *testing.T) {
	for _, body := range []string{"![diagram][asset]\n\n[asset]: /assets/diagram.png", "[download][asset] ![diagram][asset]\n\n[asset]: /assets/diagram.png"} {
		config, libraries := fixture(t, `[]`)
		lib := libraries["team"]
		lib.Catalog.Groups[0].Rules[0].Document = document + "\n" + body
		lib.Catalog.SupportingFiles["assets/diagram.png"] = []byte("image bytes")
		libraries["team"] = lib
		resolved, err := resolve(config, libraries, nil)
		if err != nil {
			t.Fatal(err)
		}
		output, err := renderRules(resolved)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(output["rules/team/techs/go/errors.md"], "../../../../../vendor/team/assets/diagram.png") {
			t.Fatal(output)
		}
	}
}

// TestRenderLinksImportedRuleAssetsToTheirVendoredCopy links an imported rule's own asset to where the source's
// snapshot stores it, under vendor/team/rules/.
func TestRenderLinksImportedRuleAssetsToTheirVendoredCopy(t *testing.T) {
	config, libraries := fixture(t, `[]`)
	lib := libraries["team"]
	lib.Catalog.Groups[0].Rules[0].Document = document + "\n![diagram](assets/errors/diagram.png)"
	lib.Catalog.SupportingFiles["techs/go/assets/errors/diagram.png"] = []byte("image bytes")
	libraries["team"] = lib
	resolved, err := resolve(config, libraries, nil)
	if err != nil {
		t.Fatal(err)
	}
	output, err := renderRules(resolved)
	if err != nil {
		t.Fatal(err)
	}
	if got := output["rules/team/techs/go/errors.md"]; !strings.Contains(got, "](../../../../../vendor/team/rules/techs/go/assets/errors/diagram.png)") {
		t.Fatal(got)
	}
}

// TestRenderRejectsInvalidUTF8 prevents local bytes from being silently replaced during JSON delivery.
func TestRenderRejectsInvalidUTF8(t *testing.T) {
	output, err := renderFixture(t, "Guidance: \xff", nil)
	var validation errs.ValidationError
	if !errors.As(err, &validation) || validation.ValidationLocation() != "local:techs/go/local.md" || !strings.Contains(err.Error(), "UTF-8") || output != nil {
		t.Fatalf("expected contextual UTF-8 error without output, got %v, %v", output, err)
	}
}

// TestRenderRejectsInvalidDocumentBytes checks callers that construct a resolved value directly.
func TestRenderRejectsInvalidDocumentBytes(t *testing.T) {
	config, libraries := fixture(t, `[]`)
	resolved, err := resolve(config, libraries, nil)
	if err != nil {
		t.Fatal(err)
	}
	resolved.Groups[0].Rules[0].Rule.Document += "\xff"
	output, err := renderRules(resolved)
	var validation errs.ValidationError
	if !errors.As(err, &validation) || validation.ValidationLocation() != "team:techs/go/errors" || !strings.Contains(err.Error(), "UTF-8") || output != nil {
		t.Fatalf("expected contextual UTF-8 error without output, got %v, %v", output, err)
	}
}

// TestRenderFlattensMetadataLineEndings keeps YAML-escaped CR, LF, and CRLF inside one rendered line.
func TestRenderFlattensMetadataLineEndings(t *testing.T) {
	for _, ending := range []string{`\r`, `\n`, `\r\n`} {
		t.Run(ending, func(t *testing.T) {
			config, libraries := fixture(t, `[]`)
			value := `"Safe` + ending + `## Extra"`
			text := "---\ntitle: " + value + "\nimpact: HIGH\nimpactDescription: " + value + "\nwhenToRead: " + value + "\n---\nGuidance."
			resolved, err := resolve(config, libraries, map[string][]byte{"techs/go/local.md": []byte(text)})
			if err != nil {
				t.Fatal(err)
			}
			output, err := renderRules(resolved)
			if err != nil {
				t.Fatal(err)
			}
			got := output["rules/local/techs/go/local.md"]
			for _, prefix := range []string{"# ", "**When to read:** ", "**Why it matters:** "} {
				if !strings.Contains(got, prefix+"Safe \\#\\# Extra\n") {
					t.Errorf("metadata was not flattened for %q: %q", prefix, got)
				}
			}
		})
	}
}
