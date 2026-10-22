---
title: "Set up your first project"
description: "Write a rule for your coding agent, then reuse rules and skills written by others."
---

We're going to enable your coding agent to get consistent instructions for how to write and review code in your codebase. In Trail Guide, that codebase is your **project**.

We will declare guidance in a **rule,** which is a Markdown file that explains one practice you want your agent to follow, such as testing changes to your code.

In this walkthrough, you'll:

1. Write a rule just for your project, in the project itself.
2. Build the files your agent will read from that rule.
3. Add a rule written by someone else. You'll get it from a **library**, a collection of rules and skills maintained separately for projects to reuse. We'll use Fabrica's public library as an example.
4. Install a **skill** from the same library: a step-by-step procedure your agent can follow, which applies the library's rules.
5. See how your coding agent finds the rules, then try a task.
6. Commit the guidance and skills so everyone working on the project can use them.

Start in the **project root**, the top-level directory of the codebase you want to work on. You need [Trail Guide installed](/start-here/install/), and Git to fetch the library. To publish rules for other projects to use instead, follow [Create your first library](/start-here/create-library/).

## 1. Set up the project

```sh
trailguide project init
```

This creates the **Trail Guide directory**, `.trailguide/`, with your **project configuration** in `.trailguide/config.yaml` and a place for the rules and skills you write yourself in `rules/` and `skills/`. The configuration also sets `skillTargets: [claude, agents]`, so skills you install later are written where Claude Code and other agents look for them. It also adds a section to your project's `AGENTS.md`, creating the file if needed, which you'll look at in step 6. Initializing Trail Guide doesn't change your project's `README.md` or your own text in `AGENTS.md`.

Later, when you add a library, Trail Guide saves a copy of its files, at the rule versions your configuration chooses, in `vendor/`. Trail Guide prepares your own rules and the ones you include from libraries in `generated/`, with a `RULES.md` index that helps agents find the rules to read.

Run the commands below from the project root. After initialization, you can also run project commands from its Git subdirectories. See [Project files](/reference/files/) for the layout.

## 2. Add your first rule

A rule you write in `.trailguide/rules/` belongs to this project. Let's add one about testing changed behavior.

First create its rule group. A rule group collects related rules and tells agents when to read them:

```sh
trailguide project add group practices/testing \
  --name Testing \
  --description 'Tests for the behavior this project provides.' \
  --when-to-read 'When adding or changing behavior, fixing bugs, or reviewing tests.'
```

Use the CLI to create a draft rule in that rule group:

```sh
trailguide project add rule practices/testing/test-changed-behavior \
  --title 'Test changed behavior' \
  --when-to-read 'When adding or changing externally visible behavior.' \
  --impact HIGH \
  --impact-description 'Prevents behavior changes from silently breaking existing use cases.'
```

Open `.trailguide/rules/practices/testing/test-changed-behavior.md` in your editor and replace its entire contents with this complete rule:

```md
---
title: Test changed behavior
whenToRead: When adding or changing externally visible behavior.
impact: HIGH
impactDescription: Prevents behavior changes from silently breaking existing use cases.
tags: testing
---

## Test changed behavior

When a change alters behavior that a caller or user relies on, add or update a test for that outcome.

For example, if a discount changes an order's total, assert the resulting total rather than which private helper was called.

Run the relevant tests before considering the change complete. Changes that do not alter behavior do not need a new test solely to accompany the edit.
```

The fields at the top describe the rule; the Markdown below tells the agent what to do. Replacing the draft also removes its unfinished-draft marker, so the rule is ready to build.

## 3. Build the guidance your agent will read

Adding a rule saves the instructions you've written. **Building prepares your project's rules for agents to use.**

With one rule, you could point your agent directly at the Markdown file. With dozens of rules from your project and several libraries, agents need help finding the relevant ones. You also need a way to choose which rules apply to your project.

Run:

```sh
trailguide project build
```

`trailguide project build` reads your rules and `.trailguide/config.yaml`, validates them, and writes the results into `.trailguide/generated/`. It:

- **Applies your choices:** includes the rules your configuration names, leaves out rules you've excluded, and substitutes your own replacements for library rules where configured.
- **Organizes the rules for reading:** creates `RULES.md` and rule group indexes with links and instructions that help agents find and read relevant rules. See [a sample `RULES.md` and the files it links to](/reference/files/#example-follow-a-rule-from-the-index).
- **Splits large indexes into pages:** keeps navigation manageable without shortening the full rule text.

If two rules give conflicting advice, you decide which to exclude or replace in configuration. `trailguide project build` applies those decisions; it doesn't detect or resolve contradictory advice on its own.

For this first build, open `.trailguide/generated/RULES.md` and follow its links to the Testing rule group and your rule. You've turned your source rule into part of the organized guidance your agent will use.

Keep editing rules in `.trailguide/rules/`, then build again to update the agent's copy. Don't edit `generated/` directly.

You can verify that the generated files match your source files:

```sh
trailguide project check
```

This checks the rule files, not whether your application follows the rules.

## 4. Include rules from the public library

Now let's reuse rules someone else has written. A **library** is a repository that publishes rule groups and skills for projects to take.

The [Fabrica public library](https://github.com/fabricahq/public-library) includes a code design practice group. Its rules help agents structure functions, modules, and folders, such as **Express operations as meaningful steps**, which keeps functions understandable without extracting unnecessary helpers. Read them before deciding to include them.

From the same project root, run:

```sh
trailguide project add source fabrica \
  --repository https://github.com/fabricahq/public-library.git \
  --rules 'practices/code-design/*'
```

This adds the library's repository to `.trailguide/config.yaml` under the source name `fabrica`, and includes every rule in its code design practice group with the selector `practices/code-design/*`. It also records the library's newest release as the source's `ref`, such as `ref: release/4`, so the configuration says which release the project takes.

Each rule in Fabrica's library has its own version. When you run `trailguide project update`, the project moves to the newest release, and each rule to its newest version, and `ref` changes with it.

Now run `trailguide project sync` to download the included rules and build the agent guidance. You don't need to run `trailguide project build` separately. `trailguide project sync` records the library release and the version of each rule it took, so your project keeps the same rules until you choose to [update them](/guides/update/).

```sh
trailguide project sync
trailguide project check
```

Open `.trailguide/generated/RULES.md` again. You'll now see your own Testing rule group and the library's "Code design" rule group. Your rule remains yours to edit. Each rule from the library shows its version and retains its source and license information.

## 5. Install a skill

Rules are granular: one practice each. A **skill** is coarser: a playbook for one kind of task, such as building a feature, that cites the rules it applies. Fabrica's library includes a **Build a feature** skill, `skills/build-feature`.

Open `.trailguide/config.yaml` and add a `skills` section to the `fabrica` source:

```yaml
schemaVersion: 2
skillTargets: [claude, agents]
sources:
  fabrica:
    repository: https://github.com/fabricahq/public-library.git
    ref: release/4
    rules:
      include: [practices/code-design/*]
    skills:
      include: [skills/build-feature]
```

Then sync and check again:

```sh
trailguide project sync
trailguide project check
```

Trail Guide writes the skill to `.claude/skills/build-feature/` and `.agents/skills/build-feature/`, where your coding agent finds skills. It also installs any skill that `build-feature` uses, and takes the rules it cites, even from rule groups you didn't include, so its links always work. The skill comes from the same library release as your rules: sync never moves a source, so it takes new rules and skills from the library release it recorded in step 4. If the library also requires skills of every project, sync installs those too; `trailguide project list skills` shows each skill's level. Those rules are there for the skill to reference; they don't join `RULES.md`.

Open `.claude/skills/build-feature/SKILL.md`. The steps come from the library. The section at the end, **Rules this skill applies**, is generated for your project: it links to your project's version of each rule the skill cites. `.trailguide/generated/SKILLS.md` lists every installed skill. To edit a skill, don't change the installed copy, which `trailguide project check` would reject; see [Install skills](/guides/install-skills/) to fork it.

## 6. Give the rules to your agent

Your own rules and the library's are now available in the repository. Your coding agent reads them because `trailguide project init` added this section to `AGENTS.md`, between `<!-- trailguide:begin -->` and `<!-- trailguide:end -->` markers. Agents that support skills also find installed skills on their own.

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

Step 2 points every agent to the [skills](/concepts/skill/) you installed. Trail Guide keeps the section current whenever you run `init`, `build`, `sync`, or `update`, and never changes your text outside the markers. Don't edit the section itself; write your own instructions above or below it. To manage `AGENTS.md` yourself instead, turn the section off with [`manageAgentsFile: false`](/reference/configuration/#agentsmd-section).

If your agent reads a different instruction file, have it import or link `AGENTS.md`. For Claude Code, add the line `@AGENTS.md` to `CLAUDE.md`.

Now ask your agent for a change as you normally would. For example, in a checkout project:

> Fix the bug where applying an expired coupon still reduces the order total.

You don't need to mention Trail Guide in each request. The project instructions tell your agent to read and apply the rules automatically.

For your first task, check that the agent reads the relevant rules and validates its changes. In this example, the testing rule calls for a test covering the expired coupon. These instructions guide the agent; they don't guarantee compliance. For a more detailed implementation and review workflow, see [For agents](/for-agents/).

## 7. Commit the guidance for your team

Once you've confirmed your agent reads and applies the rules to your liking, review and commit the entire `.trailguide/` directory, the installed skills in `.claude/skills/` and `.agents/skills/`, and `AGENTS.md` together. This includes your configuration, your own rules, the library's vendored files, and generated guidance. If your `.gitignore` excludes `/.claude/`, change that line to `/.claude/*` and add `!/.claude/skills/`, because Git can't re-include a folder inside an excluded one.

Anyone who checks out the repository, including a fresh cloud session, will then have the same rules, skills, and instructions for their agent on future tasks.

## Next steps

When you're ready, [include and customize more rules](/guides/select-rules/), [install more skills](/guides/install-skills/), or [update the rules and skills you use](/guides/update/).

To share rules and skills of your own, continue with [Create your first library](/start-here/create-library/).
