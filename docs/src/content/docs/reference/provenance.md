---
title: "Provenance"
description: "Where to find a rule's or skill's source, version, level, replacement reason, and declared license."
---

**Provenance** is where your project's rules and skills came from. Trail Guide records it in three places in the **Trail Guide directory**, `.trailguide/`: the lockfile, `lock.json`, records what the project took from each source; `config.yaml` records what you asked for, with your pins, exclusions, replacements, and their reasons; and the generated pages your agents read show each rule's and skill's source beside its guidance.

Read them when you want to understand why a rule or skill is present, investigate a library update, or find out why your project replaced a rule or skill. To find and follow the rules themselves, agents start with [the generated rule index](/reference/files/#where-agents-start).

This page shows where the records live, how Trail Guide creates them, and how to trace a rule or skill back to its source.

## Where to look

| File | What it tells you |
| --- | --- |
| `lock.json` | Which revision, rule and skill versions, and original files each source's vendor snapshot holds: the project's **lockfile**. See [Inspect the lockfile](#inspect-the-lockfile). |
| `config.yaml` | Which sources, rules, and skills you include, and your pins, exclusions, replacements, renames, and the reason for each. See [Configuration](/reference/configuration/). |
| `generated/rules/<source-name>/…` and `generated/rules/local/…` | Each active rule's ID, version, the rule it replaces and why, a link to its original file, and its declared license and credits. |
| `generated/SKILLS.md` | Each installed skill's source, release and commit, level and the library that set it, why it's installed, the skill it replaces and why, and where it's installed. |
| `generated/libraries/<source-name>/README.md` | A readable summary of the library release or commit taken, rule versions and pins, skills, and declared license terms. |
| `generated/installed-skills.json` | Which skill folders Trail Guide installed in the target folders. See [Installed skills](/reference/files/#installed-skills). |

These paths are relative to the Trail Guide directory. The **source name** is the name you gave a source in your configuration, such as `team`.

Trail Guide writes the lockfile and the generated files. To change what they describe, edit your configuration or authored rules and skills and run the appropriate [sync or build command](/reference/sync/).

## How the records are created

1. **You include rules and skills.** Configuration records the sources, the rules and skills you include from each, pins, exclusions, replacements, and renames.
2. **Sync and update record what they take.** `trailguide project update` moves each source along its `ref` and writes its entry in `lock.json` with the exact Git commit, rule versions, skill levels, and file digests. `trailguide project sync` restores exactly what that entry says, and never moves a source.
3. **Generation shows the result.** Sync, update, or build combines the vendored and authored rules and skills, then writes the guidance your agents read, with each rule's and skill's source on its page.

An **active rule** is one included in that generated guidance. An excluded rule has no page. A replacement's page also names the rule it replaced.

These files describe the current result, rather than a running history of every update. They contain no changing timestamps or machine-specific paths.

Trail Guide 0.6 and earlier also wrote `generated/provenance.json`, a report that repeated these facts. Trail Guide no longer writes it, and the next sync or build removes it.

## Trace a rule to its source

Open the rule's page in `generated/rules/`. A rule from a source is at `generated/rules/<source-name>/<rule-path>.md`, and its project ID starts with its source name, such as `team:practices/testing/check-retries`. An authored rule is under `generated/rules/local/`, and its page shows its ID with the `local:` prefix, such as `local:practices/testing/service-retries`; in configuration, such as an exclusion's `replacedBy`, you write it without the prefix.

Below its title, the page shows:

| Line | What it tells you |
| --- | --- |
| `Rule ID` | The rule's project ID. |
| `Version` | The rule's version, such as `1.3.0`, for a rule from a source that's a published version. The rule's entry in `lock.json` also records the library release that published it. A rule with unreleased changes, and an authored rule, has no version. |
| `Replaces` and `Reason` | For an authored rule that replaces a library rule, the replaced rule's ID, the library version your rule is based on, as the exclusion's [`basedOn`](/reference/configuration/#exclude-or-replace-a-rule) records it, and your configured reason. |

Under **Source and attribution**, the page links to the original rule: for a rule from a source, the file at the exact commit that supplied it, and for an authored rule, its file in `.trailguide/rules/`. The same section lists the declared license and source credits, explained [below](#find-declared-licenses-and-source-credits).

### Example: explain a replacement

Suppose your project includes rules from the `team` library but replaces its `check-retries` rule with `.trailguide/rules/practices/testing/service-retries.md`. You record the reason in configuration: “Use the retry limits required by this service.”

The replacement's page, `generated/rules/local/practices/testing/service-retries.md`, starts like this:

```md
# Use the service's retry limits

Rule ID: `local:practices/testing/service-retries`

Replaces: `team:practices/testing/check-retries`, based on version 2.0.0.

Reason: Use the retry limits required by this service.
```

Read this as: agents receive your `service-retries` rule, which replaces `team`'s `check-retries` rule, it's based on the library's version 2.0.0, and the reason comes from your project configuration. The version your lock records for `check-retries` is in its entry's `rules` in `lock.json`, and may be newer than 2.0.0.

## Trace a skill to its source

`generated/SKILLS.md` has one section per installed skill, named by its install name:

```md
### tdd

Write the test first.

- **Skill:** `fabrica:skills/tdd`, from release/7, commit `4f1c2a95…`
- **Level:** recommended by `fabrica`, release 7
- **Installed because:** included, and used by `build-feature`
- **Installed at:** [.agents/skills/tdd/SKILL.md](…), [.claude/skills/tdd/SKILL.md](…)
```

| Line | What it tells you |
| --- | --- |
| `Skill` | The skill's project ID and where it came from: the library release and commit, the commit of a non-library source, the curating library and the original repository for a [curated skill](/reference/configuration/#curated-skills), or "authored in this project". |
| `Renamed from` | The skill's own `name` when your configuration [renames](/reference/configuration/#rename-a-skill) it. |
| `Replaces` and `Reason` | For a [fork](/reference/configuration/#fork-a-skill), the skill it replaces, the commit your copy is based on, and your configured reason. |
| `Level` | The skill's [level](/reference/configuration/#skill-levels), the library that set it, and the library release whose manifest set it. A [dependency](/reference/configuration/#dependencies) installs only with the skills that use it. Authored skills and skills from non-library sources have no level. |
| `Type` | Its `trailguide-skill-type`, `playbook` or `review`. |
| `Applies` | The rules its `trailguide-rules-used` list names. |
| `Uses` and `Installed because` | The skills it uses, and whether your configuration or a level includes it, or other skills use it. |
| `Installed at` | The folders it's installed in, as in `generated/installed-skills.json`. |

The skill's entry in `lock.json` records the same level and why it's installed; see [Skills in the lockfile](#skills-in-the-lockfile).

## Inspect the lockfile

`lock.json` in the Trail Guide directory is the project's **lockfile**. It has one entry per source, keyed by source name, that records exactly what the project took from that source: the revision, the version of every rule, every skill, and a digest of every file in the source's **vendor snapshot**, `vendor/<source-name>/`. `trailguide project sync` restores exactly what the lockfile records, and never moves a source. Only `trailguide project update`, a new source, or a changed repository changes what an entry records.

A library's snapshot mirrors its Trail Guide directory, so its files keep their paths relative to the library's `.trailguide/`, such as `rules/practices/testing/verify-retry-limits.md` and `config.yaml` for the manifest. Declared license and notice files outside that directory, such as `LICENSE.md` at the repository root, keep their repository paths. A non-library source's files keep their repository paths.

```json
{
  "lockVersion": 1,
  "notice": "Written by trailguide project sync and update. Do not edit by hand.",
  "sources": {
    "fabrica": {
      "availableSkills": {},
      "commit": "9e07b3d6f0c1a4b85e2d7c3f9a61b04e8d52c7aa",
      "executable": [],
      "files": {
        "config.yaml": "a90e…13f2",
        "rules/practices/testing/verify-retry-limits.md": "4c1f…e9a2",
        "…": "…"
      },
      "groups": ["practices/testing", "techs/go"],
      "integrity": "sha256-FR7BXXdLEY6ygFjyuH+3a/k9/NCQRw+Bhh4OAobLc64=",
      "kind": "library",
      "ref": "latest-release",
      "release": 3,
      "releaseCommits": {
        "2": "4f1c2a95…",
        "3": "9e07b3d6f0c1a4b85e2d7c3f9a61b04e8d52c7aa"
      },
      "repository": "https://github.com/fabricahq/public-library.git",
      "retiredRules": ["practices/testing/check-retry-backoff"],
      "rules": {
        "practices/testing/verify-backoff": { "release": 2, "version": "1.3.0" },
        "practices/testing/verify-retry-limits": { "release": 3, "version": "2.0.0" },
        "techs/go/wrap-errors-with-operation": { "release": 3, "version": "1.1.0" }
      },
      "selectors": {
        "groups": ["practices/testing"],
        "rules": ["techs/go/wrap-errors-with-operation"],
        "skills": []
      },
      "skills": {}
    }
  }
}
```

| Field | Meaning |
| --- | --- |
| `lockVersion` | The lockfile format, `1`. A newer format, written by a later Trail Guide, fails with `unsupported-lock` and asks you to upgrade Trail Guide, since an older one would rewrite the project in its own format. |
| `notice` | A reminder that Trail Guide writes the file. |
| `sources` | One entry per configured source, keyed by its name in configuration. |

Each source's entry has these fields:

| Field | Meaning |
| --- | --- |
| `kind` | `library` when the source's repository has a [manifest](/reference/library-format/#the-manifest) at the locked commit, and `non-library` otherwise. Trail Guide detects it; configuration never declares it. |
| `repository` | The source's repository address. |
| `ref` | The source's `ref` from configuration when the entry was written, such as `release/3`, `latest-release`, or `main`. Changing it to another spelling of the same revision, such as `refs/tags/release/3` for `release/3`, keeps the locked spelling, so the entry stays current. Changing it to another library release makes the entry out of date until `trailguide project update SOURCE` takes that release, and offline checks report it at `lock.json.sources.<source-name>.ref`; changing it to a branch, tag, or commit makes the entry out of date until `trailguide project sync` takes that revision. Pins and exclusions aren't locked: configuration owns them, and a pinned rule's `version` in `rules` is the pinned version. The library's README in `generated/libraries/` shows the configured pins and their reasons. |
| `release` | The library release that supplied the library-wide files: the manifest, rule group metadata, shared assets, and license files. Every skill from the source, and every skill level, comes from it too. It is the newest library release when the source was added or last updated, or the library release your `ref` names, and never older than the `release` of any locked rule. `trailguide project sync` keeps it, including when you include more rules or skills: those come from this library release too. It's omitted when the source isn't a library, or its `ref` is a branch, tag, or commit rather than a library release. |
| `commit` | The full Git commit SHA of that library release, or of the commit your `ref` resolved to. Every file in the snapshot that isn't a rule from an older release comes from it. |
| `releaseCommits` | For a library, each library release that a locked rule comes from, and `release`, mapped to the full commit SHA of its `release/<number>` tag. Offline checks require it to list exactly those releases. |
| `selectors` | Your configured selectors when the entry was written: `groups`, the rule groups your `rules.include` selects whole, such as `practices/testing` for `practices/testing/*`; `rules`, the rules it includes by ID; and `skills`, your `skills.include` selectors, as written. A non-library source has only `skills`. Offline checks compare them with configuration, so a changed selector makes the entry out of date until `trailguide project sync`. |
| `groups` | The rule groups whose rules the entry holds, with their metadata. A rule group the selectors reach only for some of its rules is listed too. |
| `rules` | Each locked rule's ID, its `version`, and the `release` that published it, whose commit `releaseCommits` lists. A rule records the newest version that `release` or an earlier library release published, unless a pin names another. `legacyLayout: true` marks a rule from a library release written before library format 2. |
| `retiredRules` | The IDs of the rules the library retired that the source would otherwise include, sorted, and empty when there are none. A retired rule the source still includes, such as one a pin keeps at its last version, is in both `rules` and `retiredRules`, and the generated library README marks it retired. It records what the library told sync, not your configuration, so offline checks can tell an exclusion of a retired rule from a typo. `trailguide project update` refreshes it; `trailguide project sync` adds a retired rule you pin or exclude that it doesn't list yet. |
| `skills` | Each installed skill, as [Skills in the lockfile](#skills-in-the-lockfile) describes. |
| `availableSkills` | Every skill the source offers at the locked revision, installed or not, with its `level`, so offline checks can validate skill selectors and `trailguide project list skills` can list them. |
| `files` | Each file in the source's snapshot and its SHA-256 digest, written as lowercase hexadecimal text. A rule's files come from the commit of its own `release`, because the snapshot holds one version of each rule. |
| `executable` | The snapshot paths whose executable bit is set, sorted. A file's mode is part of its content, so offline checks compare it as they compare digests. |
| `curated` | The sources a library curates skills from, as [Curated sources in the lockfile](#curated-sources-in-the-lockfile) describes. Omitted when the project installs no curated skill. |
| `integrity` | A checksum of the entry, which detects an entry edited by hand or combined by a merge. Anyone can recompute it, so it doesn't prove who wrote the entry; see [Integrity](#integrity). |

A rule's `version` and `release` are `null` when the vendored file isn't a published version, which can happen only when your `ref` names a revision other than a library release. Generated guidance shows no version for it.

For a prefix selector, the snapshot must contain every rule the prefix reaches in the locked revision.

### Integrity

`integrity` is `sha256-` followed by the base64 SHA-256 of the entry's canonical form: the JSON object `{"entry": …, "name": …}`, holding the entry without `integrity` and the source's name, serialized as [RFC 8785](https://www.rfc-editor.org/rfc/rfc8785) specifies. Formatting and key order therefore don't matter, but any change to a field, an added or removed field, or moving the entry to another source's name does.

Offline, `trailguide project build` and `trailguide project check` refuse an entry whose `integrity` doesn't match, or that has none, as changed outside Trail Guide, such as by hand or in a merge resolution, because its versions can't be trusted without the source. Sync, update, and a [fork](/reference/cli/#fork-a-library-rule) refuse it too and write nothing; no check of individual facts can establish that an edited entry is right. An entry for a source you removed from configuration is discarded with its snapshot, whatever it says, unless an authored rule needs the rule group metadata in that snapshot.

When an entry was changed outside Trail Guide, restore the one Trail Guide wrote, then sync:

```sh
git checkout -- .trailguide/lock.json
trailguide project sync
```

When the entry can't be restored, take the source again at its `ref`. This resolves the source as if you had just added it, so its unpinned rules move to the newest versions that `ref` offers:

```sh
trailguide project sync --retake fabrica
```

Strict parsing also refuses a key that appears twice in an object, and a field the format doesn't define. `integrity` is an unkeyed checksum: it detects accidental edits and merge results that combine both sides, but anyone can recompute it, so it can't detect a deliberate change to both an entry and its checksum, or authenticate files against the remote repository.

### Skills in the lockfile

A source that installs skills records each one, with its level, and why the project installs it:

```json
{
  "skills": {
    "skills/build-feature": {
      "included": true,
      "level": "recommended",
      "usedBy": []
    },
    "skills/tdd": {
      "included": false,
      "level": "offered",
      "usedBy": ["skills/build-feature"]
    }
  }
}
```

| Field | Meaning |
| --- | --- |
| `level` | The skill's level in the manifest at the entry's `release`, or `null` for a non-library source. |
| `included` | `true` when your selectors or a level include the skill, and `false` when it's installed only because another skill uses it. |
| `usedBy` | The IDs of the installed skills that use it, sorted, and empty for a skill that's only included. |

A library skill's files and version come from the entry's `release`. A non-library source's skills come from its `commit`, and have no versions or levels.

### Curated sources in the lockfile

When a library's manifest lists [curated skills](/reference/configuration/#curated-skills), the library's entry holds a `curated` object with one entry per curated source the project installs skills from. The files are stored under `vendor/<source-name>/curated/<curated-source>/`, at their paths in the curated repository:

```json
{
  "curated": {
    "mattpocock": {
      "commit": "c2e7a4019b5d3f86e1a0c7d94b2f58e3a6d1c0b9",
      "executable": [],
      "files": {
        "LICENSE": "77ab…0e41",
        "skills/engineering/to-spec/SKILL.md": "3d8f…a512",
        "…": "…"
      },
      "kind": "non-library",
      "ref": "main",
      "repository": "https://github.com/mattpocock/skills.git",
      "skills": {
        "skills/engineering/to-spec": {
          "included": true,
          "level": "recommended",
          "usedBy": []
        }
      }
    }
  }
}
```

`ref` is the ref the library's manifest names, from the library release your project locked, and `commit` the commit your project resolved it to at its last update; `level` is the level the library sets. A skill's `usedBy` names the skills of the curated repository that use it, by their IDs there, and `usedByLibrary`, present only when the library's own skills use it, names those, sorted, by their IDs in the library. A curated library has a `release` too. When your project declares the curated repository as its own source, the entry still records the library's curated skills, from your source's commit: `source` names your source, and `ref` and `commit` are your source's. The library entry's `integrity` covers `curated`.

### Resolve a merge conflict in the lockfile

When two branches each sync or update a source, merging them can leave conflict markers in `lock.json`. Trail Guide refuses the file with `lock-conflict` and names each conflicted source with the ref, release, and commit each side locked, such as `fabrica: ours ref release/3, release 3, commit 9e07…, theirs ref release/4, release 4, commit 1c5a…`.

Resolve each conflicted source by keeping one side's whole entry, never lines from both, since a combined entry fails its integrity check. Leave the other sources as Git merged them. Then sync, which applies the merged configuration:

```sh
trailguide project sync
```

When the side you kept is behind the other, such as the older release, take the newer one with `trailguide project update SOURCE`.

When you'd rather not choose a side, leave the conflict markers in place and retake each conflicted source. Sync keeps the entries both sides agree on, resolves each retaken source again at its `ref`, and writes `lock.json` without markers:

```sh
trailguide project sync --retake fabrica
```

This works only when both sides of the file are valid JSON and you retake every conflicted source; otherwise sync still fails with `lock-conflict`. In that case, restore valid JSON first, such as by taking one whole side with `git checkout --ours -- .trailguide/lock.json`, then sync, with `--retake` for any source whose entry you don't trust.

### Migrate from Trail Guide 0.5

Trail Guide 0.5 kept a record in each source's snapshot, `vendor/<source-name>/_source.json`, instead of `lock.json`. Offline, build and check refuse such a project with `lock-migration-needed`. Migrate it once, and commit the result:

```sh
trailguide project sync
```

Sync reads each record, checks it and the vendored files as 0.5 did, writes `lock.json` in their place, and removes the records. It moves no source. Migration refuses, writing nothing, when:

| Error | Meaning and next step |
| --- | --- |
| `record-missing` | A source's snapshot has files but no `_source.json`. Restore the record from Git, or run `trailguide project sync --retake SOURCE`. |
| `lock-migration-blocked` | `vendor/` holds a folder that belongs to no configured source. Add the source back to configuration, or delete the folder. |
| `mixed-layout` | Both `lock.json` and a `_source.json` record are present, usually because a merge combined a migrated branch with one that wasn't. Take one branch's whole `vendor/` folder and `lock.json`, then sync. |

A record changed outside Trail Guide 0.5 is refused as an edited lock entry is, and recovers the same way.

### When the lockfile is missing

When `lock.json` is missing but `vendor/` holds files a sync wrote, every command fails with `lock-missing`, because sync never resolves a source again on its own. Restore the file with `git checkout -- .trailguide/lock.json`, and sync. To take every source again at its `ref` instead, retake each one, such as `trailguide project sync --retake fabrica --retake tools`. The same applies when `lock.json` exists but doesn't read at all, such as a truncated file: every command fails, and only a restore, or a sync that retakes every configured source, recovers it. When configuration names no source, delete `.trailguide/vendor/` instead.

When `lock.json` exists but has no entry for a configured source whose vendor folder holds files, such as after a merge resolution dropped the entry, every command fails with `lock-entry-missing`. Sync doesn't treat the source as new, since that could move it. Restore the entry from Git and sync, or take the source again at its `ref`:

```sh
trailguide project sync --retake fabrica
```

### What offline checks can verify

`trailguide project build` and `trailguide project check` compare the locked selectors with configuration and compare stored files with their digests. Version checks depend on how the source chooses versions:

| Choice | What Trail Guide verifies offline |
| --- | --- |
| Library releases, the default | Each pinned rule records its pinned version. Other rules may record any version the recorded `release` or an earlier one published. `release` is at least as new as every rule's `release`. |
| One revision, with `ref` | For a commit SHA, `commit` equals it. For a library release `release/N`, the source records release N, and every rule records a published version from library release N or earlier. A branch's or tag's recorded commit is used without checking where it points now. |

For every source, offline checks first verify the entry's [`integrity`](#integrity), so an entry changed outside Trail Guide fails until you restore the entry Trail Guide wrote or take the source again. They also verify that `rules` and `skills` list exactly the rules and skills your selectors, the levels, and the skills' dependencies reach, that each skill's dependencies and cited rules match its recorded version's metadata, and that generated guidance and installed skills show the same versions and levels. Each exclusion must name a rule in `rules` or `retiredRules`; any other exclusion could be a typo that leaves the rule you meant active, so the check reports that it names no rule the library supplies and asks you to run `trailguide project sync`, which checks it against the library.

Offline checks cannot prove that a recorded version was the newest available, or that recorded versions match the library's release tags; `trailguide project update` checks the latter against the release records before it records anything. These records also cannot authenticate files against the remote repository if someone changed both the local files and their records.

## Find declared licenses and source credits

A library declares its license and notice files in its manifest; see [License metadata](/reference/library-format/#license-metadata). Trail Guide keeps byte-for-byte copies of them in `generated/libraries/<source-name>/licenses/`, with notice copies numbered, such as `licenses/notices/001.md`, and the library's README in `generated/libraries/<source-name>/` lists them with the declared license expression, such as `MIT`.

Each page of a rule from that library repeats the declared license and links to those copies, under **Source and attribution**. The original files stay in the source's vendor snapshot, `vendor/<source-name>/`. A rule's own source credits, from its [attribution](/reference/rule-format/#rule-attribution), are listed there too, as links.

An authored rule's page has no declared license, including a replacement: replacing a library rule doesn't give your replacement the library's license.

These records preserve declarations; they are not a legal verification status. A rule with no declared license isn't public domain, and has no permission to be redistributed.
