---
title: "Include rules"
description: "Choose rule groups and rules from libraries and adapt them to your project."
---

You can **include rules** written by your team or others instead of writing every rule yourself. These rules come from **libraries**, repositories that publish rules for other projects to take.

In this guide, you'll choose **rule groups** and rules to include, record your choices in `.trailguide/config.yaml`, and add any project-specific rules or exceptions. Then you'll generate and review the files your agent will read.

Start with a project that already has a `.trailguide/` directory. If you haven't created one, follow [Set up your first project](/start-here/set-up-project/).

## Choose rule groups for your project

Start with the project's technologies and engineering practices.
Inspect its dependencies, architecture, and expected behavior, then read each library's rule group descriptions and `whenToRead` guidance.

For a TypeScript service that makes requests to other services, useful rule groups might include:

- `techs/typescript` for language rules.
- `practices/testing` for verifying behavior.
- `practices/observability` for making failures and retries visible.
- `practices/error-handling` for how failures reach callers.

Include rules that cover the project's work, including practices that do not correspond to a package or filename.
Agents choose which rule groups apply to each task using the [rule-loading process](/for-agents/).

## Include rules from each source

In `.trailguide/config.yaml`, each library you take rules from gets a named entry under `sources`, such as `fabrica`. For each source, choose:

- **Where to get it:** `repository` is the library's Git URL.
- **Which rules to include:** `rules.include` lists [selectors](/reference/configuration/#rule-selectors). A rule group prefix, such as `practices/testing/*`, includes every rule in the rule group, including rules the library adds to it later. A rule ID, such as `techs/go/wrap-errors-with-operation`, includes that rule without the rest of its rule group.
- **Which versions to use:** by default, each rule moves to its newest version when you update. Add `rules.pins` to keep individual rules at exact versions. See [Choose versions](/reference/configuration/#choose-versions).
- **Which skills to install:** optionally, `skills.include` names skills from the source. Including rules never installs a skill. See [Install skills](/guides/install-skills/).

```yaml
schemaVersion: 2
skillTargets: [claude, agents]
sources:
  fabrica:
    repository: https://github.com/fabricahq/.trailguide.git
    ref: release/4
    rules:
      include:
        - techs/typescript/*
        - practices/testing/*
        - practices/observability/*
        - practices/error-handling/*
```

You can also use `rules.exclude` to leave out individual rules, optionally replacing them with your own. We'll cover both below.

See the [complete configuration example](/reference/configuration/#complete-example) for how these fields fit together.

To include every rule a library publishes, use `"*"`.
Use `practices/*` for every practice group, or `techs/*` for every technology group.
Exclusions and replacements still apply.
When a library adds rule groups or rules that your selectors reach, `trailguide project update` adds them. Review them as part of that update.

If two sources supply `practices/testing`, their rules combine into one generated testing page, with full rules or summaries and links to individual resolved rules.
Source-prefixed IDs keep matching rule paths distinct.
Neither source automatically overrides the other.

Define your own rule groups with `_group.yaml` under `.trailguide/rules/`; they need no configuration entry.
The [Rule group concept](/concepts/groups/) explains the distinction between technology groups and practice groups.

## Adapt the selection to your project

Use shared rules as your starting point.
Add project-specific obligations, exclude a rule with a reason, or replace it with a complete rule of your own.
Keep those decisions in configuration and authored files so agents read the resolved result.

### Add a rule

Write a Markdown rule in the matching rule group under `.trailguide/rules/`:

```text
.trailguide/rules/practices/testing/test-project-contracts.md
```

The rule group needs `_group.yaml` metadata from an included library or from `.trailguide/rules/<group-id>/`.
If you later stop including, or remove, the last library supplying the rule group, sync copies the rule group's last vendored metadata to `.trailguide/rules/<group-id>/_group.yaml` and warns that it did, so the rule keeps its rule group; see [Keep the rule group of an authored rule](/reference/sync/#keep-the-rule-group-of-an-authored-rule).
The rule joins the rules from your sources, and its ID has no source name: `practices/testing/test-project-contracts`.
Use the [authoring format](/guides/write-rules/) for its metadata and body.

For a rule group only your project has, include its own `_group.yaml`. It is discovered automatically.
When a library later supplies that rule group, your metadata and rules stay in place. Your metadata supplies the project's rule group description; the library's rules join the rule group.
Rule groups from sources retain source-labeled metadata from every contributing library.

### Exclude a rule

Add its source-relative ID and a reason to `sources.<name>.rules.exclude`:

```yaml
sources:
  acme:
    rules:
      exclude:
        - id: practices/testing/avoid-snapshot-tests
          reason: Contract snapshots follow our separate review policy.
```

These are partial snippets, not complete source definitions.
Merge them into the `acme` and `fabrica` sources from the [configuration example](/reference/configuration/), retaining their repository, pins, and `include`.
Rule IDs are illustrative and must name rules the source's `include` reaches. An `id` can also be a prefix, such as `practices/observability/*`, to leave out a whole rule group that a wider `include` brings in.
An exclusion removes only the named source's rule, without introducing a replacement.
The same rule path in another source remains active.
The exclusion reason stays in project configuration; the generated files contain only active rules.

### Replace a rule

Write a complete rule of your own, then exclude the library's rule and name your rule's ID as its `replacedBy`:

```yaml
sources:
  fabrica:
    rules:
      exclude:
        - id: techs/typescript/prefer-type-aliases
          reason: Our public extension API relies on declaration merging.
          replacedBy: techs/typescript/prefer-interfaces
```

Your rule lives at `.trailguide/rules/techs/typescript/prefer-interfaces.md`.

To start from the library's text instead of writing your own, [fork the rule](/reference/cli/#fork-a-library-rule), which copies it into `.trailguide/rules/` and writes the exclusion for you, with the forked version as `basedOn`. With `basedOn`, `trailguide project update` lists the library's changes after that version, so you can decide whether your rule needs them; see [Exclude or replace a rule](/reference/configuration/#exclude-or-replace-a-rule).

The resolved rule uses your complete definition, including its ID, title, reading cue, impact, body, attribution, and asset references.
In this example, the generated ID is `techs/typescript/prefer-interfaces`.
Configuration records what it replaced and why, and the generated rule shows both below its rule ID.
Its file stays in the target rule group and does not also become an additional rule.

Replacing “prefer type aliases” with “prefer interfaces” leaves only your definition in the effective output.
Include the intended scope and exceptions in that definition.

## Generate and review

When sources, selectors, or pins change, run `trailguide project sync`. Sync takes newly included rules from the library release your project already recorded for the source; to take newer ones, [update the source](/guides/update/).
For changes limited to your own rules or exceptions, run `trailguide project build` against the existing vendor snapshots.
Review and commit the updated generated files with their inputs.

Trail Guide rejects exclusions of rules the source's `include` doesn't reach, missing replacement rules, and a rule of yours that replaces more than one rule.
It does not infer overrides from similar wording.
Rules from different sources remain active even when their paths or titles match.
If one of your rules or another source contradicts an obligation from a source, explicitly replace or exclude the affected rule inside its owning source.
Source order never resolves the conflict.
Use [Resolve conflicting rules](/guides/conflicting-guidance/) to review the combined rules and resolve competing instructions.

Review `_managed/generated/RULES.md`, each relevant rule group index, and the full resolved definitions. Check exclusions and replacements against configuration and provenance.
Commit configuration, your rules, vendor snapshots, and generated files together.
Use [Update rules and skills](/guides/update/) when you take new library releases or change what you include later.
To add procedures that apply these rules step by step, see [Install skills](/guides/install-skills/).
