// Lay out a skills/ folder by origin: each skill's folder sits in a folder named for where the skill comes from.

package skills

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// LocalOrigin names the origin folder of the skills a project authors when it isn't a library. A library's own
// skills sit in the folder its library.name names instead.
const LocalOrigin = "local"

// Folder returns the path, relative to skills/, of the folder of skill name in the origin folder origin, such as
// pstack/show-me-your-work. An empty origin is the folder name alone, as in a skills/ folder of library schema
// version 1, which kept every skill directly in skills/.
func Folder(origin, name string) string {
	if origin == "" {
		return name
	}
	return origin + "/" + name
}

// Layout maps the name of each skill in a skills/ folder to the origin folder that holds it, empty for a skill
// directly in skills/. A skill's ID, skills/<name>, leaves its origin out, so a skill's ID stays the same when it
// moves to another origin folder.
type Layout map[string]string

// FindLayout returns the layout of the skills/ folder whose file paths, relative to skills/, are files: each folder
// origin/name that holds a SKILL.md, and each folder name directly in skills/ that does, so it reads a skills/ folder
// of library schema version 1 as well. The origin must be a valid source name, and the skill's folder a valid skill
// name. A folder inside a skill's folder belongs to that skill, and the first origin, in path order, keeps a name two
// origins share. LayoutProblems reports everything FindLayout leaves out.
func FindLayout(files []string) Layout {
	layout := Layout{}
	sorted := slices.Sorted(slices.Values(files))
	direct := map[string]bool{}
	for _, file := range sorted {
		if folder, found := strings.CutSuffix(file, "/"+File); found && ValidName(folder) {
			direct[folder] = true
		}
	}
	for name := range direct {
		layout[name] = ""
	}
	for _, file := range sorted {
		folder, found := strings.CutSuffix(file, "/"+File)
		origin, name, nested := strings.Cut(folder, "/")
		if !found || !nested || direct[origin] || !ValidSourceName(origin) || !ValidName(name) {
			continue
		}
		if _, taken := layout[name]; !taken {
			layout[name] = origin
		}
	}
	return layout
}

// Path returns the path relative to skills/ of inside, a path in the folder of skill name, which must be in l.
func (l Layout) Path(name, inside string) string { return Folder(l[name], name) + "/" + inside }

// Split returns the name of the skill in l whose folder holds file, a path relative to skills/, and file's path in
// that folder. It reports false for a file no skill in l holds, such as the folder's guide.
func (l Layout) Split(file string) (name, inside string, ok bool) {
	for name, origin := range l {
		if inside, found := strings.CutPrefix(file, Folder(origin, name)+"/"); found {
			return name, inside, true
		}
	}
	return "", "", false
}

// Flatten returns files, keyed by path relative to skills/, keyed by name/inside instead, the path each would have
// with every skill directly in skills/. It leaves out the files no skill in l holds.
func Flatten[V any](l Layout, files map[string]V) map[string]V {
	result := make(map[string]V, len(files))
	for file, value := range files {
		if name, inside, ok := l.Split(file); ok {
			result[name+"/"+inside] = value
		}
	}
	return result
}

// FlattenPaths returns the paths of files, relative to skills/, as Flatten keys them, sorted, leaving out the paths
// no skill in l holds.
func FlattenPaths(l Layout, files []string) []string {
	result := []string{}
	for _, file := range files {
		if name, inside, ok := l.Split(file); ok {
			result = append(result, name+"/"+inside)
		}
	}
	slices.Sort(result)
	return result
}

// Origins says which origin folder each skill of a skills/ folder belongs in: Own names the repository's own
// skills, its library.name, or LocalOrigin for a project that isn't a library, and Forks maps the name of each skill
// that forks a source's skill to the name of that source, which names the fork's origin folder.
type Origins struct {
	Own   string
	Forks map[string]string
}

// For returns the origin folder skill name belongs in.
func (o Origins) For(name string) string {
	if source, ok := o.Forks[name]; ok {
		return source
	}
	return o.Own
}

// LayoutProblem is one problem LayoutProblems finds, at Location, a path relative to the repository's Trail Guide
// directory, such as skills/pstack/show-me-your-work.
type LayoutProblem struct {
	Location, Problem string
}

// LayoutProblems returns every problem in a skills/ folder whose file and directory paths, relative to skills/, are
// files and directories, sorted by location: a file directly in skills/ other than its guide, a skill folder
// directly in skills/, a folder that origins don't name, a file directly in an origin folder, a folder in an origin
// folder that has no SKILL.md or whose name breaks the skill name grammar, two skills with one name, and a skill in
// the wrong origin folder.
func LayoutProblems(files, directories []string, origins Origins) []LayoutProblem {
	problems := []LayoutProblem{}
	add := func(location, problem string) {
		problems = append(problems, LayoutProblem{Location: Directory + "/" + location, Problem: problem})
	}
	// skillFiles maps each folder in an origin folder, origin/name, to whether it holds a SKILL.md.
	skillFiles := map[string]bool{}
	topLevel := map[string]bool{}
	for _, file := range files {
		parts := strings.SplitN(file, "/", 3)
		switch {
		case len(parts) == 1 && file == Guide:
		case len(parts) == 1:
			add(file, "skills/ holds a folder for each origin, each holding skill folders, and its "+Guide+"; move this file into a skill's folder")
		case len(parts) == 2 && parts[1] == File:
			topLevel[parts[0]] = true
			add(parts[0], fmt.Sprintf("a skill's folder sits in the folder for its origin, so move it to %s/", Directory+"/"+Folder(origins.For(parts[0]), parts[0])))
		case len(parts) == 2:
			topLevel[parts[0]] = true
			add(file, "an origin folder holds only skill folders; move this file into a skill's folder")
		default:
			topLevel[parts[0]] = true
			folder := parts[0] + "/" + parts[1]
			skillFiles[folder] = skillFiles[folder] || parts[2] == File
		}
	}
	for _, directory := range directories {
		origin, rest, nested := strings.Cut(directory, "/")
		topLevel[origin] = true
		if name, _, deeper := strings.Cut(rest, "/"); nested && !deeper {
			if _, seen := skillFiles[origin+"/"+name]; !seen {
				skillFiles[origin+"/"+name] = false
			}
		}
	}
	allowed := slices.Collect(maps.Values(origins.Forks))
	allowed = append(allowed, origins.Own)
	for origin := range topLevel {
		if !slices.Contains(allowed, origin) && !hasSkillFile(files, origin) {
			add(origin, fmt.Sprintf("an origin folder is named for the repository's own skills, %s, or for a source a fork comes from; move its skills to the folder for their origin", origins.Own))
		}
	}
	seen := map[string]string{}
	for _, folder := range slices.Sorted(maps.Keys(skillFiles)) {
		origin, name, _ := strings.Cut(folder, "/")
		switch {
		case hasSkillFile(files, origin), !slices.Contains(allowed, origin):
			// The origin folder's own problem covers it.
		case !ValidName(name):
			add(folder, "a skill's folder name is its name, which uses 1 to 64 lowercase letters, digits, and hyphens, without a leading, trailing, or doubled hyphen; rename the folder")
		case !skillFiles[folder]:
			add(folder, "missing SKILL.md; each folder in an origin folder is a skill, so add its SKILL.md or move the folder out of skills/")
		case seen[name] != "":
			add(folder, fmt.Sprintf("%s is also a skill named %s, and a skill's name is its ID, %s; rename one of them", Directory+"/"+seen[name], name, Directory+"/"+name))
		case origin != origins.For(name) && origins.Forks[name] != "":
			seen[name] = folder
			add(folder, fmt.Sprintf("this skill forks a skill of the source %s, so it sits in that source's folder; move it to %s/", origins.Forks[name], Directory+"/"+Folder(origins.Forks[name], name)))
		case origin != origins.For(name):
			seen[name] = folder
			add(folder, fmt.Sprintf("this skill forks no skill of the source %s, so it's one of the repository's own skills; move it to %s/, or record the fork in sources.%s.skills.forks", origin, Directory+"/"+Folder(origins.Own, name), origin))
		default:
			seen[name] = folder
		}
	}
	slices.SortFunc(problems, func(a, b LayoutProblem) int { return strings.Compare(a.Location, b.Location) })
	return problems
}

// hasSkillFile reports whether files, paths relative to skills/, has a SKILL.md directly in folder.
func hasSkillFile(files []string, folder string) bool {
	return slices.Contains(files, folder+"/"+File)
}
