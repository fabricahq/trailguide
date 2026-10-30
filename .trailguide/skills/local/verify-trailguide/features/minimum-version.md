# Minimum Trail Guide version

A project or library records the oldest Trail Guide release that may work on it as `minTrailguideVersion` in `.trailguide/config.yaml`. `project init` and `library init` write the running release, `project update` raises it to the running release and never lowers it, and every command run by an older release fails with `upgrade-required` and writes nothing. Development and prerelease builds neither write nor check the field. The `AGENTS.md` section's begin marker no longer names a version, so builds by different releases agree on it.

## Sub-features

- `min-init` writes `minTrailguideVersion` with the running release on `project init` and `library init`.
- `min-dev` writes no `minTrailguideVersion` from a development build, and a development build runs on a project that requires any release.
- `min-raise` previews the raise in human and JSON output, writes nothing without `--yes`, and applies it with `--yes`.
- `min-no-lower` leaves the field alone when the running release is older or equal; a second update has nothing to do.
- `min-refuse` fails build, check, sync, update, and list from an older release with `upgrade-required`, naming both versions, and writes nothing.
- `min-library` refuses library commands from an older release, while consumers of that library, on any release, are unaffected.
- `min-workflow` writes a library check workflow that installs the minimum, and warns on update when an older workflow pins a release the raise refuses.
- `min-marker` rewrites an `AGENTS.md` begin marker that names a version once, then leaves it unchanged across releases.

## How to get to it (user POV)

- Run `trailguide project init` or `trailguide library init` with a release build.
- Run `trailguide project update` with a newer release, then again with `--yes`.
- Run any project or library command with a release older than the configuration's `minTrailguideVersion`.
- Edit `minTrailguideVersion` by hand to raise it.

## Driving it with tg-lab

Preconditions:

- `tg-lab doctor` reports `doctor: ok`.
- Release binaries built with `TG_LAB=<lab> tg-lab version 0.9.0` and `TG_LAB=<lab> tg-lab version 0.10.0`. They land on the lab's PATH as `trailguide-0.9.0` and `trailguide-0.10.0`. For a release that predates the field, build a checkout of its tag: `git worktree add /tmp/v080 v0.8.0`, then `TG_LAB=<lab> tg-lab version 0.8.0 /tmp/v080`.
- A remote with one skill, such as `skills/review/SKILL.md` with `license: MIT`, committed.

- **Init.** In a fresh project, run `TG_LAB=<lab> tg-lab run min-init -- trailguide-0.9.0 project init --non-interactive`, then `TG_LAB=<lab> tg-lab run min-init-show -- grep minTrailguideVersion .trailguide/config.yaml`. The line is `minTrailguideVersion: 0.9.0`, right after `schemaVersion`.
- **Development build.** In another fresh project, run `TG_LAB=<lab> tg-lab run min-dev -- trailguide project init --non-interactive`. The configuration has no `minTrailguideVersion`. In the first project, `TG_LAB=<lab> tg-lab run min-dev-runs -- trailguide project build` exits 0, and so does `trailguide project check` after it. Init alone writes no generated output, so check reports the project out of date until a build.
- **Raise preview.** Add the source with `--ref main`, sync, and commit. Run `TG_LAB=<lab> tg-lab run min-raise-preview -- trailguide-0.10.0 project update` and `TG_LAB=<lab> tg-lab run min-raise-preview-json -- trailguide-0.10.0 project update --json`. The human output ends its changes with `Sets minTrailguideVersion to 0.10.0 in .trailguide/config.yaml, so older releases of trailguide refuse this project.` and says it is a preview; JSON has `value.setsMinTrailguideVersion` `0.10.0`. `git status --short` is empty.
- **Raise apply.** Run `TG_LAB=<lab> tg-lab run min-raise-apply -- trailguide-0.10.0 project update --yes`. The output says `Set minTrailguideVersion to 0.10.0`, and the configuration says `0.10.0`. `TG_LAB=<lab> tg-lab run min-no-lower -- trailguide-0.10.0 project update` reports nothing to update.
- **Refuse.** Commit, then run `TG_LAB=<lab> tg-lab run min-refuse-build -- trailguide-0.9.0 project build --json`, and the same for `project check`, `project sync`, `project update --yes`, and `project list skills`. Each exits 1 with `error.code` `upgrade-required` and a message naming `0.10.0` and `0.9.0`. `git status --short` stays empty.
- **Legacy release.** Run `TG_LAB=<lab> tg-lab run min-refuse-legacy -- trailguide-0.8.0 project check`. It fails with `Error: unknown field minTrailguideVersion.` and `Location: configuration`, which the configuration reference quotes.
- **Library.** In a library remote, run `TG_LAB=<lab> tg-lab run min-library-init -- trailguide-0.10.0 library init --spdx MIT --license-file FILE --non-interactive`, add a skill, and commit. `TG_LAB=<lab> tg-lab run min-library-workflow -- grep -n minTrailguideVersion .github/workflows/trailguide.yml` shows the install step reading the minimum from the configuration. `TG_LAB=<lab> tg-lab run min-library-refuse -- trailguide-0.9.0 library check --json` fails with `upgrade-required`. A project on `trailguide-0.9.0`, and one on `trailguide-0.8.0`, that adds the library with `--ref main` syncs it with exit 0.
- **Pinned workflow.** In the library, replace the install step's `version=$(sed ...)` line and its `if` block with `version=0.9.0`, as Trail Guide 0.8.0 wrote, and commit. Run `TG_LAB=<lab> tg-lab run min-workflow-warn -- trailguide-0.11.0 project update`, from `tg-lab version 0.11.0`. The preview raises the minimum to `0.11.0` and warns that `.github/workflows/trailguide.yml installs trailguide 0.9.0` and names the `version=` line to change. `git status --short` is empty.
- **Marker.** In a project initialized by `trailguide-0.8.0`, whose `AGENTS.md` begin marker reads `<!-- trailguide:begin 0.8.0 sha256:... -->`, run `TG_LAB=<lab> tg-lab run min-marker-check -- trailguide-0.9.0 project check`. It exits 1 and lists `AGENTS.md` as holding an outdated section. Run `TG_LAB=<lab> tg-lab run min-marker -- trailguide-0.9.0 project build` and `git diff -- AGENTS.md`: only the marker changes, to `<!-- trailguide:begin sha256:... -->` with the same hash. Commit; `trailguide-0.10.0 project check` and the lab's `trailguide project check` then exit 0.

## Gotchas

- The lab's own `trailguide` is a development build: it never writes or checks the field. Use `tg-lab version` binaries for every step but `min-dev`.
- `tg-lab version` builds the checkout's working tree, so uncommitted edits are in every release binary. Its output counts them.
- Lab names can't contain dots, so name a project for a release like `consumer-0-8-0`.
- `library init` refuses a `--license-file` inside the library folder whose name it would write, such as `LICENSE.md`; keep the license text outside the remote.
- A preview that only raises the minimum still needs `--yes` without a terminal; it isn't an error that nothing else moved.
