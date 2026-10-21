---
title: "Write a skill"
description: "Turn a repeatable task into ordered steps that cite the rules each step applies, then check, version, and retire it."
---

A **skill** tells an agent how to carry out one kind of task, step by step. Writing a skill turns a workflow you keep explaining, such as how your team fixes a bug or reviews tests, into steps every agent follows the same way. A useful skill names the task that should trigger it, orders the steps, and cites the rules each step applies.

This guide covers deciding between a skill and a rule, creating a skill, writing and checking it, setting its level in a library, and versioning it. For every field and check, see [Skill format](/reference/skill-format/).

## Skill or rule?

Choose by granularity.

- A **rule** states one best practice and when it applies, such as "never write secrets to logs." A project can adopt, exclude, or replace it on its own, and it has its own version. It applies to any work that matches it, whatever task the agent is doing.
- A **skill** is coarse-grained: a playbook, technique, or way of coordinating one kind of task, such as "reproduce the bug, find the cause, fix it, then check the test." It combines many practices, and applies when an agent takes on that task.

When a step needs a practice followed, cite the rule rather than restating it. A project can then exclude or replace the rule, and the skill follows the project's choice; when the rule gets a new version, the skill applies it without a change of its own; and reviewers check the work against the same rule the implementing agent read.

Write a rule when:

- You'd correct the same mistake whatever the task, such as a missing test at a retry limit.
- The expectation needs its own exceptions, examples, or validation.

Write a skill when:

- The order of the work matters, such as writing a failing test before the fix.
- A task combines several rules, tools, or scripts that agents keep combining differently.
- You'd like an agent to run the same review the same way every time.

If a step in your draft explains how to do something well, and the explanation would hold outside this task, move it into a rule and cite the rule. See [Write a rule](/guides/write-rules/).

## Choose a type

A skill can declare one [type](/concepts/skill/#skill-types) in `trailguide-skill-type`:

- **`playbook`**: ordered steps for carrying out one kind of engineering task, citing the rules each step applies, such as building a feature or fixing a bug.
- **`review`**: examines existing work, such as a diff, a plan, or a test suite, against stated criteria and reports findings.

Ask what the skill's main job is: a playbook changes the work, and a review judges work that already exists. A playbook can include a review step, and a review can offer to fix what it found. If neither fits, leave the type out. The type is a label for people and for catalogs that browse skills by type; Trail Guide treats every skill the same way.

## Create the skill

Skills you author live in `.trailguide/skills/`, in a library and in a project alike.

### In a library

Create a draft:

```sh
trailguide library add skill fix-bug \
  --description 'Fix a reported bug and prove it with a regression test. Use when a task starts from a bug report, a failing behavior, or an error someone saw.' \
  --type playbook
```

This writes `.trailguide/skills/fix-bug/SKILL.md`, with your name, description, and type in its frontmatter, and a body with the `trailguide:draft` marker. `trailguide library check` fails until you replace the body and remove the marker.

The folder's path becomes the skill's ID, `skills/fix-bug`, and its name becomes the folder name of every installed copy, unless a project renames it. Use 1 to 64 lowercase letters, digits, and hyphens, and keep the name stable once projects install the skill; see [Skill IDs](/reference/skill-format/#skill-ids).

Write the description for an agent deciding whether to load the skill: say what the skill does, then when to use it, naming the task or request that should trigger it.

### In a project

A skill that only one project needs can stay in that project:

```sh
trailguide project add skill release-checklist \
  --description "Prepare this service's release pull request. Use when asked to cut or prepare a release." \
  --type playbook
```

This writes `.trailguide/skills/release-checklist/SKILL.md`. A skill in a repository that isn't a library has no version. It can cite rules from your sources by project ID, with the source name, such as `fabrica:practices/testing/verify-retry-limits`, and your own rules by their bare ID, such as `practices/release/tag-from-main`.

To change a skill you include from a source for one project, fork it instead of starting over:

```sh
trailguide project add skill --from fabrica:skills/fix-bug \
  --reason 'We fix bugs behind a feature flag first.'
```

The fork copies the version the project installs into `.trailguide/skills/fix-bug/`, rewrites its citations to project IDs, and records it under the source's `skills.forks`, so the fork installs in place of the source's skill. A required skill can't be forked. See [Fork a skill](/guides/fork-skills/) for editing a fork and merging the source's later changes into it. A skill your library source curates from another repository forks the same way, such as `--from fabrica:mattpocock:skills/to-spec`.

In a library, fork a skill from one of the library's sources, such as a third party's skill you want to publish with your own changes:

```sh
trailguide library add skill --from pstack:pstack/skills/show-me-your-work \
  --reason 'We ask for evidence before every claim.'
```

The fork copies the skill at the revision the source's `ref` names into `.trailguide/skills/show-me-your-work/`, and records the source, the skill, and the commit it copied in `config.yaml`. When you move the source's `ref`, `trailguide library check` lists what the source changed in the original since that commit. See [library add skill](/reference/cli/#fork-a-skill-into-the-library).

A source can exist only to track forks. Projects that use your library never read its `sources`, so declaring a third party's repository to fork from it doesn't offer that repository to them; they get only the forks, through `library.skills`. Keep the source declared for as long as you want check to compare your forks with it, and move its `ref` to compare with a newer revision. To stop tracking a fork, delete its `skills.forks` entry, and the source once nothing else uses it; the fork stays a skill of your own.

## Write the steps

Replace the draft's body with the skill's instructions:

- **Number the steps, in the order the agent takes them.** Give each step one outcome the agent can check before moving on, such as "a test that fails because of the bug."
- **Name the rules each step applies.** Mention the rule by its ID in the step, such as `practices/testing/test-behavior-not-implementation`, and list it in `trailguide-rules-used`. The installed copy ends with a section that maps each ID to the project's version of the rule, including an authored rule that replaces it.
- **Say when to stop.** Tell the agent what to do when a step can't be completed, such as a bug it can't reproduce, instead of leaving it to improvise.
- **Say what to report.** End with what the agent hands back: what it changed, what it verified, and what it couldn't.
- **Keep `SKILL.md` short.** Agents load the whole file when they use the skill. Move long background, examples, and checklists into `references/`, and link to them from the step that needs them. The Agent Skills format recommends keeping `SKILL.md` under 500 lines.
- **Put repeatable commands in scripts.** A script in `scripts/` runs the same way every time. Make it executable and commit it with its mode; Trail Guide keeps the mode when it installs the skill.

Links must stay inside the skill's folder or be absolute URLs, because each installed copy stands alone. Cite rules in `trailguide-rules-used` instead of linking to them.

## Cite rules and skills

List the rules the skill applies in `trailguide-rules-used`, and the other skills it uses in `trailguide-skills-used`, one ID per line in a YAML block string:

```yaml
metadata:
  trailguide-skill-type: playbook
  trailguide-rules-used: |
    practices/testing/reproduce-bugs-with-a-failing-test
    practices/testing/test-behavior-not-implementation
  trailguide-skills-used: |
    skills/review-tests
```

A bare ID names a rule or skill this repository authors. A library's skills cite only bare IDs, because each project chooses its own source names. A project that installs the skill takes every cited rule, even ones its rule `include` doesn't name, and installs every skill it calls.

The cited rules are for the skill. They don't join the project's `RULES.md`, so installing your skill never adds rules to unrelated work. Cite every rule a step depends on, including rules from rule groups you'd expect projects to include anyway: a project may not have included them.

A library's skill can't cite a rule or call a skill from another library. If your skill depends on another library's practice, write or adapt the rule in your library first. A skill in a repository that isn't a library can cite its sources' rules with project IDs, such as `fabrica:practices/testing/verify-retries`; `trailguide project check` fails when one of those rules is retired, so your skill never points at a rule that's gone.

## Example: a playbook

This original example is a complete playbook in a library, `.trailguide/skills/fix-bug/SKILL.md`:

```markdown
---
name: fix-bug
description: Fix a reported bug and prove it with a regression test. Use when a task starts from a bug report, a failing behavior, or an error someone saw.
license: MIT
metadata:
  trailguide-skill-type: playbook
  trailguide-rules-used: |
    practices/testing/reproduce-bugs-with-a-failing-test
    practices/testing/test-behavior-not-implementation
    practices/error-handling/make-errors-actionable
  trailguide-skills-used: |
    skills/review-tests
---

# Fix a bug

Fix one reported bug and leave a test that would catch it again. Before each step, read in full the rules it names, using the project's versions listed under "Rules this skill applies".

## 1. Reproduce the bug

Restate the report as the behavior observed and the behavior expected. Write a test that fails because of the bug, following practices/testing/reproduce-bugs-with-a-failing-test, and run it. Confirm it fails for the reported reason, not because of its setup.

If you can't reproduce the bug, stop. Report what you tried and what you observed instead, and ask for more detail.

## 2. Find the cause

Trace the failing test to the code that produces the wrong behavior. Name the cause in one sentence before changing anything. If several causes are plausible, add a check that tells them apart rather than fixing all of them.

## 3. Fix the cause

Make the smallest change that removes the cause. Don't refactor unrelated code in the same change. If the bug showed a user or caller a misleading error, fix the message too, following practices/error-handling/make-errors-actionable.

## 4. Check the test

Run the new test and confirm it passes. Then check that it tests behavior, following practices/testing/test-behavior-not-implementation: temporarily undo the fix, confirm the test fails again, and restore the fix. Run the rest of the test suite.

## 5. Review the tests

Use the review-tests skill on every test you added or changed. Fix the findings that apply to your change.

## 6. Report

Report the cause, the fix, the test that now covers the bug, and the result of the full test suite. List anything you couldn't verify.
```

Each step has one outcome, names the rules it applies, and says what to do when it can't be completed. The opinions about how to write a failing test or an actionable error live in the cited rules, so a project that replaces one of those rules gets its own practice in step 1 or step 3.

## Example: a review

A review skill states what it examines, the criteria, and what it reports, and leaves the work unchanged. This is `.trailguide/skills/review-tests/SKILL.md` in the same library:

```markdown
---
name: review-tests
description: Review added or changed tests for whether they would catch the behavior they claim to cover. Use when reviewing a pull request or diff that adds or changes tests.
license: MIT
metadata:
  trailguide-skill-type: review
  trailguide-rules-used: |
    practices/testing/test-behavior-not-implementation
    practices/testing/verify-retry-limits
---

# Review tests

1. List each test the change adds or modifies, and the behavior it claims to cover.
2. Check each test against every rule listed under "Rules this skill applies" whose conditions match it, and against any other testing rule the project's RULES.md selects for this work.
3. For each finding, report the test, the rule's ID and version, the evidence, and the consequence. Separate confirmed failures from suspicions you couldn't verify.
4. Don't change the code. Offer fixes only when asked.
```

## Check the skill

In a library, check every skill, along with the rest of the library:

```sh
trailguide library check
```

Check validates the folder, the frontmatter, the `trailguide-` keys, the rules and skills they name, and every link, and lists each problem it finds:

```text
Error: skills don't pass library check:
  - skills/fix-bug/SKILL.md: trailguide-rules-used names practices/testing/reproduce-bug-with-a-failing-test, which isn't a rule in this library
  - skills/fix-bug/SKILL.md: trailguide-skill-type is workflow; use playbook or review, or leave it out
```

[Validation](/reference/skill-format/#validation) lists every check and its message.

In a project, run `trailguide project build`, which validates your skills and installs them into the project's skill targets, then `trailguide project check`.

Check validates the format, not the guidance. Before publishing, try the skill: ask an agent to carry out a real task with it, and revise any step the agent skipped, misread, or had to ask about.

## Set the skill's level

In a library, decide how strongly projects should take the skill, and list it in the manifest in `.trailguide/config.yaml`:

```yaml
library:
  skills:
    required: [skills/ship-service]
    recommended: [skills/fix-bug, skills/review-tests]
```

- **Required** skills install in every project that uses the library, whatever the project includes, and no project can exclude or fork them. Require only what every project must follow, such as your organization's release procedure.
- **Recommended** skills install in projects that include `recommended`.
- **Offered** skills install only when a project names them or includes `"*"`. A skill you don't list is offered, so list only the skills you have an opinion about.

Levels are library-wide settings, like rule group metadata: changing one needs no change note, and the next library release publishes it. Projects see a level change, such as recommended to required, in their update preview. `trailguide library check` fails when two skills you list, including skills you [curate](/reference/configuration/#curated-skills) from other repositories, have the same name, so your projects never inherit a clash.

## Record a change note

:::note[Preview]
Skill versions and skill change notes aren't implemented yet, so `trailguide library change` records rule changes only. This section describes the planned workflow.
:::

Skills are versioned like rules. Before your library's first library release, skills need no change notes; the first library release gives every skill version `1.0.0`. After it, record every new or changed skill with `trailguide library change`, in the same pull request as the change.

For a new skill, leave out `--bump`:

```sh
trailguide library change skills/fix-bug \
  --summary 'Add a playbook for fixing a reported bug with a regression test.'
```

For a changed skill, choose the level from what the change does to what the skill produces and requires:

```sh
trailguide library change skills/fix-bug \
  --bump minor \
  --summary 'Add a step that reviews the changed tests.'
```

- **Major:** someone relying on the previous version could get a different result, such as a removed or reordered step people rely on, a new required input, or a new rule or skill the skill uses.
- **Minor:** the skill adds steps or guidance without breaking that.
- **Patch:** the skill only clarifies its wording.

When unsure, choose the larger level. Any change in the folder counts, including scripts, references, and a script's executable bit. [Version your rules](/guides/version-rules/#skill-versions) covers the levels and publishing in full.

Skills in a repository that isn't a library have no versions and need no notes.

## Retire or replace a skill

To stop publishing a library skill, delete its folder, remove it from any level in the manifest, and record the retirement. Name the skill that replaces it, when there is one:

```sh
trailguide library change skills/triage-bug \
  --retire \
  --replaced-by skills/fix-bug \
  --summary 'Covered by the fix-bug playbook, which also writes the regression test.'
```

Only a skill can replace a skill. A retired skill's ID can't be reused, and projects see the retirement, with its replacement, in their update preview. Update any skill that names the retired one in `trailguide-skills-used` in the same pull request; `trailguide library check` fails until you do.

To rename a skill, rename its folder and its `name` field, then record one note that retires the old ID, replaced by the new one, and adds the new ID:

```yaml
summary: Rename to say what the playbook produces.
skills:
  skills/triage-bug:
    change: retired
    replacedBy: skills/fix-bug
  skills/fix-bug: new
```

In a project, replace a skill you include from a source by forking it with `trailguide project add skill --from`, or by writing your own and adding a `skills.forks` entry for the source's skill with `as: skills/<name>`; see [Configuration](/reference/configuration/#fork-a-skill). To remove one of your own skills, delete its folder and any `skills.forks` entry that names it as `as`, then run `trailguide project build`.

## Next steps

- [Skill format](/reference/skill-format/) defines every field and check.
- [Install skills](/guides/install-skills/) shows how projects include and install your skills.
- [Version your rules](/guides/version-rules/) covers change notes and publishing library releases.
- [Write a rule](/guides/write-rules/) covers the rules your skills cite.
