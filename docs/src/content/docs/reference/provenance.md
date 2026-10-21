---
title: "Provenance"
description: "Where to find a rule's or skill's source, version, level, replacement reason, and declared license."
---

**Provenance** records where your project's rules and skills came from. The main file is `generated/provenance.json` in the **Trail Guide directory**, so its path from the project root is `.trailguide/generated/provenance.json`. It identifies the sources and versions your project uses, the source of each active rule and installed skill, the library that set each skill's level, and any replacements.

Read it when you want to understand why a rule or skill is present, investigate a library update, or find out why your project replaced a rule or skill. Tools can also read the JSON to report this information. To find and follow the rules themselves, agents start with [the generated rule index](/reference/files/#where-agents-start).

This page shows where the records live, how Trail Guide creates them, and how to trace a rule or skill back to its source.

## Where to look

Trail Guide keeps these related records:

| File | What it tells you |
| --- | --- |
| `generated/provenance.json` | Where active rules and installed skills came from, which versions are in use, which library set each skill's level, and why rules and skills were replaced. |
| `lock.json` | Which revision, rule and skill versions, and original files each source's vendor snapshot holds: the project's **lockfile**. |
| `generated/libraries/<source-name>/README.md` | A readable summary of the library release or commit taken, rule and skill versions and levels, and declared license terms. |
| `generated/installed-skills.json` | Which skill folders Trail Guide installed in the target folders. See [Installed skills](/reference/files/#installed-skills). |

These paths are relative to the Trail Guide directory, `.trailguide/`. The **source name** is the name you gave a source in your configuration, such as `team`.

Trail Guide writes these files. To change the information they describe, edit your configuration or authored rules and skills and run the appropriate [sync or build command](/reference/sync/).

## How the records are created

1. **You include rules and skills.** Configuration records the sources, the rules and skills you include from each, pins, exclusions, replacements, and renames.
2. **Sync and update record what they take.** `trailguide project update` moves each source along its `ref` and writes its entry in `lock.json` with the exact Git commit, rule versions, skill levels, and file digests. `trailguide project sync` restores exactly what that entry says, and never moves a source.
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
| `level` | The skill's [level](/reference/configuration/#skill-levels), `required`, `recommended`, or `offered`, as the library that listed it sets it, or `dependency` for one the library lists as a [dependency](/reference/configuration/#dependencies), which installs only with the skills in `usedBy`; `null` for an authored skill or one from a non-library source. |
| `requiredBy`, `recommendedBy`, or `offeredBy` | The library that set that level and the library release whose manifest set it, as `library` and `release`. Present when a level brought the skill in, including a curated skill; omitted when your project named the skill by ID in the source that holds it, the skill is a dependency, or the skill is authored or from a non-library source. |
| `included` | `true` when your configuration or a level includes the skill; `false` when it's installed only because other skills use it. |
| `usedBy` | The project IDs of the installed skills that use this one, sorted, and empty when none does. |
| `uses` | The project IDs of the skills this one uses, from its `trailguide-skills-used` list and your configuration's `skills.uses`. When your project replaces one, the replacement's ID is listed. |
| `rulesUsed` | The project IDs of the rules its `trailguide-rules-used` list names. Each of those rules' entries in `rules` lists this skill in `citedBy`. |
| `targets` | The folders it's installed in, relative to the project root, as in `generated/installed-skills.json`. Empty when the project has no skill targets. |
| `upstream`, `replacementReason`, and `basedOn` | For a [fork](/reference/configuration/#fork-a-skill), the forked skill's origin, your configured reason, and the commit your skill is based on; otherwise `null`. |
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
| `ref` | The source's `ref` from configuration when the entry was written, such as `release/3`, `latest-release`, or `main`. Changing it to another spelling of the same revision, such as `refs/tags/release/3` for `release/3`, keeps the locked spelling, so the entry stays current. Changing it to another library release makes the entry out of date until `trailguide project update SOURCE` takes that release, and offline checks report it at `lock.json.sources.<source-name>.ref`; changing it to a branch, tag, or commit makes the entry out of date until `trailguide project sync` takes that revision. Pins and exclusions aren't locked: configuration owns them, and a pinned rule's `version` in `rules` is the pinned version. Generated guidance and `generated/provenance.json` show the configured pins and their reasons. |
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
| `integrity` | The entry's digest, which proves that Trail Guide wrote it; see [Integrity](#integrity). |

A rule's `version` and `release` are `null` when the vendored file isn't a published version, which can happen only when your `ref` names a revision other than a library release. Generated guidance shows no version for it.

For a prefix selector, the snapshot must contain every rule the prefix reaches in the locked revision.

### Integrity

`integrity` is `sha256-` followed by the base64 SHA-256 of the entry's canonical form: the JSON object `{"entry": …, "name": …}`, holding the entry without `integrity` and the source's name, serialized as [RFC 8785](https://www.rfc-editor.org/rfc/rfc8785) specifies. Formatting and key order therefore don't matter, but any change to a field, an added or removed field, or moving the entry to another source's name does.

Trail Guide trusts only an entry it wrote. Offline, `trailguide project build` and `trailguide project check` refuse an entry whose `integrity` doesn't match, or that has none, as changed outside Trail Guide, such as by hand or in a merge resolution, because its versions can't be trusted without the source. Sync, update, and a [fork](/reference/cli/#fork-a-library-rule) refuse it too and write nothing; no check of individual facts can establish that an edited entry is right. An entry for a source you removed from configuration is discarded with its snapshot, whatever it says, unless an authored rule needs the rule group metadata in that snapshot.

When an entry was changed outside Trail Guide, restore the one Trail Guide wrote, then sync:

```sh
git checkout -- .trailguide/lock.json
trailguide project sync
```

When the entry can't be restored, take the source again at its `ref`. This resolves the source as if you had just added it, so its unpinned rules move to the newest versions that `ref` offers:

```sh
trailguide project sync --retake fabrica
```

Strict parsing also refuses a key that appears twice in an object, and a field the format doesn't define. Integrity detects mistakes and stray edits; it doesn't authenticate files against the remote repository if someone changed both the files and the lockfile and recomputed the digest.

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

When the side you kept is behind the other, such as the older release, take the newer one with `trailguide project update SOURCE`. When an entry can't be resolved cleanly, run `trailguide project sync --retake SOURCE` instead.

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

When `lock.json` is missing but `vendor/` holds files a sync wrote, every command fails with `lock-missing`, because sync never resolves a source again on its own. Restore the file with `git checkout -- .trailguide/lock.json`, and sync. To take every source again at its `ref` instead, retake each one, such as `trailguide project sync --retake fabrica --retake tools`. When configuration names no source, delete `.trailguide/vendor/` instead.

### What offline checks can verify

`trailguide project build` and `trailguide project check` compare the locked selectors with configuration and compare stored files with their digests. Version checks depend on how the source chooses versions:

| Choice | What Trail Guide verifies offline |
| --- | --- |
| Library releases, the default | Each pinned rule records its pinned version. Other rules may record any version the recorded `release` or an earlier one published. `release` is at least as new as every rule's `release`. |
| One revision, with `ref` | For a commit SHA, `commit` equals it. For a library release `release/N`, the source records release N, and every rule records a published version from library release N or earlier. A branch's or tag's recorded commit is used without checking where it points now. |

For every source, offline checks first verify the entry's [`integrity`](#integrity), so an entry changed outside Trail Guide fails until you restore the entry Trail Guide wrote or take the source again. They also verify that `rules` and `skills` list exactly the rules and skills your selectors, the levels, and the skills' dependencies reach, that each skill's dependencies and cited rules match its recorded version's metadata, and that generated provenance, guidance, and installed skills show the same versions and levels. Each exclusion must name a rule in `rules` or `retiredRules`; any other exclusion could be a typo that leaves the rule you meant active, so the check reports that it names no rule the library supplies and asks you to run `trailguide project sync`, which checks it against the library.

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
