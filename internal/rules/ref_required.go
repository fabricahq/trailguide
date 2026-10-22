// Explain to a project what to write when one of its sources has no ref.

package rules

import "fmt"

// RefRequiredProblem explains that source has no ref, which every source needs. release is the library release
// that names the fix: the one the project records, when recorded is true, or else the library's newest. A release
// of 0 means the source isn't a library or has no library release, so only a branch, tag, or commit fits.
func RefRequiredProblem(source string, release int, recorded bool) string {
	if release == 0 {
		return fmt.Sprintf("sources.%s has no ref, and its repository has no library release to take, so the project must say which revision it takes: set sources.%s.ref in .trailguide/config.yaml to a branch, tag, or full commit SHA", source, source)
	}
	which := "the library's newest release"
	if recorded {
		which = "the release this project records"
	}
	return fmt.Sprintf("sources.%s has no ref, so .trailguide/config.yaml doesn't say which library release the project takes. Add ref: release/%d to sources.%s, %s, or ref: latest-release to take the newest release at each update, then run the command again", source, release, source, which)
}
