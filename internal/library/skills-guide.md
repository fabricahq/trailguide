# Skills

Each folder here is a skill this library publishes, named for the skill, with its `SKILL.md` and any files it uses. Edit the library's skills here. A skill is offered unless the `library.skills` section of [config.yaml](../config.yaml) gives it another level.

Projects that include a skill install their own copy of it, in the folders their agents read, such as `.claude/skills/`. To publish a change to a skill, describe it with `trailguide library change`, run `trailguide library check`, and release the library with `trailguide library release`.

To base a skill on one from another repository, fork it into this folder with `trailguide library add skill --from SOURCE:SKILL`.
