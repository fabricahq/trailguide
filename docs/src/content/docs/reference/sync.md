---
title: "Sync and recovery"
description: "When to run sync, build, or check, which files they change, who owns installed skill folders, and how to recover from problems."
---

The `trailguide project sync` command restores your repository's vendor snapshots, regenerates the guidance your agents read, and installs your repository's skills. Run it after adding a source, changing what you include from it, its pins, or its `ref`, changing `skillTargets`, or to restore vendored files.

Sync takes each source from the library release or commit recorded for it, its lock, so it never moves a source. To move to newer versions, run `trailguide project update`, which previews every change and applies it only after you confirm, offering to pin each major rule change or retirement instead. See [Update rules and skills](/guides/update/).

This page explains how to run sync, what it changes, and what to do when files are missing, outdated, or left by an interrupted update. It also explains when `trailguide project build` or `trailguide project check` is enough. For how Trail Guide chooses and combines the rules and skills from each source, see [How sources work](/reference/imports/).

## Choose the right command

- **`trailguide project sync`**: Restore each source at its recorded revision, or take a new source at its newest one. Sync validates the vendored files, regenerates agent guidance, and installs skills into the target folders.
- **`trailguide project update`**: Move each source along its `ref`, previewing newer versions, new rules and skills, level changes, retirements, and new commits, and apply them after you confirm. Pinned rules, library sources whose `ref` names a tag or commit, and sources whose `ref` names a commit don't move. Update then does everything sync does.
- **`trailguide project build`**: Apply changes to your authored rules and skills or your exceptions using the files you already have. Build regenerates guidance and installed skills without contacting a repository.
- **`trailguide project check`**: Find out whether generated guidance, installed skills, the managed Trail Guide guide, and the `AGENTS.md` section are up to date. Check validates stored inputs and output, reports problems, and leaves files unchanged.

You do not need to run build after a successful sync or update; both already generate the guidance. Run check when you want to verify consistency without making changes.

## Run from your project root

After [installing Trail Guide](/start-here/install/) and [setting up your project](/start-here/set-up-project/), run:

```sh
trailguide project sync
```

In a Git repository, project commands find the nearest repository root and use its `.trailguide/config.yaml`. Outside Git, run commands from the repository root. Custom configuration locations are not supported. The paths below are relative to `.trailguide/`, except the target folders, `.claude/skills/` and `.agents/skills/`, which are relative to the repository root.

## Which files change

| File or directory | What it contains | What the commands do |
| --- | --- | --- |
| `config.yaml` | Your sources, skill targets, the rules and skills you include, pins, exceptions, renames, and, for a library, its manifest. | Sync, build, and check read it without changing it. Update reads it, and writes a pin when you keep a rule at its current version, or an exclusion when you decline a new rule or skill. |
| `README.md` | The managed Trail Guide guide. | Init, build, and sync refresh an older, unedited guide. Check verifies it without changing it. |
| `rules/`, `skills/`, and `assets/` | Rules, replacements, skills, and shared assets authored in this repository. | Sync, build, and check preserve these files. Sync and update add `rules/<group-id>/_group.yaml` only when your authored rules in a rule group would otherwise lose the rule group's metadata; see [Keep the rule group of an authored rule](#keep-the-rule-group-of-an-authored-rule). Update replaces a forked rule only when you pass `--update-fork` or answer `replace`, and merges a source's changes into a forked skill only when you pass `--merge-fork` or answer `merge`. |
| `_managed/vendor/` | One vendor snapshot per source: the original files of the included rule and skill versions, and their records. | Sync and update replace this directory. Build and check validate it without changing it. |
| `_managed/generated/` | Rules, reading indexes, `SKILLS.md`, and `installed-skills.json`. | Sync, update, and build replace this directory. Check compares it with the expected output. |
| `.claude/skills/<name>/` and `.agents/skills/<name>/` | Installed copies of skills, one folder per skill. | Sync, update, and build write, replace, and remove the skill folders Trail Guide owns, and leave every other folder alone. Check compares them with the expected output. See [Who owns the target folders](#who-owns-the-target-folders). |
| `../AGENTS.md` | Your instructions for agents, with a section Trail Guide maintains. | Together with their other files, init, sync, update, and build add or refresh the section between the `trailguide:begin` and `trailguide:end` markers, and never change the rest. Check verifies the section. See [Where agents start](/reference/files/#where-agents-start). |

Replacement includes removing files that no longer belong in the output, such as removed rules, old index pages, unused library folders, and the installed folders of skills the project no longer installs. Files you add or edit inside `_managed/vendor/`, `_managed/generated/`, or an installed skill folder can be replaced or removed. Keep your changes in configuration and authored content.

For the complete directory layout, see [Project files](/reference/files/).

## Read the command's result

Commands print human-readable output by default. Add `--json` when another tool needs one structured response:

```sh
trailguide project check --json
```

Sync, update, and build report counts and sorted lists of added, changed, and removed paths. JSON output includes those lists in `added`, `changed`, and `removed`. When the command first [recovered an interrupted one](#recover-from-an-interrupted-update), it says so before the counts, which don't include the files the recovery restored or finished, and JSON output sets `recovered` to `true`, also for an update's preview, which writes nothing of its own; a declined update says it in its message.

- Sync and update paths start with `_managed/vendor/`, `_managed/generated/`, `.claude/skills/`, or `.agents/skills/`. Paths in a target folder are relative to the repository root; the rest are relative to the Trail Guide directory. Update also lists `config.yaml` when it writes a pin or an exclusion, and the files of a fork it replaces, and both list a `rules/<group-id>/_group.yaml` they add.
- Build paths are relative to `_managed/generated/`, except paths in a target folder, which start with `.claude/skills/` or `.agents/skills/` and are relative to the repository root.
- A change to `AGENTS.md` is reported on its own line, and in JSON in `agents`.

When the target folders are linked, as [Linked target folders](#linked-target-folders) describes, each installed file is listed once, under the folder the link points to.

`trailguide project update` also reports each rule's and skill's change, versions, level, and summary. There is no separate structured summary of added or removed rule groups. To see which revisions changed, review the source records and generated [provenance records](/reference/provenance/).

Sync, update, and build also print the [rename warning](/reference/configuration/#rename-a-skill) for each renamed skill, and sync, update, and check print the [personal skill warning](#personal-skills-with-the-same-name) for each personal skill with the name of a skill the repository installs. In JSON output they're in `value.warnings`.

Check reports `status` and `problems`, including each problem's path and suggested repair command. It verifies generated guidance, installed skills, the managed Trail Guide guide, and the `AGENTS.md` section without writing any of them. A problem in a target folder has a path relative to the repository root, starting with `.claude/skills/` or `.agents/skills/`.

| Check exit code | Meaning |
| --- | --- |
| `0` | The files are current. |
| `1` | Files differ from the expected result, or an input is invalid. |
| `2` | The command was used incorrectly, such as with an invalid option. |

## Repair missing or outdated files

Use the problem reported by `trailguide project check` to choose a repair:

| Problem | What to do |
| --- | --- |
| Generated guidance is missing or outdated, but vendored files are valid. | Run `trailguide project build`. |
| Vendored files are missing, modified, or no longer match your configured sources or selectors. | Run `trailguide project sync`. Preserve any edits you intended to keep as authored rules first. |
| The managed Trail Guide guide is missing or outdated. | Run `trailguide project build` or `trailguide project sync` to refresh it and regenerate guidance. Run `trailguide project init` to refresh only setup files. |
| An installed skill folder is missing, edited, or unexpected, or `_managed/generated/installed-skills.json` is stale, and vendored files are valid. | Run `trailguide project build`. If the folders are missing because `.gitignore` excludes them, also add an exception and commit them; see [Who owns the target folders](#who-owns-the-target-folders). |
| The `AGENTS.md` section is missing or outdated. | Run `trailguide project build`. If build reports that the section was edited, move your text outside the markers and delete the section first. |
| Configuration, rule metadata, or skill metadata is invalid. | Correct the reported input, then retry the appropriate command. |

Run `trailguide project check` again after repairing the problem.

### How vendor snapshots are checked

For each source, Trail Guide records its revision, each included rule's version, each installed skill, and file digests in [`.trailguide/lock.json`](/reference/provenance/#inspect-the-lockfile). A **digest** detects whether a file's contents differ from the recorded copy, and each entry's [integrity](/reference/provenance/#integrity) detects an entry changed outside Trail Guide.

Build and check work offline. They reject missing, changed, or unexpected vendored files, invalid lockfile entries, version records that don't cover exactly the included rules and skills, and selections that no longer match your configuration. Sync fetches the recorded files again, including replacing locally modified copies.

For skills, check also fails when:

- An installed copy differs from what sync would write, such as after a hand edit to a file in `.claude/skills/build-feature/`.
- A folder that `_managed/generated/installed-skills.json` lists is missing, such as because `.gitignore` excludes it and it was never committed.
- `_managed/generated/installed-skills.json` or `_managed/generated/SKILLS.md` doesn't match the skills the configuration installs.
- Two skills the repository installs have the same install name, or a target folder has a folder Trail Guide would install a skill into but doesn't own.
- One of your authored skills cites a rule its library retired. The message names the replacement when there is one.

Check never reads the contents of target folders Trail Guide doesn't own.

Checksums detect changes relative to the stored record. They cannot establish that files are authentic if someone also changed that record. For record fields and rule origins, see [Provenance](/reference/provenance/).

## Who owns the target folders

The target folders, `.claude/skills/` and `.agents/skills/`, are shared: people and other tools install skills there too. Trail Guide owns only the skill folders it installed, as `_managed/generated/installed-skills.json` records them, and treats every other folder as someone else's:

- Sync, update, and build write each installed skill to `<target>/<install-name>/`, replacing the whole folder, and remove the folders of skills the project no longer installs, including after you remove a target from `skillTargets`. An owned folder you edited by hand is replaced or removed like any other output.
- They never change, remove, or read the contents of a folder Trail Guide didn't install.
- They create a target folder, such as `.claude/skills/`, when they first install a skill into it, and remove it when they leave it empty.

Two errors protect folders Trail Guide doesn't own. Each fails before writing anything:

| Error | Meaning and next step |
| --- | --- |
| `skill-folder-not-managed` | A target already has a folder with the install name of a skill the repository installs, and Trail Guide didn't install it, such as a hand-copied skill or another tool's. To let Trail Guide manage the skill, move or delete that folder and run the command again. To keep the folder, [rename](/reference/configuration/#rename-a-skill) or [exclude](/reference/configuration/#exclude-a-skill) the skill instead. |
| `skill-name-clash` | Two skills the repository installs have the same install name, such as `fabrica:skills/review` and `pstack:skills/review`, or a skill from a source and an authored skill. Installed folders are named by install name, so both can't be installed. [Rename](/reference/configuration/#rename-a-skill) or exclude one of them. Trail Guide never renames a skill on its own. |

When `_managed/generated/installed-skills.json` is missing or was changed outside Trail Guide, Trail Guide can't tell which folders it owns, so it treats the folders it would write as not managed. Restore the record from Git, such as with `git checkout -- .trailguide/_managed/generated/installed-skills.json`, then run the command again.

Commit the target folders. A `.gitignore` that excludes them, such as with `/.claude/`, leaves fresh checkouts without the installed skills, and `trailguide project check` reports the missing folders. Change the line to `/.claude/*` and add the exception `!/.claude/skills/`; Git can't re-include a folder inside an excluded folder. See [Commit the installed skills](/guides/install-skills/#commit-the-installed-skills).

### Linked target folders

Some projects make one target folder a symbolic link to the other, such as `.agents/skills` linked to `.claude/skills`, so that every agent reads the same files. Trail Guide detects the link and writes each skill once, into the folder the link points to. The link itself, and a link from a parent folder such as `.agents` to `.claude`, must stay inside the project; a link that points outside the project root is refused. Every other symbolic link in a target folder is refused, as elsewhere in the project.

## Personal skills with the same name

People also install **personal skills** for themselves, outside any repository, which their agents load in every repository they work in. When a personal skill has the same name as a skill the repository installs, each agent decides which one to use, and some use the personal skill without saying so. Their docs say:

| Agent | Personal skill folders | Repository skill folders it shares with Trail Guide | What it does with the pair |
| --- | --- | --- | --- |
| Claude Code | `~/.claude/skills/` | `.claude/skills/` | Uses the personal skill. |
| Codex | `~/.agents/skills/` | `.agents/skills/` | Shows both. |
| Grok Build | `~/.grok/skills/`, `~/.agents/skills/`, and Claude Code's | `.claude/skills/`, through its Claude Code compatibility | Not documented. |
| VS Code | `~/.copilot/skills/`, `~/.claude/skills/`, `~/.agents/skills/` | `.claude/skills/`, `.agents/skills/` | Not documented. |

`trailguide project sync`, `trailguide project update`, and `trailguide project check` read these personal skill folders, and `~/.codex/skills/`, which Codex's current docs no longer list. They warn about each pair, naming both paths and, for each agent that reads both copies, what it does:

```text
Warning: your personal skill ~/.claude/skills/build-feature has the same name as this project's build-feature (fabrica:skills/build-feature, installed at .claude/skills/build-feature).
  Claude Code uses your personal skill.
  Grok Build reads both, and its docs don't say which it uses.
  VS Code reads both, and its docs don't say which it uses.
Rename or remove your personal skill to use the project's.
```

The warning applies to required skills too: a personal skill wins over the repository's in the agents that prefer personal skills, and Trail Guide only warns. It never writes to your home directory, and the warning never fails a command, so `trailguide project check` in CI, which has no personal skills, passes as before. With no personal skill of the same name, nothing is printed. `_managed/generated/SKILLS.md` doesn't tell agents to prefer the repository's skills.

## Keep the rule group of an authored rule

An authored rule needs its rule group's metadata, from `rules/<group-id>/_group.yaml` or a source. Sometimes only a source supplied it, such as when you [fork](/reference/cli/#fork-a-library-rule) a rule you included by ID and the library later retires it, or you stop including the rule group or remove the source. When a sync or update would leave your authored rules in a rule group with no metadata, it writes the rule group's `_group.yaml` to `rules/<group-id>/_group.yaml` in the same step that replaces `_managed/vendor/` and `_managed/generated/`, lists the file as added, and prints a warning naming it. The metadata comes from the first source whose entry in `lock.json` lists the rule group's metadata, configured sources first: from the library release that now supplies that source's library-wide files, when it still has the rule group, and otherwise from the last vendored copy, which is also what a source you removed from the configuration supplies. The file is then yours to edit like any authored rule group metadata. A vendored copy must match the digest its lockfile entry lists: when it was changed or deleted since sync wrote it, the command fails without changing anything. Restore it by running `trailguide project sync` with the previous configuration, then make your change and sync again, or write `rules/<group-id>/_group.yaml` yourself.

## Recover from an interrupted update

Sync, update, and build keep the previous output while installing replacement files. If installation fails, the command restores the previous directories. The installed skill folders in the target folders are part of the same step: one lock and one recovery journal cover them together with `_managed/vendor/` and `_managed/generated/`, so recovery restores or finishes them all together. If the process stops during replacement, the next sync, update, or build recovers the previous output before starting its own work. Update installs the pins, exclusions, and `basedOn` values it writes to `config.yaml`, each fork it [replaces with the newest version](/reference/cli/#replace-a-fork-with-the-newest-version), and each forked skill it [merges a source's changes into](/reference/cli/#merge-a-sources-changes-into-a-forked-skill), in the same step, so recovery restores or finishes `config.yaml`, the replaced authored rules and their asset directories, the merged skill folders, `_managed/vendor/`, and `_managed/generated/` together. Likewise, recovery keeps or removes a `rules/<group-id>/_group.yaml` that sync or update added together with `_managed/vendor/` and `_managed/generated/`. A failed or interrupted update may leave empty directories under `rules/`, such as a rule group's first `assets/` directory that [replacing a fork](/reference/cli/#replace-a-fork-with-the-newest-version) created. They're harmless: every command ignores them, and you can delete them. The `AGENTS.md` section is part of that step too, so recovery restores or finishes `AGENTS.md` with the other files.

If the update completed but cleanup was interrupted, the next sync or build finishes deleting the backups. The completed update remains complete.

| Situation | What to do |
| --- | --- |
| Another Trail Guide command is still writing files. | Let it finish before starting another update. |
| A previous writing process stopped on this machine. | Retry sync or build. Trail Guide can reclaim its lock and recover the interrupted operation. |
| Check reports a pending recovery. | Run sync or build after any active writer exits. Check reports the problem but does not repair it. |
| Recovery reports edited output or backups, an incomplete lock, or a damaged journal. | Stop automatic retries, verify that no writer is running, and inspect the reported files. Preserve backups and the recovery journal for manual recovery. |

A lock records which process owns the update. Trail Guide only reclaims a lock when it can establish that the process has stopped on the same machine. Active processes, locks from another machine, and incomplete ownership records block writes.

Recovery refuses to overwrite output or backups edited after an interruption. Follow the reported error instead of deleting recovery files to force the command through.

## How updates protect your files

Sync, update, and build prepare and validate the complete replacement, including every installed skill folder, before installing it. Immediately before replacement, they check that the original input and output files, including the owned skill folders, have not changed. If another process changed them, the update stops.

Trail Guide uses these temporary directories beside your configuration, for the target folders too:

| Directory | Purpose |
| --- | --- |
| `.trailguide-lock` | Allows only one cooperating Trail Guide command to write at a time. |
| `.trailguide-transaction` | Holds the recovery journal and previous output during replacement. The journal records the update's progress. |
| `.trailguide-cleanup` | Marks a completed update whose backups can be deleted. |

After completing an update, Trail Guide renames the transaction directory to the cleanup directory before deleting backups. It also discards abandoned staging files when no journal or backups exist.

A journal that covers target folders uses a newer format than earlier versions of Trail Guide read. An earlier version that finds one stops and asks you to upgrade Trail Guide, rather than reporting a damaged journal.

Avoid editing managed directories during an update. Replacing `_managed/vendor/`, `_managed/generated/`, the installed skill folders, and, for update, `config.yaml` takes separate filesystem operations, so another program can briefly see a mixture of old and new files. Check detects an active update instead of accepting mixed output as consistent.

Cancellation can stop work before replacement starts. Once replacement begins, the command finishes or rolls back before returning.

### Filesystem requirements and limits

These protections apply to cooperating Trail Guide commands on a local filesystem with normal file-renaming behavior. They do not guarantee recovery from power loss, protection against hostile concurrent file changes, or locking on network filesystems.

All operations reject symbolic links and special files within the directories they inspect, except a link between the target folders, as [Linked target folders](#linked-target-folders) describes. They also reject regular files with multiple hard links. Ordinary aliases in the project's parent path, such as macOS `/tmp`, are supported.

| Input resource | Limit |
| --- | --- |
| Each file | 64 MiB. |
| Each directory tree | 256 MiB and 30,000 entries. |
| Each path | 64 segments. |

Reading a source has additional, stricter [limits](/reference/imports/#source-limits-and-version-errors). Output paths must remain distinct after normalization for case-insensitive filesystems.
