# Trail Guide agent guide

Use this file to understand the product we are building and the decisions it should guide.
[CONTRIBUTING.md](CONTRIBUTING.md) owns repository setup and validation commands.

## Product

### Motivation

Most people already guide their coding agents with skills, the AI skills everyone knows, officially folders that follow the open [Agent Skills](https://agentskills.io) standard. They install skills from many places, and it's hard to know where each skill came from, keep it updated, share it across an organization, or use it with more than one agent.
Skills need a package manager. Trail Guide records each skill's source and revision, updates it only when a project asks, shares it through libraries and an organization's `.trailguide` repository, and installs it into the folders each of the project's agents reads.
Publishers need a formal format that tools can read in a structured way: a library manifest, a version for each skill, change notes, and skill metadata.

Skills alone aren't enough. They're coarse-grained: suited to playbooks, techniques, and coordinating a whole task. Teams also need granular engineering rules that a project can adopt, exclude, or replace one at a time.
Trail Guide adds rules, organizes them in rule groups, lets skills cite the rules they apply, and manages both together, for libraries that publish them, organizations that standardize on them, and projects that use them.

Without that guidance, agents generate code that conflicts with the team's practices, and review turns into rework: reviewers explain expectations, request changes, and check the revision. Giving agents the right skills and rules before they write code avoids that rework.

### Mission

Help people and organizations curate, share, and apply the skills and rules their agents follow, so the code they generate with AI aligns with their best practices.

### Vision

When an individual or team starts a software project, they select the skills, technologies, and engineering practices they want to apply, from their own work, their organization, and the libraries they trust.

From then on:
Every skill and rule in the project has a known source and version, and changes only when the project updates it.
During implementation, all agents follow the organization's best practices, as declared in the project's skills and rules.
During validation, code is checked explicitly for compliance with those rules.

Organizations quickly gain confidence that agents will apply their best practices throughout implementation and review.

When code does fall short, teams capture the lesson in a new rule or skill for that project or share it across projects. Insights and best practices spread across projects and teams, so a lesson learned in one place improves future work elsewhere.

### Ideal customer profiles

## Humans

Our ideal customer is an individual or software team that wants to produce correct, maintainable, and secure code using AI. They already guide agents with skills or instruction files and find them hard to track, update, and share, or find that enforcing their best practices is painful, ineffective, or both.

The strongest fit is an individual or team with several repositories, or several agents, that share technologies and engineering practices but also have project-specific requirements. At the same time, a single-project user should still benefit from installing skills and authoring local rules before adopting a shared library.

Their best practices may be scattered across skills copied between repositories, documents, prompts, code reviews, and individual experience. They repeatedly correct agents for the same mistakes and want consistent implementation and review without restating every expectation in every task.

Humans will decide which skills and policies to adopt, and when to create an exception. They benefit from readable files and explicit conventions.

## Agents

Agents need to find relevant rules, interpret their scope, and cite evidence when reviewing work. Agents should get clear, unambiguous guidance on how to write the code or adopt a practice. When agents are confused, they should speak up and suggest an update to the rules.

### Core concepts

#### Rule

A rule expresses one independently adoptable engineering expectation in a Markdown file. It states what to do, when it applies, and what evidence would demonstrate compliance. Rules can govern code, tests, plans, documentation, and other engineering work.

A rule retains its identity and provenance when imported or rendered as a resolved definition. See [Rule](_docs/src/content/docs/concepts/rule.md) for the concept and [the rubric and template](_docs/src/content/docs/reference/rule-authoring.md) for the authoring standard.

#### Rule group

A rule group collects related rules and explains when an agent should read them. There are two kinds of rule groups:

1. Technology groups live under `techs/` and cover named languages, frameworks, tools, platforms, or protocols.
2. Practice groups live under `practices/` and cover concerns such as testing, observability, error handling, and architecture.

Rule groups guide selection; individual rules determine applicability. A testing rule may matter even when a change touches no test files. See [Rule group](_docs/src/content/docs/concepts/groups.md).

#### Skill

A skill is the AI skill people already know, officially a folder that follows the open Agent Skills standard, whose `SKILL.md` tells an agent how to carry out one kind of task, step by step. Skills are coarse-grained: suited to playbooks, techniques, and coordinating a whole task. Rules are the granular layer: one best practice each, adopted and versioned on its own. A skill cites the rules it applies instead of restating them. Skills don't belong to rule groups. A skill may declare a type, `playbook` or `review`, as a label only.

Trail Guide installs included skills into the folders agents read and checks them; it doesn't run them, decide when agents use them, or check that agents followed them. It installs skills into repositories, never into a person's home directory; it only warns when a personal skill has the same name as one a project installs. See [Skill](_docs/src/content/docs/concepts/skill.md).

#### Library

A library is a Git repository whose manifest publishes its rule groups and skills. Each library rule and skill has its own version, and a library release publishes new versions of the ones that changed. A library sets a level for each skill it offers: required, recommended, or offered. Libraries own their engineering opinions and can be public or private. An organization can publish shared defaults in `<organization>/.trailguide`, whose required skills install in every project that uses it and whose recommended skills install in projects that include them; a project explicitly chooses which sources, rules, and skills to include.

Projects may take from multiple sources, move to each rule's published versions, and each source's skills, on their own schedule, and add or replace rules and skills of their own. A source can also be a Git repository of skills that isn't a library. One configuration file and one `.trailguide/` layout serve projects and libraries alike.
Source-qualified IDs distinguish rules from different libraries; they do not resolve contradictory instructions.
See [Library](_docs/src/content/docs/concepts/libraries.md) and [Configuration](_docs/src/content/docs/reference/configuration.md).

### Principles

1. **Prevent mistakes before they become rework.**
   Give agents relevant guidance before they write code.
   Aim to get the implementation right the first time, when correcting a mistake is cheapest.

2. **Turn lessons into lasting improvements.**
   When code falls short, use the observation to improve a rule, its enforcement, or how agents discover it.
   Capture lessons locally and share them across projects when they apply more broadly.

3. **Reuse shared practices; customize deliberately.**
   Teams should adopt shared best practices without redefining them for every project.
   Projects can add requirements and make explicit exceptions when their needs differ.

4. **Provide consistent expectations across workflows.**
   Make the same resolved rules available for implementation and review.
   Giving an agent a rule does not guarantee that it will follow it.

5. **Keep rule management independent of enforcement.**
   Projects choose their agent prompts, validation tools, and enforcement mechanisms.
   Deliver clear rule files and installed skills that those workflows can consume.

### Ownership and boundaries

This public repository owns the Trail Guide conventions, importer, authoring tools, and documentation.
Independently owned libraries supply the engineering policies.
Keep Fabrica's private rule corpus separate; public examples must be original or authorized for redistribution.

Projects retain their product vision, domain knowledge, and architecture context.
Trail Guide manages which versioned rules and skills a codebase adopts, delivers the rules' resolved definitions, and installs the skills.
By default, Trail Guide keeps a Skills and Rules section in the project's `AGENTS.md`, between markers, and never changes the rest of the file; it tells agents to read `RULES.md` and `SKILLS.md`. Projects can turn the section off and write their own. Trail Guide can't guarantee that agents follow the rules or use the skills.
Projects can add their own guidance on how agents apply rules and skills, in other procedures, skills, or elsewhere in `AGENTS.md`, and choose their own validation and enforcement.
Rule and skill input validation and generated-file consistency checks are in scope; checking application compliance is not. See [product scope](_docs/src/content/docs/start-here/overview.md#what-trail-guide-does-not-do).

## Working in this repository

This repository uses Trail Guide to adopt Fabrica's [public rules](https://github.com/fabricahq/public-library) and to keep its own conventions as local rules; [its guide](.trailguide/README.md) explains how to manage them.
Before planning, implementing, reviewing, testing, or debugging a change, read [`.trailguide/_managed/generated/RULES.md`](.trailguide/_managed/generated/RULES.md) and follow its instructions to select relevant rule groups and read their rules in full.
Follow the applicable rules and their exceptions, and check your work against them before finishing.
If required rule files are unavailable or give conflicting instructions, report the issue rather than silently skipping a rule or choosing a policy.

Before implementing or reviewing code, also consult [Fabrica's engineering rules](https://github.com/fabricahq/app/tree/main/_rules) and read the individual rules relevant to the change.
Apply the portable guidance; identify app-specific assumptions and explain any adaptation needed for this CLI.
For comments, this repository's comment rules supersede the app's comment policy.

The Go CLI is released. The installable authoring skill remains separate work.
Describe proposed behavior honestly, and verify availability against the implementation before claiming support; the one exception is previewing a feature's docs while you build it.

When a product decision changes, update the owning concept, guide, or reference and its examples together.
Keep configuration details and command contracts in those documents rather than duplicating them here.
After docs changes, run the [documentation checks](CONTRIBUTING.md#documentation-website). Go tests read some documentation examples, so also run the [Go validation](CONTRIBUTING.md#validate-changes) when you change them.
When layout or interaction changes, inspect the rendered pages.

Keep this file focused on enduring product context and decisions useful to almost every agent session.
Revise or remove stale guidance instead of accumulating a history of decisions.

<!-- release-planner:begin v0.4.3 sha256:3d7c63eae9a8c94d -->
## Releases

This repository publishes releases with [Release Planner](https://github.com/fabricahq/release-planner) v0.4.3. When asked to make a release, draft, revise, or correct release notes, or retry a failed release, print the release procedure and follow it:

```sh
release-planner guide
```

First check that `release-planner version` prints `v0.4.3`. If it doesn't, or `release-planner` isn't installed, install that version:

```sh
curl -fsSL https://raw.githubusercontent.com/fabricahq/release-planner/v0.4.3/install.sh | sh -s -- --version v0.4.3
```

Read `.release-planner/policy.md` first for this repository's release policy. You prepare the release pull request; the maintainer approves the release by merging it. Never tag, publish, or merge.
<!-- release-planner:end -->
