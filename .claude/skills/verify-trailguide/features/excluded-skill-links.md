# Links to an excluded skill

Excluding a skill only keeps it out of the agents' skills folders and the available skills in `SKILLS.md`. When an installed skill links into a skill the project excludes, by ID or by level, sync keeps the files the link reaches as reading copies and warns, naming both skills by source-qualified ID, with the chain of files when the link is indirect. Build and update warn the same way, and `--json` lists each warning under `excludedSkillLinks`. A skill that both calls and links to the excluded skill gets one warning that says both.

## Sub-features

- `excl-by-id` warns and keeps reading copies when the excluded skill is excluded by ID.
- `excl-by-level` does the same when it is excluded by level, such as `recommended`, and nothing installed uses it.
- `excl-chain` adds the chain of files when the link goes through another file.
- `excl-shared` warns once for each installed skill that reaches the excluded skill through a shared file.
- `excl-uses` merges the warning for a skill that both calls (`trailguide-skills-used`) and links to the excluded skill.
- `excl-json` lists the warnings under `value.excludedSkillLinks` in sync, build, and update JSON output.
- `excl-not-installed` shows the excluded skill absent from both targets and listed under excluded skills in `SKILLS.md`.

## How to get to it (user POV)

- Add an entry to `sources.<name>.skills.exclude` in `.trailguide/config.yaml`, with an `id` and a `reason`, then run `trailguide project sync`.
- Run `trailguide project build` or `trailguide project update` on a project that already excludes the skill.

## Driving it with tg-lab

Preconditions:

- `tg-lab doctor` reports `doctor: ok`.
- A library remote `lib`. In `<lab>/remotes/lib`, write a license text to `<lab>/license.txt` and run `TG_LAB=<lab> tg-lab exec -- trailguide library init --spdx MIT --license-file <lab>/license.txt --non-interactive`. `library init` names the library `lib`, after its folder, so its own skills sit in `.trailguide/skills/lib/`. Add skills with `TG_LAB=<lab> tg-lab exec -- trailguide library add skill NAME --description "..." --non-interactive`, which writes `.trailguide/skills/lib/NAME/SKILL.md`, then replace each `SKILL.md` body, removing the `trailguide:draft` marker:
  - `review`: links to `[redesign](../redesign/SKILL.md)`.
  - `audit`: links to `[shared comparison](../../../../docs/compare.md)`, a file at the repository root.
  - `lever`: lists `skills/redesign` in `metadata.trailguide-skills-used`, and links to `[redesign](../redesign/SKILL.md)`.
  - `redesign`: links to `[steps](references/steps.md)`, with `references/steps.md` present.
  - `docs/compare.md` at the repository root: links to `[redesign](../.trailguide/skills/lib/redesign/SKILL.md)`.
  - Put `skills/redesign` under `library.skills.recommended` in `.trailguide/config.yaml`: add `skills:` with `recommended: [skills/redesign]` under `library:`.
  - Write each `SKILL.md` with a heredoc (`cat > FILE <<'EOF'`). `lever`'s frontmatter has `metadata:` with `trailguide-skills-used: |` and the line `skills/redesign` beneath it.
- `TG_LAB=<lab> tg-lab run excl-library-check -- trailguide library check` exits 0, then commit the remote.
- A fresh project `app`, initialized, with the source `lib` added at `--ref main` and `--skills '*'`.

- **By ID.** Add `exclude: [{id: skills/redesign, reason: Not here.}]` under `sources.lib.skills`, and run `TG_LAB=<lab> tg-lab run excl-by-id -- trailguide project sync`. Exit 0; stderr has one `Warning: lib:skills/review references lib:skills/redesign, which this project excludes.` line, one for `lib:skills/audit` with `The reference goes through these files of the source's repository:` and the chain through `docs/compare.md`, and one for `lib:skills/lever` that says it `uses and references` the skill.
- **Not installed.** Run `TG_LAB=<lab> tg-lab run excl-not-installed -- sh -c 'ls .claude/skills .agents/skills; grep -n redesign .trailguide/_managed/generated/SKILLS.md'`. No `redesign` folder in either target; `SKILLS.md` lists it only under excluded skills.
- **Reading copies.** Run `TG_LAB=<lab> tg-lab run excl-reading-copies -- find .trailguide/_managed/generated/references -type f`. The list includes `lib/skills/lib/redesign/SKILL.md` and `lib/skills/lib/redesign/references/steps.md`, at their paths in the library.
- **JSON.** Run `TG_LAB=<lab> tg-lab run excl-json -- trailguide project build --json`. `value.excludedSkillLinks` has one entry each for `review`, `audit`, and `lever`, sorted by skill, each with its `chain`, and `uses: true` only for `lever`.
- **By level.** In a second project, add the source with `--skills recommended --skills skills/review --skills skills/audit`, so nothing installed uses `redesign`, and exclude `[{id: recommended, reason: Not here.}]`. Run `TG_LAB=<lab> tg-lab run excl-by-level -- trailguide project sync`. The warnings for `review` and `audit` appear, and `redesign` isn't installed.
- **Update.** Commit a harmless change to the remote and run `TG_LAB=<lab> tg-lab run excl-update -- trailguide project update lib --yes`. The same warnings appear after the update is applied.

## Gotchas

- Each warning's repair names the configuration to change, such as `add skills/review to sources.lib.skills.exclude`. Following it literally should work; if it doesn't, that's a finding.
- A shared asset in `.trailguide/assets/` can't link to a skill: `library check` rejects it. Put a shared file that links to skills outside `.trailguide/`, such as `docs/compare.md`.
- An ID exclusion must name a skill the source's `include` selects; with a narrow `include`, sync rejects it. Use `include: ["*"]`.
- A skill that is only offered and not included isn't excluded, so a link to it keeps a reading copy without a warning.
- A level exclusion never removes a skill another installed skill uses, so with `lever` installed, excluding `recommended` installs `redesign` and warns about nothing. Excluding `lever` by ID as well still installs `redesign` (`SKILLS.md` says it's used by a skill the project excludes); that predates skill links, and isn't what this recipe checks.
- `trailguide project check` doesn't print these warnings; drive sync, build, or update.
- Library sources on `ref: main` add a branch warning to every sync. Count the excluded-skill warnings, not all warnings.
