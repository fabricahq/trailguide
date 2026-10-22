---
title: "What is Trail Guide?"
description: "The package manager for the agent skills and engineering rules agents follow when they write and review code."
---

Trail Guide is the package manager for your agent skills and rules.

Most teams today use skills to guide their agents, but skills aren't always the ideal package for delivering guidance, and managing skills across repos, organizations, and your local environment can be tricky.

Trail Guide solves these problems with two key insights:

1. Skills need a finer-grained companion: rules.
2. Skills and rules both need a package manager.

## Skills need rules

Skills are coarse-grained: a single skill often bundles many insights, which makes it hard to update, version, and understand. A **rule** is fine-grained: one directive, such as "Never write secrets to logs," in its own Markdown file. Skills cite the rules they apply instead of restating them.

## Skills and rules need a package manager

Trail Guide manages both skills and rules the way a package manager manages code. Each project declares the skills and rules it uses and where they come from: public or private libraries, your organization's shared library, or the project itself. Skills can also come from any Git repository of skill folders, even one that doesn't use Trail Guide.

Trail Guide records each skill's and rule's source and version, updates them only when you choose, and installs skills for Claude Code, Codex, Cursor, and other agents at once.

## Use cases

With Trail Guide, you can:

- Start every new repo with exactly the right skills and rules in place.
- Update a skill once in your organization's library, and have every repo pick up the change on its next update.
- Document which third-party skills your organization requires, recommends, or makes available for consideration, and have projects automatically install them.
- Override skills and rules on a per-project basis as needed.

### Turn retros into rules

The bigger payoff comes after an agent session. When an agent misses something you expected, run a quick retro: capture the lesson as a rule, and add it to your project or your organization's library. From then on, every agent gets that guidance when the work calls for it, so the same mistake doesn't come back. Retros also show where an existing rule or skill falls short, so you can refine it. And because each lesson is one small rule, this loop could eventually run on its own, with tools that review agent sessions and propose rules.

## How it works

1. Run `trailguide project init` to create a `.trailguide` directory in your repo.
2. In `.trailguide/config.yaml`, choose the skills and rules you want from libraries, and add any of your own.
3. Run `trailguide project sync`. Trail Guide copies in the rules, writes `RULES.md` to index them, and installs the skills where your agents look for them.
4. Trail Guide adds a **Skills and Rules** section to your `AGENTS.md` and keeps it current, so your agents read the rules and use the skills that fit each task.
5. Commit the results. When a library publishes new versions, run `trailguide project update` to adopt them.

[Set up your first project](/start-here/set-up-project/) walks through these steps.

## Core concepts

Trail Guide is built around just a few simple concepts.

### Rules

You write a single best practice as a [rule](/concepts/rule/), written as a Markdown file. A rule tells an agent what to do, when it applies, and how to apply it.

### Rule groups

You organize rules into [rule groups](/concepts/groups/). There are two kinds of rule groups: technology groups, for a _technology_ like Go, Tailwind, Next.js, Typescript, or Playwright, and practice groups, for a _practice_ like testing, logging, or error handling.

### Skills

A [skill](/concepts/skill/) is the AI skill you already know, officially an [Agent Skills](https://agentskills.io) folder: a reusable procedure, such as how to build a feature or review a pull request. Skills are coarse-grained, so a skill cites the rules it applies instead of restating them.

### Projects

You declare the rules and skills you want in a [project](/concepts/project/), specifically in the `/.trailguide` folder in your project repo. You (or your agent) can use the `trailguide` CLI to initialize a new project, and add your own rules, rule groups, and skills. We recommend committing `/.trailguide`, and the skills Trail Guide installs, to version control.

### Libraries

Finally, you probably want to include some pre-existing rules and skills from a [library](/concepts/libraries/), a repository that publishes rule groups and skills for other projects. A library can require or recommend skills for every project that uses it. You can also install skills from a Git repository that isn't a library, such as a third party's skill collection. Libraries can optionally declare license terms, which Trail Guide carries into your project.

### RULES.md

Trail Guide combines your own rules and the ones you include from sources into guidance under `.trailguide/_managed/generated/`. Agents start with `RULES.md`, an index that tells them which rule groups to read.

An abbreviated `RULES.md` entry:

```md
### Testing

**When to read this group:** When changing behavior or fixing bugs.

**Open group:** [Testing](groups/practices/testing.md)
```

The linked rule group page contains the rules in full, or links to their full definitions. For example, it might tell an agent to add a regression test when fixing a bug. See [the generated file layout](/reference/files/) for details.

### Installed skills

Trail Guide writes each installed skill into `.claude/skills/` and `.agents/skills/`, where coding agents find skills, and lists them in `.trailguide/_managed/generated/SKILLS.md`. Each installed skill that cites rules ends with links to your project's version of every rule it applies. See [Skill](/concepts/skill/#how-agents-use-installed-skills).

## What Trail Guide does not do

Trail Guide assembles rules and skills from multiple sources into a cohesive, agent-friendly collection of files in your repo, and checks that those files are valid and current.

Trail Guide installs skills into your repository, not on your computer. It doesn't manage the personal skills a person installs for all of their repositories, though it warns when one has the same name as a skill your project installs.

Trail Guide also supplies the instructions that tell your agents to follow the relevant rules and skills for each task: a **Skills and Rules** section, which tells them to read `RULES.md` and apply the rules that fit the task, and to use any installed skill listed in `SKILLS.md` that fits it. `trailguide project init` adds the section to `AGENTS.md`, and later commands keep it current between its markers without changing the rest of the file. To manage `AGENTS.md` yourself, turn the section off with [`manageAgentsFile: false`](/reference/configuration/#agentsmd-section). See [Set up your first project](/start-here/set-up-project/#6-give-the-rules-to-your-agent).

That is where Trail Guide's work ends. It can't guarantee that your agents followed your rules or used your skills. You can add your own guidance on how agents should apply them, in other procedures, in skills, or elsewhere in `AGENTS.md`.
