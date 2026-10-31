// Define shared authoring fields and collect metadata and body inputs before filesystem operations.

package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"unicode/utf8"

	"github.com/fabricahq/trailguide/internal/gitexec"
	"github.com/fabricahq/trailguide/internal/imports"
	"github.com/fabricahq/trailguide/internal/project"
	"github.com/fabricahq/trailguide/internal/rules"
	"github.com/fabricahq/trailguide/trailguide"
	"github.com/spf13/cobra"
)

// authoringFlags owns one command's scalar flags and resolves paths against the caller's directory.
type authoringFlags struct {
	command   *cobra.Command
	values    map[string]*singleString
	prompts   map[string]string
	directory string
	// git runs Git in a library's own repository; project commands don't use it.
	git gitexec.Options
	// imports fetches the sources a library curates skills from, for library check.
	imports imports.Options
	// version is this build's version, which commands check against the configuration's minTrailguideVersion.
	version      string
	introduction string
	prompted     bool
}

// newAuthoringCommand registers noninteractive flags with strict positional arity.
func newAuthoringCommand(use, description string, args cobra.PositionalArgs, options Options) (*cobra.Command, *authoringFlags) {
	cmd := &cobra.Command{Use: use, Short: description, Args: args}
	flags := &authoringFlags{command: cmd, values: map[string]*singleString{}, directory: options.Directory, version: options.Version}
	cmd.PostRunE = flags.finishPrompts
	cmd.Flags().Bool("non-interactive", false, "Require explicit flags; never prompt")
	return cmd, flags
}

// finishPrompts separates the last terminal answer from the completed command's output.
func (f *authoringFlags) finishPrompts(cmd *cobra.Command, _ []string) error {
	if !f.prompted {
		return nil
	}
	_, err := fmt.Fprintln(cmd.ErrOrStderr())
	return err
}

// add registers one scalar option that refuses accidental repetition.
func (f *authoringFlags) add(cmd *cobra.Command, name, description string) {
	value := &singleString{}
	f.values[name] = value
	cmd.Flags().Var(value, name, description)
}

// value returns an option or empty text when omitted.
func (f *authoringFlags) value(name string) string {
	if value := f.values[name]; value != nil {
		return value.value
	}
	return ""
}

// require diagnoses absent command inputs before an operation can write files.
func (f *authoringFlags) require(names ...string) error {
	for _, name := range names {
		if f.value(name) == "" {
			if !f.interactive() {
				return usage(fmt.Errorf("--%s is required: pass it as a flag, since without a terminal, or with --json or --non-interactive, the command can't ask for it", name))
			}
			label := f.command.Flags().Lookup(name).Usage
			if prompt, ok := f.prompts[name]; ok {
				label = prompt
			}
			value, err := f.askValidated(label+":", func(value string) error { return validateAnswer(name, value) })
			if err != nil {
				return err
			}
			f.values[name].value = value
		}
	}
	return nil
}

// file resolves user paths without changing process working directory.
func (f *authoringFlags) file(name string) string {
	value := f.value(name)
	if !filepath.IsAbs(value) {
		value = filepath.Join(f.directory, value)
	}
	return value
}

// options resolves the project target while file inputs remain relative to the caller.
func (f *authoringFlags) options(ctx context.Context, initialize bool) (project.Options, error) {
	directory, err := commandDirectory(ctx, f.directory, "project", initialize)
	return project.Options{Directory: directory, ToolVersion: f.version}, err
}

// group returns the explicit metadata for an existing or newly created group.
func (f *authoringFlags) group(prefix string) trailguide.GroupMetadata {
	return trailguide.GroupMetadata{Name: f.value(prefix + "name"), Description: f.value(prefix + "description"), WhenToRead: f.value(prefix + "when-to-read")}
}

// addGroupFlags defines the three required group fields with an optional creation prefix.
func (f *authoringFlags) addGroupFlags(cmd *cobra.Command, prefix string) {
	f.add(cmd, prefix+"name", `Title shown in rule indexes and group pages (e.g. "Testing")`)
	f.add(cmd, prefix+"description", `What this group covers (e.g. "Unit and integration testing")`)
	f.add(cmd, prefix+"when-to-read", "When an agent should read this group's rules")
	f.prompts = map[string]string{
		prefix + "name":         "Group name",
		prefix + "description":  "Group description",
		prefix + "when-to-read": "When to read",
	}
	cmd.Long = cmd.Short + "\n\nGROUP_PATH combines a category and group slug (e.g. practices/testing).\nThe name is its readable title (e.g. Testing or Testing and quality)."
}

// addRuleFlags keeps the two authoring scopes' field labels and body instructions consistent.
func (f *authoringFlags) addRuleFlags(cmd *cobra.Command) {
	cmd.Long = cmd.Short + "\n\nA rule is a Markdown file that gives agents guidance for a specific task or situation.\n\nRULE_PATH includes the group path and rule slug, without .md (e.g. practices/testing/my-rule).\nThe title is the rule's readable name. Enter metadata here, then edit the created\nMarkdown file to write the instructions and examples. Use --body-file to supply\nexisting rule text instead of creating a draft." + documentationHelp
	for name, description := range map[string]string{
		"title":              "Readable, action-oriented rule title",
		"when-to-read":       "When an agent should read this rule",
		"impact":             "Consequence level: CRITICAL, HIGH, MEDIUM-HIGH, MEDIUM, LOW-MEDIUM, or LOW",
		"impact-description": "Why this rule matters",
		"body-file":          "Read rule text from this UTF-8 Markdown file instead of creating a draft",
	} {
		f.add(cmd, name, description)
	}
	f.prompts = map[string]string{
		"title":              "Rule title",
		"when-to-read":       "When to read",
		"impact":             "Impact (CRITICAL, HIGH, MEDIUM-HIGH, MEDIUM, LOW-MEDIUM, LOW)",
		"impact-description": "Why it matters",
	}
}

// collectRule collects metadata and body after the domain has planned the target.
func (f *authoringFlags) collectRule(ctx context.Context, id string, isLibrary bool) (rules.RuleMetadata, *string, error) {
	f.introduction = ruleIntroduction(id, f.value("body-file"), isLibrary)
	if err := f.require("title", "when-to-read", "impact", "impact-description"); err != nil {
		return rules.RuleMetadata{}, nil, err
	}
	var body *string
	if f.value("body-file") != "" {
		text, err := readBody(ctx, f.file("body-file"))
		if err != nil {
			return rules.RuleMetadata{}, nil, err
		}
		body = &text
	}
	return rules.RuleMetadata{Title: f.value("title"), WhenToRead: f.value("when-to-read"), Impact: f.value("impact"), ImpactDescription: f.value("impact-description")}, body, nil
}

// readBody loads a bounded regular UTF-8 input without interpreting it as a rule document.
func readBody(ctx context.Context, name string) (string, error) {
	return readAuthoringText(ctx, name, "rule body")
}

// readAuthoringText reads bounded UTF-8 inputs without changing their original line endings or whitespace.
func readAuthoringText(ctx context.Context, name, label string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	info, err := os.Stat(name)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", label, err)
	}
	if !info.Mode().IsRegular() || info.Size() > 8*1024*1024 {
		return "", fmt.Errorf("%s must be a regular file no larger than 8 MiB", label)
	}
	file, err := os.Open(name)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", label, err)
	}
	defer file.Close()
	actual, err := file.Stat()
	if err != nil {
		return "", err
	}
	if !actual.Mode().IsRegular() {
		return "", fmt.Errorf("%s must be a regular file", label)
	}
	data, err := io.ReadAll(io.LimitReader(file, 8*1024*1024+1))
	if err != nil {
		return "", fmt.Errorf("read %s: %w", label, err)
	}
	if len(data) > 8*1024*1024 {
		return "", fmt.Errorf("%s exceeds 8 MiB", label)
	}
	if !utf8.Valid(data) {
		return "", fmt.Errorf("%s must contain valid UTF-8", label)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return string(data), nil
}
