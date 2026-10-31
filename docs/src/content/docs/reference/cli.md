---
title: "CLI commands"
description: "Syntax, arguments, options, and results for every supported Trail Guide command."
---

Find each command's syntax and options below. Every command also accepts `--json` and `-h` / `--help`; [shared behavior](#shared-options-and-prompts) and [output formats](#command-output) are documented at the end.

## Project commands

`trailguide project` manages rules and skills for a **project**, the codebase whose rules and skills Trail Guide manages. Initialize from the project root. In Git repositories, other project commands also work from subdirectories.

Configuration lives in `.trailguide/config.yaml` at the project root. See [Working directories](#working-directories) for repository discovery and input-file paths.

### project init

```sh
trailguide project init [options]
```

Create `.trailguide/config.yaml`, the authored `rules/` and `skills/` directories, and the managed Trail Guide guide, and add the Trail Guide section to the repository's `AGENTS.md`, creating the file if needed. Rerunning init preserves valid configuration and authored content and refreshes an unmodified guide and section.

| Option | Meaning |
| --- | --- |
| `--skill-targets TARGET` | Where to install skills: `claude` for `.claude/skills/`, `agents` for `.agents/skills/`, or `none`. Repeat for both targets. `none` can't be combined with a target. |
| `--non-interactive` | Accepted; init runs without prompts even when this flag is omitted. |

A new configuration gets `schemaVersion: 2`, the running release as [`minTrailguideVersion`](/reference/configuration/#minimum-trail-guide-version), and `skillTargets: [claude, agents]`, or `skillTargets: []` with `--skill-targets none`. Init creates no target folders: sync writes them when the repository first installs a skill.

Rerunning init on an existing configuration changes `skillTargets` only when you pass `--skill-targets`. A version 1 configuration fails with a message that shows the version 2 shape; see [No backward compatibility](/reference/configuration/#no-backward-compatibility).

Init does not fetch sources or generate rule guidance. It refuses to overwrite a manually edited Trail Guide guide or `AGENTS.md` section, and leaves an `AGENTS.md` that already points to `_managed/generated/RULES.md` outside the section alone. For file locations and ownership, see [Project and rule group guides](/reference/files/#project-and-rule-group-guides).

### project add source

```sh
trailguide project add source NAME [options]
```

Record a source in configuration without fetching its files. `NAME` is the source name, such as `team`. Run `trailguide project sync` afterward to take the included rules and install the included skills. The same command adds a library or any other Git repository with skill folders: you don't declare which it is, and sync detects whether the repository has a [manifest](/reference/library-schema/#the-manifest).

| Option | Meaning |
| --- | --- |
| `--repository URL` | Required. An accepted HTTPS or SSH [Git repository address](/reference/configuration/#repository-addresses). |
| `--ref REF` | The library release to take the source from, such as `release/4`, or `latest-release` to take the newest library release at each update, or a branch, tag, or commit, recorded as the source's `ref`. Defaults to the library's newest release, which the command reads from the repository. Required for a source without `release/<number>` tags, such as a non-library source. With a branch, such as `main`, update moves the source to the branch's newest commit. On a library, a branch takes changes the library hasn't released, without change notes, and sync and update warn about it. See [Refs](/reference/configuration/#refs). |
| `--rules SELECTOR` | A [rule selector](/reference/configuration/#rule-selectors) for `rules.include`: a rule ID, such as `practices/testing/verify-retry-limits`, a prefix, such as `practices/testing/*`, or `*`. Repeat for several. |
| `--skills SELECTOR` | A [skill selector](/reference/configuration/#skill-selectors) for `skills.include`: `recommended`, `*`, or a skill ID, such as `skills/build-feature`. Repeat for several. |
| `--non-interactive` | Never prompt. Supply all required inputs as flags. |

Quote `*` and selectors that end in `/*` so your shell does not expand them. Pass each selector as a separate option, rather than a comma-separated flag value:

```sh
trailguide project add source team \
  --repository https://github.com/example/rules.git \
  --rules 'practices/testing/*' \
  --rules 'techs/typescript/*' \
  --skills recommended \
  --non-interactive
```

The repository address and selectors are illustrative; replace them with a source you can access.

`--rules` and `--skills` are both optional: a source added with neither still installs the skills its library requires.

Without `--ref`, the command lists the repository's `release/<number>` tags, writes the newest as the source's `ref`, such as `ref: release/4`, and says so; `trailguide project update` later moves it to each newer release you confirm. A repository without library releases fails with `ref-required` and writes nothing; pass the branch, tag, or commit to take it from. To hold individual rules, add `pins` to the source in configuration, or use `trailguide project update --keep`; see [Choose versions](/reference/configuration/#choose-versions).

Sync checks the source when it first reads it. A source without `ref` fails with `ref-required`, naming the library release to write when there is one. `rules` on a non-library source fails, because it has no rules. A `--ref` that names no branch, tag, or commit fails with `version-not-found`, a branch of a library warns that the source takes changes the library hasn't released, and a skill from a non-library source with no license terms fails with `license-not-found`; see [License rules](/guides/license-rules/#license-skills). An existing source name or repository fails before writing anything; declare each repository once.

### project add group

```sh
trailguide project add group ID [options]
```

Create an authored rule group in `.trailguide/rules/ID/`, with `_group.yaml` metadata and an authoring README. `ID` is a rule group path such as `practices/testing` or `techs/typescript`.

| Option | Meaning |
| --- | --- |
| `--name TEXT` | Required. Rule group display name. |
| `--description TEXT` | Required. What the rule group covers. |
| `--when-to-read TEXT` | Required. When an agent should read the rule group. |
| `--non-interactive` | Never prompt. Supply all required inputs as flags. |

A rule group can exist before it has rules. Authored rule groups need no source declaration. Existing rule group files are preserved.

### project add rule

```sh
trailguide project add rule ID [options]
```

Create a rule in an existing authored rule group, at `.trailguide/rules/ID.md`. `ID` includes the rule group path and rule name, such as `practices/testing/check-retries`, without `.md`.

| Option | Meaning |
| --- | --- |
| `--title TEXT` | Required. Action-oriented rule title. |
| `--when-to-read TEXT` | Required. When an agent should read the rule. |
| `--impact LEVEL` | Required. One of `CRITICAL`, `HIGH`, `MEDIUM-HIGH`, `MEDIUM`, `LOW-MEDIUM`, or `LOW`. |
| `--impact-description TEXT` | Required. The consequence the rule helps prevent. |
| `--body-file PATH` | Optional UTF-8 Markdown body, without frontmatter. Relative paths start at your working directory. Omit to create an unfinished draft. |
| `--non-interactive` | Never prompt. Supply all required inputs as flags. |

Create the rule group first with `trailguide project add group`. A missing rule group fails before metadata prompts. Without `--body-file`, complete the [template](/reference/rule-authoring/#markdown-template) and remove its `trailguide:draft` marker before building. Existing rules are not overwritten.

#### Fork a library rule

```sh
trailguide project add rule ID --from LIBRARY@VERSION [options]
```

Copy one version of a library rule into `.trailguide/rules/` so the project controls its text. Use a fork to change what a rule says. To take one rule without the rest of its rule group, [include it by ID](/reference/configuration/#rule-selectors) instead, and to keep a rule at an older version, [pin it](/reference/configuration/#pin-a-rule); both keep the rule's identity and update reports. `ID` is the library rule ID, such as `practices/testing/verify-retry-limits`, and the fork keeps it: `.trailguide/rules/practices/testing/verify-retry-limits.md`.

| Option | Meaning |
| --- | --- |
| `--from LIBRARY@VERSION` | Required for a fork. `LIBRARY` is a configured source name, such as `team`, or a [repository address](/reference/configuration/#repository-addresses). `VERSION` is one of the rule's versions, such as `1.3.0`. |
| `--reason TEXT` | Why the project uses the fork instead of the library's rule. Required when the fork adds an exclusion, as in step 4 below, and refused otherwise. |
| `--non-interactive` | Never prompt. Supply all required inputs as flags. |

Metadata options and `--body-file` don't apply to a fork. The command:

1. Finds the library release that published `<VERSION>` from the release records in the library's `release/<number>` tags, and reads the rule and its asset directory at that tag's commit.
2. Writes them under `.trailguide/rules/`. When neither authored rule group metadata nor a source the project includes supplies the rule's rule group, it also creates the authored rule group from the library's rule group metadata.
3. Adds an `attribution` entry that links to the rule at the tag's commit. Libraries hosted outside GitHub.com and GitLab.com get an attribution entry only when the repository address is an HTTPS URL, and the entry links to that address.
4. When the `LIBRARY` source includes the rule and its last sync took it, adds an `exclude` entry for it with your reason, the fork's ID as `replacedBy`, and the forked version as [`basedOn`](/reference/configuration/#exclude-or-replace-a-rule), so agents read only the fork, and updates list the library's later changes to it. If the source pins the rule, the same edit removes the pin and warns that it did: the fork, not a pinned copy, now decides what agents read. If you changed that source's configuration since its last sync, run `trailguide project sync` first.

If the rule links to files in the library's shared `.trailguide/assets/` directory, the fork copies them into its own asset directory and updates the links, because authored rules can't depend on a source's files. For the same reason, the command refuses a fork whose links would point outside the copied files, such as links to declared license or notice files, or raw HTML links. Existing authored rules and exclusions are never overwritten. A forked rule has no version in your project; it changes only when you edit it. Run `trailguide project build` afterward. The library's license still applies to the copied text; see [License rules](/guides/license-rules/).

### project add skill

```sh
trailguide project add skill NAME [options]
```

Create an authored skill in `.trailguide/skills/local/NAME/`, the folder for the project's own skills, or `.trailguide/skills/<library.name>/NAME/` when the repository is also a library. `NAME` follows the [skill name grammar](/reference/skill-format/#skill-ids), such as `release-checklist`; the skill's ID is `skills/NAME`.

| Option | Meaning |
| --- | --- |
| `--description TEXT` | Required. What the skill does and when to use it, at most 1,024 characters. |
| `--type TYPE` | Optional. `playbook` or `review`, recorded as `trailguide-skill-type`. See [Skill types](/concepts/skill/#skill-types). |
| `--non-interactive` | Never prompt. Supply all required inputs as flags. |

The command writes `SKILL.md` with the name, description, and type in its frontmatter, and a body with the `trailguide:draft` marker. Complete it and remove the marker before building; an authored skill that still has the marker fails sync and build. An authored skill cites your own rules by their IDs and rules from sources by project ID, such as `fabrica:practices/testing/verify-retry-limits`; see [Skills in a project](/reference/skill-format/#skills-in-a-project). In a library, use [library add skill](#library-add-skill) instead, whose skills cite only the library's own rules. Existing skills are not overwritten, and the command refuses a name that another of the repository's skills has in any origin folder. When a skill the repository installs from a source already has the name, the command warns that sync will fail with `skill-name-clash` until you rename or exclude one of them.

#### Fork a skill

```sh
trailguide project add skill --from SOURCE:SKILL [options]
```

Copy a skill the project installs into `.trailguide/skills/<source>/`, the folder for forks of that source's skills, so the project controls its steps. `SOURCE:SKILL` names a configured source and the skill's ID in it, such as `fabrica:skills/fix-bug`, or a skill the source's library [curates](/reference/configuration/#curated-skills), by its project ID without the source's name repeated, such as `fabrica:mattpocock:skills/to-spec`. The fork keeps the skill's name, so leave out `NAME`.

| Option | Meaning |
| --- | --- |
| `--from SOURCE:SKILL` | Required for a fork. A skill that the source's last sync installed, whether the project included it or another skill uses it, including a curated one. A [required skill](/reference/configuration/#skill-levels) is refused, naming the library that requires it, and so is a curated skill the library doesn't list, which the project installs only because a listed skill uses it; fork the skill that uses it instead. |
| `--reason TEXT` | Required. Why the project uses the fork instead of the source's skill, recorded with the fork. |
| `--non-interactive` | Never prompt. Supply all required inputs as flags. |

`--description` and `--type` don't apply to a fork. The command:

1. Copies the skill's folder from `_managed/vendor/`, exactly as the project installs it, with every file's executable bit, to `.trailguide/skills/<source>/<name>/`, such as `.trailguide/skills/fabrica/fix-bug/`: the version `lock.json` records for the source, or, for a non-library source, the locked commit. A curated skill comes from the commit the library's entry holds for the repository the library curates it from. A renamed skill's fork takes its install name, so the folder and `name` match what agents already see, and the fork's entry in step 3 replaces the skill's `skills.rename` entry.
2. Rewrites the fork's `trailguide-rules-used` and `trailguide-skills-used` to project IDs, such as `fabrica:practices/testing/verify-retry-limits`, because an authored skill cites rules and skills from sources by project ID. For a non-library source, it writes the skills that `skills.uses` lists for the skill into `trailguide-skills-used`. A curated skill's uses name skills of the repository it comes from, which the library curates with it, such as `fabrica:mattpocock:skills/grilling`; a curated skill that cites rules of a curated library is forked as a draft for you to remove them from, since a project takes no rules from a curated source.
3. Adds a [`skills.forks`](/reference/configuration/#fork-a-skill) entry for the skill with `skills/<name>` as `as`, the full commit SHA it copied as `basedOn`, and your reason. For a curated skill, the entry goes under the library's source, such as `fabrica`, with the curated skill's ID, and `basedOn` is the commit of the repository the library curates it from. If you changed that source's configuration since its last sync, run `trailguide project sync` first.

:::note[Preview]
Skills have no versions yet, so the fork records the commit it copied as `basedOn` for every source, library or not.
:::

A link inside the skill's folder keeps its text and still works. A link to another skill your project installs into its skill targets, from the same source or as your fork, is changed to name that skill by the name it installs as, such as `../review/SKILL.md`, so the installed fork reaches its installed copy. Any other link that leaves the folder, such as to a shared file in the source's repository or a skill the project doesn't install, has nothing the fork can reach: the fork is marked as a draft, and its next steps list each such link, to copy the file into the fork and link to the copy, or remove the link. Its source's last sync kept those files for the skill, so the next steps install the fork with `trailguide project sync`, which drops them, instead of build. A `SKILL.md` without frontmatter, which a repository that isn't a library can supply, gets frontmatter with the skill's name, and each list the fork changes becomes a block string. When the command can't adapt the skill's `SKILL.md`, because its frontmatter is written in a way it can't edit, such as `metadata` as a flow mapping, or because the fork would fail the [checks build applies to a skill you author](/reference/skill-format/#validation), it still writes the fork, marked as a draft, and its next steps list what to fix. Build refuses the fork until you fix it and remove the `<!-- trailguide:draft -->` line. The command refuses a skill the source already excludes or forks, and a fork whose folder exists or that another fork already names as its `as`. When an update moves the source while the command waits for your reason, it writes nothing; run it again. Existing authored skills, exclusions, and forks are never overwritten. A forked skill has no version; it changes only when you edit it. Run `trailguide project build` afterward. The source's license still applies to the copied files; see [License rules](/guides/license-rules/#license-skills).

### project sync

```sh
trailguide project sync [options]
```

Take the rule and skill versions recorded for each source, validate their files, and replace `_managed/vendor/` and `_managed/generated/` in the Trail Guide directory with the complete result. Sync also writes each installed skill's folder into the repository's skill targets, removes the folders of skills it no longer installs, refreshes an older, unedited managed Trail Guide guide, and adds or refreshes the Trail Guide section of `AGENTS.md`, as build does.

Accepts the [shared options](#shared-options-and-prompts) and:

| Option | Meaning |
| --- | --- |
| `--retake SOURCE` | Resolve `SOURCE` again at its `ref`, ignoring its entry in `lock.json`, as if you had just added it. Its unpinned rules move to the newest versions that `ref` offers. Repeat the option for more sources. |

Sync needs access to every configured repository and uses your existing Git credentials. It takes the library release each source's `ref` names, or a branch's or tag's recorded commit, and restores the rule versions recorded in [`lock.json`](/reference/provenance/#inspect-the-lockfile). Sync chooses a version only when configuration asks for something the lockfile doesn't have:

| Situation | What sync takes |
| --- | --- |
| A new source, or a changed repository | The source as `trailguide project update` would: the newest library release, or the commit the source's `ref` names now. |
| A newly included rule or skill | The library release `ref` names, or the recorded commit. Each new rule gets the newest version that library release or an earlier one published. A rule or skill that isn't there, such as one the library published later, fails with `skill-not-in-revision`, naming `trailguide project update SOURCE`. |
| A pin added or changed | The pinned version, up or down, when the library release `ref` names or an earlier one published it. A later version fails, naming the release to set `ref` to. |
| A pin removed | The recorded version, unchanged. The next update offers newer versions. |
| `ref` changed to another library release | Nothing: sync fails with `ref-changed`, naming `trailguide project update SOURCE`, which takes exactly that release when it's newer than the recorded one. See [Change a ref](/reference/configuration/#change-a-ref). |
| `ref` removed | Nothing: sync fails with `ref-required`, naming the release the project records. |
| A `ref` that isn't a library release added or changed | The revision the configuration now names, as for a new source: a branch's newest commit, or a tag's or commit's revision. Until sync takes it, offline checks report the entry as out of date and name `trailguide project sync`. A `ref` changed to another spelling of the recorded revision, such as `refs/tags/release/2` for `release/2`, is current. |

Sync refuses a lockfile entry changed outside Trail Guide: an entry whose [integrity](/reference/provenance/#integrity) doesn't match, or that has none, such as after a hand edit or a merge resolution that combined both sides, fails, and sync writes nothing. Restore the entry Trail Guide wrote, such as with `git checkout -- .trailguide/lock.json`, and sync again; or run `trailguide project sync --retake SOURCE`, which takes the source again at its `ref`. To resolve a merge conflict in `lock.json`, see [Resolve a merge conflict in the lockfile](/reference/provenance/#resolve-a-merge-conflict-in-the-lockfile). The first sync of a project that Trail Guide 0.5 last synced [migrates it to `lock.json`](/reference/provenance/#migrate-from-trail-guide-05) without moving any source. Before taking anything from an entry it wrote, sync also checks it against the library's release records: each locked rule version must be the version its recorded library release publishes, and each locked library release must exist. A source using `ref` keeps the commit it locked, even after a tag moves or is gone, so sync checks it against the release records only when choosing versions reads them anyway.

In a project that Trail Guide 0.7 or earlier wrote, sync also moves `vendor/` and `generated/` under `_managed/`, and other commands refuse the project until it does; see [Migrate from Trail Guide 0.7](/reference/provenance/#migrate-from-trail-guide-07).

Sync installs the skills each source's levels and `skills.include` bring in, every skill they use, transitively, and every rule an installed library skill cites; see [Skill selectors](/reference/configuration/#skill-selectors). A skill curated by a library comes from the commit the library's entry in `lock.json` holds for it. Sync writes a skill folder in a target only when `_managed/generated/installed-skills.json` records that Trail Guide installed it there, or when no folder with that name exists. It fails, writing nothing, with `skill-folder-not-managed` when a folder with an installed skill's install name exists in a target that Trail Guide didn't install, and with `skill-name-clash` when two skills would install under the same name, from any sources, authored skills, or both; rename or exclude one of them. See [Who owns the target folders](/reference/sync/#who-owns-the-target-folders).

Sync also keeps every file an installed skill's Markdown links reach, from the same revision, and points the installed links at those files; see [Links](/reference/skill-format/#links). Before it writes anything, it checks every link, and fails, writing nothing, when one names a file the revision doesn't have, with `reference-missing`; a folder, symbolic link, rule file, or other file the project can't keep, with `reference-unsupported`; an absolute path or one outside the repository, with `reference-escapes`; or a file a forked skill's fork lacks, with `reference-replacement-missing`. The message names the file with the link, the link, the source and revision, the chain of links that reached it, and the repairs. A link into a skill the configuration excludes, by ID or by level, doesn't fail: sync keeps the files it reaches as reading copies and warns, naming the installed skill, the excluded skill, and the chain of files the link goes through, and lists the same facts under `excludedSkillLinks` in `--json` output; see [When a skill links to a skill you exclude](/guides/install-skills/#when-a-skill-links-to-a-skill-you-exclude). [project update](#project-update) checks the revisions it moves to the same way.

These changes come from edits you made to configuration, so sync applies them without a preview. To move sources to newer revisions, use [project update](#project-update). If a source fails, the previous complete output is preserved. For recovery behavior, see [Sync and recovery](/reference/sync/).

Sync warns:

- about a source that takes unreleased changes through `ref`, naming the source and its unreleased rules and skills;
- when an exclusion, a rule included by ID, or a skill included by ID names a rule or skill the library retired, or a skill folder a non-library source no longer has, since the entry no longer does anything;
- about a rule included by ID that a prefix in the same `include` already reaches, since the entry changes nothing;
- for each place a skill from a non-library source breaks the [skill format](/reference/skill-format/), and installs the skill as it is;
- for each rename it applies, with the fixed text in [Rename a skill](/reference/configuration/#rename-a-skill);
- for each personal skill with the same name as an installed skill; see [Personal skills with the same name](/reference/sync/#personal-skills-with-the-same-name);
- when it installed no skill copies because `skillTargets` is empty, though the repository has skills to install.

### project update

```sh
trailguide project update [SOURCE | SOURCE:RULE ...] [options]
```

Preview each source's move along its `ref`: to the newest library release for a release such as `release/4`, or to a branch's newest commit, with newer rule and skill versions, new rules and skills, level changes, and retirements, then apply it after you confirm, and regenerate guidance and installed skills like sync. With no arguments, every source is updated. `SOURCE`, such as `team`, limits the update to one source. `SOURCE:RULE`, such as `team:techs/react/prefer-server-components`, moves only that rule; nothing else changes, including new rules.

A source's skills all come from one revision: the library release that supplies its library-wide files, or a non-library source's commit. They move together, with those library-wide files or that commit, and `trailguide project update SOURCE:SKILL`, such as `fabrica:skills/build-feature`, is refused with a message asking you to update the whole source. `--keep` and `--update-fork` are for rules only. `--exclude` takes a skill as `SOURCE:SKILL` the same way it takes a rule, and `--merge-fork` takes only a forked skill.

| Option | Meaning |
| --- | --- |
| `--yes` | Apply the previewed changes without asking. Required to apply changes without a terminal, or with `--json`. |
| `--keep SOURCE:RULE` | Pin this rule at its current version before applying the update, so it stays where it is. Repeat for several. Requires `--reason`. Refused for a skill, which can't be pinned. |
| `--exclude SOURCE:RULE` | Exclude this new rule or skill before applying the update, so it doesn't join. Repeat for several. Requires `--reason`. Refused for a skill that becomes required. |
| `--update-fork SOURCE:RULE` | Replace your authored rule for this `replaced` row with a fork of the library's newest version, discarding your edits, and set its exclusion's `basedOn` to that version. Repeat for several. Takes no `--reason`. Refused for a forked skill, naming `--merge-fork`. See [Replace a fork with the newest version](#replace-a-fork-with-the-newest-version). |
| `--merge-fork SOURCE:SKILL` | Merge the source's changes into your forked skill, keeping your edits, and set the fork's `basedOn` to the commit merged. Refused when the merge finds text conflicts, and for a rule, naming `--update-fork`. Repeat for several. Takes no `--reason`. See [Merge a source's changes into a forked skill](#merge-a-sources-changes-into-a-forked-skill). |
| `--reason TEXT` | The reason recorded with each pin that `--keep` writes and each exclusion that `--exclude` writes. |

The preview lists, for each source:

| Change | Meaning |
| --- | --- |
| `major`, `minor`, or `patch` | The rule's old and new versions, and the summary of every version in between. |
| `new` | A rule the library added that your selectors reach, at its newest version. |
| `retired` | A rule the library retired, with its last version, its summary, and its replacement when there is one. When the library later retired the replacement too, the preview says so, and names the rule its replacements lead to, if any, instead of recommending the retired one. When your project already replaces the replacement with an authored rule, the preview names your rule instead of recommending the library's. Applying the update drops it. |
| `replaced` | A rule you [replaced with an authored rule](/reference/configuration/#exclude-or-replace-a-rule), with your rule's path. When the exclusion records `basedOn`, the row appears whenever the library's newest version is newer than `basedOn`, and lists every change after it, labeled with its version when there are several, also when a pin keeps the vendored copy where it is. Without `basedOn`, it compares with the version the project's lock records instead: it appears whenever the library's newest version is newer, also when a pin keeps the vendored copy where it is, and lists the changes since, which your rule may already have. Your rule doesn't change unless you pass `--update-fork`; compare the two to decide whether it needs the same change. |
| `pinned` | A pinned rule that has a newer version. It doesn't move; the preview shows the pin's reason. |

Skills from a library appear in the same list, by their skill IDs, with the same changes except `pinned`. When the source's library-wide files move to a newer library release, every installed skill moves with them, and the preview lists each skill whose version changed. A skill that a moving skill newly uses appears as `new`, naming the skill that uses it, and a rule it newly cites appears as `new`, at its newest version, marked reference-only unless your selectors also reach it. A skill or cited rule that no installed skill needs any longer is dropped.

A skill whose level changes shows the move under its row, such as `level: recommended -> required`, with what the change means for the project. A move that installs a skill, such as `offered -> required`, or `offered -> recommended` when you include `recommended`, appears under the skill's `added` row, and a move that stops installing one, such as `recommended -> offered` when you don't name it, under its `removed` row; a skill you forked shows it under its `replaced` row. A skill whose level changes while its files don't, and that the move neither installs nor removes, gets a `level` row of its own, so you can decide whether to include it. A skill that becomes required while your configuration excludes or forks it fails the update, naming the library and the exclusion to remove: required skills can't be excluded.

Under each skill's row, from a library or a non-library source, the preview flags every executable file that's new or changed, on its own line, such as `script added: scripts/setup.sh` or `script changed: scripts/verify.sh`: a file whose executable bit is set at the new revision and that the update adds or changes, or a file whose executable bit changed. When it flags any, it prints one warning that Trail Guide never runs these scripts but agents may, so read them before confirming. The warning doesn't block the update.

A source whose `ref` names a branch, and a non-library source whose `ref` names a tag, lists its commit move on a line under the source's name, such as `main: 3f9c2e1a5b7d -> 8a41d07c2e9f`, or `main: no new commit.` when the commit doesn't move, and warns when a tag now names a different commit than at the last update. A [curated source](/reference/configuration/#curated-skills) lists its move on a line of its own, under the library that curates it, such as `fabrica curates mattpocock  main: 3f9c2e1a5b7d -> 8a41d07c2e9f`, or `release 3 -> 4` for one that follows its newest library release, since each project resolves a curated ref when it updates. For a non-library source, and for curated skills, the preview lists one row for each installed skill folder that differs between the recorded and new commits:

| Change | Meaning |
| --- | --- |
| `changed` | A skill folder whose files differ, with the paths of the files that changed, including a file whose executable bit changed. |
| `added` | A new skill folder that the source's selectors include, such as with `"*"`. |
| `removed` | An installed skill whose folder no longer exists. Applying the update stops installing it. |

See [Update rules and skills](/guides/update/) for examples.

In a terminal, `trailguide project update` shows the preview and asks for confirmation. For each major change and retirement of a rule, you can choose to keep the rule at its current version instead; the command then asks for a reason and writes a pin. It asks you to confirm each major change, retirement, and level change of a skill too, but can't keep a skill back on its own: to hold one, decline, then exclude or fork the skill, or set the source's `ref`. For each new rule or skill that isn't required, you can choose to add it or exclude it; excluding asks for a reason and writes an exclusion. For each `replaced` rule, including one whose vendored copy a pin keeps, it names the authored files that replacing your rule would replace or remove, and you can review the changes later, or replace your rule with a fork of the newest version, as `--update-fork` does. For each forked skill whose source's changes merge into your fork with no text conflicts, you can review them later, or merge them, as `--merge-fork` does. When your answers keep, exclude, or replace rules, or merge into forked skills, it lists them before you confirm, with the files each replaced fork replaces and those it removes. The terminal shows the preview once; the result after you confirm lists the files the update changed. Before each wait on the sources, such as reading their releases or fetching the update after you confirm, a terminal also shows a short status line, such as `Fetching the update...`, on standard error; JSON output and output that isn't a terminal never include it. Without a terminal, or with `--json`, it shows the preview and writes nothing unless you pass `--yes`. The update applies exactly the versions the preview showed, and writes its pins, exclusions, `basedOn` versions, and replaced forks in the same step that replaces `_managed/vendor/` and `_managed/generated/`. When the preview has no rule or skill to move, add, or drop, and no shared files or commits to move, a terminal update applies without asking, as sync would, and when that changes no files, says, as a preview would, that no updates are available and no files were written.

A preview that writes nothing exits `0`, because available updates aren't an error; so does declining the confirmation. In JSON output, `value.applied` is `false` for a preview and `true` once the update is applied, and `value.sources` lists each source's `rules`, each with its `id`, `change`, versions (`from`, `to`, and, where they apply, `newest` or `lastVersion`), `summaries`, `summaryVersions` (the version each summary belongs to, in the same order), and, where they apply, `replacedBy` (with `replacementRetired: true` and any `currentReplacement` when the library retired the replacement too, and `replacementAuthoredRule` when your project already replaces that replacement with an authored rule), `authoredRule`, `basedOn`, `pin`, and the `decision` (`keep`, `exclude`, or `update-fork`) that `--keep`, `--exclude`, `--update-fork`, or your answers recorded, with the `reason` of a pin or exclusion. A row's `change` is one of the preview's changes: `major`, `minor`, `patch`, `new`, `retired`, `replaced`, or `pinned`. For an `update-fork` decision, a row's `overwrites` lists the authored files that the new fork replaces with its own copies, and `removes` those it deletes because the new fork doesn't have them, relative to the Trail Guide directory; both are empty for other rows. A `replaced` row's `from` is the version the project's lock records and `to` the newest version, or `newest` instead when a pin keeps the vendored copy; with `basedOn`, its `summaries` are the library's changes after `basedOn`, and otherwise the changes since `from`, not the changes its `authoredRule` lacks. The `added`, `changed`, and `removed` lists are empty for a preview; once applied, they include `config.yaml` when the update wrote a `ref`, pins, exclusions, `basedOn` versions or commits, or a higher `minTrailguideVersion`, and the files of each fork it replaced or merged into. Each source's `replacedSkills` lists its forked skills that changed, and each curated source's `replaced` those of the skills the library curates, each with its `id`, `localSkill`, `basedOn`, `from`, `to`, `files`, `removed`, and, for a fork with `basedOn` of a skill the source still has, `merge`, `no-conflicts` or `conflicts`, with the conflicting files in `conflicts`, and the `merge-fork` `decision` when the update merges into it.

In JSON output, each source in `value.sources` also has a `skills` list, whose rows have the same fields as its `rules`, and `scripts`, a list of the flagged scripts, each with its `path`, relative to the skill's folder, and its `change`, `added` or `changed`, empty when the row flags none. Each source also has `skillLevels`, the skills whose level changes, each with its `id` and the levels `from` and `to`, empty on a side where the library has no such skill. A source whose `ref` names a branch, and a non-library source whose `ref` names a tag, has `followsRef: true`. Such a source, and each entry in a source's `curated` list, has `commit`, with the commits `from` and `to`, absent when it doesn't move, and `tagMoved: true` when a tag now names a different commit. A curated source's `commit` leaves out `from` when the project didn't take skills from it before, and `to` when it no longer does; one that follows its newest library release also has `release`, with the library releases `from` and `to`. Rows for skills from a non-library source or a curated source have a `change` of `added`, `changed`, or `removed`, and `files`, the paths of the changed files relative to the skill's folder, empty for other rows. Each entry in `curated` also has `replaced`, the rows for curated skills the project replaces with a fork, each with its `id`, such as `mattpocock:skills/to-spec`, `localSkill`, `basedOn`, the commits `from` and `to`, and `files`.

The preview doesn't list changes to rules or skills you excluded without a replacement. `--keep` and pins in configuration never change which rules are included. When a rule you exclude or include by ID is retired, the entry no longer does anything; update warns about it so you can delete it, and applies the rest of the update. When the update, or a sync, would leave your authored rules in a rule group with no metadata, such as a fork of a retired rule, it writes the rule group's metadata to `rules/<group-id>/_group.yaml` in the same step, lists the file as added, and warns that it did; see [Keep the rule group of an authored rule](/reference/sync/#keep-the-rule-group-of-an-authored-rule).

When a library source's `ref` names a library release, an update that moves the source writes the release it installs to `ref`, keeping the spelling, such as `release/5` for `release/4`, in the same step that replaces `_managed/vendor/` and `_managed/generated/`. The preview says so on a line under the source, such as `Sets sources.team.ref to release/5 in .trailguide/config.yaml.`, and JSON output as the source's `setsRef`. A source whose `ref` is `latest-release` moves to the newest release, and its `ref` stays as it is. When you edited `ref` to name a newer release than the one the project records, the update takes exactly that release and leaves `ref` as written; when the library has a newer release still, the preview ends with a line such as `Release 9 is available; run trailguide project update team again to move to it.`, and JSON output gives that release as the source's `available`. An update refuses a `ref` edited to an older release with `ref-changed`.

An update raises [`minTrailguideVersion`](/reference/configuration/#minimum-trail-guide-version) to the running release when the configuration names an older one or none, in the same step, so teammates with an older Trail Guide get a clear error instead of different output. The preview says so after the sources, such as `Sets minTrailguideVersion to 0.10.0 in .trailguide/config.yaml`, and JSON output gives the release as `value.setsMinTrailguideVersion`. A raise counts as an update to apply: a terminal asks you to confirm it, and without a terminal it needs `--yes`, even when no rule or skill moves.

An update of a whole source also moves its [library-wide files](/reference/rule-versions/#what-a-version-covers), such as the manifest, rule group metadata, shared assets, and the library's license files, to the newest library release, or the commit its `ref` names now, even when no rule moves; the preview shows it as `Shared files: release 3 -> 4`, and JSON output as the source's `sharedFiles`, with the library releases `from` and `to`, which is absent when they don't move. A library source whose `ref` names a tag other than a library release, or a commit, doesn't move, and neither does a non-library source whose `ref` names a commit; change `ref` and run `trailguide project sync` to take the revision it names. A library source whose `ref` names a branch moves its library-wide files with the branch's newest commit. A `SOURCE:RULE` update leaves the library-wide files where they are, unless a rule it moves comes from a newer library release: then they move to that library release, since a rule version can rely on the shared files its library release published. The source's skills and skill levels always move with its library-wide files.

#### Skills and their scripts

A source's skills come from the revision that supplies its library-wide files, so they move with them. For each source the update names, the preview compares the skills the source installs once updated with the copies its source record holds, and lists, after the rules, one row for each skill that differs:

| Change | Meaning |
| --- | --- |
| `changed` | A skill whose files differ, with the paths of the files that changed, including a file whose executable bit changed. |
| `added` | A skill the update starts installing, such as one that `skills.include` newly names. |
| `removed` | An installed skill the update stops installing, because `skills.include` no longer names it, or, with `*`, the new revision no longer has it. |

Under each skill's row, the preview flags every executable script the update adds or changes, on its own line, such as `script added: scripts/setup.sh` or `script changed: scripts/run.sh`: a file whose executable bit is set at the new revision and that the update adds, changes, or makes executable. Every executable file of an `added` skill is flagged as added. A script the update leaves as it is isn't flagged, and neither is a file that isn't executable at the new revision, even when it changed. A script is any file with its executable bit set, which Git records as mode `100755`, wherever it is in the skill's folder; a file in `scripts/` without the bit isn't one.

When the preview flags any script, it ends with one warning:

```text
Warning: some skills in this update add or change scripts. Trail Guide never runs them, but your agents may; read them before you confirm.
```

In a terminal, the warning follows the preview, before any question. It doesn't block the update. A skill row is a change to confirm, so a terminal asks before applying it. An applied update, such as one with `--yes`, lists the same rows and warns `some skills in this update added or changed scripts. Trail Guide never runs them, but your agents may; read them before you commit the update.`

In JSON output, each source in `value.sources` has a `skills` list, empty when no skill differs. Each row has its `id`, `change`, `files`, the changed paths relative to the skill's folder for a `changed` row and empty for other rows, and `scripts`, a list of the flagged scripts, each with its `path`, relative to the skill's folder, and its `change`, `added` or `changed`, empty when the row flags none. The warning is in `value.warnings`.

When your decisions keep a `SOURCE:RULE` update's library-wide files at an older library release, its skills stay there too, and the preview shows the skills at that release. A skill that `skills.include` names and the update's revision lacks fails the update, as installing it would. A source that update doesn't move, such as a library source whose `ref` names a tag or commit, still lists in its rows a skill that the update would add or remove because `skills.include` changed. Sync doesn't flag scripts, because it never moves a source.

#### Replace a fork with the newest version

This section describes rule forks, whose update discards your edits. A skill fork's update keeps them, so it takes its own flag, `--merge-fork`; see [Merge a source's changes into a forked skill](#merge-a-sources-changes-into-a-forked-skill).

`--update-fork SOURCE:RULE` replaces the authored rule that a `replaced` row names, the exclusion's `replacedBy`, with a fork of the version the row lists changes up to: the library's newest version, also when a pin keeps the vendored copy at an older one. The new fork is exactly what [`trailguide project add rule RULE --from SOURCE@VERSION`](#fork-a-library-rule) writes, with its attribution, its assets, and the shared assets its links reach, placed at the path of the `replacedBy` rule, with links rewritten for that path. The update sets the exclusion's `basedOn` to that version, adding it to a replacement you wrote yourself, and installs the fork, `config.yaml`, `_managed/vendor/`, and `_managed/generated/` in one step, so an interrupted update restores or finishes them together.

Passing `--update-fork` is your consent to overwrite the authored rule: your edits to it are lost, so commit them first if you want to compare. The preview, human and JSON, lists every file it replaces and, separately, every file it removes, reading the new fork from the library to tell them apart. The authored rule's asset directory is replaced as a whole, including files you added to it, and the shared assets the new version links to are copied into it, as a fork copies them. Nothing outside the rule and its asset directory changes, because each rule owns its asset directory and no other rule may link into it. A pin stays: it governs only the vendored copy.

`--update-fork` refuses, as an `invalid-arguments` usage error that exits `2`, a rule the source doesn't exclude with `replacedBy`, an unknown source, a `--reason`, and a rule that `--keep` or `--exclude` also names, before reading any source. It also refuses a rule the preview doesn't list as `replaced`, because the library has nothing newer than the version your rule is based on or the update doesn't include the rule, and a rule the library retired, which has no newest version to fork. Like the rest of the update, it fails with `invalid-release-tag`, writing nothing, when the library release tag that published the fork's version names another commit than when the update previewed it, also when a pin or a `SOURCE:RULE` update keeps the vendored copy at an older version. Without `--update-fork`, `--yes` leaves every authored rule as it is.

#### Merge a source's changes into a forked skill

A `replaced` row for a skill you [forked](#fork-a-skill) with `basedOn` also says whether merging the source's changes since `basedOn` into your fork finds text conflicts. Trail Guide compares, file by file, three copies of the skill: the source's copy at `basedOn` and at the revision the update takes, each as forking it would write it, and your fork. A file only one side changed takes that side's copy. A text file both sides changed is merged line by line, as `git merge-file` merges it, and conflicts when your edits and the source's touch the same or adjacent lines. A file that one side deleted and the other changed, one both added with different contents, and a binary file both changed conflict too. A fork with no edits of its own never conflicts. The merge compares text, not meaning: changes to different lines can still contradict each other, so review the merged skill.

When the merge finds no text conflicts, the preview says so, and `--merge-fork SOURCE:SKILL`, such as `fabrica:skills/build-feature`, or `fabrica:pocock:skills/to-spec` for a skill the library curates, writes the merged fork to the folder that the fork's `as` names and sets `basedOn` to the commit merged, in the same step that installs `config.yaml`, `_managed/vendor/`, and `_managed/generated/`. Your edits stay, and later updates list only newer changes. In a terminal, the update asks whether to merge each such skill now or review it later.

When it finds conflicts, the preview lists the conflicting files, and `--merge-fork` refuses as an `invalid-arguments` usage error that exits `2`, naming the files and writing nothing. Merge the source's changes into your fork by hand, then set `basedOn` to the commit the preview names. `--merge-fork` also refuses, before reading any source, a `--reason`, an unknown source, a skill the source doesn't fork, a rule, which `--update-fork` replaces instead, and a fork without `basedOn`, since Trail Guide then can't tell your edits from the source's changes, and, once the update reads the source, a skill the preview doesn't list or that the source no longer has.

If you merged the library's changes into your authored rule by hand, record the version you merged by editing `basedOn` in `.trailguide/config.yaml`, so later updates list only newer changes.

See [Update rules and skills](/guides/update/) for the workflow.

### project build

```sh
trailguide project build [options]
```

Regenerate `_managed/generated/` from configuration, verified vendor snapshots, and authored rules and skills, and rewrite the installed skill folders in the repository's skill targets. Build works offline and does not change recorded revisions. It also creates a missing managed Trail Guide guide or refreshes an older, unedited guide alongside generated output, and adds or refreshes the Trail Guide section of the repository's `AGENTS.md`. A manually edited guide or section stops the build before it changes output. See [Where agents start](/reference/files/#where-agents-start).

Accepts the [shared options](#shared-options-and-prompts) only.

Build points installed skills' links at the files they reach, using only the vendor snapshots, and fails with the same `reference-` codes as sync. It gives sync's warning for each installed skill that links to a skill the project excludes. When a snapshot lacks a linked file, such as one an older Trail Guide synced, it fails with `reference-not-retained`; run `trailguide project sync`, which takes the file from the recorded revision without moving the source.

Run sync first if the recorded repository, selectors, pins, or `ref` no longer match configuration, or if the vendor snapshots need repair. Build validates authored skills, and fails on an unfinished draft, as sync does.

### project check

```sh
trailguide project check [options]
```

Check generated guidance, installed skills, the managed Trail Guide guide, and the Trail Guide section of `AGENTS.md` without writing files or contacting repositories. Reports stale, missing, or unexpected output and invalid inputs, including an installed skill folder that differs from what sync would write, a folder that `_managed/generated/installed-skills.json` records but that's missing, such as when `.gitignore` excludes the target folders so they were never committed, a stale `installed-skills.json`, a missing or edited copy of a file an installed skill links to, a link that no longer reaches its file, with the same `reference-` codes as build, an authored skill that cites a rule its library retired, a missing or outdated `AGENTS.md` section, a `lock.json` entry that was changed outside Trail Guide, which no command trusts until you restore it or take the source again, and a source whose `ref` names another library release than the lockfile records, which needs `trailguide project update SOURCE`; see [Integrity](/reference/provenance/#integrity). It warns about personal skills with the same name as installed skills, as sync does, without failing.

Accepts the [shared options](#shared-options-and-prompts) only.

Use check in CI to detect files that need regeneration. It uses recorded commits and rule and skill versions, and does not check whether remote tags or branches moved, or newer library releases exist. Success means the managed files agree with their inputs; it does not establish that application code follows the rules.

### project list skills

```sh
trailguide project list skills
```

List the skills in the project: every skill its sources offer, whether the project installs it, and its level, followed by the project's own authored skills, so you can see what you have and choose what to add. Lists never write files.

The listing reads `.trailguide/lock.json`, which the last sync or update wrote, so it works offline and shows the revisions the project uses, not newer ones. After changing the configuration, run `trailguide project sync` first. To take skills a newer library release adds, run `trailguide project update SOURCE`.

```text
$ trailguide project list skills
SKILL                                           LEVEL        SET BY                   STATUS
fabrica:mattpocock:skills/engineering/to-spec   recommended  fabrica, release 7       installed
fabrica:skills/build-feature                    recommended  fabrica, release 7       installed
fabrica:skills/deploy-preview                   offered      fabrica, release 7       excluded
fabrica:skills/review-change                    offered      fabrica, release 7       replaced by skills/review-change
fabrica:skills/ship-service                     required     fabrica, release 7       installed
fabrica:skills/tdd                              offered      fabrica, release 7       available
tools:skills/changelog                          -            tools, not a library     installed
skills/check-release                            -            this project             installed
skills/review-change                            -            this project             installed
```

Each row shows:

- **SKILL:** the skill's project ID. A curated skill's ID names the library and the curated source, such as `fabrica:mattpocock:skills/engineering/to-spec`; an authored skill's ID is `skills/<name>`.
- **LEVEL:** the level the library sets, `required`, `recommended`, or `offered`; `dependency` for a skill the library installs only with the skills that call it; or `-` for an authored skill or a skill from a repository that isn't a library.
- **SET BY:** the library and the library release, or the commit, whose manifest set the level; `not a library` for a source without a manifest; or `this project` for an authored skill.
- **STATUS:** `installed` for a skill the project installs, whether included, required, or used by another installed skill; `excluded` for one `skills.exclude` leaves out, by ID or by level; `replaced by` and the fork that `skills.forks` installs in its place; and `available` for one a source offers that the project doesn't install.

Source skills are sorted by project ID, and authored skills follow them. To install an available skill, add it to that source's `skills.include` and run `trailguide project sync`. A `dependency` can't be included on its own: include a skill that calls it instead.

JSON output lists the skills in `value.skills`, each with its `id`, `status`, and, where they apply, `source`, `level`, `setBy` (the `source`, `release`, and `commit` that set the level), and `replacedBy`.

### project list rules

```sh
trailguide project list rules
```

List the rules in the project: every rule it holds of its sources, with its version and how the project uses it, and the project's own authored rules. Lists never write files.

Like `list skills`, the listing reads `.trailguide/config.yaml` and `.trailguide/lock.json`, so it works offline and shows the versions the project holds, not newer ones. After changing a source's repository, ref, selectors, or pins, run `trailguide project sync` first; until then, the listing refuses the lockfile. To see what a newer library release changes, run `trailguide project update SOURCE`, which previews the changes before writing anything.

```text
$ trailguide project list rules
RULE                                                 VERSION  FROM                STATUS
fabrica:practices/testing/check-retries              1.2.0    fabrica, release 7  replaced by local:practices/testing/service-retries
fabrica:practices/testing/cover-boundary-cases       1.1.0    fabrica, release 7  active
fabrica:practices/testing/test-observable-behavior   1.0.1    fabrica, release 3  active, pinned at 1.0.1
fabrica:techs/go/use-context-loggers                 1.0.0    fabrica, release 2  excluded
local:practices/testing/service-retries              -        this project        active, replaces fabrica:practices/testing/check-retries
local:techs/go/name-errors                           -        this project        active
```

Each row shows:

- **RULE:** the rule's project ID. A rule from a source starts with the source name; an authored rule starts with `local:`.
- **VERSION:** the version the project holds, or `-` for an authored rule or a rule file with unreleased changes.
- **FROM:** the source and the library release that published the version, or the commit for a rule with unreleased changes; or `this project` for an authored rule.
- **STATUS:** `active` for a rule the configuration includes in the project's guidance; `excluded` for one an exclusion leaves out; `replaced by` and the authored rule agents receive instead; or `draft, not built` for an authored rule that still has its draft marker, which build refuses until you complete it. It adds `replaces` and the rule an authored rule replaces, `pinned at` and the version a pin keeps, and `retired` for a rule its library has retired, which `trailguide project update` removes unless a pin keeps it. An authored rule keeps `replaces` after an update removes the rule it replaces.

Rules are sorted by project ID. To add or remove rules, edit the source's `rules.include` or `rules.exclude` and run `trailguide project sync`.

JSON output lists the rules in `value.rules`, each with its `id`, `path`, `active`, and `retired`, and, where they apply, `source`, `version`, `release`, `commit`, `pin` (its `version` and `reason`), `exclusion` (its `reason`), `replacedBy`, `replaces` (the replaced rule's `id` and the `basedOn` version), and `draft`. `active` describes the configuration and lockfile, not the generated files: [`trailguide project check`](#project-check) reports whether those are current. Tools written in Go can compute the same view from a project's configuration and lockfile with the `github.com/fabricahq/trailguide/trailguide/projectview` package; see [Read a project from a tool](/reference/provenance/#read-a-project-from-a-tool).

## Library commands

`trailguide library` manages a **library**, a repository whose [manifest](/reference/library-schema/#the-manifest) publishes its authored rule groups and skills for projects to take. Library commands use the same `.trailguide/` directory and `config.yaml` as project commands, so a repository can be both. Initialize from the repository root. Other library commands find that root from subdirectories. Use `--directory PATH` to target another library; see [Working directories](#working-directories).

### library init

```sh
trailguide library init [options]
```

Add a `library` section, the [manifest](/reference/library-schema/#the-manifest), to `.trailguide/config.yaml`, creating the configuration with `schemaVersion: 2` and `sources: {}` when it doesn't exist, and create the authored `rules/` and `skills/` directories, an authoring README, and a GitHub Actions workflow, without overwriting existing authored files. Optionally copy explicitly supplied license terms into the library and declare them in `library.license`. A configuration that already has a `library` section keeps it.

The workflow, `.github/workflows/trailguide.yml`, runs `trailguide library check` on every pull request, using the release that the library's [`minTrailguideVersion`](/reference/configuration/#minimum-trail-guide-version) names when it runs, so it moves when `trailguide project update` raises the minimum. When the configuration names none, such as one a development build created, it uses the latest release. Either way, the workflow installs only that version's archive, after verifying it against the release's attested `SHA256SUMS`, and prints the version it installed in the job's log and summary. Rerunning `trailguide library init` never changes an existing workflow, so one written by Trail Guide 0.8 or earlier keeps installing the release it names; `trailguide project update` warns when raising the minimum would make that release refuse the library. See [Check changes in CI](/guides/version-rules/#check-changes-in-ci).

| Option | Meaning |
| --- | --- |
| `--directory PATH` | Directory to operate in. Defaults to your working directory; repository discovery applies. Init must target the repository root. |
| `--name NAME` | The library's [name](/reference/library-schema/#library-name), which names the folder of its own skills, `.trailguide/skills/NAME/`. Defaults to the repository folder's name, lowercased with each run of other characters turned into a hyphen, or `library` when that isn't a valid name. Ignored when the configuration already has a `library` section. |
| `--spdx EXPRESSION` | Library SPDX expression. Must be supplied together with `--license-file`. |
| `--license-file PATH` | UTF-8 license text to copy to `LICENSE.md`. Relative to your working directory, even with `--directory`. |
| `--notice-file PATH` | Optional UTF-8 notice text to copy to `NOTICE.md`. Requires both license options. Relative to your working directory. |
| `--non-interactive` | Accepted; library init runs without prompts even when this flag is omitted. |

If you omit the license options, the manifest leaves the license undeclared. Init does not infer license terms, create a Git repository, commit, or publish the library. A repository with a `rule-library.yaml` fails with a message that shows the manifest's shape; see [No backward compatibility](/reference/configuration/#no-backward-compatibility).

### library add group

```sh
trailguide library add group ID [options]
```

Create a rule group in `.trailguide/rules/ID/`, with `_group.yaml` metadata and an authoring README. `ID` is a rule group path such as `practices/testing` or `techs/typescript`. It writes the same files as `trailguide project add group`.

| Option | Meaning |
| --- | --- |
| `--directory PATH` | Directory to operate in. Defaults to your working directory; repository discovery applies. |
| `--name TEXT` | Required. Rule group display name. |
| `--description TEXT` | Required. What the rule group covers. |
| `--when-to-read TEXT` | Required. When an agent should read the rule group. |
| `--non-interactive` | Never prompt. Supply all required inputs as flags. |

Empty rule groups are valid. Existing rule group files are preserved.

### library add rule

```sh
trailguide library add rule ID [options]
```

Create a rule in an existing rule group, at `.trailguide/rules/ID.md`. `ID` includes the rule group path and rule name, such as `techs/javascript/prefer-for-of`, without `.md`.

| Option | Meaning |
| --- | --- |
| `--directory PATH` | Directory to operate in. Defaults to your working directory; repository discovery applies. |
| `--title TEXT` | Required. Action-oriented rule title. |
| `--when-to-read TEXT` | Required. When an agent should read the rule. |
| `--impact LEVEL` | Required. One of `CRITICAL`, `HIGH`, `MEDIUM-HIGH`, `MEDIUM`, `LOW-MEDIUM`, or `LOW`. |
| `--impact-description TEXT` | Required. The consequence the rule helps prevent. |
| `--body-file PATH` | Optional UTF-8 Markdown body, without frontmatter. Relative paths start at your working directory. Omit to create an unfinished draft. |
| `--non-interactive` | Never prompt. Supply all required inputs as flags. |

Create the rule group first with `trailguide library add group`; rule creation does not create missing rule groups. Without `--body-file`, complete the draft and remove its `trailguide:draft` marker before validation. Existing rules are not overwritten. After the first library release, the next steps include adding the new rule's change note with `trailguide library change`. Like `trailguide library check`, it fails with instructions in a shallow clone, or a clone with change notes but no release tags, where it can't tell whether the library has a library release.

### library add skill

```sh
trailguide library add skill NAME [options]
```

Create a skill in `.trailguide/skills/<library.name>/NAME/`, the folder for the library's own skills. `NAME` follows the [skill name grammar](/reference/skill-format/#skill-ids), such as `fix-bug`; the skill's ID is `skills/NAME`. A new skill that the manifest's levels don't list is offered; to require or recommend it, add it to `library.skills`. See [Skill levels](/reference/library-schema/#skill-levels).

| Option | Meaning |
| --- | --- |
| `--directory PATH` | Directory to operate in. Defaults to your working directory; repository discovery applies. |
| `--description TEXT` | Required. What the skill does and when to use it, at most 1,024 characters. |
| `--type TYPE` | Optional. `playbook` or `review`, recorded as `trailguide-skill-type`. See [Skill types](/concepts/skill/#skill-types). |
| `--non-interactive` | Never prompt. Supply all required inputs as flags. |

The command writes `.trailguide/skills/<library.name>/NAME/SKILL.md` with the name, description, and type in its frontmatter, and a body with the `trailguide:draft` marker. Complete the draft and remove the marker before validation; see [Write a skill](/guides/write-skills/). An invalid name, or a description that's blank or too long, fails before prompts with exit code `2`. Existing skills are not overwritten, and the command refuses a name that another of the library's skills has in any origin folder.

After the first library release, the next steps include adding the new skill's change note with `trailguide library change`. Like `trailguide library check`, it fails with instructions in a shallow clone, or a clone with change notes but no release tags, where it can't tell whether the library has a library release.

#### Fork a skill into the library

```sh
trailguide library add skill --from SOURCE:SKILL [options]
```

Copy a skill from a source the library declares into `.trailguide/skills/<source>/`, the folder for forks of that source's skills, so the library publishes its own version of it. `SOURCE:SKILL` names a source in the library's `sources` and the skill's ID in its repository, such as `pstack:pstack/skills/show-me-your-work`. The fork keeps the skill's name, so leave out `NAME`.

| Option | Meaning |
| --- | --- |
| `--from SOURCE:SKILL` | Required for a fork. A skill in the source's repository at the revision its `ref` names now, or its newest library release when its `ref` is `latest-release`. |
| `--reason TEXT` | Required. Why the library publishes its own version, recorded with the fork. |
| `--directory PATH` | Directory to operate in. Defaults to your working directory; repository discovery applies. |
| `--non-interactive` | Never prompt. Supply all required inputs as flags. |

The command:

1. Copies the skill's folder from the source's repository, with every file's executable bit, to `.trailguide/skills/<source>/<name>/`, such as `.trailguide/skills/pstack/show-me-your-work/`, where `<name>` is the skill's name, or the name the source's `skills.rename` gives it. It reads the source as library check does, so it needs access to the repository.
2. Rewrites the fork's `trailguide-skills-used` so each skill it uses, including those the source's `skills.uses` adds for it, names the library's fork of it, such as `skills/tdd`, when the library has forked it, and otherwise names it as a skill the library [curates](/reference/configuration/#curated-skills), with the source's name, such as `pstack:skills/tdd`. Projects install a curated skill only when `library.skills` lists it, so a skill that uses one the library neither forked nor lists is refused: list that skill in `library.skills`, at a level or as a dependency, or fork it first. A skill that cites rules of a library source is forked as a draft for you to remove them from, because a library's skills cite only its own rules.
3. Adds a [`skills.forks`](/reference/configuration/#fork-a-skill) entry for the skill to the source, with `skills/<name>` as `as`, the full commit SHA it copied as `basedOn`, and your reason. The library repository is a project too, so the entry records where the fork came from the way a [project's fork](#fork-a-skill) does, and the library's own agents use the fork in place of the source's skill. The source needs no `skills.include` entry for the skill.
4. When `library.skills` lists the skill as one the library curates, such as `pstack:pstack/skills/show-me-your-work`, lists `skills/<name>` in its place, at the same level. Otherwise the fork is offered.

When it can't adapt the skill's `SKILL.md` for the library, because its frontmatter can't be read or edited, it cites rules of the source's library, or the fork would fail library check's [skill checks](/reference/skill-format/#validation), it still writes the fork and its configuration, marks the fork as a draft, and lists what to fix in `SKILL.md`. Library check fails, and projects can't install the fork, until you fix it and remove the `<!-- trailguide:draft -->` line. It refuses a fork whose folder exists, a skill the source already excludes or forks, a source the configuration doesn't declare, a skill the source's library requires, and a skill that uses one of the source's skills the library neither forked nor lists. The next steps say how to install the fork in the library repository: `trailguide project update SOURCE` for a source without a `ref`, since the fork copied its newest library release, and otherwise `trailguide project sync`.

When the source's `ref` later moves, [library check](#library-check) lists the source's changes to the skill since `basedOn`. The source's license still applies to the copied files; credit it in your library's notices, as [License rules](/guides/license-rules/#license-skills) describes.

When the library forks a skill that another skill it curates from the same source uses, projects that install the curated skill would install the source's copy of the forked skill beside the fork, under the same name. [Library check](#library-check) fails then, and asks you to fork the skill that uses it too, so the fork uses your fork.

### library change

```sh
trailguide library change ID... [options]
```

Write a new [change note](/reference/rule-versions/#change-notes) for one or more rules and skills. `ID` is a rule ID, such as `practices/testing/verify-retry-limits`, or a skill ID, such as `skills/build-feature`; name several to cover related changes in one note. Before the first library release, rules and skills need no notes, so the command fails and says so.

| Option | Meaning |
| --- | --- |
| `--bump LEVEL` | `major`, `minor`, or `patch`. Required for rules and skills that have a version. Not accepted for new or retired ones. |
| `--summary TEXT` | Required. One line describing the change for project maintainers. |
| `--retire` | Record that the rules or skills are retired; see [Retired rules](/reference/rule-versions/#retired-rules) and [Retired skills](/reference/rule-versions/#retired-skills). A rule's Markdown file and asset directory, or a skill's folder, must already be gone. |
| `--replaced-by ID` | Optional with `--retire` and a single `ID`: the rule that replaces a retired rule, or the skill that replaces a retired skill. If it isn't a rule or skill in the library yet, the command warns; add it before the next library release. |
| `--non-interactive` | Never prompt. Supply all required inputs as flags. |

The command writes a new file in `.trailguide/changes/` with a unique name, such as `.trailguide/changes/2026-09-29-verify-retry-limits-7f3a9c.yaml`, and never edits or deletes existing notes. Rule IDs go in the note's `rules` map and skill IDs in its `skills` map. A rule or skill without any version is recorded as `new`. It rejects an ID that isn't a rule or skill in the library, unless `--retire` is supplied for one that has a version. It also rejects a rule that has a version but whose Markdown file and asset directory are unchanged since the latest library release, and a skill whose folder's tracked files and their modes are unchanged: edit it first, then record the change. A rule or skill that was never published can't be retired: delete it, and remove it from any pending note.

`--replaced-by` must be the same kind as the retired ID: a rule for a rule, a skill for a skill. Naming a rule to replace a skill, or the reverse, fails as a usage error with exit code `2`, such as `--replaced-by practices/testing/verify-retries is a rule, but skills/triage-bug is a skill. A skill can only be replaced by a skill.`

See [Choose a version change](/reference/rule-versions/#choose-a-version-change) for picking `LEVEL` for a rule, and [Choose a skill's version change](/reference/rule-versions/#choose-a-skills-version-change) for a skill.

### library check

```sh
trailguide library check [options]
```

Validate the manifest, all rule groups and rules, all skills and their levels, change notes, supporting assets, and declared license and notice files. After the first library release, also check that every changed rule and skill has a change note. Does not change files.

| Option | Meaning |
| --- | --- |
| `--directory PATH` | Directory to operate in. Defaults to your working directory; repository discovery applies. |
| `--non-interactive` | Accepted; library check does not prompt. |

Reports rule group, rule, and skill counts, `value.groupCount`, `value.ruleCount`, and `value.skillCount` in JSON, such as `Library is valid: 3 group(s), 6 rule(s), 3 skill(s).`, and file-specific errors. The human output leaves out the skill count when the library has no skills. Empty rule groups are valid. Unfinished marked drafts fail. Undeclared licenses produce warnings; invalid declarations, missing declared files, and declared files inside a rule's version (a rule's Markdown file or its asset directory) fail validation. Library check validates the format, not writing quality or legal permissions. Use the [authoring rubric](/reference/rule-authoring/#authoring-rubric) to review guidance quality.

Check validates every skill folder and its `SKILL.md`: the folder's name and contents, the frontmatter fields, the `trailguide-skill-type`, `trailguide-rules-used`, and `trailguide-skills-used` metadata keys, and links. It lists every skill problem it finds and fails with the code `invalid-skill`. It warns when a skill's `license` differs from the library's SPDX expression, and about a file in a skill folder that Git neither tracks nor ignores. [Validation](/reference/skill-format/#validation) lists each check and its message. Each skill's local links, and the links of the Markdown files they reach, must name files the repository has and Git doesn't ignore, as a project that installs the skill keeps them; check fails on the first link that doesn't, with its `reference-` code, the chain of links that reached it, and without reading the network. For a library that curates skills, check reads each curated skill's links in the repository it comes from.

Check also validates the manifest's skill levels. It fails when a level lists a skill the library doesn't have, a skill appears in two levels, or two listed skills, authored or curated, have the same name. To check curated skills, it reads each curated source at the `ref` the library's configuration names, so it needs access to those repositories; see [Name clashes](/reference/library-schema/#name-clashes). Levels are library-wide settings, so changing one needs no change note.

Check also compares each skill the library [forked](#fork-a-skill-into-the-library) from one of its sources, a `skills.forks` entry with `basedOn` in that source's configuration, with the source's copy at the revision its `ref` names now. For each one that differs, it lists the files that changed since `basedOn`, or says the source no longer has the skill, and the `basedOn` value to set once your fork has the changes you want; in JSON, `value.forkedSkills` lists each with its `source`, `id`, `skill`, `basedOn`, `commit`, `revision`, `files`, and `removed`. The listing doesn't fail check, and check never changes the fork. `trailguide library release --dry-run` shows the same list. This also needs access to the sources' repositories.

After the first library release, check compares each rule's [versioned content](/reference/rule-versions/#what-a-version-covers), meaning its Markdown file and its asset directory, and each skill's, meaning the tracked files in its folder and their modes, with the latest `release/<number>` tag in the current branch's history. Notes added since that tag are pending. Check fails when any of the following is true of a rule, and the same for a skill, with the message naming the skill:

- A rule changed and no pending note names it. The error names the rule and its latest version, and gives the `trailguide library change` command to run.
- A new rule has no pending note.
- A published rule was deleted without a pending note that retires it.
- A retired rule's `replacedBy` isn't a rule in the library, or a retired skill's `replacedBy` isn't a skill in it.
- A rule reuses the ID of a retired rule.
- A pending note names a rule that's unchanged since the latest library release, so the note is stale.
- A pending note names a rule that doesn't exist and isn't being retired.
- A note is invalid, such as an unknown change, a blank summary, a skill ID under `rules`, or neither `rules` nor `skills`.

Check warns when a note that a library release already published was edited, because the edit has no effect. It also warns when such a note was deleted, because notes are never deleted; restore it. When a pending note retires a rule that an earlier library release named as a retired rule's replacement, check warns about it: when the note names a replacement, that updates will point projects that still include the earlier rule on to it; otherwise, that they would be pointed at a retired rule, suggesting a replacement as `replacedBy`. A pending retirement whose `replacedBy` isn't a rule in the library fails.

A skill is compared with the newest library release whose release record has a `skills` field; when a later library release lacks it, because an older Trail Guide published it, check warns, as [Skills in the release record](/reference/rule-versions/#skills-in-the-release-record) describes.

This comparison needs the repository's history and tags. Check fails with instructions in a shallow clone, with `shallow-clone`, and in a clone whose commit has change notes but no `release/<number>` tags, such as one made with `git clone --no-tags`, with `missing-release-tags`; fetch the tags with `git fetch --tags`. Check never fetches, so it refuses such a clone; `trailguide library release` fetches the remote's release tags itself. In CI, check out with full history, such as `fetch-depth: 0`. Before the first library release, rules need no notes, and check validates everything else. A library outside a Git repository has no library releases.

When checks pass, the result previews the pending library release: each rule and skill, its change, and its current and next version, in one list with the rules first and then the skills, each sorted by ID. Each skill also shows its level, and a skill whose level changed, that became a fork or stopped being one, or whose fork moved to another `basedOn` commit, is listed with that change, and with `-` as its change when its files didn't change. When no rule or skill change is pending and no library-wide file changed, it says there is nothing to publish, as `trailguide library release` does, except before the first library release, which always publishes the library-wide files, even without rules, so check says that instead. When library-wide files, such as the manifest, rule group metadata, shared assets, or license files, changed since the latest library release, the preview lists them too, so you know a library release would publish them even when no rule changed. JSON output includes this preview in `value.pendingRelease`, with its `release` number, a `rules` list, and a `libraryFiles` list of the changed library-wide files, which is empty before the first library release. Each rule has its `id` and `change`; `from` and `to`, its version before and after the library release (`from` is absent for a new rule); for a retired rule, `lastVersion` and any `replacedBy` instead; and `summaries`, one per change note that names it, in note order, as the release record and `trailguide project update` name them. `value.pendingRelease.skills` lists pending skill changes in the same form, with each skill's `level`, its `previousLevel` when its level changed, and a `fork` with the `source`, `id`, `basedOn`, `previousBasedOn`, and whether the fork was `added`, when it's new or moved, or with `removed: true` when the skill stopped forking that source skill; a skill whose only change is its level or fork has no `change` or `summaries`. `trailguide library release` reports the rules and skills it publishes in `value.rules` and `value.skills` the same way.

### library release

```sh
trailguide library release [options]
```

Publish the pending change notes for rules and skills, and any changes to library-wide files, as a [library release](/reference/rule-versions/#library-releases). A library release that changes only skills publishes like any other. See [Version your rules](/guides/version-rules/#publish-a-library-release) for the workflow.

| Option | Meaning |
| --- | --- |
| `--directory PATH` | Directory to operate in. Defaults to your working directory; repository discovery applies. |
| `--dry-run` | Show exactly what would be published, including the release notes, without creating a tag or a GitHub Release page. |
| `--no-github-release` | Skip creating the GitHub Release page. |
| `--non-interactive` | Accepted; library release does not prompt. |

Before publishing, the command fetches from the branch's upstream remote. It refuses unless:

- you're on the remote's default branch,
- the branch matches the remote exactly, so the library release contains no unpushed commits and misses none, and
- `trailguide library check` passes.

It then computes each changed rule's and skill's next version from the pending notes, using the largest change when several notes name the same rule or skill. The first library release gives every rule and skill version `1.0.0`. The release record lists skills in its [`skills` field](/reference/rule-versions/#skills-in-the-release-record), and the release notes list skill changes in a section of their own. It creates the annotated `release/<number>` tag on the current commit, with the release notes and [release record](/reference/rule-versions/#release-record) as its message, and pushes it. No files change and nothing is committed. Finally, it creates the GitHub Release page.

`--dry-run` shows the repository, branch, commit, release number, each rule's and skill's change and versions, and the complete release notes.

If a run stops after pushing the tag, such as when the GitHub Release page can't be created, run it again: it finds the tag on the current commit and creates what's missing. A run that stopped before pushing leaves its tag only in your clone; the next run checks it against the library release the commit publishes after the remote's latest one, pushes it only when its release notes and record match exactly, and otherwise refuses with `release-tag-mismatch` and asks you to delete it. A tag that a later Trail Guide created in a newer [release record](/reference/rule-versions/#release-record) format instead fails with `unsupported-release-record`: upgrade Trail Guide and run it again rather than deleting the tag. The result reports separately whether the tag and the GitHub Release page were created. JSON output gives them as `value.tagCreated` and `value.githubRelease.created`, and describes the library release in `value.release`, `value.rules`, `value.skills`, `value.libraryFiles`, and `value.notes`. Like other optional values that aren't lists, `value.githubRelease` is left out when the library release has no GitHub Release page: on a dry run, with `trailguide library release --no-github-release`, or for a repository that isn't on GitHub.com. When the tag is published but the page can't be created, the command fails with `github-release-failed`, and the JSON response still includes `value`, with `tagCreated` and without `value.githubRelease`, so a script can tell the tag is published and only the page is missing. With no pending notes and no library-wide changes, the command reports that there is nothing to publish, with `value.release` set to `0`, `value.rules`, `value.skills`, and `value.libraryFiles` empty, and `value.tag` and `value.notes` left out, as optional values that don't apply always are, and succeeds. A tool such as `jq` reads a left-out value as `null`.

The tag uses Git's configured identity as its tagger, including the `GIT_COMMITTER_*` environment variables.

When the push fails, the command fails with `push-failed` and deletes the tag it created, so a rerun starts over. Trail Guide doesn't show what Git or the server printed, as [Text from Git, servers, and the GitHub CLI](#text-from-git-servers-and-the-github-cli) explains; its message names the cause it recognizes in that text instead:

- a repository rule or tag protection, such as a GitHub ruleset or a GitLab protected tag, with GitHub's error code when the server gave `GH006` or `GH013`,
- the HTTPS server's certificate or the SSH host key couldn't be verified,
- Git couldn't connect to the server,
- the server denied access, or authentication failed,
- a hook on the server, such as a pre-receive or update hook, declined the tag,
- the server refused the tag for a reason Trail Guide doesn't recognize, with GitHub's error code when the server gave another one, such as `GH001` for large files, or
- the push failed before the server answered, such as when your pre-push hook stopped it.

To read the server's own message, push a test tag that your tag rules cover yourself, such as `release/0`, which isn't a library release tag:

```sh
git tag release/0
git push origin refs/tags/release/0
git tag --delete release/0
```

If that push succeeds, delete the test tag from the remote with `git push origin --delete refs/tags/release/0`.

`trailguide library release` refuses before creating anything, with exit code `1` and `error.code` in JSON output, when:

| Code | Refusal |
| --- | --- |
| `not-a-repository` | The library isn't the root of a Git repository. |
| `shallow-clone` | The clone has partial history. Fetch it with `git fetch --unshallow --tags`, or check out with `fetch-depth: 0` in CI. |
| `missing-release-tags` | The commit has change notes, but neither the clone nor the remote has `release/<number>` tags. The command fetches the remote's release tags before checking, so it doesn't refuse a clone made with `git clone --no-tags`; that fetch adds them to the clone, also on a dry run. |
| `detached-head` | `HEAD` isn't on a branch. |
| `no-upstream` | The branch has no upstream branch on a remote. |
| `push-destination` | The remote pushes to several URLs, or to another repository than it fetches from. |
| `fetch-failed` | Git couldn't read or fetch from the remote. The message says whether the HTTPS server's certificate or the SSH host key couldn't be verified, Git couldn't connect to the server, or the server denied access or has no such repository, when it recognizes one of them; run `git fetch` with the remote's name to read Git's message. |
| `not-default-branch` | The branch doesn't track the remote's default branch, or the remote reports none. |
| `no-commits` | The library has no commits yet. |
| `branch-differs` | The branch has commits the remote lacks, the remote has commits the branch lacks, or the remote branch doesn't exist. |
| `remote-changed` | Someone pushed to the branch, or changed a release tag, while the command ran. |
| `release-tag-mismatch` | A release tag in the clone differs from the remote's, exists only in the clone without being the next one on this commit, or doesn't publish what this commit would. |
| `invalid-release-tag` | A release tag reachable from the commit isn't a valid annotated tag with a release record. |
| `unsupported-release-record` | A release tag uses a newer release record schema version. Upgrade Trail Guide. |
| `uncommitted-changes` | Library files, files a skill links to, or a skill file's executable bit differ from the checked-out commit. Projects keep the files a skill links to from the published commit, so it must hold them as check read them. |
| `change-notes` | Change notes don't match the rule or skill changes, as `trailguide library check` reports; other check failures keep their own codes, such as `invalid-skill`. |
| `version-limit` | A change would take a rule's or skill's version past the largest version number. |
| `git-identity` | Git doesn't know your name and email for the tag's tagger. |
| `github-cli-missing` | A GitHub Release page applies, but `gh` isn't installed. |
| `github-cli-signed-out` | A GitHub Release page applies, but `gh` isn't signed in to GitHub.com. |
| `release-too-large` | The release tag would exceed 8 MiB. |
| `limit-exceeded` | The clone has more than 20,000 release tags, or `gh` printed more than its output limit. |
| `changed-input` | Library files changed while the command read them. |

After it creates the tag, it can fail with:

| Code | Failure |
| --- | --- |
| `tag-failed` | Git couldn't create the tag, such as when tag signing fails. |
| `push-failed` | The push failed, including when the remote refused it, with the cause described above. The command deletes the tag it created. |
| `release-conflict` | Someone else published the same `release/<number>` first. |
| `github-cli-failed` | `gh` couldn't run. |
| `github-release-failed` | `gh` couldn't look up or create the GitHub Release page. The message names the cause it recognizes: GitHub's rate limit, `gh` signed out or its credentials refused, permission denied, or a repository GitHub can't find. Otherwise, when the lookup failed, it says to run `gh release view` to read `gh`'s message; when creating the page failed, it suggests checks that change nothing, `gh auth status` and `gh repo view`, and running the command again, which creates the page with its generated notes only if it's still missing. The tag is published; run the command again to create the page. |

**GitHub Release pages** are created with the [GitHub CLI](https://cli.github.com/), `gh`, for repositories on GitHub.com. Each library release gets one GitHub Release page on its `release/<number>` tag, with the release notes as its body. Before changing anything, `trailguide library release` checks that `gh` is installed and signed in, and refuses if it isn't, unless you pass `--no-github-release`. `trailguide library release --dry-run` checks it too, and refuses the same way. Repositories hosted elsewhere get tags only. The page's URL in the result is built from the repository and the tag, never taken from what `gh` printed.

<span id="help-and-version"></span>

## Help, version, and license

```sh
trailguide --help
trailguide COMMAND --help
trailguide --version
trailguide --license
```

Use `-h` or `--help` to inspect command syntax and options. For example, run `trailguide project --help` to find project commands or `trailguide library add rule --help` for rule authoring options.

| Option | Meaning |
| --- | --- |
| `-h`, `--help` | Show help for the selected command. |
| `-v`, `--version` | At the root, print the executable's version. |
| `--license` | At the root, print the full embedded MIT license and copyright notice. |
| `--json` | Return help, version, or license text in `value.text` inside a JSON response. |

Running `trailguide` or a command group such as `trailguide library` without a subcommand displays help. The `trailguide project`, `trailguide project add`, `trailguide project list`, `trailguide library`, and `trailguide library add` groups organize commands; they do not perform operations themselves. Help never prompts or writes files.

The root `--version` flag prints the tool version. To choose which versions of a library's rules and skills to take, see [Choose versions](/reference/configuration/#choose-versions).

## Working directories

In Git repositories, run `trailguide project init` or `trailguide library init` from the repository root. Initialization from a subdirectory fails without writing files. This also applies to the target of `trailguide library init --directory`.

Other commands find the nearest ancestor containing a `.git` directory or file, including worktrees and submodules. Project and library commands both use that root's `.trailguide/config.yaml`; library commands also need its `library` section. They stop at that repository boundary, even if its configuration is missing. A nested `.trailguide/` does not override the root's configuration.

Outside Git, commands use the current directory, or the directory selected by `--directory` for library commands. They do not search parent directories for configuration. Custom project configuration locations are not supported.

Relative input paths such as `--body-file`, `--license-file`, and `--notice-file` resolve from the directory where you run the command. Paths inside project configuration remain relative to `.trailguide/`.

## Shared options and prompts

Every command accepts `--json`, which prints one JSON response and disables prompts. Every command also accepts `-h` / `--help`.

Authoring commands accept `--non-interactive`. Without it, commands can prompt for missing required metadata or source selections when running in a terminal. With `--non-interactive`, `--json`, or no terminal, missing required inputs cause an error. Positional arguments such as `ID` and `ALIAS` must always be supplied.

Invalid interactive answers repeat the same question while retaining earlier answers; for a choice, such as `[yes/no]`, the error lists the valid answers. Without a terminal, or with `--json` or `--non-interactive`, a missing required input fails with exit code `2` and names the flag to pass, such as `--summary`. Explicit flags are validated without prompting for replacement values. Existing rule groups, rules, skills, and source names fail before prompts.

Both init commands, both check commands, `trailguide project list skills`, `trailguide project list rules`, and `trailguide library release` run without prompts. Of these, `trailguide library check` and `trailguide library release` accept `--non-interactive`; `trailguide project check`, `trailguide project sync`, `trailguide project update`, `trailguide project build`, `trailguide project list skills`, and `trailguide project list rules` do not need or accept it.

String options accept one value and cannot be repeated, except `--rules`, `--skills`, `--skill-targets`, `--keep`, `--exclude`, `--update-fork`, and `--merge-fork`, which accept repeated values. For a value beginning with `-`, use the equals form, such as `--description='-prefixed text'`.

Scaffolding commands validate paths and detect collisions before writing. They preserve existing authored files and roll back failed creation attempts. They do not publish content or convert arbitrary third-party material. For the authoring workflow, see [Set up your first project](/start-here/set-up-project/) or [Create your first library](/start-here/create-library/).

## Command output

Human-readable output is the default. Add `--json` for scripts:

```sh
trailguide project check --json
```

JSON mode writes one response to stdout:

| Field | Meaning |
| --- | --- |
| `ok` | `true` for success; `false` for failure or an out-of-date project check. |
| `value` | The command's result, when available. An out-of-date check still includes its report here. |
| `error` | On failure, an object with `kind` (`usage`, `validation`, `operation`, `cancelled`, or `out-of-date` for a stale project check) and `message`, plus `location` when available. Domain failures include a stable `code`, such as `needs-init`, `missing-group`, `guide-edited`, or `agents-section-edited`, and every `usage` error has the code `invalid-arguments`. A failure about a skill's link, with a code that starts with `reference-`, also has a `reference` object with the `source`, the `revision`, the `file` with the link, the link's `destination`, its `target` path, and the `chain` of files whose links reached it. `source`, `revision`, and `target` are left out when they don't apply. |
| `warnings` | Present only when there is a warning about how the command found the project, such as a project that still keeps its files in `.code-rules/`; see [Upgrade from Code Rules](/start-here/install/#upgrade-from-code-rules). It accompanies success and failure alike. Human output prints these warnings to stderr. |

Every list in a value is always present, empty when there's nothing to report, including `warnings`. Enumerated values, such as statuses, kinds, changes, and codes, are kebab-case. Only optional values that aren't lists, such as a `ref` or a `githubRelease` object, are left out when they don't apply.

Project check returns `value.status` as `up-to-date` or `out-of-date`, and a `value.problems` list. Each problem has a `kind` (`missing-file`, `stale-contents`, `unexpected-file`, `outdated-readme`, or `outdated-agents-section`), `path`, `message`, and `nextSteps`, a list of repair steps in the same form as authoring results use. Paths are relative to the Trail Guide directory, except paths in a skill target, which are relative to the project root, so the `AGENTS.md` problem's path is `../AGENTS.md`. Generated guidance, installed skills, the managed Trail Guide guide, and the `AGENTS.md` section must all be current for success.

Authoring results include `value.nextSteps`, an ordered list of steps, each an `instruction` and its copyable `commands`. Human output shows those steps after initialization and rule or rule group creation.

Sync, update, and build report `added`, `changed`, and `removed` file lists, with paths relative to the Trail Guide directory, or to `_managed/generated/` for build, except paths in a skill target, such as `.claude/skills/build-feature/SKILL.md`, which are relative to the project root. Authoring commands, such as init, add source, add group, add rule, add skill, and library change, report the absolute paths of the files they created in `added` and of those they modified, such as `config.yaml`, in `changed`. Their human output lists the same files under `Added:` and `Changed:`, relative to the working directory when they're inside it. Update also reports each source's rule changes in `value.sources`. Sync and update list their warnings, such as for an unreleased `ref`, an entry naming a retired rule, a rename, a personal skill with the same name, or authored rule group metadata they wrote, in `value.warnings`, which is empty when there are none. Sync, update, and build also list each installed skill whose links reach a skill the project excludes in `value.excludedSkillLinks`, with its `skill` and the `excluded` skill's project IDs, the `chain` of files, by path in the repository that supplied them, from the installed skill's file to the excluded skill's, and `uses`, which is `true` when the skill also calls the excluded skill; `value.warnings` has the matching warning. When they refresh the managed Trail Guide guide, `value.guide` reports its path relative to the Trail Guide directory and whether it was `created`. When they change the Trail Guide section of `AGENTS.md`, `value.agents` reports the file's path relative to the repository root and an `action`: `created` for a new file, `added` for a section added to an existing file, or `updated`. Init lists `AGENTS.md` among its `added` or `changed` files instead. Help, version, and license return their text in `value.text`.

In human mode, operational errors go to stderr. An out-of-date check prints its status, problems, and next steps on stdout. Human output, prompts, and errors show control characters, which could make a terminal clear or rewrite what it displays, as visible escapes: ESC as `\x1b`, other controls from `\x00` to `\x1f` and `\x7f` the same way, C1 controls as `\u0080` to `\u009f`, and bytes that aren't valid UTF-8 as `\xNN`. Libraries supply much of that text, such as change summaries and rule IDs. JSON output encodes text as JSON strings instead. In JSON mode, errors go in the response; stderr is reserved for failures writing that response. Unreleased preview builds also print a non-production warning with their source commit to stderr before every command, including help, version, and JSON commands. JSON output on stdout is unchanged. See [Test a PR build](https://github.com/fabricahq/trailguide/blob/main/CONTRIBUTING.md#test-a-pr-build) in the contributing guide.

## Text from Git, servers, and the GitHub CLI

Trail Guide never shows text that Git, a Git server, or the GitHub CLI, `gh`, produced: not in human output, JSON output, or errors. That includes Git's error messages, the lines a server or its hooks print, the reason in Git's `! [remote rejected]` line, the default branch a server advertises, and everything `gh` prints. Such text can hold anything, including credentials Trail Guide can't know about, such as one a Git credential helper, `GIT_ASKPASS`, or `http.extraHeader` supplies, so Trail Guide doesn't try to find and remove them. It reads the text privately to choose one of its own fixed messages and error codes, which name the likely cause and what to check. A message may name a GitHub error code it recognizes, such as `GH013`, because those are a fixed set. To read the original text, run the Git or `gh` command yourself; the message or this reference says which.

Trail Guide also never shows credentials in a URL it displays. A repository address it shows, such as a library's remote in `trailguide library release`, has any password, query, and fragment removed, and its user name too unless it's an SSH user name, which names an account. An address it can't parse is replaced by a note, including one with an authority but no scheme, such as `//host/rules`, and a `user@host:path` address with a colon before its `@`, which could start a password. The GitHub Release page URL is built from the repository and the tag, not taken from `gh`.

Trail Guide shows some values that Git computes, after checking their form: commit and object IDs, which must be hexadecimal object IDs; commit counts, which must be nonnegative integers; and object types, which must be one of Git's four. A value in any other form fails with code `git-failed` and a fixed message instead.

Trail Guide shows as they are:

- names you chose locally, such as branch and remote names,
- the repository addresses in project configuration, which can't contain credentials, and
- library content, such as rule files, change notes, rule group descriptions, and release records.

Human output still shows control characters in that text as visible escapes, as [Command output](#command-output) describes.

## Exit codes

| Code | Meaning |
| --- | --- |
| `0` | Success. |
| `1` | An operation failed, inputs are invalid, or a project check found stale output. |
| `2` | Invalid command usage, such as an unknown command, unsupported option, missing required CLI input, or options the command can't combine, such as `trailguide library change --bump` for a new rule, or a `trailguide project update` decision the preview doesn't offer, such as `--keep` for a rule the update doesn't move. |
| `130` | The command was interrupted, such as with Ctrl-C, including at a prompt. It says it was cancelled; a command interrupted before writing files writes none, and one interrupted while replacing files is recovered by the next command, as [Sync and recovery](/reference/sync/) describes. JSON output reports `error.kind` as `cancelled`. |

Usage errors point to the relevant help page. Operational errors identify the affected file or rule when available and describe the problem. A failure of `trailguide project sync` or `trailguide project update` that wrote nothing, including an update's refusal of its arguments after the command line is parsed, ends with `No files were written.`, or, when the command first recovered an interrupted earlier command, which restores or finishes that command's files, says so instead, as an update's preview and a declined or interrupted update do too, and names the source, as in `source team:`, when the problem doesn't already. An interrupted one says `cancelled; no files were written`, or, after such a recovery, that it was cancelled and what the recovery did. For repair commands and interrupted updates, see [Sync and recovery](/reference/sync/).

## Migrating from earlier commands

Only the `project` and `library` command trees are supported. Earlier command paths now fail with an unknown-command error.
Update existing scripts to use the scoped commands below. Use `-h` or `--help` for help; there is no `help` subcommand.

| Earlier command | Canonical command |
| --- | --- |
| `trailguide init` | `trailguide project init` |
| `trailguide add source` | `trailguide project add source` |
| `trailguide local add group` | `trailguide project add group` |
| `trailguide local add rule` | `trailguide project add rule` |
| `trailguide sync` | `trailguide project sync` |
| `trailguide build` | `trailguide project build` |
| `trailguide check` | `trailguide project check` |

`trailguide project add library` and `trailguide project add repository` are replaced by `trailguide project add source`, which adds either kind of source. Library command paths are unchanged. For configuration fields that changed, see [No backward compatibility](/reference/configuration/#no-backward-compatibility).
