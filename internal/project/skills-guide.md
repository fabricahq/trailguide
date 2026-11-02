# Skills

Each skill this project authors is a folder here, named for the skill, with its `SKILL.md` and any files it uses, inside a folder for where the skill comes from:

- `local/` holds the skills the project writes itself.
- A folder named for a source, such as `fabrica/`, holds the project's forks of that source's skills, which [config.yaml](../config.yaml) records under the source's `skills.forks`.

Edit the project's skills here. Trail Guide installs a copy of every skill the project includes, these and those from its sources, into each skill target that `skillTargets` in [config.yaml](../config.yaml) names, such as `.claude/skills/` and `.agents/skills/` at the repository root. Those copies are output: build, sync, and update replace them, so edits there are lost. Run `trailguide project build` after editing a skill here.

To change a skill a source supplies, fork it into the source's folder here with `trailguide project add skill --from SOURCE:SKILL`, and edit the fork. The source's own copy stays in `../_managed/vendor/`, which Trail Guide maintains.

Once the project includes a skill, `../_managed/generated/SKILLS.md` lists every installed skill, where it comes from, and where it's installed. The [project guide](../README.md) has the commands for managing skills.
