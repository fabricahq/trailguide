// Exercise library commands that read a library's release tags, through the compiled CLI and real Git history.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/test/gitfixture"
	"github.com/fabricahq/trailguide/internal/test/terminalfixture"
)

// releaseOne publishes rules a and b at 1.0.0, as the first library release must.
const releaseOne = "Library release 1.\n---\nschemaVersion: 1\nrelease: 1\nrules:\n  practices/testing/a: 1.0.0\n  practices/testing/b: 1.0.0\nchanges:\n  practices/testing/a:\n    change: new\n    summaries:\n      - Add a.\n  practices/testing/b:\n    change: new\n    summaries:\n      - Add b.\n"

// libraryRule returns a complete rule document with one line of guidance.
func libraryRule(guidance string) string {
	return "---\ntitle: Test retries\nimpact: HIGH\nimpactDescription: Catch retry bugs.\nwhenToRead: When changing retries.\n---\n" + guidance + "\n"
}

// releasedLibrary returns a fixture whose release/1 published rules a and b, and an author's full clone of it.
func releasedLibrary(t *testing.T) (*gitfixture.Fixture, string) {
	t.Helper()
	ctx := context.Background()
	fixture, err := gitfixture.New(ctx, gitfixture.Library(map[string][]byte{
		"config.yaml":                   []byte("schemaVersion: 2\nsources: {}\nlibrary: {schemaVersion: 2, name: library}\n"),
		"practices/testing/_group.yaml": []byte("name: Testing\ndescription: Testing guidance.\nwhenToRead: When testing.\n"),
		"practices/testing/a.md":        []byte(libraryRule("Test the retry limit.")),
		"practices/testing/b.md":        []byte(libraryRule("Test the backoff.")),
	}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := fixture.Close(); err != nil {
			t.Error(err)
		}
	})
	if _, err := fixture.Command(ctx, "tag", "--annotate", "--cleanup=verbatim", "--message", releaseOne, "release/1"); err != nil {
		t.Fatal(err)
	}
	dir, err := fixture.Clone(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return fixture, dir
}

// writeFiles writes each file, named by its library path, where the library repository at dir keeps it, creating
// parent directories.
func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		path := libraryFile(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

// TestLibraryCheck_PreviewsThePendingLibraryRelease shows each pending rule's versions in human and JSON output.
func TestLibraryCheck_PreviewsThePendingLibraryRelease(t *testing.T) {
	binary := buildCLI(t)
	fixture, dir := releasedLibrary(t)
	writeFiles(t, dir, map[string]string{
		"practices/testing/a.md":        libraryRule("Test the retry limit and one past it."),
		"practices/testing/retries.md":  libraryRule("Test every retry."),
		"changes/a.yaml":                "summary: Test one past the limit.\nrules:\n  practices/testing/a: minor\n",
		"changes/retries.yaml":          "summary: Add a rule about testing retries.\nrules:\n  practices/testing/retries: new\n",
		"practices/testing/_group.yaml": "name: Testing\ndescription: Guidance for testing.\nwhenToRead: When testing.\n",
	})
	out, diagnostic, code := runCLIWithEnvironment(t, binary, dir, fixture.Environment, "library", "check")
	want := "Library is valid: 1 group(s), 3 rule(s).\nWarning: License is undeclared. Decide terms before sharing this library.\n\nPending library release 2\n  practices/testing/a        minor  1.0.0 -> 1.1.0\n  practices/testing/retries  new    1.0.0\nLibrary-wide files changed since release/1:\n  practices/testing/_group.yaml\n"
	if code != 0 || diagnostic != "" || out != want {
		t.Fatalf("exit %d, stderr %q, stdout:\n%s\nwant:\n%s", code, diagnostic, out, want)
	}
	out, diagnostic, code = runCLIWithEnvironment(t, binary, dir, fixture.Environment, "library", "check", "--json")
	var response struct {
		OK    bool
		Value map[string]json.RawMessage
	}
	if err := json.Unmarshal([]byte(out), &response); err != nil || code != 0 || diagnostic != "" || !response.OK {
		t.Fatal(err, code, out, diagnostic)
	}
	wantJSON := `{"release":2,"rules":[{"id":"practices/testing/a","change":"minor","from":"1.0.0","to":"1.1.0","summaries":["Test one past the limit."]},{"id":"practices/testing/retries","change":"new","to":"1.0.0","summaries":["Add a rule about testing retries."]}],"skills":[],"libraryFiles":["practices/testing/_group.yaml"]}`
	if compact := compactJSON(t, response.Value["pendingRelease"]); compact != wantJSON {
		t.Fatalf("pendingRelease %s, want %s", compact, wantJSON)
	}
	// Counts are named for what they count, since rules is a list everywhere else.
	if string(response.Value["groupCount"]) != "1" || string(response.Value["ruleCount"]) != "3" || response.Value["groups"] != nil || response.Value["rules"] != nil {
		t.Fatalf("value %s", out)
	}
}

// TestLibraryCheck_ReportsMissingNotesAndShallowClones gives the failure's code in JSON and its repair in human output.
func TestLibraryCheck_ReportsMissingNotesAndShallowClones(t *testing.T) {
	binary := buildCLI(t)
	fixture, dir := releasedLibrary(t)
	writeFiles(t, dir, map[string]string{"practices/testing/a.md": libraryRule("Changed.")})
	out, diagnostic, code := runCLIWithEnvironment(t, binary, dir, fixture.Environment, "library", "check")
	if code != 1 || out != "" || !strings.Contains(diagnostic, "practices/testing/a changed since release/1, where its version is 1.0.0, and no pending change note names it. Record it with: trailguide library change practices/testing/a") {
		t.Fatal(code, out, diagnostic)
	}
	assertJSONError(t, binary, dir, fixture.Environment, "change-notes")
	shallow := filepath.Join(t.TempDir(), "shallow")
	if _, err := fixture.CommandIn(context.Background(), filepath.Dir(shallow), "clone", "--quiet", "--depth=1", "--template=", fixture.Repository, shallow); err != nil {
		t.Fatal(err)
	}
	out, diagnostic, code = runCLIWithEnvironment(t, binary, shallow, fixture.Environment, "library", "check")
	if code != 1 || out != "" || !strings.Contains(diagnostic, "git fetch --unshallow --tags") {
		t.Fatal(code, out, diagnostic)
	}
	assertJSONError(t, binary, shallow, fixture.Environment, "shallow-clone")
}

// assertJSONError runs library check with --json and requires a failure with code.
func assertJSONError(t *testing.T, binary, dir string, environment []string, code string) {
	t.Helper()
	out, diagnostic, exit := runCLIWithEnvironment(t, binary, dir, environment, "library", "check", "--json")
	var response struct {
		OK    bool
		Error responseError
	}
	if err := json.Unmarshal([]byte(out), &response); err != nil || exit != 1 || diagnostic != "" || response.OK || response.Error.Code != code {
		t.Fatal(err, exit, out, diagnostic)
	}
}

// compactJSON removes insignificant whitespace, keeping field order, so JSON compares as text.
func compactJSON(t *testing.T, data json.RawMessage) string {
	t.Helper()
	var compact bytes.Buffer
	if err := json.Compact(&compact, data); err != nil {
		t.Fatal(err)
	}
	return compact.String()
}

// changeNotes returns the names of the notes in the changes/ directory of the library repository at dir.
func changeNotes(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(libraryFile(dir, "changes"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

// TestLibraryChange_RecordsNotesThatCheckAccepts writes a uniquely named note on every run and reports it.
func TestLibraryChange_RecordsNotesThatCheckAccepts(t *testing.T) {
	binary := buildCLI(t)
	fixture, dir := releasedLibrary(t)
	writeFiles(t, dir, map[string]string{"practices/testing/a.md": libraryRule("Test the retry limit in Python too.")})
	args := []string{"library", "change", "practices/testing/a", "--bump", "minor", "--summary", "Add a Python example of the retry-limit test."}
	out, diagnostic, code := runCLIWithEnvironment(t, binary, dir, fixture.Environment, args...)
	notes := changeNotes(t, dir)
	generated := regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}-a-[0-9a-f]{6}\.yaml$`)
	if code != 0 || diagnostic != "" || len(notes) != 1 || !generated.MatchString(notes[0]) {
		t.Fatal(code, out, diagnostic, notes)
	}
	note := libraryFile(dir, "changes/"+notes[0])
	want := "Change note created.\nAdded:\n  .trailguide/changes/" + notes[0] + "\n\nNext: Commit the note with the change it records, then validate the library:\n  trailguide library check\n"
	if out != want {
		t.Fatalf("stdout:\n%s\nwant:\n%s", out, want)
	}
	if data, err := os.ReadFile(note); err != nil || string(data) != "summary: Add a Python example of the retry-limit test.\nrules:\n  practices/testing/a: minor\n" {
		t.Fatalf("%q %v", data, err)
	}
	if out, diagnostic, code := runCLIWithEnvironment(t, binary, dir, fixture.Environment, "library", "check"); code != 0 || !strings.Contains(out, "practices/testing/a  minor  1.0.0 -> 1.1.0") {
		t.Fatal(code, out, diagnostic)
	}
	out, diagnostic, code = runCLIWithEnvironment(t, binary, dir, fixture.Environment, append(args, "--json")...)
	var response struct {
		OK    bool
		Value authoringValue
	}
	if err := json.Unmarshal([]byte(out), &response); err != nil || code != 0 || diagnostic != "" || !response.OK {
		t.Fatal(err, code, out, diagnostic)
	}
	if len(response.Value.Added) != 1 || len(response.Value.Changed) != 0 || response.Value.Added[0] == note || !generated.MatchString(filepath.Base(response.Value.Added[0])) || len(response.Value.NextSteps) != 1 || response.Value.NextSteps[0].Commands[0] != "trailguide library check" {
		t.Fatalf("%+v", response.Value)
	}
}

// TestLibraryChange_RejectsInvalidRequests reports usage errors with exit 2 and library mismatches with a code.
func TestLibraryChange_RejectsInvalidRequests(t *testing.T) {
	binary := buildCLI(t)
	fixture, dir := releasedLibrary(t)
	writeFiles(t, dir, map[string]string{"practices/testing/a.md": libraryRule("Changed.")})
	for _, args := range [][]string{
		{},
		{"practices/testing/a", "--bump", "huge", "--summary", "Fix a."},
		{"practices/testing/a", "--bump", "minor", "--retire", "--summary", "Retire a."},
		{"practices/testing/a", "--replaced-by", "practices/testing/b", "--bump", "major", "--summary", "Replace a."},
		{"practices/testing/a", "practices/testing/b", "--retire", "--replaced-by", "practices/testing/c", "--summary", "Replace both."},
		{"practices/testing/a", "--bump", "patch", "--summary", "One.\nTwo."},
		{"practices/testing/a", "--bump", "patch", "--non-interactive"},
		{"practices/testing/a", "--summary", "Fix a.", "--json"},
	} {
		out, diagnostic, code := runCLIWithEnvironment(t, binary, dir, fixture.Environment, append([]string{"library", "change"}, args...)...)
		if code != 2 {
			t.Errorf("%q: exit %d, want 2: %s %s", args, code, out, diagnostic)
		}
	}
	if _, diagnostic, code := runCLIWithEnvironment(t, binary, dir, fixture.Environment, "library", "change", "practices/testing/a", "--bump", "patch", "--summary", "One.\nTwo."); code != 2 || !strings.Contains(diagnostic, "--summary must be one line") {
		t.Fatal(code, diagnostic)
	}
	out, _, code := runCLIWithEnvironment(t, binary, dir, fixture.Environment, "library", "change", "practices/testing/c", "--summary", "Add c.", "--json")
	var response struct{ Error responseError }
	if err := json.Unmarshal([]byte(out), &response); err != nil || code != 1 || response.Error.Code != "unknown-rule" {
		t.Fatal(err, code, out)
	}
	if notes := changeNotes(t, dir); len(notes) != 0 {
		t.Fatal("wrote notes", notes)
	}
	unreleased := t.TempDir()
	writeFiles(t, unreleased, map[string]string{"config.yaml": "schemaVersion: 2\nsources: {}\nlibrary: {schemaVersion: 2, name: library}\n", "practices/testing/_group.yaml": "name: Testing\ndescription: Testing guidance.\nwhenToRead: When testing.\n", "practices/testing/a.md": libraryRule("Test.")})
	out, _, code = runCLI(t, binary, unreleased, "library", "change", "practices/testing/a", "--summary", "Add a.", "--json")
	if err := json.Unmarshal([]byte(out), &response); err != nil || code != 1 || response.Error.Code != "no-library-release" {
		t.Fatal(err, code, out)
	}
}

// TestLibraryChange_PromptsForMissingInputs asks for the change level and summary in a terminal, retrying invalid answers.
func TestLibraryChange_PromptsForMissingInputs(t *testing.T) {
	binary := buildCLI(t)
	fixture, dir := releasedLibrary(t)
	writeFiles(t, dir, map[string]string{"practices/testing/a.md": libraryRule("Changed.")})
	args := []string{"library", "change", "practices/testing/a"}
	cancelled, err := terminalfixture.RunWithEnvironment(context.Background(), binary, dir, fixture.Environment, args, []terminalfixture.Step{{Prompt: "Change (major, minor, or patch):", Interrupt: true}})
	if err != nil || cancelled.ExitCode != 130 || !strings.Contains(cancelled.Transcript, "Error: cancelled; no files were written") || len(changeNotes(t, dir)) != 0 {
		t.Fatal(err, cancelled, changeNotes(t, dir))
	}
	steps := []terminalfixture.Step{
		{Prompt: "Change (major, minor, or patch):", Answer: "huge"},
		{Prompt: "Change (major, minor, or patch):", Answer: "minor"},
		{Prompt: "Summary:", Answer: "Add an example for custom hooks."},
	}
	result, err := terminalfixture.RunWithEnvironment(context.Background(), binary, dir, fixture.Environment, args, steps)
	if err != nil || result.ExitCode != 0 {
		t.Fatal(err, result)
	}
	for _, text := range []string{"Recording a change to: practices/testing/a", "major  Work that complied with the previous version could fail this one.", "--bump must be major, minor, or patch"} {
		if !strings.Contains(result.Transcript, text) {
			t.Fatalf("missing %q in transcript:\n%s", text, result.Transcript)
		}
	}
	notes := changeNotes(t, dir)
	if len(notes) != 1 {
		t.Fatal(notes)
	}
	if data, err := os.ReadFile(libraryFile(dir, "changes/"+notes[0])); err != nil || string(data) != "summary: Add an example for custom hooks.\nrules:\n  practices/testing/a: minor\n" {
		t.Fatalf("%q %v", data, err)
	}
}

// TestLibraryChange_ReportsMisusedFlagsAsUsageErrors exits 2 for flags the rules don't accept or a missing flag
// that it can't ask for, naming the flag, and writes nothing.
func TestLibraryChange_ReportsMisusedFlagsAsUsageErrors(t *testing.T) {
	binary := buildCLI(t)
	fixture, dir := releasedLibrary(t)
	writeFiles(t, dir, map[string]string{"practices/testing/a.md": libraryRule("Changed."), "practices/testing/new.md": libraryRule("New.")})
	for _, test := range []struct {
		args []string
		text string
	}{
		{[]string{"practices/testing/new", "--bump", "minor", "--summary", "Add it."}, "--bump isn't accepted for new rules"},
		{[]string{"practices/testing/a", "--summary", "Change it.", "--non-interactive"}, "--bump is required: pass it as a flag"},
		{[]string{"practices/testing/a", "--bump", "minor"}, "--summary is required: pass it as a flag"},
	} {
		out, diagnostic, code := runCLIWithEnvironment(t, binary, dir, fixture.Environment, append([]string{"library", "change"}, test.args...)...)
		if code != 2 || !strings.Contains(diagnostic, test.text) || len(changeNotes(t, dir)) != 0 {
			t.Fatalf("%v: exit %d, stdout %q, stderr:\n%s", test.args, code, out, diagnostic)
		}
	}
}

// TestLibraryChange_WarnsAboutAMissingReplacement in its output, after recording the retirement.
func TestLibraryChange_WarnsAboutAMissingReplacement(t *testing.T) {
	binary := buildCLI(t)
	fixture, dir := releasedLibrary(t)
	if err := os.Remove(libraryFile(dir, "practices/testing/b.md")); err != nil {
		t.Fatal(err)
	}
	out, diagnostic, code := runCLIWithEnvironment(t, binary, dir, fixture.Environment, "library", "change", "practices/testing/b", "--retire", "--replaced-by", "practices/testing/retries", "--summary", "Covered by the broader rule about testing retries.")
	if code != 0 || diagnostic != "" || !strings.Contains(out, "Warning: practices/testing/retries isn't a rule in the library yet. Add it before the next library release.\n") || len(changeNotes(t, dir)) != 1 {
		t.Fatal(code, out, diagnostic)
	}
}

// TestLibraryAddRule_NextStepsIncludeTheChangeNote after the first library release, and following them passes check.
func TestLibraryAddRule_NextStepsIncludeTheChangeNote(t *testing.T) {
	binary := buildCLI(t)
	fixture, dir := releasedLibrary(t)
	writeFiles(t, dir, map[string]string{"body.md": "Test every retry.\n"})
	add := []string{"library", "add", "rule", "practices/testing/retries", "--title", "Test retries", "--when-to-read", "When changing retries.", "--impact", "HIGH", "--impact-description", "Catch retry bugs.", "--body-file", "body.md"}
	out, diagnostic, code := runCLIWithEnvironment(t, binary, dir, fixture.Environment, add...)
	want := "\nAfter the first library release, every new rule needs a change note. After writing the rule text, record it with a summary for project maintainers:\n  trailguide library change practices/testing/retries --summary '<what the rule adds>'\n\nThen validate the library:\n  trailguide library check\n"
	if code != 0 || diagnostic != "" || !strings.HasSuffix(out, want) {
		t.Fatalf("exit %d, stderr %q, stdout:\n%s", code, diagnostic, out)
	}
	if out, diagnostic, code := runCLIWithEnvironment(t, binary, dir, fixture.Environment, "library", "check"); code != 1 || !strings.Contains(diagnostic, "practices/testing/retries is a new rule") {
		t.Fatal(code, out, diagnostic)
	}
	if out, diagnostic, code := runCLIWithEnvironment(t, binary, dir, fixture.Environment, "library", "change", "practices/testing/retries", "--summary", "Add a rule about testing retries."); code != 0 {
		t.Fatal(code, out, diagnostic)
	}
	if out, diagnostic, code := runCLIWithEnvironment(t, binary, dir, fixture.Environment, "library", "check"); code != 0 || !strings.Contains(out, "practices/testing/retries  new  1.0.0") {
		t.Fatal(code, out, diagnostic)
	}
	unreleased, err := fixture.Clone(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.CommandIn(context.Background(), unreleased, "tag", "--delete", "release/1"); err != nil {
		t.Fatal(err)
	}
	writeFiles(t, unreleased, map[string]string{"body.md": "Test every retry.\n"})
	out, diagnostic, code = runCLIWithEnvironment(t, binary, unreleased, fixture.Environment, add...)
	if code != 0 || strings.Contains(out, "library change") || !strings.HasSuffix(out, "After writing the rule text, run:\n  trailguide library check\n") {
		t.Fatalf("exit %d, stderr %q, stdout:\n%s", code, diagnostic, out)
	}
}

// TestUsageRefusals_HaveTheInvalidArgumentsCode: every refused argument or flag combination exits 2 with
// error.code invalid-arguments, so scripts can tell usage errors apart without parsing messages.
func TestUsageRefusals_HaveTheInvalidArgumentsCode(t *testing.T) {
	binary := buildCLI(t)
	fixture, dir := releasedLibrary(t)
	// A changed rule leaves only the flags to refuse.
	writeFiles(t, dir, map[string]string{"practices/testing/a.md": libraryRule("Test the retry limit, clearly.")})
	for _, args := range [][]string{
		{"library", "change", "practices/testing/a", "--bump", "major", "--retire", "--summary", "Retire a."},
		{"library", "change", "practices/testing/a", "--replaced-by", "practices/testing/b", "--bump", "major", "--summary", "Replace a."},
		{"library", "change", "practices/testing/a", "practices/testing/b", "--retire", "--replaced-by", "practices/testing/c", "--summary", "Replace both."},
		{"library", "change", "practices/testing/a", "--summary", "No bump.", "--non-interactive"},
		{"library", "change", "practices/testing/a", "--bump", "patch", "--non-interactive"},
		{"library", "change"},
		{"library", "release", "--unknown-flag"},
		{"project", "update", "--reason", "Why."},
		{"project", "update", "--keep", "team:techs/go/a", "--exclude", "team:techs/go/a", "--reason", "Why."},
		{"project", "update", "--keep", "team:techs/go/a", "--update-fork", "team:techs/go/a", "--reason", "Why."},
		{"project", "update", "--exclude", "team:techs/go/a", "--update-fork", "team:techs/go/a", "--reason", "Why."},
	} {
		out, diagnostic, code := runCLIWithEnvironment(t, binary, dir, fixture.Environment, append(args, "--json")...)
		var response struct {
			OK    bool
			Error responseError
		}
		if err := json.Unmarshal([]byte(out), &response); err != nil || code != 2 || response.Error.Kind != "usage" || response.Error.Code != "invalid-arguments" {
			t.Errorf("%v: exit %d, %v, stderr %q:\n%s", args, code, err, diagnostic, out)
		}
	}
}

// TestFlagValues_ExplainAnInvalidValueWithoutGoInternals, such as a yes-or-no flag given another value.
func TestFlagValues_ExplainAnInvalidValueWithoutGoInternals(t *testing.T) {
	binary := buildCLI(t)
	out, diagnostic, code := runCLI(t, binary, t.TempDir(), "library", "release", "--dry-run=maybe")
	if code != 2 || out != "" || !strings.HasPrefix(diagnostic, "Error: invalid value \"maybe\" for --dry-run: expected true or false.\n") || strings.Contains(diagnostic, "strconv") {
		t.Fatalf("exit %d, stdout %q, stderr:\n%s", code, out, diagnostic)
	}
}

// TestLibraryCheck_SaysThereIsNothingToPublish, as library release does, when nothing changed since the latest
// library release.
func TestLibraryCheck_SaysThereIsNothingToPublish(t *testing.T) {
	binary := buildCLI(t)
	fixture, dir := releasedLibrary(t)
	out, diagnostic, code := runCLIWithEnvironment(t, binary, dir, fixture.Environment, "library", "check")
	if code != 0 || diagnostic != "" || !strings.HasSuffix(out, "\nNothing to publish: no pending change notes and no library-wide changes since release/1.\n") || strings.Contains(out, "Pending library release") {
		t.Fatalf("exit %d, stderr %q, stdout:\n%s", code, diagnostic, out)
	}
}

// TestLibraryCheck_SaysTheFirstReleasePublishesTheLibraryWideFiles of a library with no rules yet, as library release
// would, rather than that there's nothing to publish.
func TestLibraryCheck_SaysTheFirstReleasePublishesTheLibraryWideFiles(t *testing.T) {
	binary := buildCLI(t)
	ctx := context.Background()
	fixture, err := gitfixture.New(ctx, gitfixture.Library(map[string][]byte{
		"config.yaml":                   []byte("schemaVersion: 2\nsources: {}\nlibrary: {schemaVersion: 2, name: library}\n"),
		"practices/testing/_group.yaml": []byte("name: Testing\ndescription: Testing guidance.\nwhenToRead: When testing.\n"),
	}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fixture.Close() })
	dir, err := fixture.Clone(ctx)
	if err != nil {
		t.Fatal(err)
	}
	out, diagnostic, code := runCLIWithEnvironment(t, binary, dir, fixture.Environment, "library", "check")
	if code != 0 || diagnostic != "" || strings.Contains(out, "Nothing to publish") || !strings.Contains(out, "\nPending library release 1\n  No rule changes are pending.\n  The first library release publishes the library-wide files, such as group metadata and shared assets.\n") {
		t.Fatalf("exit %d, stderr %q, stdout:\n%s", code, diagnostic, out)
	}
	// Library release agrees: it would publish library release 1.
	out, diagnostic, code = runCLIWithEnvironment(t, binary, dir, fixture.Environment, "library", "release", "--dry-run", "--no-github-release")
	if code != 0 || !strings.HasPrefix(out, "Dry run: library release 1, not published.\n") {
		t.Fatalf("dry run: exit %d, stderr %q, stdout:\n%s", code, diagnostic, out)
	}
}
