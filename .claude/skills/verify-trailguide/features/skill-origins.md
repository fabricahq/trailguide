# Skill origin folders

Every folder in a repository's `.trailguide/skills/` names where its skills come from. A library's own skills sit in `.trailguide/skills/<library.name>/`, a project's own skills in `.trailguide/skills/local/`, and a fork of a source's skill in `.trailguide/skills/<source>/`. Skill IDs stay `skills/<name>`. Library check, sync, and update refuse a `skills/` folder in any other layout, and a project's copy of a library mirrors the library's origin folders.

## Sub-features

- `origin-library-add` writes a library's new skill in `.trailguide/skills/<library.name>/`.
- `origin-library-check` rejects a skill directly in `skills/`, an origin folder the library doesn't have, and a second skill with one name.
- `origin-sync-refused` refuses to sync a library whose layout library check rejects, writing nothing.
- `origin-vendor` keeps the library's origin folders in `_managed/vendor/<source>/skills/`, so the skills' relative links resolve there.
- `origin-local` writes a project's own skill in `.trailguide/skills/local/`, and its link `../../<source>/<name>/SKILL.md` reaches that source's installed skill.
- `origin-fork` forks into `.trailguide/skills/<source>/<name>/`, where the fork's sibling links reach the source's installed skills.
- `origin-fork-merge` merges a source's change into a fork, including for a source named with a doubled hyphen, such as `team--tools`.

## How to get to it (user POV)

- In a library: `trailguide library init`, `trailguide library add skill NAME`, `trailguide library check`.
- In a project: `trailguide project add source`, `trailguide project sync`, `trailguide project add skill NAME`, `trailguide project add skill --from SOURCE:SKILL`, `trailguide project update --merge-fork SOURCE:SKILL`.

## Driving it with tg-lab

Preconditions:

- `tg-lab doctor` reports `doctor: ok`.
- A library remote `lib`: in `<lab>/remotes/lib`, write `LICENSE` and run `TG_LAB=<lab> tg-lab run lib-init -- trailguide library init --spdx MIT --license-file LICENSE --non-interactive`. `config.yaml` has `library.name: lib`.

- **Library skill.** Run `TG_LAB=<lab> tg-lab run origin-library-add -- trailguide library add skill review --description "Review a change." --type review --non-interactive`. It writes `.trailguide/skills/lib/review/SKILL.md`. Replace its body, removing the draft marker, with links to `[the checklist](references/checklist.md)`, `[audit](../audit/SKILL.md)`, and `[the guide](../../../../docs/guide.md)`; add `references/checklist.md`, a skill `audit` in `.trailguide/skills/lib/audit/` that links to `[the review checklist](../review/references/checklist.md)`, and `docs/guide.md` at the root. `trailguide library check` exits 0.
- **Rejected layouts.** Add `.trailguide/skills/flat/SKILL.md` and `.trailguide/skills/zeta/review/SKILL.md`, then run `TG_LAB=<lab> tg-lab run origin-library-check -- trailguide library check`. Exit 1, naming `skills/flat` (move it to `skills/lib/flat/`) and `skills/zeta` (not an origin folder). Remove `flat` but keep `zeta`, and commit.
- **Sync refused.** In a fresh project with the source added at `--ref main --skills '*'`, run `TG_LAB=<lab> tg-lab run origin-sync-refused -- trailguide project sync`. Exit 1, the error lists `.trailguide/skills/zeta` and ends `No files were written.`; `git status --short` is empty. Remove `zeta` from the remote, commit, and sync again: exit 0.
- **Vendor and links.** Run `TG_LAB=<lab> tg-lab run origin-vendor -- find .trailguide/_managed/vendor/lib -type f`. Skills are at `skills/lib/review/...` and `skills/lib/audit/SKILL.md`, and `docs/guide.md` is under `_repository/`. In `.claude/skills/review/SKILL.md`, the guide link points into `.trailguide/_managed/generated/references/lib/_repository/docs/guide.md` and the `audit` link stays `../audit/SKILL.md`.
- **Local skill.** Run `TG_LAB=<lab> tg-lab run origin-local -- trailguide project add skill release-checklist --description "Check a release." --type playbook --non-interactive`. It writes `.trailguide/skills/local/release-checklist/SKILL.md`. Link it to `[review](../../lib/review/SKILL.md)` and `[our notes](../../../../docs/release.md)`, add `docs/release.md`, and build: the installed copy links to `../review/SKILL.md` and `../../../docs/release.md`.
- **Fork.** Run `TG_LAB=<lab> tg-lab run origin-fork -- trailguide project add skill --from lib:skills/review --reason "Our review." --non-interactive`. It writes `.trailguide/skills/lib/review/`, keeps the `../audit/SKILL.md` link, and lists the guide link to replace. Remove that link and the draft marker, sync, and `project check` exits 0; `.claude/skills/audit/SKILL.md`'s checklist link reaches the fork's file.
- **Fork merge.** In a fresh project, add the same remote as `team--tools`, sync, fork `team--tools:skills/audit`, and build. Change `audit` in the remote and commit, then run `TG_LAB=<lab> tg-lab run origin-fork-merge -- trailguide project update team--tools --yes --merge-fork team--tools:skills/audit`. Exit 0; it changes `skills/team--tools/audit/SKILL.md`, which has the remote's change, and `project check` exits 0.

## Gotchas

- `library init` names the library after its folder; a remote created with `tg-lab remote lib` gets `library.name: lib`.
- From a library skill's `SKILL.md`, the repository root is `../../../../`, one folder deeper than before origin folders.
- A source on `ref: main` warns on every sync that it follows a branch. It's expected.
