---
title: "Create your first library"
description: "Write and validate one shared rule, then publish it for projects to take."
---

Let's publish rules that other projects can use. A **library** is a repository whose manifest publishes its rule groups, and optionally skills, for other projects to take. You maintain it independently of those projects. A **rule** is a Markdown file that explains one practice you want agents to follow.

In this walkthrough, you'll:

1. Create a library and write one rule about error messages.
2. Check the rule's format and choose terms for sharing it.
3. Commit the library and publish the rule's first version in a library release.
4. Include the rule in a project and inspect the guidance its agents will read.

You need [Trail Guide installed](/start-here/install/) and Git. You don't need to complete the project walkthrough first. Rules intended for only one codebase can live [in that project](/start-here/set-up-project/).

## 1. Create a repository for your library

Give your library its own Git repository so you can publish its rules independently of the projects that use them. Start in the directory where you keep your repositories:

```sh
mkdir engineering-rules
cd engineering-rules
git init
trailguide library init
```

`engineering-rules` is an example repository name; you can choose another. `trailguide library init` creates:

- `.trailguide/config.yaml`, with a `library` section, the **manifest**, that makes the repository a library.
- `.trailguide/rules/` and `.trailguide/skills/`, where you write the rules and skills the library publishes.
- A README for authors at the repository root.
- `.github/workflows/trailguide.yml`, which checks pull requests once the library is on GitHub.

You'll publish this repository to your Git host in step 5.

Initialize from the repository root. After that, library commands also work from its subdirectories.

You may see a reminder that the library has no declared license. We'll address that before sharing it.

## 2. Add your first rule

A **rule group** collects related rules and tells agents when to read them. Create a practice group named "Error handling" to hold rules about how your code handles errors:

```sh
trailguide library add group practices/error-handling \
  --name 'Error handling' \
  --description 'Help callers and users understand and recover from failures.' \
  --when-to-read 'When implementing or reviewing error handling and error messages.'
```

Use the CLI to create a draft rule in that rule group:

```sh
trailguide library add rule practices/error-handling/make-errors-actionable \
  --title 'Make errors actionable' \
  --when-to-read 'When writing or reviewing an error message a user or caller will receive.' \
  --impact MEDIUM \
  --impact-description 'Helps people recover from a failure without guessing what went wrong.'
```

Open `.trailguide/rules/practices/error-handling/make-errors-actionable.md` in your editor and replace its entire contents with this complete rule:

```md
---
title: Make errors actionable
whenToRead: When writing or reviewing an error message a user or caller will receive.
impact: MEDIUM
impactDescription: Helps people recover from a failure without guessing what went wrong.
tags: errors
---

## Make errors actionable

Explain what failed and what the user or caller can do next. Include relevant context that is safe to disclose.

Instead of "Invalid configuration", say "The configuration is missing a repository URL. Add a repository value under sources.acme-rules."

Do not include secrets, credentials, or private payloads in an error message. When recovery is not possible, explain the limitation rather than suggesting a retry that cannot help.

Check the message against the failure it describes: the explanation should be accurate and the suggested next step should address the cause.
```

The fields at the top describe the rule; the Markdown below tells the agent what to do. Replacing the draft removes its unfinished-draft marker.

The rule's path in `.trailguide/rules/` without `.md` is its ID: `practices/error-handling/make-errors-actionable`. Keep that path stable after projects start including it.

Your library now looks like this:

```text
engineering-rules/
  .github/workflows/trailguide.yml
  README.md
  .trailguide/
    config.yaml
    rules/
      practices/
        error-handling/
          _group.yaml
          README.md
          make-errors-actionable.md
    skills/
```

Library rules live under `.trailguide/rules/practices/` or `.trailguide/rules/techs/`, in the same Trail Guide directory every repository uses. A library doesn't need generated agent guidance to publish; that belongs to the projects that take its rules, including the library repository itself if you also use it as a project.

## 3. Check the library

You've written a rule. Now check that Trail Guide can read it:

```sh
trailguide library check
```

The result should report **1 group and 1 rule**, and preview the first library release, which gives `practices/error-handling/make-errors-actionable` version `1.0.0`. The command checks the manifest, rule group definitions, and rule format without changing your files. Read the rule yourself to decide whether its advice is clear and useful.

There is no library build step. A library publishes source rules; each consuming project builds its own agent guidance after choosing rules and applying its exclusions and replacements. See [how project builds work](/start-here/set-up-project/#3-build-the-guidance-your-agent-will-read).

## 4. Choose terms before sharing

Decide who may use, adapt, and redistribute the library. Public visibility alone does not grant those permissions. Use [License rules](/guides/license-rules/) to choose and record terms, especially when including someone else's material.

For example, if you choose MIT for your own rules, add the complete MIT text with the appropriate copyright notice to `LICENSE.md` at the repository root, then declare it in the manifest in `.trailguide/config.yaml`:

```yaml
schemaVersion: 2
sources: {}
library:
  schemaVersion: 1
  license:
    spdxExpression: MIT
    file: LICENSE.md
    notices: []
```

This records your choice; Trail Guide does not supply or infer the license text. Retain any required notices for adapted material and declare them in `notices`.

Check the library again after adding the license file and declaration:

```sh
trailguide library check
```

The check verifies that the declared files exist and that the declaration is valid. It does not decide whether you have permission to publish someone else's material.

## 5. Publish the library

Once the library passes its checks and you've reviewed the rule and license, commit the files:

```sh
git add .github README.md LICENSE.md .trailguide/
git commit -m "Create the first shared rule"
```

Create an empty repository on your Git host, then replace the example URL below with its Git URL and push:

```sh
git remote add origin https://github.com/YOUR-ORG/engineering-rules.git
git push -u origin HEAD
```

Preview your first library release:

```sh
trailguide library release --dry-run
```

It shows the release notes that will be published: this is your first library release, so every rule gets version `1.0.0`. Then publish it:

```sh
trailguide library release
```

The command tags your commit `release/1`, recording `practices/error-handling/make-errors-actionable` at version `1.0.0`, and pushes the tag. On GitHub.com, it also creates a GitHub Release page that announces the library release, using the [GitHub CLI](https://cli.github.com/), which must be installed and signed in; to publish the tag without the page, pass `--no-github-release`. If your library isn't on GitHub.com, it creates the tag only. No files change.

Your library is now available to other projects. The repository can be public or private; consuming projects need access to it.

Each rule has its own version. From now on, every change to a rule needs a **change note**, recorded with `trailguide library change`, that tells projects what changed. Change rules through pull requests, each with its note; the workflow in `.github/workflows/trailguide.yml` runs `trailguide library check` on each one. When you're ready, publish the next library release with `trailguide library release`, just as you did the first. See [Version your rules](/guides/version-rules/) for the full workflow.

## 6. Try the library in a project

In a [project you've set up](/start-here/set-up-project/), run the following from its root directory. Replace the example URL below with your library's Git URL:

```sh
trailguide project add source acme-rules \
  --repository https://github.com/YOUR-ORG/engineering-rules.git \
  --rules 'practices/error-handling/*'
```

This adds the library's repository to `.trailguide/config.yaml` under the source name `acme-rules`, with `ref: release/1`, the library release you just published, and includes every rule in its error handling practice group. The project moves to each newer release, and each rule's newest version, whenever someone runs `trailguide project update`.

Download the rules and build the project's agent guidance:

```sh
trailguide project sync
trailguide project check
```

Open `.trailguide/_managed/generated/RULES.md` and follow its "Error handling" rule group to your shared rule, which shows version `1.0.0`. `trailguide project sync` records that version, so the project keeps the same rules until someone runs `trailguide project update`.

The rules are now in the project, but its agent needs instructions to read them. If you haven't already, [connect the rules to your agent and try a task](/start-here/set-up-project/#6-give-the-rules-to-your-agent). Then commit the project's configuration, vendored rules, generated guidance, and agent instructions together.

Other projects can take rules from the same library. You maintain the shared rule in the library, and each project chooses when to take your updates.

## Optional: add a skill

A library can also hold **skills**: coarser-grained playbooks, such as how to fix a reported bug, that cite the rules they apply. Skills live in `.trailguide/skills/` beside your rules and are versioned like rules. Create a draft:

```sh
trailguide library add skill fix-bug \
  --description 'Fix a reported bug and prove it with a regression test. Use when a task starts from a bug report.' \
  --type playbook
```

Complete `.trailguide/skills/fix-bug/SKILL.md`, list the rules its steps apply, such as `practices/error-handling/make-errors-actionable`, in its `trailguide-rules-used` metadata, and run `trailguide library check`. The skill is offered: projects install it only when they name it. To recommend it to every project, or require it, add it to the manifest's `library.skills` levels; see [Skill levels](/reference/library-format/#skill-levels). Your library has published its first library release, so record the new skill with `trailguide library change skills/fix-bug --summary '...'` before you publish the next one. [Write a skill](/guides/write-skills/) walks through each step.

## Next steps

As your library grows, [write focused rules](/guides/write-rules/) and [skills](/guides/write-skills/), record each change with `trailguide library change`, and [publish them in library releases](/guides/version-rules/). Projects [choose when to take updates](/guides/update/).

For metadata and supporting files, see [Rule and rule group format](/reference/rule-format/), [Skill format](/reference/skill-format/), and [Library format](/reference/library-format/).
