---
title: "Project"
description: "The codebase that uses rules and skills, and the files Trail Guide adds to it."
---

A **project** is the codebase whose rules and skills Trail Guide manages, such as a web application, a service, or a CLI tool. The **project root** is that codebase's top-level directory.

A project can use rules and skills written just for it, ones it takes from [libraries](/concepts/libraries/) and other repositories, or both. Each project chooses which ones to include and when to take updates. A repository can be a project and a library at once, with the same files.

## What a project includes

Your codebase keeps its existing files. Trail Guide adds a **Trail Guide directory**, `.trailguide/`, at the project root, and, once you install skills, a copy of each skill in the folders your agents read:

```text
my-project/
  src/
  README.md
  .trailguide/
    config.yaml
    rules/
    skills/
    assets/
    lock.json
    _managed/
      vendor/
      generated/
  .claude/skills/
  .agents/skills/
```

Inside `.trailguide/`:

- **Configuration** in `config.yaml` names the project's sources and the rules and skills it includes from each, plus versions, renames, and any rules or skills to exclude or replace.
- **Authored content** in `rules/`, `skills/`, and `assets/` holds the rule groups, rules, skills, and shared files you write for this project.
- **The lockfile**, `lock.json`, records the revision the project took from each source and the version of every rule and skill.
- **Vendor snapshots** in `_managed/vendor/` keep a copy of what the project takes from each source, with a record of the library release or commit it came from.
- **Generated guidance** in `_managed/generated/` combines your included rules into `RULES.md`, rule group pages, and rule files that agents can read, and lists installed skills in `SKILLS.md`.

Outside `.trailguide/`, Trail Guide writes **installed skills** into `.claude/skills/` and `.agents/skills/`, the targets your configuration names. It manages only the skill folders it installed there; other tools' skills in the same folders stay untouched.

You maintain the configuration and authored content. Trail Guide manages the vendor snapshots, generated files, and installed skills. For examples of each file, see [Project files](/reference/files/).

## Set up a project

Run `trailguide project init` from your codebase's root to create its `.trailguide/` directory. In a Git repository, subsequent project commands also work from subdirectories. Outside Git, run them from the project root.

Follow [Set up your first project](/start-here/set-up-project/) to add rules, generate guidance, and tell your agent to read it.
