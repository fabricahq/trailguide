# Skill links that leave the skill

An installed skill's local Markdown links keep working offline in every target. Sync keeps every file the links reach, from the recorded revision: files inside the skill's folder install with it, a link to another installed skill points at its installed copy, and any other file becomes a reading copy in `.trailguide/_managed/generated/references/<source>/`. Build and check work offline from those copies, and a project without them gets `reference-not-retained` with `trailguide project sync` as the repair.

## Sub-features

- `refs-inside` installs files a skill links to inside its own folder, unchanged, with the skill.
- `refs-sibling` rewrites a link to another installed skill so it reaches that skill's installed copy.
- `refs-reading-copy` writes a reading copy for a linked file outside every installed skill, and rewrites the link to it.
- `refs-transitive` follows links in linked Markdown files, so files reached through them are kept too.
- `refs-offline` builds and checks with no access to the source.
- `refs-not-retained` fails build and check for a project synced before skill links, and sync repairs it from the locked revision.
- `refs-copies` restores deleted reading copies with build, offline.
- `refs-authored` checks links in the project's own skills in `.trailguide/skills/`.

## How to get to it (user POV)

- Run `trailguide project sync` after adding a source whose skills link to other files.
- Open an installed copy in `.claude/skills/` or `.agents/skills/` and follow its links.
- Run `trailguide project build` and `trailguide project check` without network access.
- Write a skill with `trailguide project add skill NAME`, link it to a file, and run `trailguide project build`.

## Driving it with tg-lab

Preconditions:

- `tg-lab doctor` reports `doctor: ok`.
- A non-library remote `refs` (`TG_LAB=<lab> tg-lab remote refs`) with these files, each skill's frontmatter holding `name`, `description`, and `license: MIT`, committed with `tg-lab commit`:
  - `skills/alpha/SKILL.md` linking to `[beta](../beta/SKILL.md)`, `[guide](../../docs/guide.md)`, and `[notes](references/notes.md)`.
  - `skills/alpha/references/notes.md`, a plain Markdown file.
  - `skills/beta/SKILL.md`, with no links.
  - `docs/guide.md` linking to `[deep](deep/detail.md)`.
  - `docs/deep/detail.md`, a plain Markdown file.
- A fresh project `app`, initialized, with `trailguide project add source refs --repository https://verify.invalid/refs --ref main --skills skills/alpha --non-interactive`.

- **Sync with only alpha.** Run `TG_LAB=<lab> tg-lab run refs-sync -- trailguide project sync`. Exit 0; the output adds `references/refs/skills/beta/SKILL.md`, `references/refs/docs/guide.md`, and `references/refs/docs/deep/detail.md` under `_managed/generated/`, and `.claude/skills/alpha/references/notes.md`.
- **Inside and reading copies.** Run `TG_LAB=<lab> tg-lab run refs-reading-copy -- cat .claude/skills/alpha/SKILL.md`. The `notes` link is unchanged; the `beta` and `guide` links point into `../../../.trailguide/_managed/generated/references/refs/`. Each rewritten link resolves to an existing file: `TG_LAB=<lab> tg-lab run refs-resolve -- sh -c 'cd .claude/skills/alpha && ls ../../../.trailguide/_managed/generated/references/refs/docs/guide.md ../../../.trailguide/_managed/generated/references/refs/skills/beta/SKILL.md'`.
- **Transitive.** Run `TG_LAB=<lab> tg-lab run refs-transitive -- cat .trailguide/_managed/generated/references/refs/docs/guide.md`. Its `deep` link still reaches `docs/deep/detail.md` beside it.
- **Sibling installed.** Add `skills/beta` to `sources.refs.skills.include` in `.trailguide/config.yaml`, run `TG_LAB=<lab> tg-lab run refs-sibling -- trailguide project sync`, then `TG_LAB=<lab> tg-lab run refs-sibling-show -- cat .claude/skills/alpha/SKILL.md`. The `beta` link is now `../beta/SKILL.md`, the installed copy, and the reading copy of `skills/beta/SKILL.md` is removed.
- **Offline.** Move the remote away with `TG_LAB=<lab> tg-lab exec -- mv <lab>/remotes/refs <lab>/remotes/refs.offline`, then run `TG_LAB=<lab> tg-lab run refs-offline-build -- trailguide project build` and `TG_LAB=<lab> tg-lab run refs-offline-check -- trailguide project check`. Both exit 0. Move it back afterwards.
- **Not retained.** Only a project synced by a CLI from before skill links can lack the copies; deleting a vendor file by hand trips the lockfile's integrity check instead, which has no `reference-not-retained` code. Build v0.8.0, the last release without skill links, into the lab: `mkdir <lab>/old && git -C <checkout> archive v0.8.0 | tar -x -C <lab>/old && (cd <lab>/old && go build -o <lab>/old/trailguide ./cmd/trailguide)`. In a fresh project with the same source, run `TG_LAB=<lab> tg-lab run refs-old-sync -- <lab>/old/trailguide project sync`, then `TG_LAB=<lab> tg-lab run refs-not-retained -- trailguide project build --json`. Exit 1 with error code `reference-not-retained`, a `reference` object, and a message that names `trailguide project sync`. Run `TG_LAB=<lab> tg-lab run refs-repair -- trailguide project sync`; exit 0, `lock.json` keeps its commit, and `project check` passes again.
- **Missing reading copies.** Run `TG_LAB=<lab> tg-lab exec -- rm -rf .trailguide/_managed/generated/references`, then `TG_LAB=<lab> tg-lab run refs-copies-check -- trailguide project check` (exit 1) and `TG_LAB=<lab> tg-lab run refs-copies-build -- trailguide project build` (exit 0, writes them back from the vendor snapshot).
- **Authored skill.** Run `TG_LAB=<lab> tg-lab exec -- trailguide project add skill helper --description "Help with things." --non-interactive`, replace the body of `.trailguide/skills/helper/SKILL.md` (removing the `trailguide:draft` marker) with a link to `[readme](../../../README.md)` after creating `README.md` at the project root, and run `TG_LAB=<lab> tg-lab run refs-authored -- trailguide project build`. Exit 0; the installed copy's link resolves to the project's `README.md`. Change the link to `missing.md` and build again: exit 1 with `reference-missing`.

## Gotchas

- A fork's link to a sibling skill the project installs is rewritten to that skill's install name; a link to any other file outside the skill is listed for you to replace. See [Fork a skill with links](./fork-skill.md).
- Sync keeps the locked commit. To pick up a new commit on the remote, run `trailguide project update refs --yes`.
- Links inside fenced code blocks and web links aren't local links and stay as they are.
- `_managed/generated/` is rewritten by every build. Look at it after the command, not before.
- `trailguide project check` reports problems but never writes; use build or sync to repair.
