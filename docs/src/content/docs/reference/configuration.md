---
title: "Configuration"
description: "Fields in .trailguide/config.yaml, the one configuration file of every project and library."
---

:::caution[Preview]
This page describes version 2 configuration and branch refs, which aren't in a released version of Trail Guide yet. The released CLI reads version 1 configuration, and its `ref` takes only a tag or a full commit SHA.
:::

Every repository that uses Trail Guide has one configuration file, `.trailguide/config.yaml`. It records the repository's sources, the rules and skills it includes from each, its exceptions, and where to install skills. A repository whose configuration also has a [`library` section](#library-manifest) is a **library**, and publishes its authored rules and skills for other repositories. A repository can be a project, a library, or both, with the same file and the same [layout](/reference/files/).

Initialize from the repository root. In Git repositories, other commands find the nearest Git root and use its `.trailguide/config.yaml`. Outside Git, run commands from the repository root. See [Working directories](/reference/cli/#working-directories).
Sync, build, and check validate the fields below.

Use one YAML document. Duplicate keys, anchors, aliases, and explicit tags are rejected. Quote selectors that start with `*`, such as `"*"`.

## Complete example

```yaml
schemaVersion: 2
skillTargets: [claude, agents]
sources:
  fabrica:
    repository: https://github.com/fabricahq/.trailguide.git
    rules:
      include: [techs/go/*, practices/testing/*]
      exclude:
        - id: practices/testing/table-tests
          reason: Our test style is documented in CONTRIBUTING.md.
      pins:
        - id: practices/testing/verify-retry-limits
          version: "1.3.0"
          reason: Waiting on the author's response to fabricahq/.trailguide#45.
    skills:
      include: [recommended, skills/deploy-preview]
      rename:
        skills/review-change: fabrica-review
  pstack:
    repository: https://github.com/example/pstack.git
    ref: main
    skills:
      include: ["*"]
      uses:
        skills/architect: [skills/how]
```

This project takes every Go rule and every testing rule from Fabrica's library except one, and keeps one rule at an older version. It installs the skills Fabrica requires, the ones it recommends, and one offered skill, and installs Fabrica's `review-change` skill under another name. From `pstack`, a repository without a manifest, it installs every skill from the newest commit on `main` at its last update.

Repository names, rule IDs, and skill IDs in examples are illustrative.
Replace them with libraries, rules, and skills your project can access.

## Fields

| Field | Meaning |
| --- | --- |
| `schemaVersion` | Required. `2`. See [No backward compatibility](#no-backward-compatibility). |
| `skillTargets` | Optional. Where installed copies of skills go: an array of `claude` (`.claude/skills/`) and `agents` (`.agents/skills/`). Missing means `[]`. See [Skill targets](#skill-targets). |
| `manageAgentsFile` | Optional. `true`, the default, has init, build, sync, and update keep a Trail Guide section in the repository's `AGENTS.md`, and check verify it. `false` leaves `AGENTS.md` alone. See [AGENTS.md section](#agentsmd-section). |
| `sources` | Map of stable source names to sources. Use an empty object for a repository that uses only its authored rules and skills. |
| `sources.<name>.repository` | Required. Explicit HTTPS or SSH Git address, including scp-style SSH. See [Repository addresses](#repository-addresses). |
| `sources.<name>.ref` | The branch, tag, commit, or library release to take the source from. Required for a source without `release/<number>` tags; see [Refs](#refs). |
| `sources.<name>.rules.include` | Optional [rule selectors](#rule-selectors): rule IDs, prefixes ending in `/*`, or `"*"`. |
| `sources.<name>.rules.exclude` | Optional list of rules to leave out, each with an `id` selector and a `reason`, and optionally a [replacement](#exclude-or-replace-a-rule). |
| `sources.<name>.rules.pins` | Optional list of rules to hold at one version, each with an `id`, a `version`, and a `reason`. See [Pin a rule](#pin-a-rule). |
| `sources.<name>.skills.include` | Optional [skill selectors](#skill-selectors): `recommended`, `"*"`, or skill IDs. |
| `sources.<name>.skills.exclude` | Optional list of skills to leave out, each with an `id` and a `reason`. See [Exclude a skill](#exclude-a-skill). |
| `sources.<name>.skills.forks` | Optional map of a skill ID to the authored skill installed in its place, with `as`, a `reason`, and optionally `basedOn`. See [Fork a skill](#fork-a-skill). |
| `sources.<name>.skills.uses` | Optional map of a skill ID to the skill IDs it calls that its own metadata doesn't declare. See [Declare the skills a skill calls](#declare-the-skills-a-skill-calls). |
| `sources.<name>.skills.rename` | Optional map of a skill ID to the install name it gets in this repository. See [Rename a skill](#rename-a-skill). |
| `library` | Optional. The [manifest](#library-manifest): what this repository publishes as a library, and the level of each skill it offers. |

Unknown fields are rejected, at every level.
Every part of a source except `repository` is optional. A source with no `include` still installs the skills its library [requires](#skill-levels), and still supplies the skills it [forks](#fork-a-skill).
Authored rules, rule groups, and skills are discovered from `rules/` and `skills/` in the Trail Guide directory; they need no configuration entry. See [Project files](/reference/files/).

## Selectors

The selection from a source is everything its `include` lists name, minus everything its `exclude` lists name. Rules and skills each have their own lists, so a rule selector never installs a skill, and a skill selector never includes a rule.

### Rule selectors

| Selector | Includes |
| --- | --- |
| `practices/testing/verify-retry-limits` | One rule, by its ID. |
| `practices/testing/*` | Every rule in the `practices/testing` rule group, at any depth, including rules the library adds to it later. |
| `practices/*` | Every practice rule, in every practice group. `techs/*` is every technology rule. |
| `"*"` | Every rule in the source. |

A selector is a rule ID, or a prefix that ends in `/*`. The `*` is allowed only at the end, after a `/`, or alone: `techs/go*`, `techs/*/testing`, and `**` are rejected. A rule ID names the rule's path in its library's `rules/` folder, without `.md`.

`exclude` entries take the same forms. Use a prefix to leave out a whole rule group that a wider `include` brings in:

```yaml
rules:
  include: [practices/*]
  exclude:
    - id: practices/observability/*
      reason: This CLI has no hosted service to observe.
```

`pins` take single rule IDs only.

Each rule ID in `include` must name a rule the source publishes, and each prefix must match at least one rule, except `"*"`. A rule included by an ID and also by a prefix is included once. When a rule brings in its rule group, it brings the rule group's metadata, so the rule group appears in the generated index with only the included rules.

### Skill selectors

| Selector | Installs |
| --- | --- |
| *(no `include`)* | The skills the library requires. |
| `recommended` | The skills the library requires and those it recommends. |
| `"*"` | Every skill the source offers, at any level. |
| `skills/deploy-preview` | That skill, whatever its level, plus the required ones. |

Skill selectors have no prefixes: `skills/*` and `recommended/*` are rejected. A library's required skills always install, whatever `include` says; see [Skill levels](#skill-levels). A skill a library curates from another repository is named as the library's manifest names it, with the curated source's name, such as `mattpocock:skills/engineering/to-spec`; see [Curated skills](#curated-skills).

`exclude` leaves skills out, each entry with a reason. An entry names one skill by its ID, or a level:

| Entry | Leaves out |
| --- | --- |
| `skills/deploy-preview` | That skill. |
| `recommended` | Every skill the library recommends, even one `include` names. Use it with `include: ["*"]` to install the offered skills without the recommended ones. |
| `"*"` | Every skill that isn't required, so the project installs only the library's required skills. Use it to take a library's rules without its optional skills. |

An exclusion wins over `include`, but never over a required skill: excluding a required skill by ID fails validation, and a level exclusion leaves required skills installed. A level exclusion also leaves in a skill that another installed skill uses, so the installed skill works; to leave such a skill out, exclude it by ID. `recommended` fails for a source that isn't a library, since such a repository recommends nothing; exclude its skills by ID, or exclude `"*"` to install none of them.

## Skill levels

A library sets one of three **levels** for each skill it publishes, in its [manifest](#library-manifest):

| Level | Installed in a project that uses the source when |
| --- | --- |
| Required | Always, whatever `include` says. The project can't exclude or fork it. |
| Recommended | `include` lists `recommended` or `"*"`, or names the skill. |
| Offered | `include` lists `"*"`, or names the skill. |

A library's own skill that no level lists is offered. A source without a manifest, a **non-library source**, offers every skill it has and requires none.

A project sees only the levels of the libraries it names as sources. When a library curates a skill from another library, the project sees the level the curating library sets, not the one the original library sets: levels don't chain.

Because required skills install with no `include`, a source declared with only a `repository` installs its library's required skills, now and after every update that adds one:

```yaml
sources:
  fabrica:
    repository: https://github.com/fabricahq/.trailguide.git
```

Renaming a required skill is allowed, since it stays installed.

## Exclude or replace a rule

A source's `rules.exclude` list leaves rules out of the generated guidance. Each entry records why, and can name an authored rule for agents to read instead:

```yaml
rules:
  exclude:
    - id: practices/testing/avoid-snapshot-tests
      reason: Contract snapshots follow our separate review policy.
    - id: techs/typescript/prefer-type-aliases
      reason: Our public extension API relies on declaration merging.
      replacedBy: techs/typescript/prefer-interfaces
      basedOn: "2.0.0"
```

| Field | Meaning |
| --- | --- |
| `id` | Required. A [rule selector](#rule-selectors). |
| `reason` | Required non-blank text explaining why the repository leaves the rule out. |
| `replacedBy` | Optional, with a single rule ID only. The ID of an authored rule in `rules/` that agents read in place of the excluded rule. It must be in the same rule group as the rule it replaces, and each authored rule can replace only one rule. |
| `basedOn` | Optional, and only with `replacedBy`. The library version of the excluded rule that your rule is based on, in quotes, such as `"2.0.0"`. [Forking a rule](/reference/cli/#fork-a-library-rule) records the forked version here, and so does [replacing a fork with the newest version](/reference/cli/#replace-a-fork-with-the-newest-version). Offline commands check only that it's a version; `trailguide project sync` checks that the library published that version of the rule, and otherwise fails with `version-not-found`, listing the versions it did publish. |

An excluded rule is still kept in the source's vendor snapshot, so you can review its changes. The `trailguide project update` preview lists a replaced rule as `replaced` so you can decide whether your rule needs the same change:

- With `basedOn`, whenever the library's newest version of the rule is newer than `basedOn`, whatever version the project includes. The preview lists every change after `basedOn`. To replace your rule with a fork of the newest version, overwriting your edits, pass `trailguide project update --update-fork SOURCE:RULE`, or answer `replace` to the terminal question; either sets `basedOn` to that version. If you merge the changes into your rule by hand instead, edit `basedOn` to the version you merged. Trail Guide never advances it otherwise, because only you know whether your rule took the changes in.
- Without `basedOn`, such as for a replacement you wrote yourself, whenever the library's newest version of the rule is newer than the version the project includes, also when a pin holds the vendored copy there. The preview lists the changes since that version, which your rule may already have. Replacing your rule with a fork, as above, also records `basedOn`.

## Exclude a skill

```yaml
skills:
  exclude:
    - id: skills/write-changelog
      reason: Release Planner writes our release notes.
```

An excluded skill isn't installed into the target folders, and `generated/SKILLS.md` lists it under its excluded skills with your reason. An entry takes only an `id` and a `reason`; to install your own skill in its place, [fork it](#fork-a-skill) instead.

Excluding a [required skill](#skill-levels) fails validation and names the library that requires it. To stop using a required skill, stop using the source.

When another installed skill calls a skill you exclude, `trailguide project sync` warns that the skill it calls isn't installed. Fork the skill instead to give the calling skill your version.

You can also exclude a rule that an installed skill cites, such as one outside your rule `include`. The skill's installed copy then says the rule is excluded by this repository, with your reason.

## Fork a skill

A source's `skills.forks` installs an authored skill of yours in place of one of the source's skills, and records where it came from:

```yaml
sources:
  pocock:
    repository: https://github.com/mattpocock/skills.git
    ref: main
    skills:
      forks:
        skills/engineering/fix-bug:
          as: skills/fix-bug
          basedOn: "9f2c4e1a7b3d5f60817a2b4c6d8e0f1a3b5c7d9e"
          reason: We use our own bug workflow.
```

| Field | Meaning |
| --- | --- |
| *(key)* | The ID of the source's skill, such as `skills/engineering/fix-bug`, or of a skill the source's library [curates](#curated-skills), such as `mattpocock:skills/to-spec`. |
| `as` | Required. The ID of your authored skill in `skills/`, `skills/<name>`, installed in place of the source's skill under its own name. |
| `reason` | Required non-blank text explaining why the repository uses its own skill. |
| `basedOn` | Optional. The full 40-character commit SHA, in quotes, of the source's copy your skill was made from. For a curated skill, it's a commit of the repository the library curates it from. Leave it out for a skill you wrote from scratch. |

[Forking a skill](/reference/cli/#fork-a-skill) copies it into `skills/` and writes the entry for you, with the commit it copied as `basedOn`. To fork one by hand, copy its folder into `.trailguide/skills/<name>/`, correct its frontmatter, and add the entry.

A fork needs no `skills.include` entry. The source supplies each skill it forks from its recorded revision, whatever `include` selects, so updates can compare it with your fork, and never installs it, nor the skills only the source's copy uses: your fork's own `trailguide-skills-used` decides what it needs. A source can exist only for its forks. If the source deletes the skill, your fork stays installed, even when `include` still names the skill, and the next update says the source no longer has it.

Each authored skill can be the fork of only one skill, and a rule can't replace a skill or the reverse. A skill can't be both forked and excluded, and a fork installs under its own name, so a forked skill can't be renamed either. Forking a [required skill](#skill-levels) fails validation, as excluding one does.

When another installed skill calls a skill you fork, its installed copy lists your skill instead in its "Skills this skill uses" line.

The `trailguide project update` preview lists a forked skill as `replaced` whenever the source's copy at the revision the update takes differs from the copy `basedOn` names, or, without `basedOn`, from the copy at the commit the project recorded. The preview lists the files that changed. Compare them with your fork, then set `basedOn` to the commit you compared, so later updates list only newer changes. [`--update-fork`](/reference/cli/#replace-a-fork-with-the-newest-version) instead replaces your fork with a new copy, overwriting your edits.

In a library repository, the same entry records where a skill the library [forked](/reference/cli/#fork-a-skill-into-the-library) came from. [`trailguide library check`](/reference/cli/#library-check) lists the source's changes to the skill whenever its `ref` moves past `basedOn`, and says so when the source deletes it. Projects that use the library install the library's own skill and never the source's.

Earlier versions recorded a fork as a `skills.include` entry plus a `skills.exclude` entry with `replacedBy` and `basedOn`. An exclusion with either field now fails validation with the `skills.forks` entry to write instead. Move the entry, and remove the skill from `skills.include` if it's there only for the fork.

## Rename a skill

Agents find skills by their install name, the `name` in `SKILL.md`, so two installed skills can't share one. When two sources ship skills with the same name, rename one of them:

```yaml
sources:
  fabrica:
    repository: https://github.com/fabricahq/.trailguide.git
    skills:
      include: [skills/review-change]
      rename:
        skills/review-change: fabrica-review
```

The installed copy's `name` becomes `fabrica-review`, and it's installed in `<target>/fabrica-review/`. The skill's ID doesn't change, so configuration, provenance, and update previews still call it `fabrica:skills/review-change`. Each key must be a skill this source installs, and each new name follows the [name grammar](/reference/skill-format/#skill-ids).

Trail Guide doesn't rewrite the old name where skill text mentions it, such as in a skill that tells the agent to use `review-change`. Every sync, update, and build that applies a rename prints the same warning:

```text
Warning: fabrica:skills/review-change is installed as fabrica-review. Mentions of review-change in skill text aren't rewritten; check the skills that call it by that name.
```

The renamed skill's own text and other skills from its source may still say `review-change`, such as a step that says to use the `review-change` skill or runs `/review-change`. After a clash, that name usually belongs to the other skill, so an agent following the old mention can run the wrong one. Check those skills, and [fork](/reference/cli/#project-add-skill) any you need to change; a fork of a renamed skill takes its new name, and the fork's entry replaces the rename. Trail Guide doesn't rewrite mentions for you, because a bare word, `/review-change`, and ordinary prose look alike, and a wrong match would corrupt the skill.

A required skill can be renamed too, because it stays installed, and a source can rename one without `include`.

When two installed skills still have the same install name, from any sources or your authored skills, sync fails with `skill-name-clash`, naming both, and writes nothing. Rename or exclude one of them. See [Who owns the target folders](/reference/sync/#who-owns-the-target-folders).

## Declare the skills a skill calls

A skill can call other skills without listing them in `trailguide-skills-used`, especially one from a [non-library source](#refs), whose skills rarely have that metadata. Declare those calls in the source's `skills.uses`:

```yaml
skills:
  uses:
    skills/architect: [skills/how]
```

Each key is a skill ID the source installs, and each value lists skill IDs in the same repository. Every ID must exist there. `uses` adds to what the skill declares itself; it can't remove a call the skill declares. Trail Guide installs the listed skills alongside the skill that calls them, and the installed copy's "Skills this skill uses" line includes them. Calls can't form a cycle. Trail Guide doesn't look for calls a skill doesn't declare: `trailguide-skills-used` and `uses` are the only ways it learns of them.

## Refs

A source's `ref` names where to take it from. Sync resolves it to a commit when it first reads the source, or after you change `ref`, records that commit in the source's vendor snapshot, and keeps it on every later `trailguide project sync` and `trailguide project build`, so installs stay reproducible. Only `trailguide project update` moves it:

| `ref` | Sync takes | Update moves to |
| --- | --- | --- |
| *(none)* | The recorded library release. | The newest library release. |
| A branch, such as `main` or `refs/heads/main` | The recorded commit. | The branch's newest commit. |
| A tag, such as `release/5`, `v1.2`, or `refs/tags/v1.2` | The recorded commit. | For a non-library source, the commit the tag names now, warning when the tag moved since the last update. A library source stays. |
| A full 40-character commit SHA | That commit. | Nowhere; it never moves. |

A bare name, such as `main`, means the repository's branch or tag of that name. When the repository has both, sync and update fail with `ambiguous-ref`; write `refs/heads/<name>` or `refs/tags/<name>` to say which one you mean. Abbreviated SHAs aren't accepted.

A branch suits a [non-library source](/reference/imports/#non-library-sources), such as a third party's skill collection with no tags, and a [curated source](#curated-skills). A library source can follow a branch too, such as `ref: main` to always take a library's newest commit, which frees the library from publishing library releases. The cost is change notes: rules and skills changed since the library's last library release have no change notes, and no version either, so update can't ask whether to keep a major change or apply a pin to them, and its preview lists the commit move and skill changes but not each rule. Sync and update warn about such a source every time they resolve its branch; leave `ref` out to follow the library's releases instead.

When the commit a branch's snapshot records is no longer in the repository, such as after a force-push, sync fails with `version-not-found` and asks you to run `trailguide project update SOURCE`, which takes the branch's newest commit. When the branch itself is renamed or deleted, update fails with `version-not-found` and names the ref; sync keeps working while the repository still has the recorded commit.

Sources have no declared type. Trail Guide decides when it fetches a source:

- A source whose repository has a [manifest](#library-manifest) is a **library**. It publishes skill levels, and versioned rules and skills once it has library releases. A library without `release/<number>` tags must have a `ref`.
- A source without a manifest is a **non-library source**, such as a third party's skill collection. Every skill in it is offered, it has no rules or versions, and it must have a `ref`, because it has no library releases to follow. `rules` is rejected on it.

All of a source's skills come from its one recorded revision, so skills that call each other always come from the same commit. Skills have no pins. To hold a skill back, exclude or fork it, or set the source's `ref`, which holds everything from that source. When a source's repository gains or loses its manifest, so that the source's fields no longer fit it, `trailguide project update` fails and names the fields to change.

### Take one revision

Taking a library at a revision other than a library release, such as a tag or commit with unreleased changes, opts that source out of versions for what changed: rules and skills whose files match a published version record that version, and the rest record `null`, have no version to cite, and update previews list the files that changed but no summaries of the changes. `trailguide project sync` and `trailguide project update` warn about such a source, naming its unreleased rules and skills, and the generated library summary in `generated/libraries/<source-name>/README.md` says so. `trailguide project check` still passes. Update doesn't move a library source whose `ref` is a tag or commit; change `ref` and run `trailguide project sync` to take another revision. A library source whose `ref` names a branch moves to the branch's newest commit, as [Refs](#refs) describes.

A source can use `rules.pins` or `ref`, not both.

## Pin a rule

```yaml
rules:
  pins:
    - id: practices/testing/verify-retry-limits
      version: "1.3.0"
      reason: Waiting on the author's response to acme/.trailguide#45.
```

| Field | Meaning |
| --- | --- |
| `id` | Required. One rule ID the source includes. Prefixes aren't accepted. |
| `version` | Required. An exact published version of the rule, such as `"1.3.0"`. Quote it so YAML reads it as text. |
| `reason` | Required non-blank text explaining why the rule stays at this version. `trailguide project update` shows it next to any newer version, so the decision keeps its context. |

A pin holds the rule at exactly that version:

- `trailguide project update` never moves a pinned rule. Its preview lists the pin, the newest available version, and your reason.
- When you add or change a pin, `trailguide project sync` moves the rule to the pinned version, up or down, when the source's recorded library release or an earlier one published it. Sync never moves the source, so a pin to a version published after the recorded library release fails and names `trailguide project update SOURCE`, which takes it.
- Removing a pin doesn't move the rule; the next `trailguide project update` offers its newest version.

To hold a rule at the version you have, pin it to the version recorded in the source's `_source.json`, which also appears in the rule's generated file. `trailguide project update --keep` writes that pin for you. Skills can't be pinned; see [Refs](#refs).

A pinned rule that its library retires stays at its version, and update reports the retirement. To keep a rule the library is retiring, pin it before or during the update that retires it.

## Skill targets

`skillTargets` says where installed copies of skills go:

| Value | Folder | Read by |
| --- | --- | --- |
| `claude` | `.claude/skills/` at the repository root | Claude Code |
| `agents` | `.agents/skills/` at the repository root | Codex, Cursor, and other agents that read the shared folder |

```yaml
schemaVersion: 2
skillTargets: [claude, agents]
```

`trailguide project init` writes `[claude, agents]`; `--skill-targets claude`, `agents`, or `none` changes it. Each value can appear once.

`skillTargets: []` is valid, and so is leaving the field out. With no targets, Trail Guide still records every skill the repository includes, required skills too, lists them in `generated/SKILLS.md`, and checks them, but writes no installed copies, and sync warns that it installed nothing. Use it to record what your sources require before you choose agent folders.

Each installed skill gets a folder `<target>/<install-name>/`. Trail Guide writes and removes only the skill folders it installed, and leaves every other folder in the target alone, such as a skill another tool installed. Trail Guide never writes to your home directory. See [Who owns the target folders](/reference/sync/#who-owns-the-target-folders).

## AGENTS.md section

By default, project commands keep a section in the repository's `AGENTS.md` that tells agents to read the project's rules and skills; see [Where agents start](/reference/files/#where-agents-start). To manage your agent instructions yourself, turn it off:

```yaml
schemaVersion: 2
manageAgentsFile: false
sources: {}
```

Commands then neither write nor check `AGENTS.md`. They don't remove a section they wrote earlier: delete it yourself, along with its markers, and tell your agent to read `.trailguide/generated/RULES.md` in your own words.

## Library manifest

The `library` section makes a repository a library. It publishes the repository's authored rules, rule groups, skills, and assets, and sets the level of each skill it offers:

```yaml
# fabricahq/.trailguide: .trailguide/config.yaml
schemaVersion: 2
sources:
  mattpocock:
    repository: https://github.com/mattpocock/skills.git
    ref: main
library:
  license:
    spdxExpression: MIT
    file: LICENSE.md
    notices: []
  skills:
    required: [skills/ship-service]
    recommended:
      - skills/build-feature
      - mattpocock:skills/engineering/to-spec
    offered:
      - mattpocock:skills/productivity/grilling
```

| Field | Meaning |
| --- | --- |
| `library.license` | Optional. The terms that cover everything the library publishes. See [License metadata](/reference/library-format/#license-metadata). |
| `library.skills.required` | Skills every project that uses the library installs. |
| `library.skills.recommended` | Skills a project installs when it includes `recommended`. |
| `library.skills.offered` | Skills a project installs when it names them or includes `"*"`. |

An empty `library: {}` makes a repository a library with no levels and no declared license. Each level is an optional list. Each entry is either an authored skill's ID, such as `skills/ship-service`, or a curated skill, `<source>:<skill-id>`, where `<source>` names an entry in this repository's own `sources`. A skill can appear in one level only. An authored skill that no level lists is offered.

`trailguide library check` validates the manifest, and fails when two listed skills, authored or curated, have the same install name, so projects never inherit a clash from one library. See [Library format](/reference/library-format/).

The manifest replaces the separate `rule-library.yaml` that earlier versions used. A repository with a `rule-library.yaml` fails to load; see [No backward compatibility](#no-backward-compatibility).

### Curated skills

A library can list skills from another repository in its levels, such as a third party's skill it wants its projects to use. The repository it takes them from is a **curated source**: an ordinary entry in the library's own `sources`, with the `ref` the library tracks. A library curates skills only; rules in its sources are for its own use, and it never publishes them.

- **Every curated skill is listed.** A skill from a curated source reaches projects only when a level lists it.
- **Files come from the original repository.** A skill's identity is its repository plus its path. Projects fetch a curated skill's files from the curated source, not from the library, and provenance records which library required, recommended, or offered it.
- **Refs resolve per project.** A library release doesn't lock a curated source's `ref`, which can name a branch, a tag, or a commit, or be left out to follow the curated repository's library releases. Each project resolves it when it first syncs and again when it runs `trailguide project update`, and records the commit in the library source's `_source.json`, so `ref: main` gives each project the newest commit at its own update, without a new library release. `trailguide project sync` keeps the recorded commit. `trailguide library check` resolves the ref on every run to check the skills the library lists, and fails, naming the ref, when the repository has no such branch, tag, or commit.
- **A project can fork a curated skill.** `trailguide project add skill --from fabrica:mattpocock:skills/to-spec` copies it into the project and replaces it, as for a library's own skill, recording the curated repository's commit as `basedOn`; see [Fork a skill](/reference/cli/#fork-a-skill). A library that wants its own version of a skill it curates forks it with [`trailguide library add skill --from`](/reference/cli/#fork-a-skill-into-the-library), which lists the fork in its place.
- **A project can override the ref.** When a project declares a source with the same repository as a curated source, its own `ref` decides which commit the curated skills come from. Repository identity decides the match, not the source name; see [Repository addresses](#repository-addresses).
- **The library's skills can call them.** A skill the library authors can name a curated skill in its [`trailguide-skills-used`](/reference/skill-format/#skills-a-skill-uses), such as `anthropic:skills/skill-creator`, as long as a level lists it. A project that installs the calling skill installs the curated skill too, whatever its level, and provenance records which of the library's skills use it.

The library repository is a project too: it installs the required skills of the sources it curates from, as any project would, and whatever it includes from them for its own use. Listing a curated skill in `library.skills` publishes it to projects but doesn't install it in the library repository. When the library's own skills call it there, also list it in that source's `skills.include`.

For example, Fabrica's library authors `add-skill`, which calls Anthropic's `skill-creator`. The library's configuration curates `skill-creator`, publishes both skills, and installs `skill-creator` for its own use:

```yaml
# fabricahq/.trailguide: .trailguide/config.yaml
schemaVersion: 2
skillTargets: [claude]
sources:
  anthropic:
    repository: https://github.com/anthropics/skills.git
    ref: main
    skills:
      include: [skills/skill-creator] # installs it in this repository
library:
  skills:
    recommended: [skills/add-skill]
    offered: [anthropic:skills/skill-creator] # publishes it to projects
```

`add-skill` names the curated skill by the ID `library.skills` lists it under:

```markdown
<!-- fabricahq/.trailguide: .trailguide/skills/add-skill/SKILL.md -->
---
name: add-skill
description: Add a skill to this library.
metadata:
  trailguide-skills-used: |
    anthropic:skills/skill-creator
---
```

A project that uses the library names only the library and the skill it wants:

```yaml
# A project: .trailguide/config.yaml
schemaVersion: 2
skillTargets: [claude]
sources:
  fabrica:
    repository: https://github.com/fabricahq/.trailguide.git
    skills:
      include: [skills/add-skill]
```

`trailguide project sync` installs `add-skill` from Fabrica's library and `skill-creator` from anthropics/skills, though the project never names either `skill-creator` or Anthropic's repository.

## Repository addresses

Use a complete Git address so the host is explicit:

```yaml
repository: https://gitlab.com/my-team/engineering/rules.git
```

Supported forms include:

```text
https://github.com/my-team/rules.git
https://gitlab.com/my-team/engineering/rules.git
ssh://git@git.example.org:2222/srv/rules.git
git@git.example.org:engineering/rules.git
git@git.example.org:/srv/rules.git
```

Nested GitLab namespaces, private hosts, and explicit ports are supported.
The `.git` suffix is optional. Git uses the caller's credentials; do not embed HTTPS credentials or SSH passwords in configuration.
SSH usernames are allowed.

Repository addresses do not accept query strings, fragments, `git::` prefixes, getter options, or `//subdirectory` selection.
Local paths, `file:`, unauthenticated `git:`, plain HTTP, and remote-helper protocols are outside this format.
The `owner/name` shorthand isn't accepted; use `https://github.com/owner/name.git` instead.

Trail Guide preserves the supplied address in provenance and requires the snapshot to match it exactly.
Two addresses name the same repository when they match after normalizing host spelling, standard GitHub.com and GitLab.com transport aliases, and optional `.git` suffixes. This decides duplicate sources, and which project source overrides a [curated source's](#curated-skills) ref.
GitHub path matching ignores case; generic repository paths and GitLab paths retain case.
Other hosts retain their transport, username, port, and path distinctions.
For example, `git@host:rules.git` is home-relative, while `ssh://git@host/rules.git` is absolute. Trail Guide does not equate them.

Validation rejects ambiguous paths, including dot segments, encoded separators in URLs, malformed URI escapes, raw whitespace, and backslashes.
SCP paths are literal Git paths, so percent signs in that form are not URI escapes.
Syntax validation does not fetch or authenticate a repository.

Generated source links use pinned GitHub.com or GitLab.com URLs for recognized standard endpoints.
For other hosts, they point to the retained source file under `vendor/<source>/`.
If a relative document or image is missing from that snapshot, generation fails with instructions to retain it or supply an explicit URL.
Trail Guide does not infer a host's web interface from its name. See [How sources work](/reference/imports/).

## Source names and IDs

Choose stable source names such as `fabrica` or `acme`.
Names must match `[a-z][a-z0-9-]*`. `techs` and `practices` are reserved, because authored rules use those names in `generated/rules/`.
Declare each repository once.

Inside a source's configuration, rule and skill IDs are relative to that source, without its name. Everywhere else, in generated files, provenance, review findings, and commands such as `trailguide project update fabrica:skills/build-feature`, an included rule or skill has a **project ID** that starts with its source name:

```text
fabrica:practices/testing/verify-retry-limits
acme:practices/testing/verify-retry-limits
fabrica:skills/build-feature
fabrica:mattpocock:skills/engineering/to-spec
practices/testing/test-project-contracts
skills/fix-bug
```

- A rule ID is the rule's path in its repository's `rules/` folder, without `.md`.
- A skill ID is the skill folder's path: `skills/<name>` for a library's skills, and the folder's path from the repository root for a non-library source's, such as `skills/engineering/to-spec`.
- A curated skill's project ID adds the curated source's name after the library's, as the manifest lists it. It keeps that ID when the project declares the curated repository as its own source, which only decides the commit the skill comes from.
- Authored rules and skills have no source name.

The source name keeps identical IDs from different sources distinct. Renaming a source changes its project IDs and any external references to them. Replacing a repository under an existing source name also requires reviewing its `exclude`, `pins`, `uses`, and `rename` entries.

The installed copy of a skill goes in a folder named only by its install name, because agents find skills by name. See [Rename a skill](#rename-a-skill).

## Choose versions

Each rule and skill in a library has its own [version](/reference/rule-versions/). A project takes each source from one library release or commit, its **lock**, and moves it only when someone updates:

- `vendor/<source-name>/_source.json` records the library release or commit, and the exact version of every included rule and skill. [`trailguide project sync`](/reference/cli/#project-sync) restores exactly that, so every checkout has the same content.
- [`trailguide project update`](/reference/cli/#project-update) previews newer versions, new rules and skills, level changes, and retirements, and applies them after you confirm.

`trailguide project sync` never moves a source. A rule or skill you newly include comes from the recorded library release or commit. When it isn't there, such as a skill the library published after your last update, sync fails and names the command that moves the source, `trailguide project update SOURCE`.

A project chooses rule versions one by one, with pins, but not skill versions. All of a source's skills come from its one recorded revision, and move together when it moves. A skill's version still says what changed, in release notes, `generated/SKILLS.md`, the installed copy's marker line, provenance, and the update preview.

### Where each rule's files come from

Each rule's [Markdown file and asset directory](/reference/rule-versions/#what-a-version-covers) come from the library release that published its version. Library-wide files, such as rule group metadata, shared assets, the manifest, and license files, and every skill's whole folder come from the source's one recorded library release, or from the revision your `ref` names. That library release is never older than the one that published any included rule version. When it no longer has the metadata of an included rule's rule group, such as the rule group of a retired rule you pinned, that rule group's metadata comes from the newest library release among its included rule versions that still has it.

Offline `trailguide project build`, `trailguide project check`, and ordinary agent work use the recorded versions without contacting the repository.

## Rule groups

Rules from the same rule group ID combine into one generated rule group, whatever source supplies them.
Including `practices/testing/*` from two sources combines both sets of rules into one effective testing rule group, with a rule group page and individual resolved rule files.
Source order never establishes precedence.

Every valid authored rule in `rules/` joins its rule group, and needs no configuration entry.
An authored rule with the same ID as an included rule doesn't replace it; name it in an exclusion's `replacedBy` to do that.
An authored rule that an exclusion names as `replacedBy` appears once under its own ID; the excluded rule is removed from active output.
The complete authored definition supplies the metadata, guidance, attribution, and assets. Configuration and provenance retain the replacement relationship.
An authored `_group.yaml` defines a rule group, including an empty one, without any configuration entry.
If authored metadata exists for an included rule group, its complete description and reading cues take precedence for discovery.
Otherwise, descriptions from every contributing library remain source-labeled.
Provenance retains all rule group metadata and identifies the sources supplying the effective discovery guidance.
Authored rules without either authored or included rule group metadata are errors, with the missing `_group.yaml` path in the diagnostic. When a sync or update removes the last source supplying an authored rule's rule group, it writes that rule group's last vendored metadata to `rules/<group-id>/_group.yaml` instead of failing; see [Keep the rule group of an authored rule](/reference/sync/#keep-the-rule-group-of-an-authored-rule).

The generated index shows rule group names, applicability guidance, and explicit **Open group** links. Without authored metadata, guidance from multiple libraries remains labeled by source.
Trail Guide does not silently choose one library's description over another's.

## Conflicting rules

Source-qualified IDs prevent naming collisions, but distinct rules can still require incompatible behavior in the same situation.
Both remain active unless the repository explicitly excludes or replaces a rule under its owning source.
Trail Guide does not infer priority from source order or interpret prose to resolve contradictions.

See [Resolve conflicting rules](/guides/conflicting-guidance/) for examples, an agent review prompt, and ways to resolve competing instructions.

## Skill precedence

Skill decisions layer in this order, each overriding the ones before it: a library's levels, then an org library's, then the project's configuration, then each person's [personal skills](/reference/sync/#personal-skills-with-the-same-name). Required skills are a floor: the project can add, rename, and replace around them, but can't remove them.

A personal skill with the same name as a skill the repository installs wins in some agents, even over a required skill. Trail Guide can't prevent that, and doesn't write to your home directory. It warns instead; see [Personal skills with the same name](/reference/sync/#personal-skills-with-the-same-name).

## No backward compatibility

Trail Guide reads only version 2 configuration. A version 1 configuration, or a repository with a `rule-library.yaml`, fails to load, with a message that shows the version 2 shape:

```text
Error: .trailguide/config.yaml uses schemaVersion 1, which this version of Trail Guide doesn't read. Rewrite it in the version 2 shape, for example:

  schemaVersion: 2
  skillTargets: [claude, agents]
  sources:
    fabrica:
      repository: https://github.com/fabricahq/public-library.git
      rules:
        include: [practices/testing/*]
        exclude:
          - id: practices/testing/avoid-snapshot-tests
            reason: ...

Move authored rules from local/ to rules/, and authored skills to skills/. See https://trailguide.fabricahq.com/reference/configuration/
```

There's no upgrade command. Rewrite the file by hand:

| Version 1 | Version 2 |
| --- | --- |
| `groups: [practices/testing]` | `rules: {include: [practices/testing/*]}` |
| `groups: "*"` | `rules: {include: ["*"]}` |
| `rules: [techs/go/wrap-errors]` | Add the IDs to `rules.include`. |
| `exclude` and `pins` maps keyed by rule ID | `rules.exclude` and `rules.pins` lists, each entry with an `id`. |
| `replacedBy: local/techs/go/x.md` | `replacedBy: techs/go/x`, the authored rule's ID. |
| `.trailguide/local/<group-id>/` | `.trailguide/rules/<group-id>/` |
| `rule-library.yaml` and root-level `techs/`, `practices/`, and `assets/` | A `library` section in `.trailguide/config.yaml`, and `.trailguide/rules/` and `.trailguide/assets/`. |

## Version boundaries

`schemaVersion` describes this configuration.
The caller-supplied `toolVersion` identifies the tool that generated the output.

Each source's pins, and its `ref` when present, state which versions the project wants; every other rule and skill follows the source's lock, which only `trailguide project update` moves.
Vendored provenance records each included rule's and skill's version, library release, and commit, or a non-library source's commit, used by offline commands. It is output Trail Guide owns, not a second version the project chooses.

See [Library format](/reference/library-format/) for library contents and [Include rules](/guides/select-rules/#adapt-the-selection-to-your-project) for examples of exceptions.
