# Skills

Each skill this library publishes is a folder here, named for the skill, with its `SKILL.md` and any files it uses, inside a folder for where the skill comes from:

- The folder named for the library, its `library.name` in [config.yaml](../config.yaml), holds the skills the library writes itself.
- A folder named for one of the library's sources holds the library's forks of that source's skills, which config.yaml records under the source's `skills.forks`.

Edit the library's skills here. A skill is offered unless the `library.skills` section of config.yaml gives it another level.

Projects that include a skill install their own copy of it, in the folders their agents read, such as `.claude/skills/`. To publish a change to a skill, describe it with `trailguide library change`, run `trailguide library check`, and release the library with `trailguide library release`.

To base a skill on one from another repository, fork it into the source's folder here with `trailguide library add skill --from SOURCE:SKILL`.
