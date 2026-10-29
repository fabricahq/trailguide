---
title: "Skill"
description: "A reusable procedure that tells an agent what to do, step by step, and which rules each step applies."
---

A **skill** is the AI skill you already know, officially a folder that follows the open [Agent Skills](https://agentskills.io) standard. It holds a `SKILL.md` file that tells an agent how to carry out one kind of task, plus any scripts, references, or assets it bundles.

Skills are coarse-grained. A skill that builds a feature combines many practices, from where the code lives to how it's tested and verified, and not all of them apply every time. That makes skills good for playbooks, techniques, and coordinating a whole task, but clumsy as the only way to deliver guidance: you can't adopt, update, or version one practice inside a skill on its own.

[Rules](/concepts/rule/) are the granular layer: one best practice per file, with its own version. A skill cites the rules it applies, so each practice is written once, and every skill that applies it stays current.

Trail Guide installs skills into your repository, records where each one came from and which version it is, and checks them. It doesn't run skills, decide when an agent uses one, or check that the agent followed it.

## What a skill contains

A skill uses the standard format from the [Agent Skills specification](https://agentskills.io/specification): a folder named after the skill, holding a `SKILL.md` file and any scripts, references, or assets it bundles.

```text
build-feature/
  SKILL.md
  references/
    slicing.md
  scripts/
    verify.sh
```

`SKILL.md` starts with YAML frontmatter, followed by the instructions. The standard requires two fields: `name`, which agents find the skill by, and `description`, which agents read to decide when to use it.

Trail Guide extends the format with three keys of its own, under the standard's `metadata` field:

```markdown
---
name: build-feature
description: Build a feature from an agreed spec, one slice at a time. Use when starting implementation of a new feature or a change to an existing one.
metadata:
  trailguide-skill-type: playbook
  trailguide-rules-used: |
    practices/testing/test-behavior-not-implementation
    practices/verification/verify-on-the-surface-users-touch
  trailguide-skills-used: |
    skills/tdd
---

1. Read the spec and name the slice you're building.
2. Decide where the code lives before writing it.
...
```

- `trailguide-skill-type` labels what kind of skill it is, `playbook` or `review`. See [Skill types](#skill-types).
- `trailguide-rules-used` lists the rules the skill applies. Trail Guide takes those rules with the skill, and the installed copy links to your project's version of each one. See [Where cited rules live](#where-cited-rules-live).
- `trailguide-skills-used` lists other skills the skill calls. Trail Guide installs them alongside it.

A skill can also set the text an agent's interface shows for it, with the `display-name`, `short-description`, and `default-prompt` keys. They name no agent, and Trail Guide maps them into the file each agent reads, such as `agents/openai.yaml` for Codex, when it installs the skill. See [Text agents show](/reference/skill-format/#text-agents-show).

All these keys are optional. The standard sets `metadata` aside for properties it doesn't define, so a skill with these keys is still a standard skill, and agents that don't read them use it as usual. For every field and check, see [Skill format](/reference/skill-format/). To write one, follow [Write a skill](/guides/write-skills/).

## Skill types

A skill type classifies what a skill is for, so skills are easier to search and browse. The payoff comes from interfaces that show types, such as a catalog like [Rulemart](https://rulemart.fabricahq.com) that lets you browse every review skill across libraries.

There are probably many useful kinds of skills, but we're conservative about which types we name, because removing or redefining a type later would break the skills that use it. For now there are two, and a skill can declare one:

| Type | What the skill does | Examples |
| --- | --- | --- |
| `playbook` | Ordered steps for carrying out one kind of engineering task, citing the rules each step applies. | Build a feature, fix a bug, migrate a dependency |
| `review` | Examines existing work, such as a diff, plan, test suite, or rendered UI, against stated criteria and reports findings. | Review a pull request, audit a test suite |

The test is the skill's main job: a playbook changes the work, and a review judges work that already exists. A review may offer to fix what it found, and a playbook may include a review step.

If a skill doesn't clearly fit either type, leave the type out. The type is only a label: Trail Guide installs, checks, and updates typed and untyped skills the same way.

## Where cited rules live

A skill cites rules from its own repository, so the rules it depends on always travel with it:

- **A library's skill** cites only rules in the same library, by their ID there, such as `practices/testing/test-behavior-not-implementation`. Any project that installs the skill from that library gets those rules too, at the same release.
- **A skill you author in your project** can cite your project's own rules, and also rules from the project's sources by project ID, such as `fabrica:practices/testing/test-behavior-not-implementation`.
- **A skill from a repository that isn't a library** has no rules to cite, so Trail Guide ignores its `trailguide-rules-used`.

In a library, `trailguide library check` fails when a skill cites a rule that doesn't exist or was retired, so a typo never reaches an agent. See [Rules a skill applies](/reference/skill-format/#rules-a-skill-applies).

## Files a skill links to

A skill's instructions often link to other files: a sibling skill it compares itself with, a shared checklist, or a diagram. Those links keep working wherever the skill is installed. When a project installs the skill, Trail Guide keeps a copy of every file the skill's links reach, and every file those files link to, from the same revision of the same repository. For a skill a library curates, that's the repository the library curates it from. It changes the installed links to point at those copies, in every agent's skills folder.

A linked file is for reading, not a skill to run. Linking to a sibling skill doesn't install it, add it to `SKILLS.md`, or put it in an agent's skills folder; to install it, include it, or have the skill call it with `trailguide-skills-used`. When the linked skill is installed anyway, the link points at its installed copy, under the name the project gives it, or at the project's fork of it.

Trail Guide never overrides your choices to keep a link working. When a link points into a skill your project excludes, or into a fork that lacks the file, sync fails before changing anything and lists the repairs. Web links stay as they are and are never downloaded. See [Links](/reference/skill-format/#links).

## Skills and rules

Keep each practice in a rule, and cite it from the skills that apply it. When a skill needs a practice followed, it cites the rule rather than restating it:

- A project can exclude or replace the rule, and every skill that cites it picks up the project's choice.
- When the rule gets a new version, every skill that cites it applies the new version, without a change to the skill.
- Reviewers check work against the same rules the implementing agent followed.

Skills don't belong to [rule groups](/concepts/groups/), and including a rule group's rules never installs a skill. When you install a skill, Trail Guide also takes the rules it cites, even ones your rule `include` doesn't name. These **cited rules** are for reference: the skill links to them, but they don't join `RULES.md`, so installing a skill never widens the rules that apply to all of your project's work.

## Where skills come from

A repository gets skills from three places:

- **Libraries.** A [library](/concepts/libraries/) publishes the skills in its `.trailguide/skills/` folder, and can list skills it **curates** from other repositories. It sets a level for each one; see [Skill levels](#skill-levels).
- **Non-library sources.** Any Git repository of skills that hasn't adopted Trail Guide, so it has skill folders but no library manifest, such as a third party's skill collection. Every skill in it is offered. Because it has no library releases, its `ref` names a branch, tag, or commit, and Trail Guide records the exact commit.
- **Your repository.** Skills you author live in `.trailguide/skills/`.

When a skill calls others in the same repository, Trail Guide installs them alongside it from that repository and revision.

A library's own skill can also call a skill the library curates from another repository. For example, Fabrica's library authors `add-skill` and curates Anthropic's `skill-creator` from the anthropics/skills repository. When `add-skill` lists `anthropic:skills/skill-creator` in its `trailguide-skills-used`, a project that installs `add-skill` also gets `skill-creator`, with its files taken from anthropics/skills, not from Fabrica's library.

A skill's ID is its folder's path in the repository that holds it, such as `skills/build-feature`. Its identity is that repository plus that path, so two sources that reach the same repository name the same skill. In a project, an ID is qualified by the source name you chose, such as `fabrica:skills/build-feature`. Configuration uses these IDs to include, exclude, rename, and replace skills.

A project can [exclude](/guides/install-skills/#exclude-a-skill) a source's skill it doesn't want, with a reason, or [fork](/guides/fork-skills/) it into an authored skill that installs in its place under the same name. Updates list the source's later changes to a skill the project forked, and can merge them into the fork while keeping the project's edits.

See [Install skills](/guides/install-skills/) to add skills to a project.

## Skill levels

A library sets a **level** for each skill it publishes, saying how strongly it asks its projects to install it:

| Level | What a project that uses the library gets |
| --- | --- |
| **Required** | Always installed, whatever the project's `include` says. The project can't exclude or fork it. |
| **Recommended** | Installed when the project includes `recommended`. |
| **Offered** | Installed when the project names it, or includes `"*"`. |

A library's own skill that no level lists is offered, so a library lists only the skills it has an opinion about. An organization can make some skills the floor for every project by requiring them in its org library: a project that takes anything from that library installs them. A project sees only the levels of the libraries it names; levels don't chain through a library's own sources.

A library can also list a skill as a **dependency**, which projects install only with the library's skills that call it and can't choose on their own, such as a third party's skill that the library's own skill wraps in its workflow. See [Dependencies](/reference/configuration/#dependencies).

A library can also list skills from another repository at a level, such as a third party's skill its teams should use. Projects fetch those **curated skills** from the original repository, at a commit each project resolves when it updates, and record which library required, recommended, or offered them. See [Curated skills](/reference/configuration/#curated-skills).

## Names, renames, and clashes

Agents find skills by name, so the folder an installed skill goes in is named after its install name, which is its own `name` unless your project renames it. Two installed skills can't share a name. When two sources ship skills with the same name, rename one of them in configuration; sync fails until no clash remains. A rename changes only the installed name: it doesn't rewrite the old name where other skills mention it, and every sync that applies one warns about that. See [Rename a skill](/reference/configuration/#rename-a-skill).

A person can also install **personal skills** outside any repository, which their agents load in every repository. When a personal skill has the same name as a skill your repository installs, each agent decides which one to use: Claude Code uses the personal skill, Codex shows both, and Grok Build and VS Code read both without documenting which they use. Trail Guide never writes to your home directory; it warns when it finds such a pair. See [Personal skills with the same name](/reference/sync/#personal-skills-with-the-same-name).

Skill decisions layer in this order, each overriding the ones before it: a library, an org library, the project, then each person's personal skills. Required skills are a floor the project can't remove.

## How a skill is versioned

:::note[Preview]
Libraries version their skills and record skill change notes, but projects don't read skill versions yet: today a project's skills move with their source's revision, and its generated files and update preview show no skill version.
:::

A library skill has its own semantic version, which covers its whole folder and follows the same levels as a [rule's](/concepts/rule/#how-a-rule-is-versioned):

- **Major:** the skill now produces or requires something different, such as a removed or reordered step that people rely on, a new required input, or a new rule or skill it uses.
- **Minor:** the skill adds steps or guidance without breaking how it was used.
- **Patch:** the skill only clarifies its wording.

Library authors record each change in a change note, and a library release publishes the new versions. Unlike rules, a project doesn't choose skill versions or pin skills: all of a source's skills come from one revision, the library release or commit the project recorded for that source, so skills that call each other always match. A skill's version tells you what changed. Projects see it in their generated files and in the `trailguide project update` preview, and every installed skill moves when its source updates. To hold a skill back, exclude it, [fork it](/guides/install-skills/#fork-a-skill), or set the source's `ref`, which holds everything from that source. Authored skills and skills from non-library sources have no versions.

See [Rule and skill versions](/reference/rule-versions/) for the details.

## How agents use installed skills

When you sync, Trail Guide writes a copy of each installed skill into the folders your agents read: `.claude/skills/` for Claude Code and `.agents/skills/` for Codex, Cursor, and other agents that follow the shared convention. You commit these copies, so every checkout and every fresh cloud session has the same skills.

The copy in `.agents/skills/` also gets `agents/openai.yaml`, the file Codex reads a skill's display name, short description, and default prompt from, generated from the skill's metadata when it sets any of them. A skill that ships its own file keeps it unchanged.

When a skill cites rules, its installed `SKILL.md` ends with a generated section, **Rules this skill applies**, that points at your project's version of every rule the skill cites, including any authored rule that replaces one. Agents read those rules in full before the step that uses them.

Trail Guide also writes `.trailguide/_managed/generated/SKILLS.md`, which lists every installed skill, its source, level, and version, how many rules it applies, and the skills it calls. Agents in tools without native skill support can read it there.

When a skill and a rule disagree, agents should follow the rule and report the conflict, so someone can fix the skill. See [For agents](/for-agents/).
