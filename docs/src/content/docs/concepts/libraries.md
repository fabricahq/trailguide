---
title: "Library"
description: "Share rule groups and skills across projects, and set which skills each project gets."
---

A **library** is a repository whose manifest publishes its authored [rule groups](/concepts/groups/) and [skills](/concepts/skill/) for other repositories. Libraries let you share engineering practices across projects, within your organization, or with third parties.

You can publish your team's rules and skills in a library, or take them from libraries others publish. Rules and skills needed by only one project can live in that project.

## What a library contains

A library lives in a Git repository, in the same Trail Guide directory, `.trailguide/`, that every repository uses. It contains:

- **The manifest**, the `library` section of `.trailguide/config.yaml`, which makes the repository a library, declares its license, and sets the [level](#skill-levels) of each skill it offers.
- **Rule groups** under `rules/techs/` and `rules/practices/`, each with rule group metadata and Markdown rule files.
- **Skills** under `skills/`, one folder per skill, each with a `SKILL.md`. Optional.

For example, a repository named "engineering-rules" might contain:

```text
engineering-rules/
  LICENSE.md
  .trailguide/
    config.yaml
    rules/
      practices/testing/
        _group.yaml
        test-changed-behavior.md
    skills/
      build-feature/
        SKILL.md
```

The repository name is your choice. Rules can also include [supporting assets](/reference/rule-format/#supporting-assets). A library can also be a project: the same configuration can take rules and skills from other sources for the library's own work.

## Skill levels

A library sets a **level** for each skill it offers: **required** skills install in every project that uses the library, **recommended** skills install in projects that include the library's recommendations, and **offered** skills install only when a project names them. An organization can publish an **org library**, such as `<organization>/.trailguide`, whose required and recommended skills become the shared default for its projects.

A library can also list skills from another repository at a level, **curating** them, such as a third party's skill its teams should use. See [Skill levels](/concepts/skill/#skill-levels).

## How projects use a library

Each [project](/concepts/project/) names the libraries it uses as **sources** in `.trailguide/config.yaml`, and includes rules and skills from each with selectors, such as `practices/testing/*` or `recommended`. A project can use several libraries and add its own rules and skills.

Running `trailguide project sync` downloads the included rules and skills, generates the guidance agents read, and installs the skills. Rule groups with the same ID combine, and the project's exclusions and replacements determine which rules appear.

A project records the library release it took from each library, and the version of every rule and skill. Running `trailguide project update` moves them to newer versions and reports every one that changed, so each project takes updates on its own schedule. A project can also pin a rule to a version, with a reason, or include only specific rules instead of whole rule groups. Skills aren't pinned: all of a library's skills come from one library release, and move together when the project updates. See [Include rules](/guides/select-rules/), [Install skills](/guides/install-skills/), and [Update rules and skills](/guides/update/) for the steps.

## How a library publishes rules

:::note[Preview]
Projects don't read skill versions yet: a library release records each skill's version, but a project installs a library's skills at the release's commit without showing their versions.
:::

A library doesn't have one version for all of its rules and skills. Each rule and skill has its own version ([rules](/concepts/rule/#how-a-rule-is-versioned), [skills](/concepts/skill/#how-a-skill-is-versioned)), so a project can see exactly which rules changed in a library release and whether work that complied with the previous version could fail the new one.

When an author changes a rule or skill, they add a change note saying how large the change is and what changed. A **library release** turns the pending notes into new versions at once, and can hold one change or many. Projects take only published versions, unless one deliberately takes an exact commit, so they never see changes that haven't been published.

`trailguide library release` publishes a library release as one Git tag that records every rule's and skill's version, along with one GitHub Release page that lists the changes. A library release has a number for reference, but it isn't a version of the library: projects choose versions rule by rule, and take a library's skills together from one library release. See [Version your rules](/guides/version-rules/).

## Sharing a library

Libraries can be public or private. Projects keep a vendor snapshot of what they take, so use private libraries only in projects allowed to hold and share that content.

Trail Guide preserves declared license text and notices with the rules and skills projects take. For choosing terms, see [License rules](/guides/license-rules/).

To publish your own, follow [Create your first library](/start-here/create-library/). For file details, see [Library schema](/reference/library-schema/).
