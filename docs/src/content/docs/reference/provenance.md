---
title: "Provenance"
description: "Where to find a rule's or skill's source, version, level, replacement reason, and declared license."
---

:::caution[Preview]
This page describes the records Trail Guide writes for version 2 configuration and branch refs, which aren't in a released version of Trail Guide yet.
:::

**Provenance** records where your project's rules and skills came from. The main file is `generated/provenance.json` in the **Trail Guide directory**, so its path from the project root is `.trailguide/generated/provenance.json`. It identifies the sources and versions your project uses, the source of each active rule and installed skill, the library that set each skill's level, and any replacements.

Read it when you want to understand why a rule or skill is present, investigate a library update, or find out why your project replaced a rule or skill. Tools can also read the JSON to report this information. To find and follow the rules themselves, agents start with [the generated rule index](/reference/files/#where-agents-start).

This page shows where the records live, how Trail Guide creates them, and how to trace a rule or skill back to its source.

:::note[Preview]
Skills, source record format 3, and the `kind`, `skillInclude`, `skills`, and `executable` fields, and the `upstream`, `replacementReason`, and `basedOn` fields of a skill, come in the next Trail Guide release.
:::

## Where to look

Trail Guide keeps these related records:

| File | What it tells you |
| --- | --- |
| `generated/provenance.json` | Where active rules and installed skills came from, which versions are in use, which library set each skill's level, and why rules and skills were replaced. |
| `vendor/<source-name>/_source.json` | Which commit and original files the vendor snapshot of one source holds: the source's **lock**. |
| `generated/libraries/<source-name>/README.md` | A readable summary of the library release or commit taken, rule and skill versions and levels, and declared license terms. |
| `generated/installed-skills.json` | Which skill folders Trail Guide installed in the target folders. See [Installed skills](/reference/files/#installed-skills). |

These paths are relative to the Trail Guide directory, `.trailguide/`. The **source name** is the name you gave a source in your configuration, such as `team`.

Trail Guide writes these files. To change the information they describe, edit your configuration or authored rules and skills and run the appropriate [sync or build command](/reference/sync/).

## How the records are created

1. **You include rules and skills.** Configuration records the sources, the rules and skills you include from each, pins, exclusions, replacements, and renames.
2. **Sync and update record what they take.** `trailguide project update` moves each source along its `ref` and writes its `_source.json` record with the exact Git commit, rule and skill versions, skill levels, and file checksums. `trailguide project sync` restores exactly what that record says, and never moves a source.
3. **Generation records the result.** Sync, update, or build combines the vendored and authored rules and skills, then writes `generated/provenance.json` alongside the guidance your agents read.

An **active rule** is one included in that generated guidance. An excluded rule has no active rule entry. A replacement has an entry that also identifies the rule it replaced. A rule taken only because an installed skill cites it has an entry marked `referenceOnly`. Skills work the same way, in their own list.

These files describe the current result, rather than a running history of every update. They contain no changing timestamps or machine-specific paths.

## Trace a rule to its source

Open `generated/provenance.json` and find the rule's ID in the `rules` array. A rule from a source has a project ID that starts with its source name, such as `team:practices/testing/check-retries`; an authored rule's ID has no source name.

Each rule entry includes:

| Field | What to look for |
| --- | --- |
| `id` and `group` | The active rule's identity and rule group. |
| `citedBy` | The project IDs of the installed skills whose `trailguide-rules-used` list names this rule, sorted, and empty when none does. |
| `referenceOnly` | `true` when the rule is taken only because installed skills cite it, so `RULES.md` and the rule group pages don't list it; otherwise `false`. |
| `origin` | The source and file supplying the active rule. A source's origin also identifies the repository and the exact commit the rule came from. `version` records the rule's version, such as `"1.3.0"`, and `release` the library release that published it; both are `null` when the vendored file isn't a published version. |
| `upstream` | The origin of the rule an authored rule replaces, including its `version`; otherwise `null`. |
| `replacementReason` | Your configured reason for the replacement; otherwise `null`. |
| `basedOn` | The library version of the replaced rule that the authored rule is based on, as the exclusion's [`basedOn`](/reference/configuration/#exclude-or-replace-a-rule) records it, such as the version a fork copied; otherwise `null`. It can differ from `upstream`'s `version`, which is the version the project's lock records. |
| `license`, `licenseBasis`, and `attribution` | Declared terms and source credits, explained below. |

An authored rule's origin has `source: null` and a `file` relative to the Trail Guide directory. Its `repository`, `resolvedCommit`, `version`, and `release` fields are `null` because the rule comes from your repository. A [fork](/reference/cli/#fork-a-library-rule) of a library rule records its source in `attribution` instead, when the library is on GitHub.com or GitLab.com or its repository address is an HTTPS URL.

### Example: explain a replacement

Suppose your project includes rules from the `team` library but replaces its `check-retries` rule with `.trailguide/rules/practices/testing/service-retries.md`. You record the reason in configuration: “Use the retry limits required by this service.”

The replacement's entry contains these fields. This excerpt omits the other origin and license fields:

```json
{
  "id": "practices/testing/service-retries",
  "group": "practices/testing",
  "origin": {
    "source": null,
    "file": "rules/practices/testing/service-retries.md"
  },
  "upstream": {
    "source": "team",
    "file": "rules/practices/testing/check-retries.md",
    "version": "2.1.0"
  },
  "replacementReason": "Use the retry limits required by this service.",
  "basedOn": "2.0.0"
}
```

Read this as: agents receive your `service-retries` rule, it replaces version 2.1.0 of `team`'s `check-retries` rule, it is based on the library's version 2.0.0, and the reason comes from your project configuration. The replaced rule's full origin also records its repository and exact commit.

## Trace a skill to its source

Installed skills are in the top-level `skills` array, one entry per skill the project installs. Skills have no rule group.

```json
{
  "id": "fabrica:skills/tdd",
  "name": "tdd",
  "renamedFrom": null,
  "type": "playbook",
  "origin": {
    "source": "fabrica",
    "curatedFrom": null,
    "repository": "https://github.com/fabricahq/public-library.git",
    "resolvedCommit": "4f1c2a95…",
    "folder": "skills/tdd",
    "version": "1.1.0",
    "release": 2
  },
  "level": "recommended",
  "recommendedBy": { "library": "fabrica", "release": 7 },
  "included": true,
  "usedBy": ["fabrica:skills/build-feature"],
  "uses": [],
  "rulesUsed": ["fabrica:practices/testing/test-behavior-not-implementation"],
  "targets": [".agents/skills/tdd", ".claude/skills/tdd"],
  "upstream": null,
  "replacementReason": null,
  "basedOn": null,
  "license": { "spdxExpression": "MIT", "…": "…" },
  "licenseBasis": "library"
}
```

| Field | What to look for |
| --- | --- |
| `id` and `name` | The skill's project ID and its install name, which names its installed folders. |
| `renamedFrom` | The skill's own `name` when your configuration [renames](/reference/configuration/#rename-a-skill) it; otherwise `null`. |
| `type` | Its `trailguide-skill-type`, `playbook` or `review`, or `null`. |
| `origin` | The source and folder supplying the installed skill, with the repository and exact commit. `version` and `release` record the skill's version and the library release that published it; both are `null` for an authored skill, a skill from a non-library source, or one with unreleased changes. An authored skill's origin has `source: null` and `null` for `repository` and `resolvedCommit`. For a [curated skill](/reference/configuration/#curated-skills), `source` is the library that listed it, `curatedFrom` the curated source's name in that library's manifest, and `repository` and `resolvedCommit` the original repository and the commit your project resolved; otherwise `curatedFrom` is `null`. |
| `level` | The skill's [level](/reference/configuration/#skill-levels), `required`, `recommended`, or `offered`, as the library that listed it sets it; `null` for an authored skill or one from a non-library source. |
| `requiredBy`, `recommendedBy`, or `offeredBy` | The library that set that level and the library release whose manifest set it, as `library` and `release`. Present when a level brought the skill in, including a curated skill; omitted when your project named the skill by ID in the source that holds it, or the skill is authored or from a non-library source. |
| `included` | `true` when your configuration or a level includes the skill; `false` when it's installed only because other skills use it. |
| `usedBy` | The project IDs of the installed skills that use this one, sorted, and empty when none does. |
| `uses` | The project IDs of the skills this one uses, from its `trailguide-skills-used` list and your configuration's `skills.uses`. When your project replaces one, the replacement's ID is listed. |
| `rulesUsed` | The project IDs of the rules its `trailguide-rules-used` list names. Each of those rules' entries in `rules` lists this skill in `citedBy`. |
| `targets` | The folders it's installed in, relative to the project root, as in `generated/installed-skills.json`. Empty when the project has no skill targets. |
| `upstream`, `replacementReason`, and `basedOn` | For an authored skill that replaces one from a source, the replaced skill's origin, your configured reason, and the version or, for a non-library source, the commit your skill is based on; otherwise `null`. |
| `license` and `licenseBasis` | Declared terms, explained [below](#find-declared-licenses-and-source-credits). |

## Inspect library versions and rule group guidance

Besides `rules` and `skills`, the same `generated/provenance.json` file contains four top-level fields:

| Field | What it records |
| --- | --- |
| `generatedNotice` | A reminder that Trail Guide generates the file, and how to regenerate it. |
| `toolVersion` | The Trail Guide version that generated the files. |
| `sources` | Each named source, its `kind`, `library` or `non-library`, its repository, its `ref` and `pins`, the library release or commit that supplied its rule group metadata and terms, its rule and skill `include` selectors, the rule groups and skills they reached, its curated sources, and declared terms. |
| `groups` | Each rule group's ID, descriptions and reading guidance, and which sources supply the effective guidance. |

For each source, `ruleInclude` and `skillInclude` record the selectors you wrote, while `groups` and `skills` list what they reached. For example, `"practices/*"` asks for every practice rule; `groups` records which practice groups existed in the library release taken.

Within each rule group record, `guidance` keeps the metadata labeled by source. `effectiveGuidanceSources` identifies which sources supply the guidance agents see. Authored rule group metadata takes precedence when present; otherwise the guidance from all contributing libraries remains effective.

## Inspect the vendor snapshot

Each source has a separate `vendor/<source-name>/_source.json` file. It describes the **vendor snapshot**: the original files the project took from the source, and which version of each rule and skill they belong to. It is the source's lock: `trailguide project sync` restores exactly what it records. The directory name identifies the source.

A library's snapshot mirrors its Trail Guide directory, so its files keep their paths relative to the library's `.trailguide/`, such as `rules/practices/testing/verify-retry-limits.md` and `config.yaml` for the manifest. Declared license and notice files outside that directory, such as `LICENSE.md` at the repository root, keep their repository paths. A non-library source's files keep their repository paths.

```json
{
  "formatVersion": 3,
  "kind": "library",
  "checksum": "5d1b…c07e",
  "kind": "library",
  "repository": "https://github.com/fabricahq/public-library.git",
  "release": 3,
  "resolvedCommit": "9e07b3d6f0c1a4b85e2d7c3f9a61b04e8d52c7aa",
  "ruleInclude": [
    "practices/testing/*",
    "techs/go/wrap-errors-with-operation"
  ],
  "skillInclude": [],
  "groups": [
    "practices/testing",
    "techs/go"
  ],
  "rules": {
    "practices/testing/verify-backoff": {
      "version": "1.3.0",
      "release": 2,
      "commit": "4f1c2a95…"
    },
    "practices/testing/verify-retry-limits": {
      "version": "2.0.0",
      "release": 3,
      "commit": "9e07b3d6…"
    },
    "techs/go/wrap-errors-with-operation": {
      "version": "1.1.0",
      "release": 3,
      "commit": "9e07b3d6…"
    }
  },
  "retiredRules": [
    "practices/testing/check-retry-backoff"
  ],
  "skills": {},
  "retiredSkills": [],
  "files": {
    "config.yaml": "a90e…13f2",
    "rules/practices/testing/verify-retry-limits.md": "4c1f…e9a2",
    "…": "…"
  },
  "executable": []
}
```

| Field | Meaning |
| --- | --- |
| `formatVersion` | The snapshot format version, `3`. A record in any other format, such as one written for version 1 configuration, whose file paths don't match this layout, fails with advice to delete `.trailguide/vendor/<source-name>/` and run `trailguide project sync`, which takes the source again as a newly added source: at the newest library release, or the commit your `ref` names now, except rules you pin. To keep what you had, pin rules or set `ref: release/<number>` first. A newer format, written by a later Trail Guide, fails with `unsupported-source-record` and asks you to install a newer Trail Guide instead, since syncing would rewrite the project in the older format. Unknown fields are always rejected. |
| `kind` | `library` when the source's repository has a [manifest](/reference/library-format/#the-manifest) at the recorded commit, and `non-library` otherwise. Trail Guide detects it; configuration never declares it. |
| `checksum` | The SHA-256 checksum of the record's other fields, as sync wrote them. Offline, `trailguide project build` and `trailguide project check` refuse a record whose checksum doesn't match, or that has none, as changed outside `trailguide project sync`, such as by hand or in a merge resolution, because its versions and releases can't be trusted without the source. `trailguide project sync`, `trailguide project update`, and a [fork](/reference/cli/#fork-a-library-rule) refuse it too and write nothing, as sync does when it removes a source whose record holds the rule group metadata an authored rule needs; a removed source whose record no authored rule needs is discarded with the rest of its files: it never records again a record it didn't write, whatever the record says, since no check of individual facts can establish that an edited record is right. To recover, restore a record sync wrote with `git checkout -- .trailguide/vendor/<source-name>/_source.json`, or, during a merge conflict, take one side with `git checkout --ours` or `git checkout --theirs` for that file, then run `trailguide project sync`, which applies any configuration changes; or delete `.trailguide/vendor/<source-name>/` and run `trailguide project sync`, which takes the source again, with its unpinned rules at their newest versions. To resolve a merge conflict in `_source.json`, take one side of the file rather than combining them, and let sync apply the merged configuration. A record that sync can't read at all, such as one with an unknown field, fails with advice to delete `.trailguide/vendor/<source-name>/` and sync. |
| `repository` | The source's repository address. |
| `ref` | The source's `ref` from configuration when the snapshot was recorded, omitted when configuration has none. Changing it to another spelling of the same revision, such as `refs/tags/release/2` for `release/2`, keeps the recorded spelling, so the record stays current. Changing it to another revision makes the record out of date until `trailguide project update SOURCE` takes that revision. Pins and exclusions aren't recorded: configuration owns them, and a pinned rule's `version` in `rules` is the pinned version. So adding, rewording, or removing a pin that moves nothing, or an exclusion, by hand or with a fork, never makes the record out of date, unless the entry names a retired rule that `retiredRules` doesn't list yet; then offline checks ask for `trailguide project sync`, which records the retirement. Generated guidance and `generated/provenance.json` show the configured pins and their reasons. |
| `release` | The library release that supplied the library-wide files: the manifest, rule group metadata, shared assets, and license files. Every skill from the source, and every skill level, comes from it too. It is the newest library release when the source was added or last updated, or the library release your `ref` names, and never older than the `release` of any recorded rule. `trailguide project sync` keeps it, including when you include more rules or skills: those come from this library release too. It's omitted when the source isn't a library, or your `ref` isn't a library release. |
| `resolvedCommit` | The full Git commit SHA of that library release, or of the commit your `ref` resolved to, which sync records when you add a source and update moves when the branch or tag moved. |
| `ruleInclude` | Your configured `rules.include` selectors, as written. Empty when you include no rules. |
| `skillInclude` | Your configured `skills.include` selectors, as written. Empty when you name none; the library's required skills are recorded in `skills` all the same. |
| `groups` | The rule groups whose rules the record holds, with their metadata. A rule group the selectors reach only for some of its rules is listed too. |
| `rules` | Each recorded rule's ID, its `version`, the `release` that published it, and that library release's full `commit`. A rule records the newest version that the recorded `release` or an earlier one published, unless a pin names another. |
| `retiredRules` | The IDs of the rules the library retired that the source would otherwise include, sorted, and empty when there are none. A retired rule the source still includes, such as one a pin keeps at its last version, is in both `rules` and `retiredRules`, which says it's retired, and the generated library README marks it so. The rules it lists are those your selectors reach, and those the previous record held or listed, so an exclusion left after you stop including a retired rule still only warns. It records what the library told sync, not your configuration, so offline checks can tell an exclusion of a retired rule from a typo. `trailguide project update` refreshes it; `trailguide project sync` keeps it byte for byte unless the selectors, `ref`, or `release` change, so a pin or exclusion edit leaves `_source.json` unchanged when the record already covers the rule it names. When you pin or exclude a retired rule the record doesn't list yet, sync checks it against the library and adds it. |
| `files` | Each retained path and its SHA-256 checksum, written as lowercase hexadecimal text. Each file is stored at its snapshot path, including the files of rules from a library release other than `release`, because the snapshot holds one version of each rule. |
| `executable` | The retained paths whose executable bit is set, sorted. A file's mode is part of its content, so offline checks compare it as they compare checksums. |

A **checksum** detects whether a file's contents differ from the recorded copy. The `files` map covers the original file bytes and excludes `_source.json` itself.

A rule's `version` and `release` are `null` when the vendored file isn't a published version, which can happen only when your `ref` names a revision other than a library release. Generated guidance shows no version for it.

For a prefix selector, the snapshot must contain every rule the prefix reaches in the recorded revision, and record the exact selector.

### Skills in the source record

A source that installs skills records each one, with its version, its level, and why the project installs it:

```json
{
  "skills": {
    "skills/build-feature": {
      "version": "1.3.0",
      "release": 3,
      "commit": "9e07b3d6…",
      "level": "recommended",
      "included": true,
      "usedBy": []
    },
    "skills/tdd": {
      "version": "1.1.0",
      "release": 2,
      "commit": "4f1c2a95…",
      "level": "offered",
      "included": false,
      "usedBy": ["skills/build-feature"]
    }
  },
  "rules": {
    "practices/testing/test-behavior-not-implementation": {
      "version": "2.0.0",
      "release": 3,
      "commit": "9e07b3d6…",
      "citedBy": ["skills/build-feature", "skills/tdd"]
    }
  }
}
```

| Field | Meaning |
| --- | --- |
| `skills` | Each recorded skill's ID; its `version` as the record's `release` lists it, the `release` that published that version, and that library release's full `commit`, as `rules` records them; its `level` in the manifest at the record's `release`, `null` for a non-library source; `included`, `true` when your selectors or a level include it; and `usedBy`, the IDs of the recorded skills that use it, empty for a skill that's only included. The skill's files come from the record's `release`, where they match the published version. Excluded skills are recorded too, so update can preview their changes. |
| `retiredSkills` | The IDs of the skills the library retired that the source would otherwise install, as `retiredRules` records rules. |
| `rules.<id>.citedBy` | The source-relative IDs of the installed skills that cite the rule, sorted. Omitted when no installed skill cites it. A rule that only `citedBy` brings in is taken for reference only; your selectors don't reach it. |

The `checksum` covers these fields too.

A non-library source's record has no `release`, `ruleInclude`, `groups`, `rules`, or `retiredRules`, and its skills have no versions or levels:

```json
{
  "formatVersion": 3,
  "kind": "non-library",
  "checksum": "6f3a…c2d8",
  "repository": "https://github.com/example/pstack.git",
  "ref": "main",
  "resolvedCommit": "8a41d07c3e5b9f2a1d6c4e8b7a90f3c2d1e5b6a4",
  "skillInclude": [
    "skills/architect"
  ],
  "skills": {
    "skills/architect": {
      "level": null,
      "included": true,
      "usedBy": []
    },
    "skills/how": {
      "level": null,
      "included": false,
      "usedBy": ["skills/architect"]
    }
  },
  "retiredSkills": [],
  "files": {
    "LICENSE": "e3b0…b855",
    "skills/architect/SKILL.md": "1c4d…9a0f",
    "…": "…"
  },
  "executable": []
}
```

`ref` records the configured value, and `resolvedCommit` the exact commit every skill came from. `trailguide project sync` keeps that commit; only `trailguide project update` moves a source whose `ref` names a branch, or a tag that moved.

### Curated sources in the source record

When a library's manifest lists [curated skills](/reference/configuration/#curated-skills), the library's record holds a `curated` object with one entry per curated source the project installs skills from. The files are stored under `vendor/<source-name>/curated/<curated-source>/`, at their paths in the curated repository:

```json
{
  "curated": {
    "mattpocock": {
      "kind": "non-library",
      "repository": "https://github.com/mattpocock/skills.git",
      "ref": "main",
      "resolvedCommit": "c2e7a4019b5d3f86e1a0c7d94b2f58e3a6d1c0b9",
      "skills": {
        "skills/engineering/to-spec": {
          "level": "recommended",
          "included": true,
          "usedBy": []
        }
      },
      "files": {
        "LICENSE": "77ab…0e41",
        "skills/engineering/to-spec/SKILL.md": "3d8f…a512",
        "…": "…"
      },
      "executable": []
    }
  }
}
```

`ref` is the ref the library's manifest names, from the library release your project recorded, and `resolvedCommit` the commit your project resolved it to at its last update; `level` is the level the library sets. A skill's `usedBy` names the skills of the curated repository that use it, by their IDs there, and `usedByLibrary`, present only when the library's own skills use it, names those, sorted, by their IDs in the library. A curated library's skills also record `version`, `release`, and `commit`. When your project declares the curated repository as its own source, the entry still records the library's curated skills, from your source's commit: `source` names your source, and `ref` and `resolvedCommit` are your source's. The library record's `checksum` covers `curated`.

### What offline checks can verify

`trailguide project build` and `trailguide project check` compare the recorded selectors with configuration and compare stored files with their checksums. Version checks depend on how the source chooses versions:

| Choice | What Trail Guide verifies offline |
| --- | --- |
| Library releases, the default | Each pinned rule records its pinned version. Other rules may record any version the recorded `release` or an earlier one published. `release` is at least as new as every rule's `release`. |
| One revision, with `ref` | For a commit SHA, `resolvedCommit` equals it. For a library release `release/N`, the source records release N, and every rule records a published version from library release N or earlier. A branch's or tag's recorded commit is used without checking where it points now. |

For every source, offline checks first verify the record's `checksum`, so a record changed outside sync fails until you restore a record sync wrote or take the source again. They also verify that `rules` and `skills` list exactly the rules and skills your selectors, the levels, and the skills' dependencies reach, that each skill's dependencies and cited rules match its recorded version's metadata, and that generated provenance, guidance, and installed skills show the same versions and levels. Each exclusion must name a rule in `rules` or `retiredRules`; any other exclusion could be a typo that leaves the rule you meant active, so the check reports that it names no rule the library supplies and asks you to run `trailguide project sync`, which checks it against the library.

Offline checks cannot prove that a recorded version was the newest available, or that recorded versions match the library's release tags; `trailguide project update` checks the latter against the release records before it records anything. These records also cannot authenticate files against the remote repository if someone changed both the local files and their records.

## Find declared licenses and source credits

Provenance keeps license declarations and attribution alongside rule origins. Each source and rule has one `license` object, or `null` when no license was declared.

A rule or skill from a library with a declared license records that declaration and `licenseBasis: "library"`. A skill's own `license` field in `SKILL.md` is informational and doesn't change it. Within the license object, `spdxExpression` records a declared license expression, such as `MIT`. The rule's `attribution` field preserves source credits as URLs and descriptions.

The license record distinguishes original files from the copies retained with generated guidance:

| Field inside `license` | Paths in a source record | Paths in a rule record |
| --- | --- | --- |
| `files` | Original license files, relative to the repository root. | Original license files under `vendor/<source-name>/`, relative to the Trail Guide directory. |
| `attributionFiles` | Original notice files, relative to the repository root. | Original notice files under `vendor/<source-name>/`, relative to the Trail Guide directory. |
| `generatedFiles` | Retained license copies, relative to `generated/`. | The same generated license copies. |
| `generatedAttributionFiles` | Retained notice copies, relative to `generated/`. | The same generated notice copies. |

For example, a rule's `files` entry might be `vendor/team/LICENSE.md`, with `libraries/team/licenses/LICENSE.md` at the same position in `generatedFiles`. Generated rule links point to the retained copy. Notice paths correspond in the same way, by position in `attributionFiles` and `generatedAttributionFiles`; generated notice copies are numbered, such as `libraries/team/licenses/notices/001.md`.

The source record also has a `licenseFiles` list of the files belonging to its declaration, relative to the repository root. These lists describe files for one library-wide license declaration.

A skill from a [non-library source](/reference/imports/#non-library-sources), including a curated skill from one, records the terms Trail Guide found for it. `licenseBasis` says where they came from: `"skill"` for the `license` field in its `SKILL.md`, `"skill-folder"` for a `LICENSE*` file in its folder, or `"repository"` for `LICENSE*` and `NOTICE*` files at the repository root. `spdxExpression` holds the `license` field when that's a valid SPDX expression, and is otherwise `null`; `files` and `attributionFiles` list the license and notice files found, and the root files' retained copies are in `generatedFiles` and `generatedAttributionFiles`. A curated skill keeps its own repository's terms, never the curating library's.

Authored rules and skills, including replacements, have `license: null` and `licenseBasis: "undeclared"`. Replacing a library rule does not automatically assign its library's license to your replacement. The library's declaration remains in its source record.

These fields preserve declarations; they are not a legal verification status. `"undeclared"` does not mean public domain or permission to redistribute. For how authors declare terms and credits, see [License metadata](/reference/library-format/#license-metadata) and [Rule attribution](/reference/rule-format/#rule-attribution).
