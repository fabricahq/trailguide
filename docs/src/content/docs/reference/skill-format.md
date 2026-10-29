---
title: "Skill format"
description: "What Trail Guide accepts in a skill folder and its SKILL.md: IDs, frontmatter, metadata keys, links, files, and every library check."
---

This page defines what Trail Guide accepts in a [skill](/concepts/skill/): its folder, its `SKILL.md` file, and the Trail Guide keys in its metadata. The same format applies to every skill a repository authors, whether it's a library, a project, or both. For how to write a skill well, see [Write a skill](/guides/write-skills/).

A skill uses the [Agent Skills](https://agentskills.io) format. Trail Guide adds optional `metadata` keys and checks the rest of the format as the Agent Skills specification defines it, so a skill that passes `trailguide library check` also works in tools that read Agent Skills directly.

## Skill folders

A repository's authored skills live in `.trailguide/skills/`, one folder per skill, beside `rules/`. [Project files](/reference/files/) shows the whole layout.

```text
skills/
  build-feature/
    SKILL.md
    references/
      slicing.md
    scripts/
      verify.sh
  review-tests/
    SKILL.md
```

Each skill folder holds a `SKILL.md` file and anything the skill bundles. The Agent Skills format suggests `scripts/` for code the agent runs, `references/` for documents it reads when needed, and `assets/` for templates and other files it uses, but any layout inside the folder works. The `assets/` folder inside a skill belongs to the skill; it isn't a rule asset directory, and the shared `.trailguide/assets/` directory isn't part of any skill.

`skills/` holds only skill folders, and is flat: a skill folder can't contain another skill's `SKILL.md`, and it can't contain `_group.yaml`. Skills don't belong to rule groups, so a skill folder is never inside `rules/`.

### Git-tracked files

A skill's content is the files Git tracks in its folder. Files Git ignores, such as `__pycache__/`, `node_modules/`, or `.venv/` that a skill's scripts create, are neither versioned nor installed. `trailguide library check` warns about a file in a skill folder that Git neither tracks nor ignores, because it isn't part of the skill until you commit it. A repository outside Git has no library releases, and Trail Guide reads every file in each skill folder.

Symbolic links, submodules, and Git LFS pointers aren't supported in a skill folder, as for every file Trail Guide takes from a source.

### File modes

A file's executable bit is part of the skill's content. Git records it as mode `100755` for an executable file and `100644` for any other file. Trail Guide keeps the mode in a project's vendor snapshot and in every installed copy, so a step that runs `./scripts/verify.sh` works after install. Changing only a file's mode is a change to the skill, and after the first library release it needs a change note, like any other change.

### File contents

Markdown files in a skill folder, including `SKILL.md`, must be UTF-8, so their links can be checked. Other files can be binary; Trail Guide keeps their original bytes. The [source limits](/reference/imports/#source-limits-and-version-errors) on file size and count apply to skill files as to any other vendored file.

## Skill IDs

A skill's ID is its folder's path in the repository that holds it. For a skill a repository authors, that's its path in the Trail Guide directory, `skills/<name>`, such as `skills/build-feature`. For a skill in a [non-library source](#skills-from-non-library-sources), it's the folder's path from the repository root, such as `skills/engineering/to-spec`. A skill's identity is its repository plus its ID, so two sources that reach the same repository name the same skill.

In a project, an ID from a source is qualified by the source name the project chose, such as `fabrica:skills/build-feature`. A skill a library curates adds the curated source's name, such as `fabrica:mattpocock:skills/engineering/to-spec`. An authored skill's ID has no source name. See [Source names and IDs](/reference/configuration/#source-names-and-ids).

`<name>` follows the Agent Skills name grammar:

- 1 to 64 characters,
- only lowercase letters `a`–`z`, digits `0`–`9`, and hyphens `-`,
- not starting or ending with a hyphen, and
- no two hyphens in a row.

The folder name is the skill's name, and it must equal the `name` field in its `SKILL.md`. Renaming the folder changes the skill's ID. In a library, a rename retires the old ID, replaced by the new one; see [Retire or replace a skill](/guides/write-skills/#retire-or-replace-a-skill). A retired skill's ID can't be reused.

The name is also the skill's install name, the folder name of every installed copy, such as `.claude/skills/build-feature/`, so two skills with the same name can't be installed in one project, even from different sources. A project resolves a clash by [renaming](/reference/configuration/#rename-a-skill) one of them, which changes the install name but not the ID.

## Frontmatter

`SKILL.md` starts with YAML frontmatter, from the first byte of the file, followed by a non-blank Markdown body with the skill's instructions:

```markdown
---
name: build-feature
description: Build a feature from an agreed spec, one slice at a time. Use when starting implementation of a new feature or a change to an existing one.
license: MIT
metadata:
  trailguide-skill-type: playbook
  trailguide-rules-used: |
    practices/architecture/decide-module-boundaries-first
    practices/testing/test-behavior-not-implementation
  trailguide-skills-used: |
    skills/tdd
---

1. Read the spec and name the slice you're building.
2. ...
```

These are the Agent Skills fields. Unknown top-level fields are rejected, because tools that read Agent Skills, such as claude.ai, reject them too.

| Field | Meaning |
| --- | --- |
| `name` | Required. The skill's name, following the [name grammar](#skill-ids). Must equal the folder name. |
| `description` | Required. One non-blank string of at most 1,024 characters saying what the skill does and when to use it. Agents read it to decide when to load the skill, so name the task that should trigger it. |
| `license` | Optional string naming the skill's license, such as an SPDX expression. Informational in a library; see [Licenses](#licenses). |
| `compatibility` | Optional non-blank string of at most 500 characters describing what the skill needs to run, such as tools, network access, or a product. |
| `allowed-tools` | Optional string listing tools the skill is approved to use, separated by spaces, as the Agent Skills format defines it. Trail Guide preserves it and doesn't interpret it. |
| `metadata` | Optional map of string keys to string values. Trail Guide reads the [keys below](#metadata) and preserves every other key unchanged. |

The frontmatter uses one YAML document. Duplicate keys, anchors, aliases, and explicit tags are rejected. A value that YAML reads as something other than a string, such as `true` or `2026-10-14`, must be quoted where a string is required.

A skill has no version field, and no metadata key sets its version. In a library, its version comes from library releases; see [How a skill is versioned](/concepts/skill/#how-a-skill-is-versioned).

## Metadata

Trail Guide reads six optional keys under `metadata`: the three `trailguide-` keys in the table below, and three keys for the [text agents show](#text-agents-show). Each value is a string, as the Agent Skills format requires, so the file stays valid everywhere skills are read.

| Key | Value | Checks |
| --- | --- | --- |
| `trailguide-skill-type` | `playbook` or `review` | Must be a type on the canonical list. Unknown values fail `trailguide library check`. |
| `trailguide-rules-used` | The IDs of the rules the skill applies, one per line | Each must be a current rule in this repository, or, outside a library, in a source the project names. |
| `trailguide-skills-used` | The IDs of the skills this skill calls, one per line | Each must be a current skill in the same repository, other than this one, or, in a library, a skill the library curates, and the skills called can't form a cycle. |

Other `metadata` keys, such as `author`, are preserved and ignored, apart from the [interface keys](#text-agents-show) below. Keys that start with `trailguide-` are reserved for Trail Guide, so any other `trailguide-` key fails `trailguide library check`.

### Skill type

`trailguide-skill-type` labels the skill as a `playbook` or a `review`; see [Skill types](/concepts/skill/#skill-types). Leave it out when neither fits. The type is a plain label: Trail Guide installs, checks, and updates every skill the same way, whatever its type.

The canonical type list is `canonical-skill-types.yaml` in the Trail Guide repository. Each Trail Guide release builds the list in, so a new type needs a Trail Guide release, and your library's CI must use that release before a skill can declare the type. Only `trailguide library check` checks the type: a project installs a skill whose type its Trail Guide doesn't know without complaint, and shows the type as written.

### Rules a skill applies

`trailguide-rules-used` lists the rules the skill applies, one library rule ID per line, in a YAML block string:

```yaml
metadata:
  trailguide-rules-used: |
    practices/architecture/decide-module-boundaries-first
    practices/testing/test-behavior-not-implementation
    practices/verification/verify-on-the-surface-users-touch
```

In a library, each line is a rule ID in this library: the rule's path in `rules/` without `.md`, and without a source name, because each project chooses its own source names. Surrounding whitespace and blank lines are ignored. Each ID can appear once. A library's skill can cite rules from any rule group in its library, and only rules in its library. A skill in a repository that isn't a library can also cite its sources' rules by project ID; see [Skills in a project](#skills-in-a-project).

When a project installs the skill, Trail Guide takes every cited rule, even ones the project's rule `include` doesn't name, and appends a generated section to the installed `SKILL.md` that links to the project's version of each one. Cited rules are for reference only: they never join `RULES.md`. See [Installed copies](#installed-copies).

### Skills a skill uses

`trailguide-skills-used` lists the other skills this skill calls, one skill ID per line, in the same form:

```yaml
metadata:
  trailguide-skills-used: |
    skills/architect
    skills/tdd
```

Each line is a skill ID in the same repository, `skills/<name>`. A skill can't call itself, and the skills called can't form a cycle, directly or through other skills. When a project installs the skill, Trail Guide also installs every skill it calls, and the skills those call in turn, from the same repository and revision. A project declares calls a skill's metadata leaves out with [`skills.uses`](/reference/configuration/#declare-the-skills-a-skill-calls); Trail Guide doesn't search skill text for them.

A library's skill can also call a skill the library [curates](/reference/configuration/#curated-skills), by the ID the library's manifest lists it under, such as `anthropic:skills/skill-creator`:

```yaml
metadata:
  trailguide-skills-used: |
    anthropic:skills/skill-creator
```

The library must list the curated skill in `library.skills`, at a level or as a [dependency](/reference/configuration/#dependencies). When a project installs the calling skill, Trail Guide also installs the curated skill, from the repository the library curates it from, at the commit the project resolves for that repository, as it installs any curated skill, whatever level the library gives it. A curated skill brings the skills it calls in its own repository, but not skills that repository curates in turn.

### Text agents show

Some agents show a skill in their interface with its own display name, a short description, and a prompt that starts it. Three optional `metadata` keys hold that text, without naming an agent:

```yaml
metadata:
  display-name: "Add Skill"
  short-description: "Create and place Fabrica agent skills"
  default-prompt: "Use $add-skill to create or update a Fabrica agent skill."
```

| Key | Value |
| --- | --- |
| `display-name` | The name the interface shows for the skill, such as `Add Skill`. |
| `short-description` | A short line about the skill, for lists. Agents still choose the skill by its `description`. |
| `default-prompt` | A prompt that starts the skill, such as `Use $add-skill to create or update a Fabrica agent skill.` |

Each value is a string; a value that isn't one fails like any other unreadable frontmatter. Trail Guide maps the keys into the file each agent reads. Today that's Codex, which reads `agents/openai.yaml` in the skill folder: when a project installs the skill, Trail Guide writes that file into the copy in `.agents/skills/` only, with `interface.display_name`, `short_description`, and `default_prompt`. The display name falls back to the skill's install name, a key the skill leaves out stays out of the file, and a skill that sets none of the keys gets no file. When a project [renames](/reference/configuration/#rename-a-skill) the skill, each `$<name>` mention of the skill's own name in these values becomes `$<install-name>`, so the prompt starts the skill as installed:

```yaml
# Generated by Trail Guide from the display-name, short-description, and default-prompt metadata in SKILL.md. Do not edit.
interface:
  display_name: "Add Skill"
  short_description: "Create and place Fabrica agent skills"
  default_prompt: "Use $add-skill to create or update a Fabrica agent skill."
```

The keys cover only this text. To set anything else Codex reads from `agents/openai.yaml`, such as icons, tool dependencies, or `policy.allow_implicit_invocation`, ship the file in your skill folder. A skill that ships its own file, such as a third party's skill you install or curate, keeps it unchanged in every copy, and Trail Guide generates nothing for it, whatever its metadata says. Mentions of the skill's name in your own file aren't changed when a project renames the skill.

When your skill's `agents/openai.yaml` sets only `interface.display_name`, `short_description`, or `default_prompt`, `trailguide library check` warns and suggests the metadata keys instead, so the file isn't tied to one agent. It doesn't warn about a file with any other setting.

## Links

A skill's Markdown can link to web pages, to its own files, and to other files in its repository: a sibling skill it compares itself with, a shared checklist in `assets/`, or a diagram outside `.trailguide/`. Write each link as a path relative to the file it's in, such as `references/slicing.md`, `../redesign/SKILL.md`, or `../../assets/review-checklist.md`, so it works while you read the repository. Installed copies keep working too: when a project installs the skill, Trail Guide keeps a copy of every file its links reach, from the same revision, and changes the installed links to point at those copies. See [Files a skill links to](/concepts/skill/#files-a-skill-links-to).

Trail Guide reads Markdown links, images, and reference definitions in every Markdown file of the skill, and in every Markdown file those links reach, so a link from a linked file is kept too. Links in inline code, code blocks, and prose aren't links, and links in raw HTML aren't followed. External URLs, links that start with `#` or `?`, and other addresses with a scheme, such as `mailto:`, stay as they are and are never downloaded. Files other than Markdown, such as scripts, aren't read for links.

A local link must name a file in the skill's repository. It fails when it names:

- A file the revision doesn't have.
- A folder, a symbolic link, or a submodule.
- An absolute path, a path that leaves the repository, or one with backslashes.
- A rule file, or another file in `.trailguide/` that isn't in a skill folder or `assets/`. Cite rules with `trailguide-rules-used` instead, and the installed copy links to the project's version of each one.

A linked file can be anywhere else in the repository, including a folder at the repository root that has the name of one in `.trailguide/`, such as `assets/notes.md`. A project keeps each linked file from outside `.trailguide/` under `_repository/` in its copy of the library, at its path from the repository root, so it never takes the place of one of the library's own files. A linked file outside the skill folder isn't a skill: a project keeps it to read and never installs it in an agent's skills folder.

A project's own skills, in its `.trailguide/skills/`, including forks that replace a source's skill, follow the same rules, checked against the project's files as the command leaves them, with or without skill targets. A link into a skill target folder, such as `../../../.claude/skills/review/SKILL.md`, must name a skill the command installs, and a link into `.trailguide/_managed/vendor/` fails, since sync replaces those files. A link to a file Trail Guide generates, such as `.trailguide/_managed/generated/SKILLS.md`, must name one the command writes; Trail Guide checks that file's own links when it generates it. Their installed links point at the project's file where it is, or at the installed copy of another of the project's skills. When a linked file changes while the command runs, it fails with `concurrent-change` and writes nothing.

## Licenses

A library's license, declared in its [manifest](/reference/library-schema/#license-metadata), covers its skills as it covers its rules. The `license` field in `SKILL.md` is informational: Trail Guide preserves it, and when it differs from the library's SPDX expression, `trailguide library check` warns. Skills from a non-library source have no library declaration, so their license comes from the skill or the repository instead. See [License rules](/guides/license-rules/#license-skills).

## Validation

`trailguide library check` validates every skill in the library, whatever a project selects. It reports every skill problem it finds, one per line, each starting with the file or folder it's about, and fails with the code `invalid-skill`:

```text
$ trailguide library check
Error: skills don't pass library check:
  - skills/build-feature/SKILL.md: trailguide-rules-used names practices/testing/test-behaviour-not-implementation, which isn't a rule in this library
  - skills/build-feature/SKILL.md: trailguide-skills-used names skills/tdd-loop, which isn't a skill in this library
  - skills/build-feature/SKILL.md: trailguide-skill-type is design; use playbook or review, or leave it out
```

Check fails with these problems:

| Problem | Message |
| --- | --- |
| A file directly in `skills/` | `skills/README.md: skills/ holds only skill folders` |
| Two skills the manifest lists, authored or curated, with the same name | `library.skills lists skills/review and mattpocock:skills/review, which both install as review; list one of them, or rename the folder of your own skill` |
| A skill listed in more than one level | `library.skills lists skills/fix-bug as both required and recommended; list it once` |
| A skill listed both at a level and as a dependency | `library.skills.dependencies[0]: skills/format-skill is already offered, so projects can choose it; a dependency installs only with the skills that use it, so list it in one place` |
| A dependency that no skill of the library uses | `library.skills.dependencies lists skills/format-skill, but no skill of the library names it in trailguide-skills-used, and a dependency installs only with the skills that use it, so no project would install it. Name it in the trailguide-skills-used of the skills that call it, or move it to a level` |
| A dependency that only unreachable dependencies use | `library.skills.dependencies lists skills/lint, but only other dependencies that no project would install name it in trailguide-skills-used, and a dependency installs only with the skills that use it, so no project would install it. Name it in the trailguide-skills-used of a skill a project can choose, or move it or a dependency that uses it to a level` |
| A listed skill that doesn't exist | `library.skills.recommended names skills/fix-bugs, which isn't a skill in this library` |
| A curated skill whose source isn't in `sources`, or that the source doesn't have at its `ref` | `library.skills.offered names mattpocock:skills/productivity/grill, which mattpocock doesn't have at main` |
| A folder name that breaks the [name grammar](#skill-ids) | `skills/Build_Feature: a skill name uses 1 to 64 lowercase letters, digits, and hyphens, without a leading, trailing, or doubled hyphen` |
| A skill folder without `SKILL.md` | `skills/build-feature: expected SKILL.md in the skill folder` |
| A skill inside another skill's folder | `skills/build-feature/steps/SKILL.md: a skill folder can't contain another skill; move it to its own folder in skills/` |
| Rule group metadata in a skill folder | `skills/build-feature/_group.yaml: a skill folder can't contain group metadata` |
| A symbolic link, submodule, or Git LFS pointer | `skills/build-feature/scripts/verify.sh: symbolic links, submodules, and Git LFS pointers aren't supported in a skill folder` |
| A Markdown file that isn't UTF-8 | `skills/build-feature/references/slicing.md: expected UTF-8 Markdown` |
| No frontmatter at the start of `SKILL.md` | `skills/build-feature/SKILL.md: expected YAML frontmatter at the start of the file` |
| Invalid YAML, such as a duplicate key or an alias | `skills/build-feature/SKILL.md: ` followed by the YAML problem and its line |
| A blank body | `skills/build-feature/SKILL.md: expected instructions after the frontmatter` |
| An unfinished draft | `skills/build-feature/SKILL.md: complete the draft and remove its trailguide:draft marker` |
| An unknown top-level field | `skills/build-feature/SKILL.md: unknown field version; SKILL.md accepts name, description, license, compatibility, allowed-tools, and metadata` |
| A missing or blank `name` or `description` | `skills/build-feature/SKILL.md: description is required` |
| A `name` that differs from the folder name | `skills/build-feature/SKILL.md: name is build-features, but the folder is build-feature; they must match` |
| A `description` over 1,024 characters | `skills/build-feature/SKILL.md: description is 1,310 characters; use at most 1,024` |
| A `compatibility` over 500 characters, or blank | `skills/build-feature/SKILL.md: compatibility is 612 characters; use at most 500` |
| A field or `metadata` value that isn't a string | `skills/build-feature/SKILL.md: metadata.trailguide-skill-type must be a string` |
| A reserved `trailguide-` key Trail Guide doesn't define | `skills/build-feature/SKILL.md: metadata.trailguide-groups isn't a Trail Guide key; use trailguide-skill-type, trailguide-rules-used, or trailguide-skills-used` |
| A type that isn't on the canonical list | `skills/build-feature/SKILL.md: trailguide-skill-type is design; use playbook or review, or leave it out` |
| A cited rule that doesn't exist | `skills/build-feature/SKILL.md: trailguide-rules-used names practices/testing/test-behaviour-not-implementation, which isn't a rule in this library` |
| A cited rule with a source name or `.md` | `skills/build-feature/SKILL.md: trailguide-rules-used names fabrica:practices/testing/test-behavior-not-implementation; cite a rule by its ID in this library, such as practices/testing/test-behavior-not-implementation` |
| A cited rule that a library release or a pending change note retired, with a replacement | `skills/build-feature/SKILL.md: trailguide-rules-used names practices/testing/check-retry-backoff, which is retired and replaced by practices/testing/verify-retries; cite the replacement` |
| A cited rule that was retired without a replacement | `skills/build-feature/SKILL.md: trailguide-rules-used names practices/code-design/comment-intent, which is retired; remove it` |
| An ID listed twice | `skills/build-feature/SKILL.md: trailguide-rules-used names practices/testing/verify-retries twice` |
| A used skill that doesn't exist | `skills/build-feature/SKILL.md: trailguide-skills-used names skills/tdd-loop, which isn't a skill in this library` |
| A used curated skill that `library.skills` doesn't list | `skills/add-skill/SKILL.md: trailguide-skills-used names anthropic:skills/skill-creator, which isn't a skill the library curates; list it in library.skills, at a level or as a dependency, so projects that install the skill can install it too` |
| A used curated skill whose source name `library.skills` doesn't use | `skills/add-skill/SKILL.md: trailguide-skills-used names anthropik:skills/skill-creator, but library.skills lists no skill from a source named anthropik; the library curates skills from anthropic. Correct the source name, or name a skill of this library as skills/<name>` |
| A curated skill named without its source name | `skills/add-skill/SKILL.md: trailguide-skills-used names skills/skill-creator, which isn't a skill in this library; the library curates anthropic:skills/skill-creator, so name it that way` |
| A used skill that is retired | `skills/build-feature/SKILL.md: trailguide-skills-used names skills/triage-bug, which is retired and replaced by skills/fix-bug; use the replacement`, or `which is retired; remove it` when nothing replaces it |
| A skill that uses itself | `skills/build-feature/SKILL.md: trailguide-skills-used names the skill itself` |
| Skills that use each other in a cycle | `skills/build-feature/SKILL.md: trailguide-skills-used forms a cycle: skills/build-feature -> skills/tdd -> skills/build-feature` |
| A local link to a file the library doesn't have, including one Git ignores, or a link from a file a skill's links reach, with code `reference-missing` | `.trailguide/skills/build-feature/SKILL.md: links to references/slices.md (.trailguide/skills/build-feature/references/slices.md), which the library doesn't have. Add the file, or fix the link; projects that install the skill keep a copy of every file its links reach.` |
| A link to a folder, a symbolic link, a rule file, or another file a project can't keep, with code `reference-unsupported` | `.trailguide/skills/build-feature/SKILL.md: links to ../../rules/practices/testing/verify-retries.md (.trailguide/rules/practices/testing/verify-retries.md), which is a file in .trailguide/ that isn't a skill's file or a shared asset in .trailguide/assets/, such as a rule, which installed skills cite instead of linking. Ask the maintainer to cite a rule with trailguide-rules-used instead of linking to it, or to move the file out of .trailguide/.` |
| An absolute link, or one that leaves the repository, with code `reference-escapes` | `.trailguide/skills/build-feature/SKILL.md: links to /etc/checklist.md, which is an absolute path, so it depends on where the files are read rather than on the repository. Ask the maintainer to link with a path relative to the file.` |
| A skill that reuses the ID of a retired skill | `skills/triage-bug reuses the ID of a skill that release/5 retired. Retired IDs can't be reused; give the skill a new ID.` |

After the first library release, check also compares every skill with the latest library release and fails, with the code `change-notes`, when a skill's change notes don't match its changes, exactly as for rules. See [Change notes](/reference/rule-versions/#change-notes) and [library check](/reference/cli/#library-check).

Check warns, without failing, when:

- A skill's `license` differs from the library's declared SPDX expression: `skills/build-feature/SKILL.md: license is Apache-2.0, but the library declares MIT. The library's license applies to its skills; make them match.`
- A skill declares a `license` and the library declares none: `skills/build-feature/SKILL.md: license is MIT, but the library declares no license. Declare the library's terms in library.license.`
- A file in a skill folder is neither tracked nor ignored by Git: `skills/build-feature/scripts/new-check.sh isn't tracked by Git, so it isn't part of the skill. Add it with git add, or ignore it.`
- A skill ships its own `agents/openai.yaml` that sets only the text the metadata keys generate: `skills/add-skill/agents/openai.yaml: it sets only interface text, which Trail Guide generates for each project from SKILL.md. Set metadata.display-name, short-description, and default-prompt in SKILL.md instead, then delete this file.` See [Text agents show](#text-agents-show).

To find curated skills' names, check reads each curated source at its `ref`, so it needs access to those repositories.

Check validates the format, not whether the steps are good. Use [Write a skill](/guides/write-skills/) to review a skill's guidance.

## Installed copies

When a project installs a skill, Trail Guide writes a copy of its folder into each of the project's skill targets, such as `.claude/skills/build-feature/`. Every file is copied as the source has it, with its executable bit, and `SKILL.md` gets two changes, plus a new `name` when the project [renames](/reference/configuration/#rename-a-skill) the skill:

- A marker comment becomes the first line after the frontmatter, so the frontmatter still starts the file:

  ```markdown
  <!-- trailguide:installed fabrica:skills/build-feature 1.3.0. Do not edit; fork it with trailguide project add skill --from. -->
  ```

- When the skill cites rules, a generated **Rules this skill applies** section is appended at the end. It links to the project's version of each rule the skill cites, including an authored rule that replaces one, and marks a rule the project excluded. A **Skills this skill uses** line follows when the skill calls others.

The copy in `.agents/skills/` also gets a generated `agents/openai.yaml` when the skill sets [interface text](#text-agents-show) and doesn't ship that file itself.

Don't edit installed copies: sync rewrites them, and `trailguide project check` fails when one differs from what sync would write. [Installed skills](/reference/files/#installed-skills) shows the complete generated section and how Trail Guide records the folders it owns.

## Skills in a project

A repository that isn't a library authors skills in the same `.trailguide/skills/<name>/` folders, with the same format, and two differences:

- They can cite rules and call skills from the project's sources, by project ID with the source name, because a project can use several sources: `fabrica:practices/testing/test-behavior-not-implementation` or `fabrica:skills/tdd`. Bare IDs, such as `practices/architecture/follow-bounded-contexts`, name the repository's own rules and skills. Trail Guide takes each cited rule, and installs each called skill, from the source it names.
- They have no versions and need no change notes.

`trailguide project add skill` creates one, and `trailguide project add skill --from` forks a skill from a source into one; see [project add skill](/reference/cli/#project-add-skill). Sync and build validate these skills with the checks above and fail on the first problem. `trailguide project check` also fails when one cites a rule its library retired, naming the replacement when there is one, so a project's skills never point at a rule that's gone.

## Skills from non-library sources

A repository without a library manifest, such as a third party's skill collection, can supply skills too. Each folder in it that has a `SKILL.md` file is a skill, and its ID is the folder's path from the repository root, such as `skills/engineering/to-spec`. Trail Guide checks these skills leniently: where a skill breaks this format, such as a `name` that doesn't match its folder or a missing `description`, sync warns and installs the skill as it is. The checks that protect your project still fail: unsafe paths, [links](#links) the project can't keep working, symbolic links, submodules, Git LFS pointers, and the [source limits](/reference/imports/#source-limits-and-version-errors). A non-library source's `trailguide-rules-used` lists are ignored, because it has no rules. Its `trailguide-skills-used` lists name skills in the same repository; the project declares calls they leave out with `skills.uses`. See [Non-library sources](/reference/imports/#non-library-sources).

## Related pages

- [Skill](/concepts/skill/) explains what a skill is and how it relates to rules.
- [Write a skill](/guides/write-skills/) walks through writing, checking, and versioning a skill.
- [Library schema](/reference/library-schema/) covers where skills live in a library.
- [Rule and skill versions](/reference/rule-versions/) defines what a skill version covers.
