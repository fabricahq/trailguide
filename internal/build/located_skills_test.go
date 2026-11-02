// Place the authored skills that build tests write by ID in the origin folders their configuration gives them.

package build

import (
	"strings"

	"github.com/fabricahq/trailguide/internal/rules"
	"github.com/fabricahq/trailguide/internal/skills"
)

// locate returns local, authored skills by ID, as Options.LocalSkills takes them: each in the origin folder that
// config's skill origins give it, local for the project's own skills and the source's name for a fork.
func locate(config rules.Configuration, local map[string]SkillFiles) map[string]skills.Skill[[]byte] {
	result := map[string]skills.Skill[[]byte]{}
	for id, files := range local {
		name := strings.TrimPrefix(id, skills.Directory+"/")
		result[id] = skills.Skill[[]byte]{Name: name, Origin: config.SkillOrigins().For(name), Files: files.Files, Executable: files.Executable}
	}
	return result
}
