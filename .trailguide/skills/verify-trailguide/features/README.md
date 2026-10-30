# Trail Guide verification map

This directory is the maintained source for verifying what users of the `trailguide` CLI see. Read this index before driving, then use the matching feature file as the recipe.

## Baseline preconditions

- Build a lab from the checkout under test with `tg-lab new`, and pass the printed `TG_LAB=<lab>` to every `tg-lab` call.
- `TG_LAB=<lab> tg-lab doctor` ends with `doctor: ok`, and its `built at` line names the commit you mean to verify.
- Sources are lab remotes, reached as `https://verify.invalid/NAME`. Nothing reaches the network.
- Never point the lab's `trailguide` at a real project or the trailguide checkout itself. Projects live in `<lab>/projects/`.

## Driving conventions

- Start every recipe from a fresh project unless its preconditions say otherwise. Fixture names in a recipe are literal; keep them so transcripts match across runs.
- Run commands that prove something through `tg-lab run LABEL -- ...`, and setup through `tg-lab exec -- ...`.
- Commit a remote with `tg-lab commit` after each edit, before the project syncs or updates.
- Prefer `--json` for assertions, and also capture the human output a user reads when the feature is about wording, such as warnings and errors.
- Pass `--non-interactive` to commands that would otherwise prompt.

## Proof and skip reporting

- Proof is a transcript in the evidence folder: the command, stdout, stderr, and exit code.
- A command that writes needs a second, read-only view of what it wrote: the file, `trailguide project check`, or `git status --short`.
- A command that fails must also prove what it promised about files, usually that it wrote none.
- Name transcripts by sub-feature ID, so the report can map each one to the feature it proves.
- Report an unreachable step with the attempted command and the unmet precondition. Don't report it as verified through another path.

## Feature entry contract

Each feature file starts with an H1 title and one paragraph describing the user-visible behavior. It then uses exactly four H2 sections in this order.

1. `Sub-features` lists short IDs with one line for each behavior.
2. `How to get to it (user POV)` lists every user entry point.
3. `Driving it with tg-lab` starts with `Preconditions:` and uses labeled bullets that pair each user action with an exact command and observable result.
4. `Gotchas` lists traps that can waste or invalidate a verification run.

Keep implementation details out of the map. Name only user paths, required state, commands, and observable proof.

## Features

- [Install skills from a source](./install-skills.md) covers init, adding a skill repository or library as a source, sync, listing skills, and check.
- [Skill links that leave the skill](./skill-references.md) covers retained linked files, rewritten links, reading copies, offline build and check, and repairing a project that lacks the copies.
- [Links to an excluded skill](./excluded-skill-links.md) covers the warning, its JSON form, indirect chains, and the merged warning for a skill that uses and links to the excluded skill.
- [Broken skill links](./reference-errors.md) covers the errors sync, update, and library check report for links a project can't deliver, and that they write nothing.
- [Fork a skill with links](./fork-skill.md) covers forking an installed skill whose links reach other files, and a fork that drops a linked file.
