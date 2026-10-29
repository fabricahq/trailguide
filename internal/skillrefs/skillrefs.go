// Package skillrefs owns the local file references in skill Markdown: which links name files, where they resolve
// in the repository that supplied the file, the graph of files a set of skills' links reach, which files those
// skills need retained so every link can be read, and the error that explains a link that can't be. It does no file, Git, or network access: callers lend it the
// files of one repository revision through a View, and decide where retained files are delivered.
package skillrefs

import (
	"fmt"
	"net/url"
	"path"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/fabricahq/trailguide/internal/errs"
	"github.com/fabricahq/trailguide/internal/librarytree"
	"github.com/fabricahq/trailguide/internal/rules"
)

// The reasons a reference fails. Each is also the failure's code, so scripts can tell them apart.
const (
	// Missing is a destination the source's revision doesn't have.
	Missing = "reference-missing"
	// NotRetained is a destination the project's retained copy of a source lacks, which an explicit sync restores
	// from the revision the lockfile records, as when a snapshot was taken by a Trail Guide that didn't retain files
	// for reading.
	NotRetained = "reference-not-retained"
	// ReplacementMissing is a destination in a skill the project forks whose fork has no file at that path.
	ReplacementMissing = "reference-replacement-missing"
	// Escapes is a destination outside the repository that supplied the file, or an absolute path.
	Escapes = "reference-escapes"
	// Unsupported is a destination Trail Guide can't retain: a folder, a symbolic link or submodule, or a file it
	// can't store beside a library's own files.
	Unsupported = "reference-unsupported"
)

// Error reports one local link in a skill's Markdown whose destination can't be read where the skill is delivered.
type Error struct {
	// Reason is one of the reasons above.
	Reason string
	// Source names where the file comes from, such as sources.fabrica, or "the library" for a library's own check.
	// Revision is the commit that supplied the file, or empty for a working tree.
	Source, Revision string
	// File is the repository path of the file with the link, Destination the link as written, and Target the
	// repository path it names; Target is empty when the destination names no path in the repository.
	File, Destination, Target string
	// Chain lists, from a delivered skill's file to File, the repository paths of the files whose links reach File;
	// it holds File alone when a delivered skill's own file has the link.
	Chain []string
	// Problem says why the destination can't be read and how to repair it.
	Problem string
}

var _ errs.ValidationError = (*Error)(nil)

// Error names the link, its source and revision, the reference chain when the link is indirect, and the problem.
func (e *Error) Error() string { return e.ValidationLocation() + ": " + e.ValidationProblem() }

// ValidationLocation returns the source and the file with the link.
func (e *Error) ValidationLocation() string {
	if e.Source == "" {
		return e.File
	}
	return e.Source + ": " + e.File
}

// ValidationProblem explains the failure to a human.
func (e *Error) ValidationProblem() string {
	text := "links to " + e.Destination
	if e.Target != "" && e.Target != e.Destination {
		text += " (" + e.Target + ")"
	}
	if e.Revision != "" {
		text += " at commit " + e.Revision
	}
	text += ", which " + e.Problem
	if len(e.Chain) > 1 {
		text += " The link is reached through " + strings.Join(e.Chain, " -> ") + "."
	}
	return text
}

// Link is one local file reference in a Markdown file.
type Link struct {
	// Start and End delimit the destination's bytes in the file, as rules.MarkdownDestinations locates them.
	Start, End int
	// Destination is the link as CommonMark reads it.
	Destination string
	// Target is the repository path the destination names, and Suffix its query and fragment, kept as written.
	Target, Suffix string
}

// IsMarkdown reports whether file is a Markdown file, whose links Trail Guide follows.
func IsMarkdown(file string) bool {
	extension := strings.ToLower(path.Ext(file))
	return extension == ".md" || extension == ".markdown"
}

// Links returns, in document order, the links, images, and reference definitions of data, the Markdown file at
// repository path file, that name a file in the same repository, skipping its frontmatter. It skips destinations
// with a scheme, such as https:, protocol-relative ones, and those that only name a fragment or query of the file
// itself. Code spans and fenced code hold no links. It fails with an *Error, whose Chain the caller fills, for an
// absolute path, a destination that leaves the repository, and one that can't be decoded.
func Links(file string, data []byte) ([]Link, error) {
	text := string(data)
	start := rules.MarkdownBodyStart(text)
	destinations, err := rules.MarkdownDestinations(text[start:])
	if err != nil {
		return nil, &Error{Reason: Unsupported, File: file, Destination: "a link", Problem: "can't be located in the Markdown: " + err.Error()}
	}
	links := []Link{}
	for _, destination := range destinations {
		value := destination.Value
		if value == "" || strings.HasPrefix(value, "#") || strings.HasPrefix(value, "?") || strings.HasPrefix(value, "//") || external(value) {
			continue
		}
		raw, suffix := value, ""
		if at := strings.IndexAny(raw, "?#"); at >= 0 {
			raw, suffix = raw[:at], raw[at:]
		}
		decoded, decodeErr := url.PathUnescape(raw)
		problem := ""
		switch {
		case decodeErr != nil || !utf8.ValidString(decoded) || strings.ContainsFunc(decoded, func(r rune) bool { return r < 32 || r == 127 }):
			problem = "can't be decoded as a relative path; ask the maintainer to fix the link."
		case strings.ContainsRune(decoded, '\\'):
			problem = "uses backslashes, which only some systems read as path separators; ask the maintainer to use / instead."
		}
		if problem != "" {
			return nil, &Error{Reason: Unsupported, File: file, Destination: value, Problem: problem}
		}
		if strings.HasPrefix(decoded, "/") {
			return nil, &Error{Reason: Escapes, File: file, Destination: value, Problem: "is an absolute path, so it depends on where the files are read rather than on the repository. Ask the maintainer to link with a path relative to the file."}
		}
		target := path.Clean(path.Join(path.Dir(file), decoded))
		if target == ".." || strings.HasPrefix(target, "../") {
			return nil, &Error{Reason: Escapes, File: file, Destination: value, Problem: "is outside the repository that supplies the file, so no revision of it can supply the destination. Ask the maintainer to link within the repository, or to a web address."}
		}
		links = append(links, Link{Start: start + destination.Start, End: start + destination.End, Destination: value, Target: target, Suffix: suffix})
	}
	return links, nil
}

// external reports whether destination starts with a URI scheme, such as https: or mailto:.
func external(destination string) bool {
	scheme, _, found := strings.Cut(destination, ":")
	if !found || scheme == "" || strings.ContainsAny(scheme, "/?#") {
		return false
	}
	for i, r := range scheme {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || i > 0 && (r >= '0' && r <= '9' || r == '+' || r == '-' || r == '.')) {
			return false
		}
	}
	return true
}

// Rewrite returns data, a Markdown file, with each of links whose destination to returns as a different,
// non-empty path pointing there instead, keeping the link's suffix. to receives each link and returns the new path,
// unescaped, or "" to keep the link as written.
func Rewrite(data []byte, links []Link, to func(Link) string) []byte {
	var result []byte
	last := 0
	for _, link := range links {
		replacement := to(link)
		if replacement == "" {
			continue
		}
		value := rules.EncodeLinkPath(replacement) + link.Suffix
		if value == link.Destination {
			continue
		}
		result = append(result, data[last:link.Start]...)
		result = append(result, rules.EscapeDestination(value)...)
		last = link.End
	}
	if result == nil {
		return data
	}
	return append(result, data[last:]...)
}

// Origin says how a repository's paths map to the paths of the files a project keeps from it in a source's vendor
// snapshot. A library's snapshot mirrors its Trail Guide directory, so its skills, in .trailguide/skills/, are
// kept under skills/, and its shared assets under assets/; a file outside .trailguide/ is kept under
// librarytree.RepositoryFolder at its path from the repository root. Any other repository's files keep their
// paths. Prefix is the folder of the snapshot that holds the repository's files: empty for the source's own
// repository, and curated/<name>/ for one a library curates skills from.
type Origin struct {
	Library bool
	Prefix  string
}

// Key returns the path, in the source's snapshot, of the file at repository path file, and false when a library's
// snapshot can't hold it: a file in .trailguide/ other than a skill's or a shared asset, such as a rule, which
// installed skills cite instead of linking.
func (o Origin) Key(file string) (string, bool) {
	if !o.Library {
		return o.Prefix + file, true
	}
	if inside, found := strings.CutPrefix(file, librarytree.Directory+"/"); found {
		if first, _, _ := strings.Cut(inside, "/"); first == "skills" || first == "assets" {
			return o.Prefix + inside, true
		}
		return "", false
	}
	return o.Prefix + librarytree.RepositoryFolder + "/" + file, true
}

// Path returns the repository path of the file at path key in the source's snapshot, the inverse of Key, and
// false when key isn't in the origin's folder of the snapshot.
func (o Origin) Path(key string) (string, bool) {
	inside, found := strings.CutPrefix(key, o.Prefix)
	if !found {
		return "", false
	}
	if !o.Library {
		return inside, true
	}
	if file, outside := strings.CutPrefix(inside, librarytree.RepositoryFolder+"/"); outside {
		return file, true
	}
	if first, _, _ := strings.Cut(inside, "/"); first == "skills" || first == "assets" {
		return librarytree.Directory + "/" + inside, true
	}
	return inside, true
}

// Kind is what a repository holds at a path.
type Kind int

// The kinds of entry a View reports.
const (
	Absent Kind = iota
	File
	Folder
	// Other is a symbolic link, a submodule, or anything else that isn't a regular file or a folder.
	Other
)

// View lends one revision of a repository's files by repository path. Read returns a regular file's bytes, and
// fails as the caller's limits require; Retain reads only files Stat reports as File.
type View interface {
	Stat(file string) Kind
	Read(file string) ([]byte, error)
}

// Retention is what Retain needs to collect the files a set of skills links to in one repository.
type Retention struct {
	View   View
	Origin Origin
	// Folders lists the repository paths of the skill folders the snapshot holds whole: a link to a file in one
	// needs nothing retained, and its links are followed only when the folder's files are in Start.
	Folders []string
	// Start lists the repository paths of the Markdown files of the delivered skills, whose links are followed.
	Start []string
	// Reading lists the repository paths of the folders, among Folders, of the skills the project excludes: a
	// project delivers a reading copy of a file in one that a link reaches, so the links of a Markdown file in one
	// are followed once a link reaches it.
	Reading []string
	// Absent is the reason for a destination View doesn't have, Missing or NotRetained, and AbsentProblem says what
	// to do about it.
	Absent, AbsentProblem string
	// Source and Revision identify the repository's revision in errors.
	Source, Revision string
}

// Retain returns, sorted, the repository paths of the files outside Folders that the links in Start reach,
// directly or through the Markdown files it retains or reaches in Reading, each once. A link to a file inside
// Folders must name a file the View has. It fails as Analyze does, for the first link, in Start order and then
// document order, that names a destination the View doesn't have, a folder, a symbolic link or submodule, or a
// file the snapshot can't hold.
func (r Retention) Retain() ([]string, error) {
	within := func(folders []string, file string) bool {
		return slices.ContainsFunc(folders, func(folder string) bool { return strings.HasPrefix(file, folder+"/") })
	}
	graph, err := Analyze(r.View, slices.Sorted(slices.Values(r.Start)), func(_ string, link Link) (bool, *Error) {
		switch r.View.Stat(link.Target) {
		case Absent:
			return false, &Error{Reason: r.Absent, Problem: r.AbsentProblem}
		case Folder:
			return false, &Error{Reason: Unsupported, Problem: "is a folder. Trail Guide keeps the files that links name, so ask the maintainer to link to a file in it."}
		case Other:
			return false, &Error{Reason: Unsupported, Problem: "is a symbolic link or submodule, which Trail Guide doesn't install. Ask the maintainer to replace it with a file."}
		}
		if within(r.Folders, link.Target) {
			return within(r.Reading, link.Target) && IsMarkdown(link.Target), nil
		}
		if _, ok := r.Origin.Key(link.Target); !ok {
			return false, &Error{Reason: Unsupported, Problem: "is a file in .trailguide/ that isn't a skill's file or a shared asset in .trailguide/assets/, such as a rule, which installed skills cite instead of linking. Ask the maintainer to cite a rule with trailguide-rules-used instead of linking to it, or to move the file out of .trailguide/."}
		}
		return IsMarkdown(link.Target), nil
	}, r.Source, r.Revision)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(graph.Targets(), func(file string) bool { return within(r.Folders, file) }), nil
}

// Classify decides what a link in the Markdown file at repository path from means to an analysis: whether
// Analyze follows the links of its target, a Markdown file, or a failure with the Reason and Problem of an *Error
// whose other fields Analyze fills.
type Classify func(from string, link Link) (follow bool, failure *Error)

// Reader lends the bytes of one revision of a repository's files by repository path.
type Reader interface {
	Read(file string) ([]byte, error)
}

// Graph is the analysis of the local links that a set of Markdown files in one repository revision reaches: the
// bytes and links of each file it followed, read once, and the file whose link first reached each one.
type Graph struct {
	files map[string]*analyzed
	// order lists the followed files in the order Analyze reached them.
	order []string
}

// analyzed is one followed file: its bytes, its links in document order, and the file whose link first reached
// it, or "" for a starting file.
type analyzed struct {
	data  []byte
	links []Link
	via   string
}

// Analyze reads each of start, Markdown files, through reader, and each Markdown file their links reach that
// classify follows, once each, breadth first, so cycles end. It fails with the first *Error, in start order and
// then document order, that Links returns for a file or classify returns for a link, completed with the link, the
// source and revision, and the chain of files that reaches it, and with reader's error.
func Analyze(reader Reader, start []string, classify Classify, source, revision string) (*Graph, error) {
	g := &Graph{files: map[string]*analyzed{}}
	queue := []string{}
	reach := func(file, via string) {
		if _, seen := g.files[file]; !seen {
			g.files[file] = &analyzed{via: via}
			queue = append(queue, file)
		}
	}
	for _, file := range start {
		reach(file, "")
	}
	fail := func(failure *Error) error {
		failure.Source, failure.Revision, failure.Chain = source, revision, g.Chain(failure.File)
		return failure
	}
	for len(queue) > 0 {
		file := queue[0]
		queue = queue[1:]
		g.order = append(g.order, file)
		data, err := reader.Read(file)
		if err != nil {
			return nil, err
		}
		links, err := Links(file, data)
		if err != nil {
			return nil, fail(err.(*Error))
		}
		g.files[file].data, g.files[file].links = data, links
		for _, link := range links {
			follow, failure := classify(file, link)
			if failure != nil {
				failure.File, failure.Destination, failure.Target = file, link.Destination, link.Target
				return nil, fail(failure)
			}
			if follow {
				reach(link.Target, file)
			}
		}
	}
	return g, nil
}

// Files returns the files the analysis followed, in the order it reached them.
func (g *Graph) Files() []string { return slices.Clone(g.order) }

// Data returns the bytes the analysis read of file, a file it followed.
func (g *Graph) Data(file string) []byte { return g.files[file].data }

// Links returns the links of file, a file the analysis followed, in document order.
func (g *Graph) Links(file string) []Link { return g.files[file].links }

// Targets returns, sorted, every file a followed file links to, once.
func (g *Graph) Targets() []string {
	targets := []string{}
	for _, file := range g.order {
		for _, link := range g.files[file].links {
			targets = append(targets, link.Target)
		}
	}
	slices.Sort(targets)
	return slices.Compact(targets)
}

// Chain returns the files whose links first reached file, a file the analysis followed, from a starting file to
// file itself.
func (g *Graph) Chain(file string) []string {
	chain := []string{file}
	for next := g.files[file]; next != nil && next.via != ""; next = g.files[next.via] {
		chain = append([]string{next.via}, chain...)
	}
	return chain
}

// Reach walks the links of start, files the analysis followed, breadth first, going on into a followed file only
// when through accepts it, and calls visit once for each file the walk's links name, with the chain of files from
// a file of start to the one whose link first names it. Each starting set gets its own walk, so it reports what it
// reaches through a file that another set's links reached first.
func (g *Graph) Reach(start []string, through func(file string) bool, visit func(target string, chain []string)) {
	via := map[string]string{}
	queue := []string{}
	for _, file := range start {
		if _, ok := g.files[file]; ok {
			via[file] = ""
			queue = append(queue, file)
		}
	}
	named := map[string]bool{}
	for len(queue) > 0 {
		file := queue[0]
		queue = queue[1:]
		for _, link := range g.files[file].links {
			if !named[link.Target] {
				named[link.Target] = true
				visit(link.Target, Chain(file, via))
			}
			if _, seen := via[link.Target]; seen {
				continue
			}
			if _, followed := g.files[link.Target]; followed && through(link.Target) {
				via[link.Target] = file
				queue = append(queue, link.Target)
			}
		}
	}
}

// Chain returns the files whose links reach file, from the first, which via maps to "", to file itself. via maps
// each reached file to the file whose link reached it.
func Chain(file string, via map[string]string) []string {
	chain := []string{file}
	for next := via[file]; next != ""; next = via[next] {
		if slices.Contains(chain, next) {
			break
		}
		chain = append([]string{next}, chain...)
	}
	return chain
}

// ReplacementProblem returns the problem text for a link's destination in what, a skill the project forks whose
// fork has no file at that path, with the repairs a project can make.
func ReplacementProblem(what string) string {
	return fmt.Sprintf("is in %s, and the fork has no file at that path. Add the file to the fork, ask the source's maintainer to change the link, or exclude the skill that links to it.", what)
}
