# Broken skill links

When a skill links to a file a project can't deliver, the commands fail before writing anything. The file may be missing at the recorded revision, the link may name a folder, a symbolic link, or a rule file, or it may escape the repository. Sync and update fail with a typed error (`reference-missing`, `reference-unsupported`, `reference-escapes`) that names the file with the link, the link, its target, the source and revision, and the chain of files that reached it, and leave the project's files as they were. `trailguide library check` reports the same problems to the library's maintainer.

## Sub-features

- `err-missing-sync` fails sync for a link to a file the revision doesn't have, with `reference-missing`, and writes nothing.
- `err-chain` names the chain of files when the broken link is in a file another link reached.
- `err-folder` fails for a link to a folder with `reference-unsupported`.
- `err-escapes` fails for a link that leaves the repository with `reference-escapes`.
- `err-update` fails update the same way, leaving the lockfile at the previous commit.
- `err-library-check` reports a broken link in a library with `trailguide library check`.
- `err-json` returns the error under `error.reference` in `--json` output, with `file`, `destination`, `target`, and `chain`.

## How to get to it (user POV)

- Run `trailguide project sync` or `trailguide project update` against a source whose skills link to files it can't deliver.
- Run `trailguide library check` in a library whose skills have broken links.

## Driving it with tg-lab

Preconditions:

- `tg-lab doctor` reports `doctor: ok`.
- A non-library remote `broken` with `skills/alpha/SKILL.md` (frontmatter with `name`, `description`, `license: MIT`) linking to `[checklist](references/checklist.md)`, and `skills/alpha/references/checklist.md` linking to `[gone](gone.md)`, which doesn't exist. Commit it.
- A fresh project `app`, initialized and committed with `TG_LAB=<lab> tg-lab commit <lab>/projects/app "Init"`, with the source added at `--ref main` and `--skills skills/alpha`.

- **Missing file.** Run `TG_LAB=<lab> tg-lab run err-missing-sync -- trailguide project sync`. Exit 1; the message names `skills/alpha/references/checklist.md`, `gone.md`, the commit, and says no files were written.
- **Nothing written.** Run `TG_LAB=<lab> tg-lab run err-missing-nothing-written -- git status --short`. Only `.trailguide/config.yaml` is modified from the add-source step; no `.claude/`, `.agents/`, or `lock.json`.
- **Chain and JSON.** Run `TG_LAB=<lab> tg-lab run err-json -- trailguide project sync --json`. `error.code` is `reference-missing` and `error.reference.chain` lists `skills/alpha/SKILL.md` then `skills/alpha/references/checklist.md`.
- **Folder.** Change the remote's link to `[folder](../../docs/)` with a `docs/readme.md` present, commit, and run `TG_LAB=<lab> tg-lab run err-folder -- trailguide project sync --json`. `error.code` is `reference-unsupported`.
- **Escapes.** Change the link to `[out](../../../outside.md)` and commit, then run `TG_LAB=<lab> tg-lab run err-escapes -- trailguide project sync --json`. `error.code` is `reference-escapes`.
- **Update.** Fix the link and sync successfully, commit the project, then break the link again in a new remote commit and run `TG_LAB=<lab> tg-lab run err-update -- trailguide project update broken --yes`. Exit 1, and `git status --short` shows `lock.json` unchanged.
- **Library check.** In a library remote (see [Links to an excluded skill](./excluded-skill-links.md) for setup), link a skill to a missing file and run `TG_LAB=<lab> tg-lab run err-library-check -- trailguide library check --json`. Exit 1 with `reference-missing` and the location of the file with the link.

## Gotchas

- After a failed first sync there is no lock, so the next sync takes the branch's newest commit. After a successful sync, only update moves the source.
- The project must be committed before a failure for `git status` to prove nothing was written.
- Error messages are prefixed with the source's config key, such as `sources.broken:`; quote them in full.
