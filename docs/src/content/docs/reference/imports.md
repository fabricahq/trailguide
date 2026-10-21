---
title: "How sources work"
description: "How a repository's sources become vendor snapshots, generated guidance, and installed skills, what changes when you update, and what Trail Guide supports."
---

A **source** is a Git repository that your repository takes rules or skills from: a library, such as your organization's, or any other repository that holds skills.

Sources let you reuse your team's engineering practices across projects without writing and maintaining the same rules and skills in each one. You can also take rules and skills from third-party libraries and repositories whose practices you want to follow. Each project chooses which rules and skills to include, adds its own, and replaces ones it includes to fit its needs.

This page explains which files Trail Guide copies from a source and how it combines them with your authored rules and skills and your exceptions. It also covers the checks that protect your project during an update and the limits on what Trail Guide can read. For step-by-step instructions, see [Include rules](/guides/select-rules/) and [Install skills](/guides/install-skills/).

## From a source to project guidance

When you run `trailguide project sync`, Trail Guide:

1. Reads your configuration to find each source, the rules and skills you include from it, and your exceptions. A **rule group** collects related rules, such as testing practices or TypeScript conventions.
2. Copies what you include into your project. The copy of one source is its **vendor snapshot**. Each rule in it comes from the library release that published its version, and every skill from the source's one recorded library release or commit.
3. Combines the included rules with your authored rules and configured exceptions, then generates files for your agents to read.
4. Installs each skill, and the skills it calls, into your [skill targets](/reference/configuration/#skill-targets), such as `.claude/skills/`.

These files live in the **Trail Guide directory**, `.trailguide/` at the repository root:

| Directory | What it contains |
| --- | --- |
| `vendor/` | One vendor snapshot per source, with its record of versions and commits. |
| `rules/`, `skills/`, and `assets/` | Rules, skills, and shared assets authored in this repository, including replacements for ones you include. |
| `generated/` | The rules and reading indexes your agents use, including `SKILLS.md`. |

Installed skills live outside it, in the target folders at the repository root. For the complete layout, see [Project files](/reference/files/). Once the vendor snapshots are stored, `trailguide project build` can regenerate guidance and installed skills without network access.

## What kind of source it is

Sources have no declared type. Trail Guide decides each time it fetches one:

- A repository with a [library manifest](/reference/library-format/#the-manifest) in `.trailguide/config.yaml` is a **library**. It publishes rules, rule groups, and skills from its Trail Guide directory, sets skill levels, and, once it has `release/<number>` tags, versioned rules and skills.
- A repository without one is a **non-library source**: any repository of skills that hasn't adopted Trail Guide. It supplies skills only, and every skill in it is offered. See [Non-library sources](#non-library-sources).

Every source has a `ref`, so Trail Guide never takes a repository from an unknown revision, and `config.yaml` says which library release each library source is at. `trailguide project add source` adds either kind; see [project add source](/reference/cli/#project-add-source).

## What gets copied

Each source has a **source name** in your configuration, such as `team`. Trail Guide stores its vendor snapshot under `vendor/team/`.

From a library, the snapshot includes:

- The rules you include, including rules your project excludes or replaces, with their asset directories.
- Supporting files, such as images and examples, from the library's shared `assets/` directory, when an included rule links to them.
- Rule group metadata, which describes each rule group and when to read it.
- The library's `config.yaml`, for its manifest, and its declared license and notice files.
- Each installed skill's whole folder, `skills/<name>/`, including skills your project excludes or replaces, and the skills that installed skills call.
- The rules that installed skills cite, with their asset directories and rule group metadata.
- The folders of [curated skills](/reference/configuration/#curated-skills) the project installs, under `curated/<curated-source>/`, from the commit the project resolved for that curated source.

Keeping the original rules and skills lets you review a source's changes even when your project uses a replacement. Make project-specific changes in your authored content; syncing replaces the vendored files.

The snapshot contains no Git history or `.git` directory. Trail Guide reads the original files through temporary Git storage and removes that temporary storage afterward.

A skill's folder is copied as Git tracks it: only tracked files, so ignored files such as `__pycache__/` are never copied, and with each file's executable bit, so a script such as `scripts/verify.sh` stays runnable in the vendored and installed copies. Trail Guide never runs it.

## How Trail Guide chooses the rules your agents read

Each source's `rules.include` names rules by ID or by prefix, such as `practices/testing/*` for a rule group, `practices/*` for every practice rule, or `"*"` for every rule; see [Rule selectors](/reference/configuration/#rule-selectors). Trail Guide finds the matching rules in the source before applying your exceptions, and records both your selectors and the rules they matched. Invalid rule groups and rules without rule group metadata cause an error instead of being silently skipped.

Trail Guide then decides which rules are **active**, meaning included in the generated guidance:

1. Starts with the rules your `include` selectors match, and discovers authored rule groups from their `_group.yaml` files.
2. Removes the rules your `exclude` entries match.
3. Adds the authored rules your exclusions name as `replacedBy`, in place of the rules they exclude.
4. Adds your remaining authored rules.

Each included rule's project ID starts with its source name. For example, `team:practices/testing/check-retries` identifies the `check-retries` rule from the `team` source. This keeps rules from different sources distinct, even when their paths match. Authored rules have no source name.

A replacement contributes its complete authored definition: ID, title, metadata, body, attribution, and links to supporting files. It must belong to the same rule group as the rule it replaces. Each authored rule appears only once, even when used as a replacement.

## How Trail Guide chooses the skills to install

Skills don't belong to rule groups, so including a rule group's rules never installs a skill. For each source, Trail Guide installs:

1. The skills the library [requires](/reference/configuration/#skill-levels), whatever `skills.include` says.
2. The skills `skills.include` adds: the recommended ones for `recommended`, every one the source offers for `"*"`, and each skill it names.
3. Every skill those skills call, transitively, from the same repository and revision: each skill named in a skill's `trailguide-skills-used` list, plus each skill your configuration's [`skills.uses`](/reference/configuration/#declare-the-skills-a-skill-calls) adds. A skill reached only this way is installed because another skill uses it, and the records and `generated/SKILLS.md` say which skill. A library's own skill can also call a skill the library curates from another repository, such as `anthropic:skills/skill-creator`. That skill installs too, with its files taken from the other repository at the commit your project resolved for it.

It also imports each skill your [`skills.forks`](/reference/configuration/#fork-a-skill) entries name, when the revision has it, so updates can compare it with your fork. It then removes the skills your `skills.exclude` and `skills.forks` entries name, which can't include a required skill, adds the authored skills your forks name as `as`, applies your [renames](/reference/configuration/#rename-a-skill), and adds your authored skills in `skills/` and the skills they call. Each skill's `trailguide-skills-used` and `trailguide-rules-used` lists come from the revision the project installs. When two installed skills would have the same install name, sync fails with `skill-name-clash`.

A curated skill's files come from the curated source's repository, at the commit the project resolved for it, or at the commit of the project's own source for that repository when it declares one.

Trail Guide also takes every rule in each installed library skill's `trailguide-rules-used` list, from the same library, and every rule an authored skill cites by project ID, from the named source, even when your `include` doesn't name it. These **cited rules** are reference-only: each gets its generated rule file, and the installed skill's [generated section](/reference/files/#installed-skills) links to it, but `RULES.md` and the rule group pages don't list it, so installing a skill never widens the rules your agents apply to all work. A cited rule your project also includes is an ordinary rule. A cited rule your project excludes or replaces follows that choice: the installed skill says it's excluded, or links to your replacement. Rules cited by a skill you exclude aren't taken for it.

Trail Guide does not read rule text to detect contradictory instructions. Two sources can supply conflicting rules, and both remain active unless you configure an exclusion or replacement. The order of sources in your configuration does not establish priority. See [Resolve conflicting rules](/guides/conflicting-guidance/).

## Where agents read the result

Trail Guide writes the active rules and reading indexes to `generated/`. Agents start at `generated/RULES.md`, open relevant rule groups, and read the applicable rules in full.

The generated files also include library summaries, retained license files, and **provenance**: records of where rules and skills came from, which rules they replaced, and which library required, recommended, or offered each skill. Replacement targets and reasons stay in configuration and provenance, outside the rule guidance. Rule group descriptions remain labeled by source so you can see which library supplied them.

The same files support implementation and review. Your project chooses how to check that agents follow the rules; Trail Guide does not enforce compliance. Likewise, Trail Guide installs skills but doesn't run them, decide when agents use them, or check that agents followed them.

## What changes when you update

Each source's `vendor/<source-name>/_source.json` records the library release or commit the source was taken from, and the version of every rule and skill it includes: its **lock**. Running `trailguide project sync` again restores exactly that, so every checkout of the project gets the same rules and skills. Sync never moves a source, and when you change its `ref`, sync fails and names `trailguide project update SOURCE`, which takes it. A rule or skill you newly include comes from the library release `ref` names, or the recorded commit, and when it isn't there, sync fails and names `trailguide project update SOURCE`. Sync chooses a revision only for a new source or a changed repository, and a rule's version only for a pin you add or change; see [project sync](/reference/cli/#project-sync).

`trailguide project update` moves each source along its `ref`, previews every change, and applies it once you confirm:

- A library whose `ref` names a library release, such as `release/4`, moves to its newest library release, and the update writes that release to `ref`. When you edited `ref` to name a newer release than the one the project records, the update takes exactly that release instead, and leaves `ref` as it is. Each rule moves to its newest version, unless it's [pinned](/reference/configuration/#pin-a-rule), and all of the source's skills move together. A library whose `ref` is `latest-release` moves to its newest library release, and `ref` stays `latest-release`.
- A non-library source whose `ref` names a branch moves to the branch's newest commit, and one whose `ref` names a tag moves to the tag's commit, warning when the tag moved. A commit never moves.
- A library whose `ref` names another tag, or a commit, stays there until you change `ref`. A library whose `ref` names a branch moves to the branch's newest commit, and warns that it takes changes the library hasn't released.
- Each curated source a library lists is resolved again from the `ref` the library names, or from your own source for that repository.

With the same configuration and recorded versions, Trail Guide produces the same paths and file contents. With unchanged vendored files, authored content, tool version, and rendering options, a build produces the same generated guidance and installed skills. Reordering sources or selectors in configuration does not change their generated order.

`trailguide project update` reports each changed rule and skill with its change, versions, level, and summary, and applies the changes only after you confirm. Both commands report changed files, including rule group metadata, version records, and installed skill folders.

### How rule versions are resolved

The newest library release is the one with the highest `release/<number>` tag. Each [library release](/reference/rule-versions/#library-releases) tag's message records every rule's version and the changes that library release published.

To choose a rule's version, Trail Guide reads the rule's history from the release records in the `release/<number>` tags, and picks the version the source's recorded library release lists, or the one the rule is pinned to. It then copies the rule's [Markdown file and asset directory](/reference/rule-versions/#what-a-version-covers) from the tagged commit of the library release that published that version. Library-wide files, including the manifest and the shared files rules link to, come from the one library release that `vendor/<source-name>/_source.json` records as `release`, which is never older than the library release of any included rule version. A new source takes the newest library release, `trailguide project update` moves to it, and `trailguide project sync` keeps the recorded one; see [What a version covers](/reference/rule-versions/#what-a-version-covers).

Skills aren't chosen one by one. Trail Guide copies every skill's whole folder from the tagged commit of the library release recorded as `release`, and takes each skill's version from that release record's `skills` field, which holds their versions, changes, and retirements, and each skill's level from the manifest at that commit; see [Release record](/reference/rule-versions/#release-record). The changes between two library releases' records give the summaries the update preview shows. When that library release was published by a version of Trail Guide that doesn't write the `skills` field, Trail Guide takes the skills and their versions from the newest release record that has it, and warns.

A rule or skill absent from a library release was retired, and the release record's `retired` entry records why. A rule the project pinned before its retirement keeps its pinned version. A retired skill is no longer installed once the project's library release moves past its retirement.

To find versions, Trail Guide lists only `release/` tags, fetches their messages without the library's history, and fetches only the files it copies. It records each included rule's version, library release, and commit in `_source.json` and generated provenance, and shows the version in generated guidance.

### Tracing rules to their source

Trail Guide records the versions you requested, and each included rule's exact version and commit. See [Provenance](/reference/provenance/) for these records.

Links to original files on GitHub.com and GitLab.com use the recorded commit, so moving a tag does not change their destination. For other Git hosts, links point to the stored files; provenance retains the repository address and commit.

Trail Guide preserves rule attribution in both vendored and generated files and keeps attribution links valid. It verifies and retains the license and notice files declared in the library manifest, includes them in integrity checks, and reports their changes during updates.

Generated rules link to retained terms under `generated/libraries/<source-name>/licenses/`. See [License rules](/guides/license-rules/) for how library authors declare those files.

## Non-library sources

A source whose repository has no library manifest at the revision Trail Guide reads is a **non-library source**. It supplies only skills, and has no release tags, versions, or change notes:

- **Revision.** `ref` is required. `trailguide project sync` reads the commit recorded in `_source.json`, and `trailguide project update` resolves `ref` again: a branch's newest commit, or a tag's commit. A bare name, such as `main`, can name a branch or a tag. Either way, `_source.json` records the exact commit, and every skill from the source comes from it.
- **Skills.** Each folder in the repository that has a `SKILL.md` file is a skill, offered at no level, with its folder's path from the repository root as its ID. Trail Guide copies each installed skill's folder, as Git tracks it, to its path under `vendor/<source-name>/`, such as `vendor/pstack/skills/architect/`.
- **Calls.** A skill's `trailguide-skills-used` list names skills in the same repository. Declare calls it leaves out with [`skills.uses`](/reference/configuration/#declare-the-skills-a-skill-calls). Its `trailguide-rules-used` list is ignored, because the repository has no rules.
- **Validation.** Lenient: where a skill doesn't follow the Agent Skills format, such as a `name` that doesn't match its folder, a missing `description`, or a link outside the skill's folder, sync warns and installs the skill as it is. The checks that protect your project still fail: unsafe paths, symbolic links, submodules, Git LFS pointers, and the [source limits](#source-limits-and-version-errors).
- **Licenses.** Trail Guide looks for terms in this order: the skill's `license` field in `SKILL.md`, a `LICENSE*` file in the skill's folder, and `LICENSE*` and `NOTICE*` files at the repository root. It records what it finds in provenance, and retains the root files, when present, under `generated/libraries/<source-name>/licenses/`, like a library's declared files. When a skill has none of these, sync fails with `license-not-found`.

`rules` is rejected on a non-library source.

## Checks before updating your files

Trail Guide fetches and validates every source, and every curated source the project's libraries list, before replacing your project's vendor snapshots, generated guidance, or installed skills. If any of them cannot be fetched or validated, your previous complete set of rules and skills stays in place.

Validation rejects:

- Invalid or reserved source names, repeated repositories, and duplicate rule IDs that include the same source name.
- Selectors, exclusions, pins, renames, or `skills.uses` entries that name a rule or skill the source doesn't supply. An entry naming a rule the library retired produces a warning instead, when the source would otherwise include that rule. Offline checks accept the same exclusions, because the snapshot records those retired rules in `retiredRules`.
- An exclusion or fork of a required skill.
- A missing `replacedBy` rule or fork, or one authored rule or skill named as the replacement for more than one.
- Two installed skills with the same install name, and installed skills that would overwrite a folder Trail Guide doesn't own; see [Who owns the target folders](/reference/sync/#who-owns-the-target-folders).
- Skills whose calls form a cycle, including through `skills.uses`.
- Invalid metadata, unsafe file paths, and symbolic links.

Included library rules and skills must pass validation even if you exclude or replace them. A library skill gets the same checks as `trailguide library check` applies, except that sync doesn't check the value of `trailguide-skill-type`, so a project can install a skill whose type a newer Trail Guide added; see [Skill format](/reference/skill-format/#validation). File checks also ensure that paths stay within their allowed directories.

Trail Guide detects concurrent writes and interrupted updates so a mixture of old and new output cannot pass a consistency check. For file replacement and recovery behavior, see [Sync and recovery](/reference/sync/).

## Git access and supported files

Reading a source requires Git 2.30 or later on macOS or Linux. Trail Guide accepts HTTPS and SSH repository addresses, including scp-style SSH addresses and nested repository paths. See [Repository addresses](/reference/configuration/#repository-addresses) for accepted formats.

Trail Guide uses your Git credentials and certificate and host-key verification settings. It does not store credentials in configuration or provenance. A valid repository address does not guarantee that the repository is reachable or appropriate for your network.

Git URL rewrites still apply. If a rewrite uses another protocol, your Git configuration must explicitly allow that protocol. Executable `ext` helpers are always disabled. Trail Guide passes Git arguments separately and applies the same path checks and resource limits across hosts.

Trail Guide reads original Git file contents without checking out the source. It does not run library scripts, skill scripts, Git hooks, or checkout filters. It preserves a skill file's executable bit, but never runs the file. Selected symbolic links, submodules, and Git LFS pointers are unsupported; Trail Guide does not fetch submodule contents.

### Supporting files

Trail Guide copies supporting material from [two asset locations](/reference/rule-format/#supporting-assets):

- **A rule's own assets:** the adjacent `assets/<rule-name>/` directory. Trail Guide copies this directory in full when it copies the rule.
- **Shared assets:** files in the library's `.trailguide/assets/` directory. Trail Guide copies the files a selected rule or its Markdown assets link to, including files they link to in turn, from the library release that supplies the source's library-wide files.

Markdown links, images, and reference links must point to files within the allowed locations. Missing files and links into another rule's private assets cause an error. Trail Guide preserves external URLs as links without downloading their contents.

A rule or Markdown attachment cannot link to another rule document on disk, even if that rule is also selected. Each rule must work independently because projects can exclude or replace it. Put shared supporting explanations in the library's shared assets directory.

Within an included rule group, Markdown files outside asset directories count as rules, including files in nested folders. Declared license and notice files and the `README.md` at the rule group's root are exceptions. Put other supporting Markdown, such as `_README.md`, in an asset directory.

Markdown assets, license files, and notice files must use UTF-8 text. Other assets can be binary files; Trail Guide preserves their original bytes.

A skill's folder can hold any files, such as `scripts/`, `references/`, and `assets/`; its `assets/` folder is the skill's own and follows none of the rule asset rules above. Links in a skill's files must be absolute URLs or relative paths that stay inside the skill's own folder, so each installed copy works on its own. A library skill with any other link fails validation; a non-library source's skill gets a warning.

## Source limits and version errors

Trail Guide rejects paths in a source that become identical when letter case is ignored and Unicode names are normalized to NFC. This includes directory names and prevents ambiguous paths across filesystems.

Each source has these limits when Trail Guide reads it:

| Resource | Limit |
| --- | --- |
| Fetching and finding rule versions | 120 seconds per library. |
| Entries in the Git file tree | 10,000 entries. |
| Git file-tree listing | 8 MiB. |
| Each retained file | 8 MiB. |
| All retained files combined | 64 MiB. |
| Git's listing of release tags | 8 MiB and 20,000 records, including extra records Git uses to identify commits behind annotated tags. |
| Each release tag | 8 MiB. |
| Each [release record](/reference/rule-versions/#release-record) | 10,000 entries each in `rules`, `changes`, and `retired`, 10,000 each in `skills.versions`, `skills.changes`, and `skills.retired`, and 20,000 in `libraryFiles`. A record over a limit fails with `invalid-release-tag`. |

The retained-file limits apply after fetching. They do not cap network traffic or Git's temporary disk use. Finding rule versions uses the same Git access settings and deadline as fetching.

When resolving refs, finding versions, or finding license terms, Trail Guide can report:

| Error | Meaning and next step |
| --- | --- |
| `ref-changed` | A library source's `ref` names another library release than the one the project records. Sync refuses it, naming `trailguide project update SOURCE`, which takes a newer release. Update refuses an older one, since it moves a source only forward. |
| `ref-required` | The source has no `ref`, or its `ref` is `latest-release` and the repository has no `release/<number>` tags. For a library, the message names the release to write, such as `ref: release/4`, the one the project records or, for a new source, the newest. A non-library source, or a library that hasn't published its first library release, needs a branch, tag, or commit as its `ref`. |
| `version-not-found` | A pin names a version the rule never published, the branch, tag, or commit in `ref` doesn't exist, such as a branch that was renamed or deleted, or the server says it doesn't have a commit that `vendor/<source-name>/_source.json` records, because the library rewrote its history or someone force-pushed the branch. The message names the ref. Check the pin or `ref`. For a missing recorded commit of a source whose `ref` is a branch, or of a non-library source whose `ref` is a tag, run `trailguide project update SOURCE` to take the commit the ref names now. For any other library's missing recorded commit, ask the library's maintainer to restore it, or set `ref` to a tag or commit the library still has; deleting `vendor/<source-name>/` and syncing is the last resort, because it chooses versions again. A fetch that fails for any other reason, such as a dropped connection, reports that reason's code instead, never this one. |
| `ambiguous-ref` | The source's `ref` is a bare name, such as `main`, that the source has as both a branch and a tag. Set `ref` to `refs/heads/<name>` for the branch or `refs/tags/<name>` for the tag. |
| `unsupported-release-record` | A library release's [release record](/reference/rule-versions/#release-record) uses a newer format than this Trail Guide reads, because a later Trail Guide published it. Upgrade Trail Guide. |
| `skill-not-in-revision` | Configuration includes a skill or rule that isn't in the library release the source's `ref` names, or its recorded commit, such as one published after the project's last update. Run `trailguide project update SOURCE` to move the source, or change its `ref`. |
| `license-not-found` | A skill from a non-library source has no license terms: no `license` field, no `LICENSE*` file in its folder, and no `LICENSE*` or `NOTICE*` file at the repository root. Ask the repository's owner to add a license, or stop including the skill, and any skill whose `skills.uses` names it. |

When Trail Guide can't read a source's repository, it can report:

| Error | Meaning and next step |
| --- | --- |
| `connection-failed` | Git couldn't reach the repository's host, such as when the host name doesn't resolve, the connection is refused or times out, or the TLS connection fails. Check the repository address and your network connection. |
| `https-certificate-failed` | The HTTPS server's certificate couldn't be verified. Check that your system trusts it: Git's `http.sslCAInfo` setting, your system's certificate store, and any proxy that intercepts TLS. |
| `ssh-host-key-failed` | The SSH host key couldn't be verified. Check the server's entry in your `known_hosts` file. |
| `not-found-or-no-access` | Git or the server reported that the repository doesn't exist or that your Git credentials can't read it; servers report both the same way. Check the address and your credentials. |
| `object-fetch-refused` | The server refused to send a file by its object ID, which Trail Guide needs to read one version of each rule without downloading the whole repository. GitHub.com and GitLab.com allow it; a self-hosted server needs Git protocol version 2 or `uploadpack.allowAnySHA1InWant`. |
| `git-failed` | Another Git failure, including one whose cause Trail Guide doesn't recognize. The message says which step failed, such as fetching library files. |

These messages never include what Git or the server printed, which can hold credentials, such as one that `url.*.insteadOf` rewriting adds to the repository address. Trail Guide reads that text only to choose the code and message, as [Text from Git, servers, and the GitHub CLI](/reference/cli/#text-from-git-servers-and-the-github-cli) describes. To read Git's message, run `git ls-remote` with the repository address yourself.
