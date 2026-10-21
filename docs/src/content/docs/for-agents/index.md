---
title: "Plan, write, and review"
description: "A suggested workflow for agents using the project\u2019s resolved engineering rules."
---

This page suggests an agent workflow for using the project's committed resolved rules during planning, implementation, and review.
Adapt it to your project, or use your own prompts and tooling. Trail Guide supplies the rule files; it does not run this workflow or require a particular validation or enforcement method.
See [product scope](/start-here/overview/#what-trail-guide-does-not-do).

## Project instructions

`trailguide project init`, `build`, `sync`, and `update` keep this section in the project's `AGENTS.md`, between `<!-- trailguide:begin -->` and `<!-- trailguide:end -->` markers. If your agent reads another instruction file, have it import or link `AGENTS.md`, or use the section as a starting point for a direct agent prompt.

```markdown
## Skills and Rules

Before planning, implementing, reviewing, testing, or debugging a change:

1. Read `.trailguide/generated/RULES.md` and follow its instructions to
   select relevant groups and read their rules in full, including linked
   files and additional index pages.
2. If `.trailguide/generated/SKILLS.md` exists, read it and use the skills
   relevant to the task. Read each skill's instructions and the rules it
   cites before performing the steps that apply them.
3. Follow the applicable rules and their exceptions while doing the work.
4. Before finishing, check your work against those rules and run the
   relevant validation. Briefly report what you verified and any gaps.

If required rule files are unavailable or give conflicting instructions,
report the issue rather than silently skipping them or choosing a policy.

To add, change, or update rules or skills, follow `.trailguide/README.md`.
```

`.trailguide/generated/RULES.md` then tells the agent how to select rule groups, read rules, and report findings, so these instructions stay short.
The generated index is an explicit entry point.
Do not rely on automatic discovery of nested `AGENTS.md` files to load the rules.

## Plan and implement

1. Read the task and the generated index.
2. Identify affected technologies and engineering practices.
3. Compare each rule group's **When to read this group** cue with the work. Use its **Description** to understand scope. Open every relevant or plausibly relevant rule group.
4. Read every rule in each opened rule group completely, including all pages and linked full definitions. Then assess applicability using each rule's **When to read** cue, guidance, and exceptions.
5. Account for applicable obligations in the plan and implementation.
6. Revisit selection if the work expands.

### Example: retry a failed request

For retries in a TypeScript service, consider `techs/typescript`, `practices/testing`, `practices/observability`, and `practices/error-handling`.
Inspect the intended behavior, dependencies, imports, surrounding code, and changed files.
Practice groups can apply even when no test files or logging packages change.

Open each relevant or plausibly relevant rule group, read every rule in it completely, then apply the rules whose conditions match the work. Respect their exceptions.
A rule group match alone is not evidence of a violation.

A behavior change can require testing rules before anyone edits a test file.
When an obligation depends on local contracts, read project context alongside the rules.

Completion means the relevant rules informed the work, including their scope and exceptions.
Reading the index alone is not enough.

## Use installed skills

A project can also install [skills](/concepts/skill/): step-by-step procedures for one kind of task, such as building a feature or reviewing a pull request. Trail Guide writes them into `.claude/skills/` and `.agents/skills/`, where tools with native skill support find them. When the project has skills, `RULES.md` links to `.trailguide/generated/SKILLS.md`, which lists each installed skill with its description, type, and installed path. If your tool doesn't load skills on its own, read `SKILLS.md` and open a skill's `SKILL.md` when its description matches the task.

When you use a skill:

1. Read its `SKILL.md` in full, including the generated **Rules this skill applies** section at the end, when it has one.
2. Before each step, read in full every rule that section lists for the work the step does, at the generated path it gives. These are the project's versions of the rules: a rule the project wrote to use instead of a library rule is listed in its place, and a rule the project excluded is marked as excluded, with the reason. Don't apply an excluded rule.
3. Follow the steps in order. When the skill uses other skills, its **Skills this skill uses** line names them; use their installed copies.
4. Keep selecting rule groups from `RULES.md` as described above. A skill doesn't replace that selection: a rule that applies to the work applies whether or not the skill cites it.

Some rules a skill cites aren't listed in `RULES.md`, because the project takes them only for its skills. Read them when the skill's steps call for them; they don't apply to other work.

If a skill's step and a rule disagree, follow the rule and report the conflict, naming the skill's ID and version from its marker line and the rule's ID and version, so someone can fix the skill. If a step can't be followed, such as when it names a script or rule that isn't there, stop and report it rather than improvising a substitute.

Don't edit installed copies; sync rewrites them. To change a skill for this project, fork it with `trailguide project add skill --from`, which copies it into `.trailguide/skills/`; see [Write a skill](/guides/write-skills/). A skill the project's library requires can't be forked or excluded; report the problem to the user instead.

When `trailguide project sync`, `update`, or `check` warns that a personal skill has the same name as one the project installs, tell the user: some agents use the personal skill instead of the project's. Don't rename or remove the user's personal skills yourself.

## Review independently

Select rule groups from the requested behavior and implementation, rather than accepting the writing agent's selection as complete.
For each finding, cite the resolved rule ID and its version when the rule group page shows one, the applicable condition, observed evidence, and practical consequence.

Impact describes the consequence a rule helps prevent; it does not determine applicability, override exceptions, or set finding severity.
Read **Why it matters** for context and assess the actual consequence of each finding.

Separate confirmed failures from hypotheses that need verification.
`trailguide project check` establishes file consistency, not application compliance.

## Handle gaps and conflicts

If a relevant rule group is missing, report the missing coverage.
If resolved rules conflict, identify both IDs and ask the project owner to resolve the intended policy.
Use [Resolve conflicting rules](/guides/conflicting-guidance/) for the review criteria and explicit resolution options.
Keep the recorded rules and skills during ordinary work. Taking changes from the project's sources is a separate step, `trailguide project update`, and major changes need the project owner's consent; don't run it as part of another task.
When asked to keep a rule from a source at its current version, read the rule's version from the source's entry in `.trailguide/lock.json` and add a pin under that source's `rules.pins`, with the version and the reason you were given, such as `{id: practices/testing/verify-retry-limits, version: "1.3.0", reason: "…"}`. Then run `trailguide project sync`.
Skills can't be pinned. When asked to keep a skill from a source as it is, ask whether to fork it, exclude it, or hold its whole source with `ref`; see [Install skills](/guides/install-skills/#exclude-a-skill).

## Write or review rules and skills themselves

When the task changes a rule, use the [authoring rubric and template](/reference/rule-authoring/).
Follow those documents directly. The [Trail Guide authoring skill](/guides/write-rules/#write-with-the-skill) is planned and is not available yet.

Read the target library's conventions and evaluate each rule against every rubric criterion.
In a library that has published its first library release, add or update the rule's change note with `trailguide library change` in the same change, and choose `major` whenever work that complied with the previous version could fail the new one. See [Record changes to a library rule](/reference/rule-authoring/#record-changes-to-a-library-rule).
Report unmet criteria with the relevant passage and a concrete revision.
Separate unclear wording from unresolved engineering policy; ask the author to resolve the latter.

When the task changes a skill, follow [Write a skill](/guides/write-skills/) and the [Skill format](/reference/skill-format/). Keep each practice in a rule: when a step needs a practice followed, cite the rule in `trailguide-rules-used` instead of restating it, and propose a rule when none exists. In a library that has published its first library release, record the skill's change note with `trailguide library change skills/<name>` in the same change.
