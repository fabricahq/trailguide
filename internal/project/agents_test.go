// Verify the AGENTS.md section keeps the project's own text and refuses edits it would lose.

package project

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/filetxn"
	"github.com/fabricahq/trailguide/internal/rules"
)

// TestRenderAgentsSection records a digest of the lines between the markers, and no version, and names the project's
// directory.
func TestRenderAgentsSection(t *testing.T) {
	section := renderAgentsSection(".trailguide", "")
	lines := strings.SplitAfter(section, "\n")
	marker := agentsBeginMarker.FindStringSubmatch(strings.TrimSuffix(lines[0], "\n"))
	if marker == nil || marker[1] != "" || lines[0] != "<!-- trailguide:begin sha256:"+marker[2]+" -->\n" || marker[2] != agentsDigest(strings.Join(lines[1:len(lines)-2], "")) || lines[len(lines)-2] != "<!-- trailguide:end -->\n" {
		t.Fatalf("markers:\n%s", section)
	}
	for _, want := range []string{"## Skills and Rules\n", "Read `.trailguide/_managed/generated/RULES.md`", "If `.trailguide/_managed/generated/SKILLS.md` exists, read it", "follow `.trailguide/README.md`"} {
		if !strings.Contains(section, want) {
			t.Fatalf("missing %q:\n%s", want, section)
		}
	}
	if legacy := renderAgentsSection(".code-rules", ""); strings.Contains(legacy, ".trailguide/") || !strings.Contains(legacy, "Read `.code-rules/_managed/generated/RULES.md`") {
		t.Fatalf("section for .code-rules/:\n%s", legacy)
	}
}

// TestPlanAgentsSection covers creating, appending, refreshing, and leaving alone, and keeps every line outside the
// markers byte for byte.
func TestPlanAgentsSection(t *testing.T) {
	current := renderAgentsSection(".trailguide", "")
	// Trail Guide 0.8 and earlier recorded their version in the begin marker, which a refresh keeps.
	versioned := strings.Replace(current, "trailguide:begin sha256:", "trailguide:begin 0.8.0 sha256:", 1)
	for name, test := range map[string]struct {
		existing *string
		want     string
		action   string
	}{
		"missing":                   {nil, current, "created"},
		"empty":                     {ptr(""), current, "added"},
		"own text":                  {ptr("# Guide\n\nRun make test."), "# Guide\n\nRun make test.\n\n" + current, "added"},
		"blank line":                {ptr("# Guide\n\n"), "# Guide\n\n" + current, "added"},
		"versioned marker":          {ptr("# Guide\n\n" + versioned + "\n## Releases\n"), "", ""},
		"versioned other directory": {ptr(strings.Replace(renderAgentsSection(".code-rules", ""), "trailguide:begin sha256:", "trailguide:begin 0.8.0 sha256:", 1)), versioned, "updated"},
		"other directory":           {ptr(renderAgentsSection(".code-rules", "")), current, "updated"},
		"current":                   {ptr("# Guide\n\n" + current + "Footer\n"), "", ""},
		"pasted":                    {ptr("## Engineering rules\n\n1. Read `.trailguide/_managed/generated/RULES.md`.\n"), "", ""},
		"crlf current":              {ptr(strings.ReplaceAll(current, "\n", "\r\n")), "", ""},
		"pasted elsewhere":          {ptr("Read `.code-rules/_managed/generated/RULES.md`.\n"), "Read `.code-rules/_managed/generated/RULES.md`.\n\n" + current, "added"},
	} {
		t.Run(name, func(t *testing.T) {
			root := agentsProject(t, test.existing)
			plan, err := planAgentsSection(context.Background(), root, rules.Configuration{})
			if err != nil {
				t.Fatal(err)
			}
			defer plan.close()
			if test.action == "" {
				if plan.content != nil {
					t.Fatalf("planned a change:\n%s", plan.content)
				}
				return
			}
			if plan.content == nil || string(plan.content) != test.want || plan.change.Action != test.action || plan.change.Path != "AGENTS.md" {
				t.Fatalf("got %+v:\n%s", plan.change, plan.content)
			}
		})
	}
}

// TestPlanAgentsSectionRefuses an edited section and broken markers, so no command overwrites what someone wrote.
func TestPlanAgentsSectionRefuses(t *testing.T) {
	current := renderAgentsSection(".trailguide", "")
	for name, test := range map[string]struct{ existing, code string }{
		"edited":      {strings.Replace(current, "Briefly report", "Always report", 1), "agents-section-edited"},
		"old pointer": {"## Engineering rules\n\n1. Read `.trailguide/generated/RULES.md`.\n", "agents-pointer-outdated"},
		"begin only":  {"# Guide\n<!-- trailguide:begin 1.2.3 sha256:0123456789abcdef -->\n", "agents-section-invalid"},
		"end first":   {"<!-- trailguide:end -->\n" + current, "agents-section-invalid"},
		"two begins":  {"<!-- trailguide:begin 1.2.3 sha256:0123456789abcdef -->\n" + current, "agents-section-invalid"},
		"two ends":    {current + "<!-- trailguide:end -->\n", "agents-section-invalid"},
		"link":        {current, "unsafe-file"},
		"malformed":   {strings.NewReplacer("trailguide:begin 1.2.3", "trailguide:begin", "trailguide:end", "trailguide:end-x").Replace(current), "agents-section-invalid"},
	} {
		t.Run(name, func(t *testing.T) {
			root := agentsProject(t, &test.existing)
			if name == "link" {
				agents := filepath.Join(filepath.Dir(root.Name()), "AGENTS.md")
				if err := os.Rename(agents, agents+".real"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink("AGENTS.md.real", agents); err != nil {
					t.Fatal(err)
				}
			}
			_, err := planAgentsSection(context.Background(), root, rules.Configuration{})
			var failure *filetxn.Error
			if !errors.As(err, &failure) || failure.Code != test.code {
				t.Fatalf("got %v, want %s", err, test.code)
			}
		})
	}
}

// TestBuildAgentsSection refreshes a versioned section through build, keeps the project's own text, and makes check
// and build fail, writing nothing, for an edited section.
func TestBuildAgentsSection(t *testing.T) {
	ctx := context.Background()
	root, options := localProject(t)
	agents := filepath.Join(options.Directory, "AGENTS.md")
	own := "# Project\n\nUse tabs.\n\n"
	data, err := os.ReadFile(agents)
	if err != nil {
		t.Fatal(err)
	}
	// A section that Trail Guide 0.8 wrote records its version, which build keeps, because 0.8 requires one.
	versioned := strings.Replace(string(data), "trailguide:begin sha256:", "trailguide:begin 0.8.0 sha256:", 1)
	if err := os.WriteFile(agents, []byte(own+versioned), 0644); err != nil {
		t.Fatal(err)
	}
	changes, err := Build(ctx, options)
	if err != nil || changes.Agents != nil {
		t.Fatalf("%+v %v", changes, err)
	}
	if got, err := os.ReadFile(agents); err != nil || string(got) != own+versioned {
		t.Fatalf("%s %v", got, err)
	}
	if result, err := Check(ctx, options); err != nil || !result.Current() {
		t.Fatalf("%+v %v", result, err)
	}
	edited := own + strings.Replace(renderAgentsSection(".trailguide", ""), "Briefly report", "Always report", 1)
	if err := os.WriteFile(agents, []byte(edited), 0644); err != nil {
		t.Fatal(err)
	}
	var failure *filetxn.Error
	if _, err := Check(ctx, options); !errors.As(err, &failure) || failure.Code != "agents-section-edited" {
		t.Fatalf("check: %v", err)
	}
	writeFixture(t, root, "_managed/generated/RULES.md", "stale")
	if _, err := Build(ctx, options); !errors.As(err, &failure) || failure.Code != "agents-section-edited" {
		t.Fatalf("build: %v", err)
	}
	if got, err := os.ReadFile(agents); err != nil || string(got) != edited {
		t.Fatalf("build changed the edited section: %s %v", got, err)
	}
	if generated, err := filetxn.ReadOptional(ctx, root, "_managed/generated/RULES.md"); err != nil || string(generated) != "stale" {
		t.Fatalf("a refused build wrote generated output: %v", err)
	}
}

// TestAgentsFileTurnedOff leaves AGENTS.md alone, and unchecked, when the configuration turns off manageAgentsFile.
func TestAgentsFileTurnedOff(t *testing.T) {
	ctx := context.Background()
	root, options := localProject(t)
	writeFixture(t, root, "config.yaml", "schemaVersion: 2\nmanageAgentsFile: false\nsources: {}\n")
	agents := filepath.Join(options.Directory, "AGENTS.md")
	if err := os.Remove(agents); err != nil {
		t.Fatal(err)
	}
	if changes, err := Build(ctx, options); err != nil || changes.Agents != nil {
		t.Fatalf("%+v %v", changes, err)
	}
	if result, err := Check(ctx, options); err != nil || !result.Current() {
		t.Fatalf("%+v %v", result, err)
	}
	if result, err := Initialize(ctx, options); err != nil || len(result.Written()) != 0 {
		t.Fatalf("%+v %v", result, err)
	}
	if _, err := os.Lstat(agents); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("wrote AGENTS.md: %v", err)
	}
}

// TestCheckAcceptsAPastedSection leaves a pointer to RULES.md outside the markers to the project, as earlier versions
// asked users to paste it.
func TestCheckAcceptsAPastedSection(t *testing.T) {
	ctx := context.Background()
	_, options := localProject(t)
	pasted := "## Engineering rules\n\n1. Read `.trailguide/_managed/generated/RULES.md` and follow its instructions.\n"
	agents := filepath.Join(options.Directory, "AGENTS.md")
	if err := os.WriteFile(agents, []byte(pasted), 0644); err != nil {
		t.Fatal(err)
	}
	if changes, err := Build(ctx, options); err != nil || changes.Agents != nil {
		t.Fatalf("%+v %v", changes, err)
	}
	if result, err := Check(ctx, options); err != nil || !result.Current() {
		t.Fatalf("%+v %v", result, err)
	}
	if got, err := os.ReadFile(agents); err != nil || string(got) != pasted {
		t.Fatalf("%s %v", got, err)
	}
}

// agentsProject returns the Trail Guide directory of a repository whose AGENTS.md holds existing, or has none when nil.
func agentsProject(t *testing.T, existing *string) *os.Root {
	t.Helper()
	repository := t.TempDir()
	if err := os.Mkdir(filepath.Join(repository, ".trailguide"), 0755); err != nil {
		t.Fatal(err)
	}
	if existing != nil {
		if err := os.WriteFile(filepath.Join(repository, "AGENTS.md"), []byte(*existing), 0644); err != nil {
			t.Fatal(err)
		}
	}
	root, err := os.OpenRoot(filepath.Join(repository, ".trailguide"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { root.Close() })
	return root
}

func ptr(s string) *string { return &s }

// TestAgentsPlanNoticesAChange fails with concurrent-change when AGENTS.md changes, appears, or disappears after
// planning, so no command replaces text written meanwhile and check reports what the file holds.
func TestAgentsPlanNoticesAChange(t *testing.T) {
	for name, test := range map[string]struct {
		existing *string
		change   func(string) error
	}{
		"edited":    {ptr("# Guide\n"), func(file string) error { return os.WriteFile(file, []byte("# Other\n"), 0644) }},
		"created":   {nil, func(file string) error { return os.WriteFile(file, nil, 0644) }},
		"removed":   {ptr("# Guide\n"), os.Remove},
		"unchanged": {ptr("# Guide\n"), func(string) error { return nil }},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			root := agentsProject(t, test.existing)
			plan, err := planAgentsSection(ctx, root, rules.Configuration{})
			if err != nil {
				t.Fatal(err)
			}
			defer plan.close()
			if err := test.change(filepath.Join(filepath.Dir(root.Name()), "AGENTS.md")); err != nil {
				t.Fatal(err)
			}
			err = plan.requireUnchanged(ctx)
			if name == "unchanged" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			var failure *filetxn.Error
			if !errors.As(err, &failure) || failure.Code != "concurrent-change" {
				t.Fatalf("got %v, want concurrent-change", err)
			}
		})
	}
}

// TestInitRefusesAnEditedSectionFirst fails before writing any scaffolding when AGENTS.md holds an edited section.
func TestInitRefusesAnEditedSectionFirst(t *testing.T) {
	repository := t.TempDir()
	edited := strings.Replace(renderAgentsSection(".trailguide", ""), "Briefly report", "Always report", 1)
	if err := os.WriteFile(filepath.Join(repository, "AGENTS.md"), []byte(edited), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := Initialize(context.Background(), Options{Directory: repository})
	var failure *filetxn.Error
	if !errors.As(err, &failure) || failure.Code != "agents-section-edited" {
		t.Fatalf("got %v, want agents-section-edited", err)
	}
	if _, err := os.Lstat(filepath.Join(repository, ".trailguide", "config.yaml")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("init wrote config.yaml: %v", err)
	}
}
