// Explain the limits a library's skill levels put on the projects that use it, and on the library itself.

package rules

import (
	"fmt"
)

// RequiredSkillProblem explains why a project can't exclude or fork skill id, which the library that source names
// requires in release, or 0 for a revision other than a library release.
func RequiredSkillProblem(source, id string, release int) string {
	at := ""
	if release != 0 {
		at = fmt.Sprintf(" in release %d", release)
	}
	return fmt.Sprintf("%s's library requires %s%s, so every project that uses the library installs it, and the project can't exclude or fork it. To diverge from it, stop using sources.%s, or ask the library's maintainer to make the skill recommended or offered", source, id, at, source)
}

// LibraryRequiresProblem explains why the repository can't exclude skill id of source, which its own library section
// requires, so the projects that use the library install it.
func LibraryRequiresProblem(source, id string) string {
	return fmt.Sprintf("library.skills.required lists %s:%s, so every project that uses this library installs it, and so does this repository; stop excluding it, or remove it from library.skills.required", source, id)
}
