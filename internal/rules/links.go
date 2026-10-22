// Interpret CommonMark links and supporting-file ownership without filesystem access.

package rules

import (
	"net/url"
	"path"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"

	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/parser"
	"github.com/yuin/goldmark/v2/text"

	"github.com/fabricahq/trailguide/internal/librarytree"
)

var externalScheme = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*:`)

// markdownLinks finds Markdown destinations and HTML href/src attributes, excluding code and complete frontmatter envelopes.
func markdownLinks(document string) ([]string, error) {
	links, rawHTML, err := documentLinks(document)
	if err != nil {
		return nil, err
	}
	links = append(links, htmlLinks(rawHTML)...)
	slices.Sort(links)
	return slices.Compact(links), nil
}

// RawHTMLLinks returns the href and src attribute values of the raw HTML in a Markdown document, excluding code
// and complete frontmatter envelopes, sorted and without duplicates. Generation rejects the relative ones.
func RawHTMLLinks(document string) ([]string, error) {
	_, rawHTML, err := documentLinks(document)
	if err != nil {
		return nil, err
	}
	links := htmlLinks(rawHTML)
	slices.Sort(links)
	return slices.Compact(links), nil
}

// documentLinks returns a Markdown document's link, image, and reference destinations, and its raw HTML joined
// for one tokenizer, excluding code and complete frontmatter envelopes.
func documentLinks(document string) (links []string, rawHTML string, err error) {
	if !utf8.ValidString(document) {
		return nil, "", invalid("document", "expected UTF-8 text")
	}
	source := []byte(document[MarkdownBodyStart(document):])
	root := parser.New().Parse(source)
	links = []string{}
	var raw strings.Builder
	err = ast.Walk(root, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n := node.(type) {
		case *ast.RawHTML:
			raw.WriteString(n.Value.Value(source))
			raw.WriteByte('\n')
		case *ast.HTMLBlock:
			raw.Write(n.Value.Bytes(source))
			raw.WriteByte('\n')
		}
		var destination text.SingleLineValue
		switch n := node.(type) {
		case *ast.Link:
			destination = n.Destination
		case *ast.Image:
			destination = n.Destination
		case *ast.LinkReferenceDefinition:
			destination = n.Destination
		default:
			return ast.WalkContinue, nil
		}
		links = append(links, destination.Value(source))
		return ast.WalkContinue, nil
	})
	if err != nil {
		return nil, "", err
	}
	return links, raw.String(), nil
}

// htmlLinks reads real href/src attributes; one tokenizer preserves script and textarea context across inline nodes.
func htmlLinks(fragment string) []string {
	links := []string{}
	tokenizer := html.NewTokenizer(strings.NewReader(fragment))
	for {
		kind := tokenizer.Next()
		if kind == html.ErrorToken {
			return links
		}
		if kind != html.StartTagToken && kind != html.SelfClosingTagToken {
			continue
		}
		for _, attribute := range tokenizer.Token().Attr {
			if attribute.Key == "href" || attribute.Key == "src" {
				links = append(links, attribute.Val)
			}
		}
	}
}

// RelativeTarget resolves a decoded Markdown destination within its source root.
// External URLs return local=false. Query and fragment spelling remains unchanged.
func RelativeTarget(destination, file string) (target, suffix string, local bool, err error) {
	if externalScheme.MatchString(destination) || strings.HasPrefix(destination, "//") {
		return "", "", false, nil
	}
	raw := destination
	if at := strings.IndexAny(raw, "?#"); at >= 0 {
		raw, suffix = raw[:at], raw[at:]
	}
	decoded, decodeErr := url.PathUnescape(raw)
	if decodeErr != nil || !utf8.ValidString(decoded) {
		return "", "", false, invalid(file, "invalid encoded link "+destination)
	}
	if strings.ContainsRune(decoded, '\\') || strings.ContainsFunc(decoded, func(r rune) bool { return r < 32 || r == 127 }) {
		return "", "", false, invalid(file, "unsafe relative link "+destination)
	}
	target = file
	if decoded != "" {
		if strings.HasPrefix(decoded, "/") {
			target = path.Clean(strings.TrimLeft(decoded, "/"))
		} else {
			target = path.Clean(path.Join(path.Dir(file), decoded))
		}
	}
	if target == ".." || strings.HasPrefix(target, "../") {
		return "", "", false, invalid(file, "link escapes source root: "+destination)
	}
	return target, suffix, true, nil
}

// LibraryTarget resolves a decoded Markdown destination in the library file at library path file, as
// RelativeTarget does, from where the file lives in its repository, and returns the target's library path. A
// destination starting with / is relative to the Trail Guide directory, .trailguide/, as in a project's authored
// files, so /assets/diagram.svg names a shared asset. A local destination that holds no library file, such as one in
// .trailguide/_managed/vendor/, fails.
func LibraryTarget(destination, file string) (target, suffix string, local bool, err error) {
	location := librarytree.RepositoryPath(file)
	target, suffix, local, err = RelativeTarget(destination, location)
	if err != nil || !local {
		return "", "", local, err
	}
	raw, _, _ := strings.Cut(destination, "?")
	raw, _, _ = strings.Cut(raw, "#")
	if decoded, _ := url.PathUnescape(raw); strings.HasPrefix(decoded, "/") {
		target = librarytree.Directory + "/" + target
	}
	path, ok := librarytree.LibraryPath(target)
	if !ok {
		return "", "", false, invalid(file, "links to "+target+", which isn't part of the library; link to this rule's own assets directory, a shared asset in .trailguide/assets/, or a declared license or notice file")
	}
	return path, suffix, true, nil
}

// LibraryTargets returns the distinct library paths that the library file at library path file links to, as
// LibraryTarget resolves them, in path order, including HTML href/src attributes.
func LibraryTargets(document, file string) ([]string, error) {
	links, err := markdownLinks(document)
	if err != nil {
		return nil, err
	}
	targets := []string{}
	for _, link := range links {
		target, _, local, err := LibraryTarget(link, file)
		if err != nil {
			return nil, err
		}
		if local {
			targets = append(targets, target)
		}
	}
	slices.Sort(targets)
	return slices.Compact(targets), nil
}

// AssetDirectory returns a shared or rule-owned asset directory, or empty for other paths.
func AssetDirectory(file string) string {
	parts := strings.Split(file, "/")
	if len(parts) > 1 && parts[0] == "assets" {
		return "assets/"
	}
	for i := 2; i < len(parts)-2; i++ {
		if parts[i] == "assets" {
			return strings.Join(parts[:i+2], "/") + "/"
		}
	}
	return ""
}

// RuleAssetDirectory returns the directory owned by one rule, including nested rules.
func RuleAssetDirectory(file string) string {
	return path.Dir(file) + "/assets/" + strings.TrimSuffix(path.Base(file), ".md") + "/"
}

// RequireAllowedTarget permits self-links, shared/owned assets, and declared terms.
// Links to other rule documents fail regardless of selection or target existence.
func RequireAllowedTarget(file, target string, terms []string) error {
	if file == target || slices.Contains(terms, target) {
		return nil
	}
	if _, err := librarytree.GroupFromPath(target, file); err == nil {
		return invalid(file, "links to other rule documents are not allowed: "+target+"; move shared supporting material to the shared assets/ directory")
	}
	own := AssetDirectory(file)
	if own == "" {
		own = RuleAssetDirectory(file)
	}
	destination := AssetDirectory(target)
	if destination == "assets/" || (destination != "" && destination == own) {
		return nil
	}
	return invalid(file, "unsupported supporting-file link: "+target+"; use this rule's assets directory or the shared assets/ directory")
}

// MarkdownTargets returns distinct source-relative dependencies in deterministic path order, including HTML href/src attributes.
func MarkdownTargets(document, file string) ([]string, error) {
	links, err := markdownLinks(document)
	if err != nil {
		return nil, err
	}
	targets := []string{}
	for _, link := range links {
		target, _, local, err := RelativeTarget(link, file)
		if err != nil {
			return nil, err
		}
		if local {
			targets = append(targets, target)
		}
	}
	slices.Sort(targets)
	return slices.Compact(targets), nil
}
