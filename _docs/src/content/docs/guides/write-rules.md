---
title: "Write a rule"
description: "Author a focused engineering obligation with applicability, examples, and verification guidance."
---

A **rule** is a Markdown file that tells an agent how to apply one engineering practice. Writing a rule turns a recurring expectation or review comment into guidance you can reuse across tasks. A useful rule explains when it applies, what to do, and how to verify the result.

This guide explains how to draft and review a rule using the shared template and quality criteria. You'll choose a rule group for the rule, work through an example, and learn how to keep its identity and source attribution intact.

You can write rules with any agent or editor. For the CLI steps to create your first rule, follow [Set up your first project](/start-here/set-up-project/) or [Create your first library](/start-here/create-library/).

A rule states one expectation. To give agents ordered steps for a whole task, such as fixing a bug, that apply several rules, write a [skill](/concepts/skill/) that cites them instead; see [Write a skill](/guides/write-skills/).

## Rubric, template, and skill

- The [authoring rubric](/reference/rule-authoring/#authoring-rubric) defines what makes a useful rule and how to review it.
- The [Markdown template](/reference/rule-authoring/#markdown-template) gives each rule a consistent starting structure.
- The Trail Guide authoring skill guides an agent through drafting, revising, and reviewing rules using those two references.

The rubric is authoritative.
The skill references it rather than maintaining a separate copy of the authoring guidance.
You can also use the rubric and template without a skill, with any agent or editor.

:::note[Skill availability]
The installable authoring skill is not available yet.
Use the rubric and template with any agent or editor.
The workflow below describes how the skill is intended to work.
:::

## Write with the skill

1. Give the agent a best practice, its intended scope, and any supporting context or sources.
2. The skill reads the rubric, template, and target library's conventions.
3. The agent drafts one rule per file, specifying `whenToRead` and separately considering implementation and validation guidance.
4. The agent checks the draft against every rubric criterion and revises unclear passages.
5. The agent presents the rule and any unresolved policy questions for review.

The same skill can revise an existing rule or review a proposed rule against the rubric.
It should ask about missing policy decisions instead of silently choosing them for the library owner.

## Choose a rule group

Use a technology group when the obligation depends on a named technology.
Use a practice group when it transfers across technologies.
Place project-specific contracts in the applicable project's own rules, in its `.trailguide/rules/`.

For a new rule group, use a [canonical rule group ID](/concepts/groups/#canonical-rule-group-ids) if one fits, and add [rule group metadata](/reference/rule-format/#rule-group-metadata) that helps agents recognize relevant work.

## Example rule

This original example uses the rule format:

````md
---
title: Verify retry limits
whenToRead: "When planning, implementing, reviewing, or diagnosing bounded retry behavior."
impact: HIGH
impactDescription: prevents a transient failure from causing unbounded requests
tags: testing, retries
---

## Verify retry limits

When a change adds bounded retries, test that requests stop after the configured limit.

### Implementation

Write a deterministic test that keeps the request failing until the configured attempt limit.
It does not require a retry mechanism where none is needed.

### Rationale

A retry limit prevents repeated failures from producing unbounded requests.
A test should catch changes that accidentally bypass that limit.

### Incorrect example

Make the first request fail and the second succeed, then assert success.
This verifies recovery but never reaches the retry limit.
The test would still pass if the operation allowed unlimited attempts.

### Correct example

For a limit of three attempts, make every attempt fail.
Assert that the operation stops after three calls and returns the documented failure.

### Validation

Run the test with the stop condition removed in a disposable checkout.
The test should fail.
````

## Keep identity stable

A rule's library ID is its relative path without `.md`.
Consuming projects qualify it with their configured source name, such as `fabrica:practices/testing/verify-retry-limits`.
Renaming or moving the file changes its ID.
Consumers should remove exclusions, replacements, pins, and `rules.include` entries that referenced the old path; those entries no longer do anything.
In a library, a rename retires the old ID with the new one as its replacement, and projects see it in their update preview like a major change.

## Record the change

After the first library release, add a change note with every rule you write or edit, in the same pull request:

```sh
trailguide library change practices/testing/verify-retry-limits \
  --bump patch \
  --summary 'Clarify the incorrect example.'
```

Omit `--bump` for a new rule. [Record changes to a library rule](/reference/rule-authoring/#record-changes-to-a-library-rule) explains how to choose between `major`, `minor`, and `patch`, and [Version your rules](/guides/version-rules/) covers publishing. Local project rules have no versions and need no notes.

Keep source attribution in the rule's metadata or Markdown body and preserve any required notices.
Builds carry that attribution into the individual generated rule file; no separate attribution file is required.
Unknown frontmatter fields, including `source:`, are rejected. Use `attribution` to record adaptation sources, and configuration to declare replacements. The generated rule links to its original file.
When publishing or adapting rules, follow [License rules](/guides/license-rules/) to make permissions and attribution explicit.


## Adapt third-party rules

For a step-by-step example with inspectable license provenance, follow [Adapt a third-party rule](/guides/adapt-rules/).

The original repository does not have to change. The definition you give Trail Guide must use its input format: a technology group or practice group with `_group.yaml`, a Markdown rule with the required metadata, and retained supporting files. The body follows the flexible authoring rubric; it does not need every template heading.

For an existing compatible Trail Guide library, [include its rules](/guides/select-rules/) instead. To use a third-party skill as a skill, install it from its repository instead of adapting it into a rule; see [Install skills](/guides/install-skills/). For a linter rule, style guide, skill whose practice you want as a rule, or other document that is not a compatible library:

1. Identify the exact source revision and establish permission to copy, adapt, and redistribute the material for your intended use. Preserve the applicable license and notices.
2. Create an adapted definition in a compatible library, which may contain just this one rule. Preserve the original separately when useful for reviewing future updates.
3. Add the required metadata and an activity-based `whenToRead` cue. Preserve the obligation, important conditions, exceptions, and examples; explain deliberate changes. A detector's analysis limitations do not automatically become exceptions to a written rule.
4. Declare the library-wide license and notice files in the library's [manifest](/reference/library-schema/#the-manifest). Record per-rule [attribution](/reference/rule-format/#rule-attribution) with a commit-pinned source URL and describe the adaptation.
5. Review the adaptation against the [authoring rubric](/reference/rule-authoring/), then generate and inspect the resolved rule, its license links, and its source link. Commit the adapted source and retained notices together.

An agent can help prepare the adaptation, but Trail Guide does not currently convert arbitrary repositories automatically. Do not place modified material in `_managed/vendor/` and claim it is an unchanged snapshot of the original commit. The adapted library has its own repository and rule versions while retaining the earlier attribution chain. A project's own rules are for guidance you author for that project.

For later updates, compare the original pinned material with the new source, then deliberately revise the adaptation. Updating a source citation alone does not establish that the adapted rule incorporates the newer guidance.
