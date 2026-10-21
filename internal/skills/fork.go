// Rewrite the frontmatter of a skill's SKILL.md for its fork, a copy that a project or library authors.

package skills

import (
	"slices"
	"strconv"
	"strings"
)

// Fork returns data, a SKILL.md whose metadata is metadata, with frontmatter that names the skill name, which a
// SKILL.md from a repository that isn't a library can lack, and with skillsUsed and rulesUsed as its
// trailguide-skills-used and trailguide-rules-used. It rewrites only what differs, replacing a list it changes with a
// block string. It reports false when the result doesn't read back as exactly that, with the description, type, and
// reserved keys unchanged, such as for a list inside a flow mapping, which it can't edit.
func Fork(data []byte, metadata Metadata, name string, skillsUsed, rulesUsed []string) ([]byte, bool) {
	text := string(data)
	eol := "\n"
	if first, _, _ := strings.Cut(text, "\n"); strings.HasSuffix(first, "\r") {
		eol = "\r\n"
	}
	// A quoted name stays a string even when it reads as a number or a boolean, such as 2048.
	nameLine := "name: " + strconv.Quote(name) + eol
	switch {
	case !metadata.Frontmatter:
		text = "---" + eol + nameLine + "---" + eol + text
	case metadata.Name == "":
		opening, rest, _ := strings.Cut(text, "\n")
		text = opening + "\n" + nameLine + rest
	}
	if !slices.Equal(skillsUsed, metadata.SkillsUsed) {
		text = setMetadataList(text, SkillsUsedKey, skillsUsed, eol)
	}
	if !slices.Equal(rulesUsed, metadata.RulesUsed) {
		text = setMetadataList(text, RulesUsedKey, rulesUsed, eol)
	}
	forked, err := ReadMetadata([]byte(text), "")
	ok := err == nil && forked.Name == name && slices.Equal(forked.SkillsUsed, skillsUsed) && slices.Equal(forked.RulesUsed, rulesUsed) && forked.Description == metadata.Description && forked.Type == metadata.Type && slices.Equal(forked.ReservedKeys, metadata.ReservedKeys)
	return []byte(text), ok
}

// setMetadataList returns text, a SKILL.md whose frontmatter starts at its first line, with key of the top-level
// metadata block mapping set to ids, one per line, as a literal block string, adding the key, and the mapping, when
// missing. eol ends each line it writes. It returns text unchanged when the frontmatter has no closing line or
// metadata isn't a block mapping, which the caller detects by reading the result.
func setMetadataList(text, key string, ids []string, eol string) string {
	lines := strings.SplitAfter(text, "\n")
	content := func(i int) string { return strings.TrimSpace(lines[i]) }
	indent := func(i int) int { return len(lines[i]) - len(strings.TrimLeft(lines[i], " ")) }
	closing := slices.IndexFunc(lines[1:], func(line string) bool { return strings.TrimRight(line, "\r\n") == "---" }) + 1
	if closing == 0 {
		return text
	}
	block := func(at int) []string {
		result := []string{strings.Repeat(" ", at) + key + ": |" + eol}
		for _, id := range ids {
			result = append(result, strings.Repeat(" ", at+2)+id+eol)
		}
		return result
	}
	mapping := slices.IndexFunc(lines[1:closing], func(line string) bool { return strings.HasPrefix(line, "metadata:") }) + 1
	if mapping == 0 {
		return strings.Join(slices.Insert(lines, closing, append([]string{"metadata:" + eol}, block(2)...)...), "")
	}
	if value := strings.TrimSpace(strings.TrimPrefix(content(mapping), "metadata:")); value != "" && !strings.HasPrefix(value, "#") {
		return text
	}
	// last is the mapping's last line, and child the indentation of its keys, or -1 before its first key.
	last, child := mapping, -1
	for i := mapping + 1; i < closing && (content(i) == "" || indent(i) > 0); i++ {
		if content(i) == "" {
			continue
		}
		if child < 0 && !strings.HasPrefix(content(i), "#") {
			child = indent(i)
		}
		last = i
	}
	if child < 0 {
		child = 2
	}
	for i := mapping + 1; i <= last; i++ {
		if indent(i) != child || !strings.HasPrefix(content(i), key+":") {
			continue
		}
		// The value runs to the last line indented more than the key, which covers every scalar style.
		end := i
		for j := i + 1; j <= last && (content(j) == "" || indent(j) > child); j++ {
			if content(j) != "" {
				end = j
			}
		}
		return strings.Join(slices.Replace(lines, i, end+1, block(child)...), "")
	}
	return strings.Join(slices.Insert(lines, last+1, block(child)...), "")
}
