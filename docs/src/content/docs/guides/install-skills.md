---
title: "Install skills"
description: "Install skills from libraries and other repositories into the folders your agents read, and manage their levels, names, and exceptions."
---

A **[skill](/concepts/skill/)** tells an agent how to carry out one kind of task, step by step, and which rules each step applies. Installing skills with Trail Guide gives every checkout of your project the same skills, from the revisions you choose, beside the rules they cite.

Skills come from **sources**: the repositories your project takes skills and rules from. A source is usually a [library](/concepts/libraries/), but any Git repository of skill folders works too.

In this guide, you'll see which skills your sources offer, install a library's required and recommended skills, and find out where they land. You'll also install skills from a repository that isn't a library, resolve a name clash, exclude or fork a skill, write your own, and commit the result.

Start with a project that already has a `.trailguide/` directory. If you haven't created one, follow [Set up your first project](/start-here/set-up-project/). To choose rules as well, see [Include rules](/guides/select-rules/).

## Check your skill targets

Trail Guide installs a copy of each skill into the folders your agents read. `skillTargets` in `.trailguide/config.yaml` names them:

```yaml
schemaVersion: 2
skillTargets: [claude, agents]
```

`claude` installs into `.claude/skills/`, which Claude Code reads. `agents` installs into `.agents/skills/`, which Codex, Cursor, and other agents that follow the shared convention read. `trailguide project init` writes both, and nothing is written to either folder until the project installs a skill.

To record which skills your sources require before you choose agent folders, set `skillTargets: []`. Trail Guide still records and checks every skill and lists it in `.trailguide/generated/SKILLS.md`, but writes no installed copies, and sync warns that it installed nothing. See [Skill targets](/reference/configuration/#skill-targets).

## Add a library

Add your organization's library as a source:

```sh
trailguide project add source fabrica \
  --repository https://github.com/fabricahq/.trailguide.git \
  --skills recommended \
  --non-interactive
```

This records the source in `.trailguide/config.yaml`:

```yaml
sources:
  fabrica:
    repository: https://github.com/fabricahq/.trailguide.git
    ref: release/4
    skills:
      include: [recommended]
```

Then sync:

```sh
trailguide project sync
```

What a library's skills install depends on the **level** the library sets for each one:

- **Required** skills always install. Even a source with no `include` at all installs them, now and after every update that adds one.
- **Recommended** skills install because you included `recommended`.
- **Offered** skills install only when you name them, or include `"*"` for everything the library offers.

A library can also list a skill as a **dependency**: it installs with the library's skills that call it, and you can't include it on its own. `trailguide project list skills` shows it with the level `dependency`.

To add an offered skill, name its ID beside `recommended`:

```yaml
skills:
  include: [recommended, skills/deploy-preview]
```

Including a rule group's rules never installs a skill, and including a skill never includes a rule group's rules. See [Skill selectors](/reference/configuration/#skill-selectors).

Sync takes the source's newest library release, records it in `.trailguide/lock.json`, and installs every skill from that one release. Later syncs keep that release, even for skills you include later: a skill that isn't in the recorded release makes sync fail and name `trailguide project update fabrica`, the command that moves the source. See [Update skills](/guides/update/#update-skills).

## See what's available

List the skills in your project, with their levels:

```sh
trailguide project list skills
```

```text
SKILL                                            LEVEL        SET BY              STATUS
fabrica:mattpocock:skills/engineering/to-spec    recommended  fabrica, release 7  installed
fabrica:mattpocock:skills/productivity/grilling  offered      fabrica, release 7  available
fabrica:skills/build-feature                     recommended  fabrica, release 7  installed
fabrica:skills/deploy-preview                    offered      fabrica, release 7  installed
fabrica:skills/review-change                     offered      fabrica, release 7  available
fabrica:skills/ship-service                      required     fabrica, release 7  installed
fabrica:skills/tdd                               offered      fabrica, release 7  installed
```

Each row shows a skill's ID, its level and the library release that set it, and whether the project installs it. `installed` covers skills you included, required skills, and skills another installed skill uses; `available` means a source offers it and the project doesn't install it. The list comes from the library release your project recorded, so it doesn't show skills a newer release adds until you run `trailguide project update fabrica`.

The `mattpocock:` skills are **curated**: Fabrica's library lists them at a level, but their files come from the original repository, at a commit your project resolves when it updates. See [Curated skills](/reference/configuration/#curated-skills).

Add `--json` to get the list for scripts. See [project list skills](/reference/cli/#project-list-skills) for every column and status.

## What lands where

After sync, each installed skill appears in several places. Trail Guide writes all of them, so don't edit them by hand:

- **`.claude/skills/build-feature/`** and **`.agents/skills/build-feature/`**: the installed copies your agents load, one folder per target.
- **`.trailguide/vendor/fabrica/skills/build-feature/`**: the original files, as the library published that version.
- **`.trailguide/generated/SKILLS.md`**: one entry per installed skill, with its description, ID and version, level and the library that set it, type, installed paths, how many rules it applies, the skills it calls, and why it's installed.
- **`.trailguide/generated/installed-skills.json`**: which folders in the targets Trail Guide installed.

An installed copy matches the library's folder, scripts and their executable bits included, with two additions to `SKILL.md`:

- A marker comment as the first line after the frontmatter, naming the skill's project ID and version, such as `<!-- trailguide:installed fabrica:skills/build-feature 1.3.0. Do not edit; fork it with trailguide project add skill --from. -->`.
- When the skill cites rules, a generated section at the end, **Rules this skill applies**, which links to your project's version of each rule the skill cites, and a **Skills this skill uses** line when it calls others.

Don't edit installed copies: the next sync replaces them, and `trailguide project check` reports the difference. To change a skill, [fork it](#fork-a-skill).

Trail Guide writes only the skill folders it installs. Other folders in `.claude/skills/` and `.agents/skills/`, such as a skill another tool installed, stay as they are. When one of them has the name of a skill you install, sync fails with `skill-folder-not-managed` instead of overwriting it. See [Installed skills](/reference/files/#installed-skills) and [Who owns the target folders](/reference/sync/#who-owns-the-target-folders).

`generated/RULES.md` gains one line pointing to `SKILLS.md`, so agents whose tools don't load skills from the target folders still find them through the instructions in your `AGENTS.md`.

## Skills a skill calls, and the rules it cites

A skill can call other skills and cite rules. When you install it, Trail Guide installs those too:

- **Skills it calls.** Every skill in its `trailguide-skills-used` list, and the skills those call, from the same repository and revision, or, for a skill the library [curates](/reference/configuration/#curated-skills), from the repository the library curates it from. `SKILLS.md` says each was installed because another skill uses it.
- **Rules it cites.** Every rule in its `trailguide-rules-used` list, from the same library, even ones your rule `include` doesn't name. These cited rules are reference-only: each gets its generated rule file, and the installed skill links to it, but `RULES.md` doesn't list it. Installing a skill never widens the rules your agents apply to all work.

If you exclude or replace a cited rule, the installed skill follows your choice: it links to your replacement, or says the rule is excluded by this project, with your reason. A cited rule your project also includes is an ordinary rule.

When a skill calls another without saying so in `trailguide-skills-used`, as many third-party skills do, declare the call under `skills.uses`, so Trail Guide installs both:

```yaml
skills:
  uses:
    skills/architect: [skills/how]
```

Trail Guide doesn't search skill text for calls it wasn't told about. See [Declare the skills a skill calls](/reference/configuration/#declare-the-skills-a-skill-calls).

## Install skills from a repository that isn't a library

Not every useful skill lives in a library. Any Git repository that holds skill folders works as a source, such as a third party's collection. Add it the same way:

```sh
trailguide project add source pstack \
  --repository https://github.com/example/pstack.git \
  --ref main \
  --skills skills/architect \
  --non-interactive
```

```yaml
sources:
  pstack:
    repository: https://github.com/example/pstack.git
    ref: main
    skills:
      include: [skills/architect]
```

Then run `trailguide project sync`. You don't declare what kind of source it is: Trail Guide finds that the repository has no library manifest, so it's a **non-library source**.

- **Every skill is offered.** Nothing installs until you name it, or include `"*"` for every skill in the repository.
- **A ref is required.** A non-library source has no library releases, so `ref` must name a branch, tag, or commit. With a branch, sync installs the newest commit on it the first time and records it; later syncs keep that commit, and `trailguide project update` moves to the branch's newest commit and shows which skill folders and files changed.
- **IDs are paths.** Each folder with a `SKILL.md` is a skill, and its ID is the folder's path in the repository, such as `skills/architect`.
- **Expect warnings, not failures.** Trail Guide checks these skills leniently: where one doesn't follow the Agent Skills format, sync warns and installs it as it is. A non-library source has no rules or versions, so `rules` is rejected on it.
- **Check the license.** Trail Guide looks for terms in the skill's `license` field, then a `LICENSE*` file in its folder, then `LICENSE*` and `NOTICE*` files at the repository root. When a skill has none, sync fails with `license-not-found`; ask the owner to add a license, or stop including the skill.

Trail Guide never runs a skill's scripts. Before you install or update a skill from a repository you don't control, read its files, scripts included: your agents may run them. To help, the `trailguide project update` preview flags each new or changed executable script and warns you to read it; see [Update skills](/guides/update/#update-skills).

## Try a newer version of a curated skill

A library chooses the `ref` it tracks for each repository it curates skills from, and your project resolves that ref when it updates. To take a curated skill from another revision, declare the same repository as your own source, with your own `ref`:

```yaml
sources:
  fabrica:
    repository: https://github.com/fabricahq/.trailguide.git
    ref: release/4
    skills:
      include: [recommended]
  mattpocock:
    repository: https://github.com/mattpocock/skills.git
    ref: v2.0.0
```

The skills Fabrica curates from that repository now come from `v2.0.0`, still at the level Fabrica sets and with the same IDs, such as `fabrica:mattpocock:skills/engineering/to-spec`. Repository identity decides the match, not the source name. Remove the source to go back to the library's ref.

## Resolve a name clash

Agents find skills by name, so two installed skills can't share one. When two sources ship skills with the same name, sync fails with `skill-name-clash` and writes nothing:

```text
Error: skill-name-clash: fabrica:skills/review-change and pstack:skills/review-change both install as review-change. Rename one with skills.rename, or exclude one. No files were written.
```

Rename one of them:

```yaml
sources:
  fabrica:
    repository: https://github.com/fabricahq/.trailguide.git
    ref: release/4
    skills:
      include: [recommended, skills/review-change]
      rename:
        skills/review-change: fabrica-review
```

The installed copy is now `fabrica-review`, in `.claude/skills/fabrica-review/`. Its ID stays `fabrica:skills/review-change`. Trail Guide doesn't rewrite the old name where other skills mention it, so every sync that applies the rename warns:

```text
Warning: fabrica:skills/review-change is installed as fabrica-review. Mentions of review-change in skill text aren't rewritten; check the skills that call it by that name.
```

Check whether any installed skill tells the agent to use `review-change`, the renamed skill's own text included. After the clash, `review-change` is the other source's skill, so an agent following that mention runs the wrong one. [Fork](#fork-a-skill) any skill you need to change. You can rename a required skill too, because it stays installed.

## Exclude a skill

Exclude a skill you don't want, such as one that `recommended` or `"*"` brings in, with a reason:

```yaml
sources:
  fabrica:
    skills:
      include: [recommended]
      exclude:
        - id: skills/write-changelog
          reason: Release Planner writes our release notes.
```

These are partial snippets, not complete source definitions. An excluded skill isn't installed, but it stays in `vendor/`, so updates can show you its changes. When another installed skill calls it, sync warns that the skill it calls isn't installed.

You can't exclude or fork a [required skill](/reference/configuration/#skill-levels): sync fails and names the library that requires it. Required skills are your organization's floor; to diverge from one, stop using the source.

You can't pin a skill to a version either: all of a source's skills come from one revision and move together when the source updates. To keep a skill as it is, [fork it](#fork-a-skill), or leave the source at the library release its `ref` names, which holds every rule and skill from that source. See [Update skills](/guides/update/#update-skills).

## Fork a skill

To change what an included skill says, fork it into your project:

```sh
trailguide project add skill \
  --from fabrica:skills/fix-bug \
  --reason 'We use our own bug workflow.' \
  --non-interactive
```

The command copies the version your project installs into `.trailguide/skills/fix-bug/`, and records the fork under the source:

```yaml
skills:
  forks:
    skills/fix-bug:
      as: skills/fix-bug
      basedOn: "9f2c4e1a7b3d5f60817a2b4c6d8e0f1a3b5c7d9e"
      reason: We use our own bug workflow.
```

Edit the fork, then run `trailguide project build`. Agents get your fork in place of the library's skill, which the project no longer installs. `basedOn` is the commit the fork copied. `trailguide project update` lists the library's later changes to the skill and can merge them into your fork, keeping your edits, with `--merge-fork fabrica:skills/fix-bug`. The library's license still applies to the copied text. For the whole workflow, including changes that conflict with your edits, see [Fork a skill](/guides/fork-skills/).

## Write your own skill

Create a skill for this project alone:

```sh
trailguide project add skill review-migration \
  --description 'Review a database migration for safety before it merges. Use when a change adds or edits a migration.' \
  --type review \
  --non-interactive
```

The command writes a draft `.trailguide/skills/review-migration/SKILL.md`. Authored skills have no versions; they change when you edit them. In `trailguide-rules-used`, an authored skill cites rules by project ID, such as `fabrica:practices/testing/test-behavior-not-implementation` or `practices/testing/test-project-contracts` for one of your own, because it can cite rules from any source. Write its steps and remove the `trailguide:draft` marker, then run `trailguide project build` to install it; build fails while the marker is there. Follow [Write a skill](/guides/write-skills/) for how to write one that works.

## Personal skills with the same name

People also install skills for themselves, outside any repository, such as in `~/.claude/skills/`. When one of those **personal skills** has the same name as a skill your project installs, each agent decides which to use, and some use the personal one without saying so. `trailguide project sync`, `update`, and `check` read the known personal skill folders and warn about each pair, naming both paths and what each agent does:

```text
Warning: your personal skill ~/.claude/skills/build-feature has the same name as this project's build-feature (fabrica:skills/build-feature, installed at .claude/skills/build-feature).
  Claude Code uses your personal skill.
  Grok Build reads both, and its docs don't say which it uses.
  VS Code reads both, and its docs don't say which it uses.
Rename or remove your personal skill to use the project's.
```

Trail Guide never writes to your home directory, and the warning doesn't fail the command. See [Personal skills with the same name](/reference/sync/#personal-skills-with-the-same-name).

## Commit the installed skills

Commit `.trailguide/` and the installed skill folders together:

```sh
git add .trailguide .claude/skills .agents/skills
```

Committed copies give every checkout, CI job, and fresh cloud session the same skills without running sync. Run `trailguide project check` in CI: it fails when an installed copy differs from what sync would write, when a folder Trail Guide installed is missing, or when `generated/installed-skills.json` is out of date.

If your `.gitignore` excludes a target folder, such as with `/.claude/`, check reports the folders as missing in every fresh checkout. Add an exception for the skills folder. Git can't re-include a folder inside an excluded folder, so first change the ignore line to exclude the folder's contents instead:

```text
/.claude/*
!/.claude/skills/
```

This keeps ignoring the rest of `.claude/`, such as local settings, while committing installed skills.
