# Install skills from a source

A user sets up a project, adds a Git repository of skills or a library as a source, and syncs. Sync records the source's revision in `.trailguide/lock.json`, installs each included skill into `.claude/skills/` and `.agents/skills/`, and lists it in `.trailguide/_managed/generated/SKILLS.md`. `project list skills` shows what is installed and available, and `project check` confirms the generated files are current.

## Sub-features

- `install-init` creates `.trailguide/` and `AGENTS.md` in a new project.
- `install-add-source` records a source with its ref and skill selectors in `.trailguide/config.yaml`.
- `install-sync` installs the included skills into both targets and writes the lockfile.
- `install-list` lists installed and available skills with their levels.
- `install-check` reports the project up to date, and reports an edited installed copy.

## How to get to it (user POV)

- Run `trailguide project init` in a project's Git root.
- Run `trailguide project add source NAME --repository URL --ref REF --skills SELECTOR`, or edit `sources` in `.trailguide/config.yaml`.
- Run `trailguide project sync`, then `trailguide project list skills` and `trailguide project check`.

## Driving it with tg-lab

Preconditions:

- `tg-lab doctor` reports `doctor: ok`.
- A remote `skills` with two skills, created like this. Run `TG_LAB=<lab> tg-lab remote skills`, then in `<lab>/remotes/skills` write `skills/alpha/SKILL.md` with frontmatter `name: alpha`, `description: Alpha skill.`, `license: MIT` and a body, and `skills/beta/SKILL.md` the same way for `beta`. Run `TG_LAB=<lab> tg-lab commit <lab>/remotes/skills "Add skills"`.
- A project `app` from `TG_LAB=<lab> tg-lab project app`. Run the commands below in `<lab>/projects/app`.

- **Init.** Run `TG_LAB=<lab> tg-lab run install-init -- trailguide project init --non-interactive`. Exit 0; output lists `.trailguide/config.yaml` and `AGENTS.md` as added, and `config.yaml` contains `skillTargets: [claude, agents]`.
- **Add source.** Run `TG_LAB=<lab> tg-lab run install-add-source -- trailguide project add source skills --repository https://verify.invalid/skills --ref main --skills skills/alpha --non-interactive`. Exit 0; `config.yaml` has `sources.skills` with `ref: main` and `include: [skills/alpha]`, and the output's next step is `trailguide project sync`.
- **Sync.** Run `TG_LAB=<lab> tg-lab run install-sync -- trailguide project sync`. Exit 0; `.claude/skills/alpha/SKILL.md` and `.agents/skills/alpha/SKILL.md` exist, start with the `trailguide:installed skills:skills/alpha` marker after the frontmatter, and `beta` is not installed. `lock.json` records the remote's commit.
- **List.** Run `TG_LAB=<lab> tg-lab run install-list -- trailguide project list skills --json`. `skills:skills/alpha` is `installed` and `skills:skills/beta` is `available`.
- **Check.** Run `TG_LAB=<lab> tg-lab run install-check -- trailguide project check`. Exit 0 and `Status: up to date.`
- **Edited copy.** Append a line to `.claude/skills/alpha/SKILL.md`, then run `TG_LAB=<lab> tg-lab run install-check-edited -- trailguide project check`. Exit 1, naming the edited file. Run `TG_LAB=<lab> tg-lab exec -- trailguide project build` to restore it.

## Gotchas

- A non-library source requires `--ref`; without one, add source fails.
- A skill without a `license` field or `LICENSE` file fails sync with `license-not-found`.
- Sync keeps the commit the lockfile records. After committing to the remote, run `trailguide project update skills --yes` to move it (without a terminal, update only previews unless you pass `--yes`); sync alone won't pick up the change.
- A library source on `ref: main` prints a warning about following a branch on every sync. It's expected; don't report it as a failure of the feature under test.
