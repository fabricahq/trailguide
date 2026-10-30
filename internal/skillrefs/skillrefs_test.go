// Test which Markdown links name files, where they resolve, and which files a set of skills needs retained.

package skillrefs

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestLinks_FindsLocalFileLinksOnly(t *testing.T) {
	document := "---\nname: review\ndescription: See [x](frontmatter.md).\n---\n" +
		"Compare [Redesign](../redesign/SKILL.md#when-to-use) and ![diagram](images/flow.png).\n\n" +
		"See [the checklist][list] and [the web](https://example.com/guide.md), [mail](mailto:a@example.com), [here](#steps), [query](?x=1), [shared](//cdn.example.com/a.md).\n\n" +
		"Run `cat missing/path.md` or:\n\n```sh\ncat [not a link](nowhere.md)\n```\n\n" +
		"[list]: ../../shared/review%20checklist.md\n"
	links, err := Links(".trailguide/skills/review/SKILL.md", []byte(document))
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, link := range links {
		got = append(got, link.Target+link.Suffix)
		if document[link.Start:link.End] == "" {
			t.Errorf("empty span for %s", link.Destination)
		}
	}
	want := []string{".trailguide/skills/redesign/SKILL.md#when-to-use", ".trailguide/skills/review/images/flow.png", ".trailguide/shared/review checklist.md"}
	if !slices.Equal(got, want) {
		t.Fatalf("links = %q, want %q", got, want)
	}
}

func TestLinks_RejectsAbsolutePathsAndLinksOutOfTheRepository(t *testing.T) {
	for _, test := range []struct{ destination, reason string }{
		{"/etc/passwd", Escapes},
		{"../../../outside.md", Escapes},
		{"..\\other\\SKILL.md", Unsupported},
	} {
		_, err := Links("skills/review/SKILL.md", []byte("[x]("+test.destination+")\n"))
		var failure *Error
		if !errors.As(err, &failure) || failure.Reason != test.reason || failure.Destination != test.destination {
			t.Errorf("%s: got %v, want %s", test.destination, err, test.reason)
		}
	}
}

func TestRewrite_ChangesOnlyLinksWithANewPathAndKeepsFragments(t *testing.T) {
	document := []byte("[A](../a/SKILL.md#steps) [B](../b/SKILL.md) [web](https://example.com)\n")
	links, err := Links("skills/x/SKILL.md", document)
	if err != nil {
		t.Fatal(err)
	}
	got := Rewrite(document, links, func(link Link) string {
		if link.Target == "skills/a/SKILL.md" {
			return "../../references/a b/SKILL.md"
		}
		return "../b/SKILL.md"
	})
	if want := "[A](../../references/a%20b/SKILL.md#steps) [B](../b/SKILL.md) [web](https://example.com)\n"; string(got) != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestOrigin_MapsALibrarysPathsToItsSnapshotAndBack(t *testing.T) {
	library := Origin{Library: true, Prefix: "curated/kit/"}
	for file, key := range map[string]string{".trailguide/skills/a/SKILL.md": "curated/kit/skills/a/SKILL.md", ".trailguide/assets/flow.png": "curated/kit/assets/flow.png", "docs/guide.md": "curated/kit/_repository/docs/guide.md", "assets/notes.md": "curated/kit/_repository/assets/notes.md", "skills/a/SKILL.md": "curated/kit/_repository/skills/a/SKILL.md", "config.yaml": "curated/kit/_repository/config.yaml"} {
		if got, ok := library.Key(file); !ok || got != key {
			t.Errorf("Key(%s) = %s, %v; want %s", file, got, ok, key)
		}
		if got, ok := library.Path(key); !ok || got != file {
			t.Errorf("Path(%s) = %s, %v; want %s", key, got, ok, file)
		}
	}
	for _, file := range []string{".trailguide/rules/techs/go/errors.md", ".trailguide/config.yaml"} {
		if key, ok := library.Key(file); ok {
			t.Errorf("Key(%s) = %s; want no snapshot path", file, key)
		}
	}
	if key, ok := (Origin{}).Key("skills/a/SKILL.md"); !ok || key != "skills/a/SKILL.md" {
		t.Errorf("a repository that isn't a library keeps its paths: %s, %v", key, ok)
	}
}

// memoryView lends files by repository path, with the folders their paths imply.
type memoryView map[string]string

func (v memoryView) Stat(file string) Kind {
	if _, ok := v[file]; ok {
		return File
	}
	for name := range v {
		if strings.HasPrefix(name, file+"/") {
			return Folder
		}
	}
	return Absent
}

func (v memoryView) Read(file string) ([]byte, error) { return []byte(v[file]), nil }

func TestRetain_FollowsLinksTransitivelyOnceAndEndsCycles(t *testing.T) {
	view := memoryView{
		"skills/a/SKILL.md":    "[B](../b/SKILL.md) [shared](../../shared/list.md) [again](../../shared/list.md) [own](notes.md)",
		"skills/a/notes.md":    "",
		"skills/b/SKILL.md":    "[A](../a/SKILL.md) [C](../c/SKILL.md) [image](diagram.png)",
		"skills/b/diagram.png": "png",
		"skills/c/SKILL.md":    "[B](../b/SKILL.md#top)",
		"shared/list.md":       "[B](../skills/b/SKILL.md)",
	}
	retention := Retention{View: view, Folders: []string{"skills/a"}, Start: []string{"skills/a/SKILL.md"}, Absent: Missing}
	retained, err := retention.Retain()
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"shared/list.md", "skills/b/SKILL.md", "skills/b/diagram.png", "skills/c/SKILL.md"}; !slices.Equal(retained, want) {
		t.Fatalf("retained = %v, want %v", retained, want)
	}
}

func TestRetain_ReportsTheChainToAMissingDestination(t *testing.T) {
	view := memoryView{
		"skills/a/SKILL.md": "[B](../b/SKILL.md)",
		"skills/b/SKILL.md": "[C](../c/SKILL.md)",
	}
	_, err := Retention{View: view, Folders: []string{"skills/a"}, Start: []string{"skills/a/SKILL.md"}, Absent: Missing, AbsentProblem: "the revision doesn't have.", Source: "sources.team", Revision: "abc"}.Retain()
	var failure *Error
	if !errors.As(err, &failure) || failure.Reason != Missing || failure.File != "skills/b/SKILL.md" || failure.Target != "skills/c/SKILL.md" || !slices.Equal(failure.Chain, []string{"skills/a/SKILL.md", "skills/b/SKILL.md"}) {
		t.Fatalf("got %#v", err)
	}
	if want := "sources.team: skills/b/SKILL.md: links to ../c/SKILL.md (skills/c/SKILL.md) at commit abc, which the revision doesn't have. The link is reached through skills/a/SKILL.md -> skills/b/SKILL.md."; err.Error() != want {
		t.Fatalf("message:\n%s\nwant:\n%s", err, want)
	}
}

func TestRetain_RejectsFoldersAndFilesTheSnapshotCantHold(t *testing.T) {
	view := memoryView{".trailguide/skills/a/SKILL.md": "", ".trailguide/rules/techs/go/errors.md": "", ".trailguide/skills/b/SKILL.md": ""}
	for destination, reason := range map[string]string{"../b": Unsupported, "../../rules/techs/go/errors.md": Unsupported, "../c/SKILL.md": Missing} {
		view[".trailguide/skills/a/SKILL.md"] = "[x](" + destination + ")"
		_, err := Retention{View: view, Origin: Origin{Library: true}, Folders: []string{".trailguide/skills/a"}, Start: []string{".trailguide/skills/a/SKILL.md"}, Absent: Missing}.Retain()
		var failure *Error
		if !errors.As(err, &failure) || failure.Reason != reason {
			t.Errorf("%s: got %v, want %s", destination, err, reason)
		}
	}
}

func TestAnalyze_ReadsEachFileOnceAndReachesSharedFilesFromEveryStart(t *testing.T) {
	files := memoryView{
		"a/SKILL.md":     "[shared](../docs/shared.md)\n",
		"b/SKILL.md":     "[shared](../docs/shared.md)\n",
		"docs/shared.md": "[x](../x/SKILL.md) and [x again](../x/SKILL.md)\n",
		"x/SKILL.md":     "[a](../a/SKILL.md)\n",
	}
	reads := map[string]int{}
	reader := countingReader{files, reads}
	graph, err := Analyze(reader, []string{"a/SKILL.md", "b/SKILL.md"}, func(_ string, link Link) (bool, *Error) {
		return link.Target == "docs/shared.md", nil
	}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"a/SKILL.md", "b/SKILL.md", "docs/shared.md"}; !slices.Equal(graph.Files(), want) {
		t.Errorf("files = %v, want %v", graph.Files(), want)
	}
	for file, count := range reads {
		if count != 1 {
			t.Errorf("read %s %d times", file, count)
		}
	}
	if chain := graph.Chain("docs/shared.md"); !slices.Equal(chain, []string{"a/SKILL.md", "docs/shared.md"}) {
		t.Errorf("chain = %v", chain)
	}
	if links := graph.Links("docs/shared.md"); len(links) != 2 || string(graph.Data("docs/shared.md")[links[0].Start:links[0].End]) != "../x/SKILL.md" {
		t.Errorf("links = %+v", links)
	}
	for _, start := range []string{"a/SKILL.md", "b/SKILL.md"} {
		chains := map[string][]string{}
		graph.Reach([]string{start}, func(string) bool { return true }, func(target string, chain []string) { chains[target] = chain })
		if want := []string{start, "docs/shared.md"}; !slices.Equal(chains["x/SKILL.md"], want) {
			t.Errorf("from %s, x is reached through %v, want %v", start, chains["x/SKILL.md"], want)
		}
	}
}

type countingReader struct {
	files memoryView
	reads map[string]int
}

func (r countingReader) Read(file string) ([]byte, error) {
	r.reads[file]++
	return r.files.Read(file)
}
