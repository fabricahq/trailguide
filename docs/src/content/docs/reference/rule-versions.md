---
title: "Rule and skill versions"
description: "What a rule or skill version covers, library release tags, change levels, change notes, and retired rules and skills."
---

Each rule and each skill in a library has its own [semantic version](https://semver.org/). A version isn't a field in the rule file or the skill's `SKILL.md`, so frontmatter has no `version` field; each library release's tag records every rule's and skill's version. A **library release** publishes new versions of one or more rules and skills at once, from a single change to a large collection of updates.

:::caution[Preview]
Skill versions and skill change notes aren't implemented yet. Trail Guide versions rules and requires change notes for rule changes after the first library release.
:::

This page is the exact specification. [Version your rules](/guides/version-rules/) explains the concepts and walks through the workflow.

## What a version covers

A rule version covers the rule itself:

- The rule's Markdown file.
- The rule's own asset directory, `assets/<rule-name>/`.

A project that takes a rule version gets exactly these files as they were in that version.

A skill version covers the skill's whole folder, `.trailguide/skills/<name>/`:

- Every file in the folder that Git tracks, including `SKILL.md`, scripts, references, and assets. Files Git ignores aren't part of any version.
- Each file's executable bit, so making a script executable, or no longer executable, is a change to the skill.

A project that installs a skill version gets exactly these files, with their modes, as they were in that version. Unlike a rule's, a skill's version isn't chosen on its own: a project installs all of a source's skills from the library release that supplies its library-wide files, described next, and the skill's version tells you what changed. The skills it uses come from that library release too. The rules it cites aren't part of its version: each has its own version, and a project takes each at the version its configuration chooses. A skill's [level](/reference/library-format/#skill-levels) isn't part of its version either: levels are set in the manifest, a library-wide file.

Everything else is a **library-wide file**: the [manifest](/reference/library-format/#the-manifest) in `.trailguide/config.yaml`, with its skill levels, curated sources, and license declaration; rule group metadata; shared files in `.trailguide/assets/`; and license and notice files. Every file is rule content, skill content, or a library-wide file, so a library can't declare a rule's Markdown file, a file in a rule's asset directory, or a file in a skill folder as a license or notice file; see [License metadata](/reference/library-format/#license-metadata). Library-wide files aren't part of any rule or skill version, and changing them needs no change note. A project receives them from one library release, which is never older than the library release of any rule version it takes, or from the revision its `ref` names. A new source takes them from the newest library release; `trailguide project update` moves them to the newest library release, even when no rule changes; and `trailguide project sync` keeps the library release recorded in `.trailguide/lock.json`, also when you include more rules or skills. The source's skills and skill levels always come from that same library release, so they move whenever it does. A rule pinned to an older version can therefore link to a newer copy of a shared file, so keep everything that defines a rule's obligation in the rule itself, and use shared files only to explain and illustrate.

## Library releases

A **library release** publishes every change note added since the previous library release as new rule and skill versions, together with any changes to library-wide files. It's a single annotated Git tag, `release/<number>`, such as `release/4`, on a commit of the default branch. Publishing changes no files and creates no commit.

The content at the tagged commit is exactly what the library release published. Change notes stay in the repository; the tag marks which notes it published, because the next library release publishes only notes added after it.

Release numbers start at `1`, and each library release adds one. They aren't semantic versions: a library release can hold changes of every size to different rules and skills, and each one's version describes its own change. The newest library release is the one with the highest number. Trail Guide ignores tags other than `release/<number>`. `trailguide library release` creates these tags; don't create, move, or delete them by hand.

### Release record

The tag's message is the library release's permanent record. It has two parts, separated by a line containing only `---`:

1. The release notes in Markdown, the same text as the [GitHub Release page](#github-release-pages).
2. A YAML record that Trail Guide reads:

```yaml
formatVersion: 1
release: 4
rules:
  practices/code-design/organize-code-by-feature: 1.1.0
  practices/testing/verify-backoff: 1.3.0
  practices/testing/verify-retries: 1.0.0
  practices/testing/verify-retry-limits: 2.0.0
  techs/react/prefer-server-components: 1.4.0
  techs/react/test-hooks-in-isolation: 2.2.0
changes:
  practices/testing/verify-retries:
    change: new
    summaries:
      - Add a broader rule about testing retries.
  practices/testing/verify-retry-limits:
    change: major
    from: 1.3.0
    summaries:
      - Require a test at the limit for every retry policy.
  techs/react/test-hooks-in-isolation:
    change: minor
    from: 2.1.0
    summaries:
      - Add an example for custom hooks.
retired:
  practices/testing/check-retry-backoff:
    lastVersion: 1.2.0
    replacedBy: practices/testing/verify-retries
    summaries:
      - Covered by the broader rule about testing retries.
libraryFiles:
  - practices/testing/_group.yaml
```

| Field | Meaning |
| --- | --- |
| `formatVersion` | The release record format, `1`. Required. |
| `release` | The release number. |
| `rules` | Every current rule and its version after this library release. |
| `changes` | Each rule this library release changed or added: its `change` (`new`, `major`, `minor`, or `patch`), its previous version as `from` (absent for a new rule), and its `summaries`: a list with one summary per change note that named the rule, in note order. |
| `retired` | Each rule this library release retired: its `lastVersion`, its `summaries`, as in `changes`, and its `replacedBy` rule when there is one. |
| `libraryFiles` | Library-wide files this library release changed, such as the manifest, rule group metadata, and shared assets. |
| `skills` | The library's skills: their versions, changes, and retirements. See [Skills in the release record](#skills-in-the-release-record). |

Each `summaries` list has at least one item, and each item follows the rules of a [change note's](#change-notes) `summary`: one non-blank line without control characters.

Release records are read by every later version of Trail Guide, so the format is forward compatible. Readers ignore fields they don't know, at any level and whatever their values, such as timestamps, so a later Trail Guide can add fields that older versions skip. `formatVersion` changes only for an incompatible change; readers check it before anything else in the record, and a Trail Guide that finds a higher `formatVersion` than it reads stops with `unsupported-release-record` and asks you to install a newer Trail Guide. The whole record still uses one YAML document without duplicate keys, anchors, aliases, or explicit tags, and every field above keeps its rules; for example, a summary that YAML reads as a date must be quoted. Change notes, which people write by hand, stay strict: unknown fields fail.

A rule's version is plain `major.minor.patch` numbers, each at most 999,999,999, without prerelease or build suffixes. A new rule starts at `1.0.0`. The rule ID is the rule's path in `.trailguide/rules/` without `.md`.

A rule's full history is the `changes` entries for it across every `release/<number>` tag. To find which library release published a version, such as one a project pins, Trail Guide finds the tag whose `changes` entry has that version; the rule's files are at that tag's commit. Projects fetch release tags without the library's history.

### Skills in the release record

A library that has skills records them in one top-level `skills` field, beside the rule fields. This release record publishes a new skill, a major change to another, and a retirement:

```yaml
formatVersion: 1
release: 5
rules:
  practices/code-design/organize-code-by-feature: 1.1.0
  practices/testing/verify-backoff: 1.3.0
  practices/testing/verify-retries: 1.0.0
  practices/testing/verify-retry-limits: 2.0.0
  techs/react/prefer-server-components: 1.4.0
  techs/react/test-hooks-in-isolation: 2.2.0
skills:
  versions:
    skills/build-feature: 2.0.0
    skills/fix-bug: 1.0.0
    skills/review-tests: 1.1.0
  changes:
    skills/build-feature:
      change: major
      from: 1.3.0
      summaries:
        - Require an agreed design before the first slice.
    skills/fix-bug:
      change: new
      summaries:
        - Add a playbook for fixing a reported bug with a regression test.
  retired:
    skills/triage-bug:
      lastVersion: 1.2.0
      replacedBy: skills/fix-bug
      summaries:
        - Covered by the fix-bug playbook, which also writes the regression test.
```

| Field | Meaning |
| --- | --- |
| `skills.versions` | Every current skill and its version after this library release. Required when `skills` is present; empty when every skill is retired. |
| `skills.changes` | Each skill this library release changed or added, in the same form as the rule `changes`: its `change`, `from`, and `summaries`. |
| `skills.retired` | Each skill this library release retired, in the same form as the rule `retired`: its `lastVersion`, `summaries`, and its `replacedBy` skill when there is one. |

`skills.changes` and `skills.retired` are left out when they're empty. A skill's version, ID, and history follow the rules for a rule's: plain `major.minor.patch` numbers, starting at `1.0.0`; the ID is `skills/<name>`; and its full history is the `skills.changes` entries for it across every `release/<number>` tag. The first library release that publishes skills lists every one of them as `new`; when it's the library's first library release, each skill's summary is the placeholder `Add the skill.`, as each rule's is `Add the rule.`

Skills never appear in `rules`, `changes`, or `retired`, and rules never appear in `skills`, so `formatVersion` stays `1`. A Trail Guide that doesn't know skills, and other tools that read release records, such as catalogs, ignore the `skills` field as they ignore any unknown field, and keep reading the library's rules.

Trail Guide writes `skills` in every library release it publishes once the library has, or has had, a skill. A library release that a Trail Guide without skill support published later has no `skills` field. Trail Guide then takes skill versions, and the commit to compare skills with, from the newest library release whose record has the field, and warns, for example: `release/6 has no skills field, because a Trail Guide without skill support published it. Skill versions come from release/5. Raise the Trail Guide version this library's CI uses before publishing skills.` Projects that take from the library get the same warning from `trailguide project sync` and `trailguide project update`. To avoid it, raise the Trail Guide version your library's CI pins before you publish skills; see [Check changes in CI](/guides/version-rules/#check-changes-in-ci).

### GitHub Release pages

For a repository on GitHub.com, each library release also creates one **GitHub Release page** on its tag, with the release notes as its body: the page GitHub uses to announce a library release, which people can browse and get notified about. It's an announcement only; Trail Guide never reads it. In these docs, a *library release* is a publication of a library's rule versions, as described on this page, and a *GitHub Release page* is only its announcement on GitHub.

The release notes are generated from the change notes and the release record: an opening line that counts the changes; sections for new rules, major, minor, and patch changes, and retired rules, in that order, each included only when it has entries; a **Skills** section that lists every new, changed, and retired skill, when there are any; a sentence saying the library release also updates shared files, when `libraryFiles` isn't empty, except in the first library release, which adds them all; and a collapsed table of every rule's version, followed by one of every skill's version when the library has skills. A library release that changes only skills opens by counting the skill changes, and publishes like any other. A library release that changes no rules or skills opens with a line saying so and that it updates shared files. The first library release opens by saying how many rules it publishes, and lists its new rules without their summaries, which are all the same placeholder, `Add the rule.`, in its release record. The notes never list library-wide files by path; `libraryFiles` in the release record does. [Publish a library release](/guides/version-rules/#publish-a-library-release) shows a complete example. You can edit the GitHub Release page on GitHub, for example to add an introduction; the tag message keeps the generated text.

A project normally follows each rule's newest version, and the newest library release for its skills, but it can also take exactly what one library release published, by naming its tag; see [Take one revision](/reference/configuration/#take-one-revision).

## Choose a version change

Rule versions describe the rule's **obligation**: what work must do to comply with it.

| Change | Definition | Example |
| --- | --- | --- |
| `major` | Work that complied with the previous version could fail this one. | Lower the required retry limit, or require a test the rule previously only recommended. |
| `minor` | Work that complied with the previous version still complies, and this version adds new guidance. | Add a Python example of the same test. |
| `patch` | Work that complied with the previous version still complies, and this version adds no new guidance. | Fix a misleading sentence or a typo in an example. |

To decide, ask two questions in order. First, could work that complied with the previous version fail this one? If yes, the change is major. If no, does this version add new guidance, such as a new example or advice for a new situation? If yes, the change is minor; if no, it's a patch.

These edits show how the definitions apply:

| Edit | Change |
| --- | --- |
| Narrow an exception, lower a limit, or turn a recommendation into a requirement. | `major` |
| Replace the obligation with a different one, even when it seems equally strict. | `major` |
| Add an example in another language, or advice for a situation the rule already allowed. | `minor` |
| Reword a sentence, fix a typo, or correct an example that contradicted the rule's own text. | `patch` |

Widening where a rule applies can be major, even though it only adds text: code in the newly covered situation may not comply. Adding a new case is minor only when all work that complied with the previous version still complies. When unsure, choose the larger change.

### Choose a skill's version change

Skill versions describe what the skill **produces and requires**: the steps an agent takes, the inputs it needs, and the rules and skills it depends on.

| Change | Definition | Example |
| --- | --- | --- |
| `major` | Someone relying on the previous version could find this one produces or requires something different. | Remove or reorder a step people rely on, require a new input, or start using a rule or skill that a project might not have. |
| `minor` | The skill adds steps or guidance, and everything that relied on the previous version still works. | Add an optional step that checks accessibility, or a reference with more examples. |
| `patch` | The skill only clarifies its wording. | Fix a typo, or reword a step without changing what it asks for. |

Ask the same two questions in order. First, could someone or something that relied on the previous version, such as a team's workflow or another skill that uses this one, break or get a different result? If yes, the change is major. If no, does this version add steps or guidance? If yes, it's minor; if no, it's a patch. Changing a script's behavior, or its executable bit, follows the same questions. When unsure, choose the larger change.

### Write the summary

Write each change's summary for a project maintainer deciding whether to update. Name what changed in the obligation or guidance, such as "Require a test at the limit for every retry policy." Avoid describing the edit itself, such as "Update the rule."

## Change notes

After the first library release, every change to a rule or skill needs a **change note**: a file that names the rules and skills it covers, how much each changed, and a summary for projects that update. The next library release publishes the notes added since the previous one.

Before the first library release, no rule or skill has a version, so they need no notes. The first library release gives every rule and skill version `1.0.0`.

Notes live in the library's `.trailguide/changes/` directory. Each is a YAML file with a unique name, so changes made in parallel never conflict. `trailguide library change` names new notes by date, rule, and a random suffix, such as `.trailguide/changes/2026-09-29-verify-retry-limits-7f3a9c.yaml`, but any unique name ending in `.yaml` works. Notes are never deleted.

```yaml
summary: Require a test at the limit for every retry policy.
rules:
  practices/testing/verify-retry-limits: major
```

| Field | Meaning |
| --- | --- |
| `summary` | Required non-blank line of text describing the change for project maintainers, without control characters such as tabs or escape sequences. For a retirement, explain why. |
| `rules` | Map of the rules the note covers to their change: `major`, `minor`, or `patch` for a rule that has a version; `new` for a rule that doesn't; or `retired`. A retired rule can instead map to an object with `change: retired` and a `replacedBy` rule ID. |
| `skills` | Map of the skills the note covers to their change, in the same form as `rules`. A retired skill's `replacedBy` is a skill ID. |

A note needs at least one of `rules` and `skills`, and can have both. A rule can't replace a skill, and a skill can't replace a rule: a `replacedBy` under `rules` must be a rule ID, and one under `skills` a skill ID.

Change notes use one YAML document. Duplicate keys, anchors, aliases, explicit tags, and unknown fields are rejected.

A new rule:

```yaml
summary: Add a broader rule about testing retries.
rules:
  practices/testing/verify-retries: new
```

A retired rule, with its replacement:

```yaml
summary: Covered by the broader rule about testing retries.
rules:
  practices/testing/check-retry-backoff:
    change: retired
    replacedBy: practices/testing/verify-retries
```

When nothing replaces a retired rule, map it to `retired`, and explain why in the summary.

A changed skill:

```yaml
summary: Require an agreed design before the first slice.
skills:
  skills/build-feature: major
```

A retired skill, with its replacement:

```yaml
summary: Covered by the fix-bug playbook, which also writes the regression test.
skills:
  skills/triage-bug:
    change: retired
    replacedBy: skills/fix-bug
```

One note can cover several rules and skills, such as a rename or a set of related edits. When several pending notes name the same rule or skill, the library release uses the largest change and lists each note's summary.

Edit a pending note's wording freely; the library release uses the notes as they are when it's published. Editing a note that a library release already published has no effect, and `trailguide library check` warns about it.

`trailguide library check` requires every rule and skill changed since the latest `release/<number>` tag to be named in a note added since that tag, and rejects notes that don't match a change. See [library check](/reference/cli/#library-check) for the complete list. The [Version your rules](/guides/version-rules/) guide shows the workflow.

## Retired rules

**Retiring** a rule ends its history: the library stops publishing it. Retire a rule when a better rule replaces it, such as a narrow retry-limit rule folded into a broader rule about testing retries, or when the practice is no longer recommended, such as a rule about how agents should comment code after the author concludes they shouldn't add those comments at all. The retirement names its replacement when there is one; its summary explains why.

To retire a rule, delete its Markdown file and asset directory and add a [change note](#change-notes) that maps it to `retired`. The library release records the retirement in its release record, with the rule's last version, the summary, and any replacement.

The rule's last numbered version stays its final version. A project that pinned the rule before the retirement keeps that version. A retired rule's ID can't be reused; give a new rule a new ID.

Projects that update past a retirement see it in the update preview, with its summary and any replacement.

Renaming or moving a rule changes its ID. Record it in one note that retires the old ID, replaced by the new ID, and adds the new ID as `new`.

## Retired skills

Retiring a skill works the same way. Delete its folder, remove it from the manifest's levels, and add a change note that maps it to `retired` under `skills`, naming a replacement skill when there is one. The library release records the retirement in `skills.retired`. The skill's last version stays its final version, and its ID can't be reused. Skills can't be pinned, so a project that wants to keep using a retired skill forks it before it updates past the retirement.

Projects that update past the retirement see it in the update preview, and applying the update removes the installed copies. A skill in the library that still names the retired skill in `trailguide-skills-used` fails `trailguide library check`, which names the replacement, so update it in the same pull request. Renaming a skill changes its ID: retire the old ID, replaced by the new one, and add the new ID as `new`, in one note.
