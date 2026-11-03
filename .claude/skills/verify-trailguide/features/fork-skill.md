# Fork a skill with links

A user forks an installed skill with `trailguide project add skill --from SOURCE:SKILL`, which copies its folder, linked files inside it included, into `.trailguide/skills/<source>/`, the origin folder for forks of that source's skills. The fork installs in place of the source's skill, and links from other installed skills into the forked skill reach the fork's files. When the fork lacks a file another skill links to, build fails with `reference-replacement-missing` and names the repairs.

## Sub-features

- `fork-copy` copies the skill's folder, including files its links reach inside it, and records the fork in `config.yaml`.
- `fork-install` installs the fork in place of the source's skill after build.
- `fork-links` points other installed skills' links into the forked skill at the fork's files.
- `fork-missing` fails build with `reference-replacement-missing` when the fork lacks a file another skill links to, and writes nothing.
- `fork-sibling` points the fork's links to other installed skills at their installed copies, by install name.
- `fork-outside` lists each link the fork can't keep, marks the fork as a draft, and installs it with sync once fixed.

## How to get to it (user POV)

- Run `trailguide project add skill --from SOURCE:SKILL --reason TEXT`, then `trailguide project build`.
- Edit or delete files in the fork under `.trailguide/skills/<source>/<name>/`, then build.

## Driving it with tg-lab

Preconditions:

- `tg-lab doctor` reports `doctor: ok`.
- A remote with `skills/review/SKILL.md` linking to `[checklist](references/checklist.md)`, `skills/review/references/checklist.md`, and `skills/audit/SKILL.md` linking to `[checklist](../review/references/checklist.md)`. Give non-library skills `license: MIT`, or use a library remote.
- A fresh project with the source at `--ref main`, including both skills, synced and committed.

- **Fork.** Run `TG_LAB=<lab> tg-lab run fork-copy -- trailguide project add skill --from SRC:skills/review --reason "Our review." --non-interactive`. Exit 0; `.trailguide/skills/SRC/review/SKILL.md` and `references/checklist.md` exist, and `config.yaml` has a `skills.forks` entry with `as: skills/review`.
- **Install.** Run `TG_LAB=<lab> tg-lab run fork-install -- trailguide project build`, then `TG_LAB=<lab> tg-lab run fork-install-show -- head -8 .claude/skills/review/SKILL.md`. The marker names the fork, not the source's skill.
- **Links follow the fork.** Edit `.trailguide/skills/SRC/review/references/checklist.md`, build, and run `TG_LAB=<lab> tg-lab run fork-links -- sh -c 'cat .claude/skills/audit/SKILL.md'`. The `checklist` link resolves to the installed fork's file, which has the edit.
- **Missing in fork.** Delete `.trailguide/skills/SRC/review/references/checklist.md` and its link from the fork's `SKILL.md`, then run `TG_LAB=<lab> tg-lab run fork-missing -- trailguide project build --json`. Exit 1, `error.code` is `reference-replacement-missing`, and the message names `audit`'s link and the repairs. `git status --short` shows no change under `.claude/` or `.agents/`.

- **Sibling and outside links.** With a remote whose `skills/alpha/SKILL.md` links to `[beta](../beta/SKILL.md)` and `[guide](../../docs/guide.md)`, and a project installing `alpha` and `beta`, run `TG_LAB=<lab> tg-lab run fork-outside -- trailguide project add skill --from SRC:skills/alpha --reason Ours. --non-interactive`. The output lists the `guide` link under `In the fork:`, and the next step is `trailguide project sync`. Remove that link and the `trailguide:draft` line, then run `TG_LAB=<lab> tg-lab run fork-sibling -- trailguide project sync` and `cat .claude/skills/alpha/SKILL.md`: the `beta` link reaches `.claude/skills/beta/SKILL.md`. With `skills.rename` giving beta another name, the fork's link uses that name.

## Gotchas

- A fork's link to a sibling folder, such as `../beta/SKILL.md`, names the skill the project installs from the same source as `beta`, since the fork sits in that source's origin folder. A link from one of the project's own skills reaches it as `../../SRC/beta/SKILL.md`.
- A source name may have a doubled hyphen, such as `team--tools`, though a skill name may not; the fork's folder is then `.trailguide/skills/team--tools/<name>/`, and `project update --merge-fork` writes there.

- Delete the fork's own link to the file too; otherwise build first reports the fork's own broken link as `reference-missing`.
- A fork of a required library skill fails validation; fork an offered or recommended one.
