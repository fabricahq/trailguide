// Maintain the Trail Guide section of the repository's AGENTS.md, leaving every line outside its markers to the project.

package project

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/fabricahq/trailguide/internal/filetxn"
	"github.com/fabricahq/trailguide/internal/librarytree"
	"github.com/fabricahq/trailguide/internal/rules"
)

// agentsFile is the instruction file Trail Guide maintains a section in, at the repository root, the parent of the
// Trail Guide directory. Agents that read another file, such as Claude Code's CLAUDE.md, import or link it.
const agentsFile = "AGENTS.md"

// agentsSectionTemplate is the text between the section's markers, written for a project in .trailguide/;
// documentation pages quote it.
//
//go:embed agents-section.md
var agentsSectionTemplate string

// The section is fenced by these markers. The begin marker records the first 16 hexadecimal digits of the SHA-256 of
// the lines between the markers, so a refresh can tell an unedited section, which it replaces when the text changed,
// from a hand edit, which it refuses to overwrite. Trail Guide 0.8 and earlier also wrote their version before the
// digest; the marker still matches, and the next refresh drops it.
var (
	agentsBeginMarker = regexp.MustCompile(`^<!-- trailguide:begin (?:\S+ )?sha256:([0-9a-f]{16}) -->$`)
	agentsEndMarker   = regexp.MustCompile(`^<!-- trailguide:end -->$`)
)

// AgentsChange reports how a command changed the Trail Guide section of AGENTS.md.
type AgentsChange struct {
	Path string `json:"path"` // Relative to the repository root.
	// Action is created when the command created AGENTS.md, added when it added the section to an existing file, and
	// updated when it replaced an older section.
	Action string `json:"action"`
}

// renderAgentsSection returns the section, markers included, for a project whose Trail Guide directory is named
// directory, such as .trailguide.
func renderAgentsSection(directory string) string {
	inner := strings.ReplaceAll(agentsSectionTemplate, ".trailguide/", directory+"/")
	return fmt.Sprintf("<!-- trailguide:begin sha256:%s -->\n%s<!-- trailguide:end -->\n", agentsDigest(inner), inner)
}

func agentsDigest(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:8])
}

// agentsSection locates the Trail Guide section in AGENTS.md's lines. begin and end are -1 when it has none.
type agentsSection struct {
	lines      []string
	begin, end int
	edited     bool
}

// inspectAgentsSection finds the section in existing, failing with agents-section-invalid when its markers are
// malformed, unpaired, out of order, or repeated.
func inspectAgentsSection(existing string) (agentsSection, error) {
	s := agentsSection{lines: strings.SplitAfter(existing, "\n"), begin: -1, end: -1}
	invalid := func(problem string) error {
		return failure("agents-section-invalid", agentsFile+": "+problem+"; fix or remove the trailguide markers, then run the command again", nil)
	}
	for i, line := range s.lines {
		trimmed := strings.TrimRight(line, "\r\n")
		switch {
		case agentsBeginMarker.MatchString(trimmed):
			if s.begin != -1 {
				return s, invalid("found more than one trailguide:begin marker")
			}
			s.begin = i
		case agentsEndMarker.MatchString(trimmed):
			if s.end != -1 {
				return s, invalid("found more than one trailguide:end marker")
			}
			s.end = i
		case strings.HasPrefix(strings.TrimSpace(trimmed), "<!-- trailguide:"):
			// A damaged marker would otherwise hide the section, and the command would add a second one.
			return s, invalid("found a malformed trailguide marker")
		}
	}
	if (s.begin == -1) != (s.end == -1) || s.end < s.begin {
		return s, invalid("found a trailguide:begin or trailguide:end marker without its pair")
	}
	if s.begin != -1 {
		marker := agentsBeginMarker.FindStringSubmatch(strings.TrimRight(s.lines[s.begin], "\r\n"))
		// A checkout that converts line endings to CRLF keeps an unedited section.
		s.edited = agentsDigest(strings.ReplaceAll(strings.Join(s.lines[s.begin+1:s.end], ""), "\r\n", "\n")) != marker[1]
	}
	return s, nil
}

func (s agentsSection) found() bool { return s.begin != -1 }

// current returns the section, markers included, with LF line endings, ending in a newline.
func (s agentsSection) current() string {
	text := strings.ReplaceAll(strings.Join(s.lines[s.begin:s.end+1], ""), "\r\n", "\n")
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return text
}

// replace swaps the section, markers included, keeping every line outside it.
func (s agentsSection) replace(section string) string {
	return strings.Join(s.lines[:s.begin], "") + section + strings.Join(s.lines[s.end+1:], "")
}

// appendAgentsSection adds section after the file's own text, separated by one blank line.
func appendAgentsSection(existing, section string) string {
	if existing == "" {
		return section
	}
	if !strings.HasSuffix(existing, "\n") {
		existing += "\n"
	}
	if !strings.HasSuffix(existing, "\n\n") {
		existing += "\n"
	}
	return existing + section
}

// agentsPlan is the AGENTS.md change a command makes, in the same transaction as its other output. previous is the
// file as planned, nil when absent, and content is nil when the file needs no change. close releases the repository
// root.
type agentsPlan struct {
	repository *os.Root
	previous   []byte
	content    []byte
	change     *AgentsChange
}

// planAgentsSection plans the AGENTS.md beside the Trail Guide directory at root so it holds the current section. It
// plans no change, without reading the file, when config turns off manageAgentsFile, and when
// the section is current, or when the file has no section but already points to the project's
// _managed/generated/RULES.md, as the block Trail Guide once asked users to paste does: that file stays the project's
// until its pointer is removed. It fails with agents-pointer-outdated when such a file points to generated/RULES.md,
// where Trail Guide 0.7 and earlier kept it, with agents-section-edited when the section's lines changed since Trail
// Guide wrote them, and with agents-section-invalid when its markers are broken.
func planAgentsSection(ctx context.Context, root *os.Root, config rules.Configuration) (plan agentsPlan, err error) {
	if config.SkipAgentsFile {
		return agentsPlan{}, nil
	}
	plan.repository, err = os.OpenRoot(filepath.Dir(root.Name()))
	if err != nil {
		return agentsPlan{}, fmt.Errorf("open the repository root for %s: %w", agentsFile, err)
	}
	defer func() {
		if err != nil {
			plan.repository.Close()
			plan = agentsPlan{}
		}
	}()
	data, err := readAgentsFile(ctx, plan.repository)
	if err != nil {
		return plan, err
	}
	plan.previous = data
	directory := filepath.Base(root.Name())
	want := renderAgentsSection(directory)
	existing := string(data)
	s, err := inspectAgentsSection(existing)
	if err != nil {
		return plan, err
	}
	var content, action string
	switch {
	case !s.found() && strings.Contains(existing, directory+"/"+librarytree.Generated+"/RULES.md"):
		return plan, nil
	case !s.found() && strings.Contains(existing, directory+"/"+librarytree.LegacyGenerated+"/RULES.md"):
		return plan, failure("agents-pointer-outdated", fmt.Sprintf("%s: points agents to %s/%s/RULES.md, where Trail Guide 0.7 and earlier kept the rules, which are now in %s/%s/RULES.md. Change the path, or delete the text that points there so Trail Guide keeps a section in the file instead, then run the command again", agentsFile, directory, librarytree.LegacyGenerated, directory, librarytree.Generated), nil)
	case !s.found():
		content, action = appendAgentsSection(existing, want), "added"
		if data == nil {
			action = "created"
		}
	case s.edited:
		return plan, failure("agents-section-edited", agentsFile+": the Trail Guide section was edited after Trail Guide wrote it; move your text outside the trailguide:begin and trailguide:end markers and delete the section, then run the command again to restore it", nil)
	case s.current() == want:
		return plan, nil
	default:
		content, action = s.replace(want), "updated"
	}
	plan.content = []byte(content)
	plan.change = &AgentsChange{Path: agentsFile, Action: action}
	return plan, nil
}

// readAgentsFile reads AGENTS.md from the repository root, returning nil when it's absent, and refuses a link.
func readAgentsFile(ctx context.Context, repository *os.Root) ([]byte, error) {
	data, err := filetxn.ReadOptional(ctx, repository, agentsFile)
	var unsafe *filetxn.Error
	if errors.As(err, &unsafe) && unsafe.Code == "unsafe-file" {
		return nil, failure("unsafe-file", agentsFile+": Trail Guide keeps a section in this file and doesn't write through a link; make AGENTS.md the file, and link or import it from the file your agent reads, such as CLAUDE.md, then run the command again", err)
	}
	return data, err
}

// include adds the planned AGENTS.md to a transaction's targets and reports it in changes.
func (p agentsPlan) include(targets map[filetxn.Target]map[string][]byte, changes *FileChanges) {
	if p.content == nil {
		return
	}
	targets[filetxn.AgentsFile] = map[string][]byte{string(filetxn.AgentsFile): p.content}
	changes.Agents = p.change
}

// requireUnchanged fails with concurrent-change when AGENTS.md changed since planning, so a transaction never
// replaces text written meanwhile and check never reports a section it no longer holds.
func (p agentsPlan) requireUnchanged(ctx context.Context) error {
	if p.repository == nil {
		return nil
	}
	data, err := readAgentsFile(ctx, p.repository)
	if err != nil {
		return err
	}
	if (data == nil) != (p.previous == nil) || !bytes.Equal(data, p.previous) {
		return failure("concurrent-change", agentsFile+": changed while the command ran; run the command again", nil)
	}
	return nil
}

func (p agentsPlan) close() {
	if p.repository != nil {
		p.repository.Close()
	}
}
