---
title: "Rule group"
description: "How rule groups organize rules and help agents find relevant guidance."
---

A **rule group** is a collection of related [rules](/concepts/rule/) and tells agents when to read them.

Rule groups can live in a **library** for projects to take, or in a **project** for that project alone. Projects can combine rule groups from libraries with their own rules.

Rule groups hold rules only. [Skills](/concepts/skill/) don't belong to rule groups, and including a rule group's rules never installs a skill.

## Rule group types

There are two kinds of rule groups: **technology groups** under `techs/` and **practice groups** under `practices/`. Every rule group is one or the other. Both live in the `rules/` folder of the Trail Guide directory, `.trailguide/rules/`.

### Technologies

**Technology groups** live under `techs/` and cover named technologies, such as "TypeScript", "React", "Playwright", or "AWS".

For example, `techs/playwright` holds rules about writing browser tests with Playwright.

### Practices

**Practice groups** live under `practices/` and cover practices that apply across technologies, such as "Testing", "Observability", or "Error handling".

For example, `practices/testing` holds rules about testing behavior, regardless of the language or test framework.

## Rule group identities

A rule group's ID is its path starting with `techs/` or `practices/`. A rule group named "Automated Tests" might have the ID `practices/testing`.

When a project builds or syncs, its own rules and the rules it includes from sources with the same rule group ID appear together in the generated rule group.

Use `--name` when creating a rule group to give it a readable title; the title does not change its ID.

## Canonical rule group IDs

Each library chooses its own rule group IDs, so the same technology could be `techs/go` in one library and `techs/golang` in another. The canonical rule group list, `canonical-groups.yaml` at the root of the [Trail Guide repository](https://github.com/fabricahq/trailguide), names the shared IDs, such as `techs/go` and `practices/testing`. Each entry has a display name and a line saying which rules belong in the rule group.

When you create a rule group, use a canonical ID if one fits. Tools and catalogs can read the list to combine rule groups across libraries, for example to show every library's Go rules together. A rule group whose ID isn't on the list works the same way in projects, but those tools show it on its own. The list has no aliases: a rule group ID is part of every rule's ID, so another name for the same rule group is a different rule group.

The list on `main` changes as maintainers add rule groups. A tool should read the file at an explicit commit, such as `https://raw.githubusercontent.com/fabricahq/trailguide/<commit>/canonical-groups.yaml`, and update that commit deliberately, as it would a dependency.

To propose a canonical rule group, open a pull request that adds an entry to `canonical-groups.yaml`. Say which rules belong in it and how it differs from the rule groups already on the list.

## What a rule group contains

A rule group is a folder containing:

- **Metadata** in `_group.yaml`: its name, description, and when agents should read it.
- **Rules**: one Markdown file per rule.

For example, relative to the repository root:

```text
/.trailguide/rules/practices/testing/
  _group.yaml
  verify-retry-limits.md
  test-changed-behavior.md
```

The layout is the same whether the rule group belongs to a library or only to a project.

You maintain the metadata and rules you author. The CLI supplies rule group READMEs; you don't need to keep them in sync. Building or syncing creates the guidance agents read.

See [Rule and rule group format](/reference/rule-format/#rule-group-metadata) for the fields and file layout.

## Assets

Rules can optionally link to diagrams, sample data, or longer explanations in `assets/`. You maintain these supporting files; Trail Guide includes them in the generated guidance. See [Supporting assets](/reference/rule-format/#supporting-assets) for the layout.

## When to read a rule group

Agents start at the generated `RULES.md`, choose rule groups using their `whenToRead` descriptions, then read those rule groups' rules. Each rule's conditions and exceptions determine whether it applies to the task.

For the complete reading process, see the [agent workflow](/for-agents/). To choose rule groups for your project, follow [Include rules](/guides/select-rules/).
