---
title: "License rules"
description: "Choose clear terms for rules and skills, and preserve them when projects take rules and install skills."
---

A library's **license** states the terms for using, copying, modifying, and sharing its rules and skills. Clear terms help projects understand what they can do with your guidance and what notices they need to keep.

This guide walks through choosing what your terms should cover, declaring them in your library's manifest, and checking what Trail Guide preserves when projects take your rules. Trail Guide carries declared license text and notices with the rules; it does not choose a license or grant additional rights.

## Decide what the license covers

Distinguish the rule content from application code written using its guidance.
A library's terms should answer these questions:

- May teams read the rules with agents and use the guidance in commercial projects?
- May they copy rule files into private or public repositories?
- May they combine rules into aggregates, modify them, and redistribute the result?
- What attribution, copyright notices, and license terms must accompany those copies?
- Does the library-wide declaration cover all included rules and code examples?
- Are there restrictions on publishing a competing rule library or service?

For a reusable best-practice library, we recommend expressly permitting teams to apply its guidance without requiring their application to adopt the library's license.
Copying protected example code or rule text into an application is a separate use that the chosen terms should address.

In the United States, copyright distinguishes ideas and methods from their original expression.
The [U.S. Copyright Office's explanation](https://www.copyright.gov/help/faq/faq-general.html) provides context for that distinction.
It does not settle the interpretation of a particular license, contract, or copied example.
Have the chosen terms reviewed for the rights and restrictions you intend before publishing them.

## Declare the library's terms

The Trail Guide format does not require a particular license.

Keep the actual license text in `LICENSE.md` at the repository root, or another explicitly named file.
The `library.license` section of the [manifest](/reference/library-format/#the-manifest), in `.trailguide/config.yaml`, identifies the terms for the whole library and any accompanying notice files:

```yaml
library:
  formatVersion: 1
  license:
    spdxExpression: MIT
    file: LICENSE.md
    notices: []
```

Paths resolve from the repository root and identify source files only. Trail Guide copies the contents unchanged to `generated/libraries/<source-name>/licenses/LICENSE.md` and `generated/libraries/<source-name>/licenses/notices/001.md`, `002.md`, and so on. Notice numbering follows unique declaration order; output destinations are not configurable.
For example, list `NOTICE.md` in `notices` when the library supplies a notice that must accompany copies of its content.
No extra notice file is required when the license and rule files already contain the necessary notices.
The [file reference](/reference/library-format/#license-metadata) defines these fields.

Use an SPDX expression for standard terms and a distinct `LicenseRef-…` for custom or modified terms. The [SPDX expression specification](https://spdx.github.io/spdx-spec/v2.3/SPDX-license-expressions/) defines identifiers and compound expressions. The identifier supplements the retained text. Trail Guide validates the `library.license.spdxExpression` syntax and identifiers and preserves your declaration. It does not verify that the declaration matches the retained terms or grants the permissions you need.

A library has one license declaration covering all its rules and rule groups. Overrides for a single rule or rule group are unsupported.
Keep attribution with each rule, in its body or [structured attribution](/reference/rule-format/#rule-attribution). List required notices in the library manifest.
If material requires a different declaration, maintain it in a separate compatible library. Do not relabel third-party material merely to fit a library's license.

A source citation is not a license grant.
Before adapting material from a book, article, or another library, establish the rights needed for the intended use.

## What sync preserves

Sync copies license and declared notice files, like other library-wide files, from the library release that supplies the source's library-wide files, which `trailguide project update` moves to the newest library release, or from the revision a source's `ref` names. See [What a version covers](/reference/rule-versions/#what-a-version-covers).
The offline builder accepts preassembled snapshots and checks that declared files are present.
It returns unchanged license and notice copies at generated paths, but does not fetch source files or write them to disk.
The consuming project retains them alongside the vendored rules:

```text
.trailguide/
  vendor/
    fabrica/
      LICENSE.md
      config.yaml
      rules/practices/testing/...
  generated/
    RULES.md
    libraries/fabrica/
      README.md
      licenses/LICENSE.md
    groups/practices/testing.md
    rules/fabrica/practices/testing/verify-retry-limits.md
```

Preserve copyright notices and per-rule attribution in both the vendored source and the generated rule file.
Keep the library license and declared notices with the snapshot and preserve their links after relocation.
Project build and check verify snapshot digests to detect changed files. The offline builder checks declared file presence without computing digests.
Sync reports changed license files. A semantic update report that highlights license changes is not available yet.

Each individual resolved rule identifies its source and applicable preserved license.
For example, `generated/rules/fabrica/practices/testing/verify-retry-limits.md` includes this source footer after its guidance.
The repository and commit in this example are illustrative.

```markdown
## Source and attribution

**Rule source:** [Original rule](https://github.com/example/rules/blob/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/.trailguide/rules/practices/testing/verify-retry-limits.md)

**Declared license:** MIT

- [LICENSE.md](../../../../libraries/fabrica/licenses/LICENSE.md)
```

Guidance precedes this footer; preserved source metadata follows it. Every rule from a source displays its library's declaration; explicit attribution citations also appear in the footer.
Generated rule files link to these standardized license and notice copies, and the originals stay in the source's vendor snapshot.
The library summary in `generated/libraries/<source-name>/README.md` lists the declared license expression and the retained files.
When a rule group includes rules from multiple libraries, retain each rule's licensing information instead of assigning one library's license to the rule group.
Copied rule text remains subject to its applicable terms even when the project changes its wording or replaces a library's rule with an adaptation.
Maintain third-party adaptations in a compatible library with the applicable library-wide declaration. A project's own rules and replacements are for original project guidance; an ID alone cannot establish licensing.

## License skills

A library's license declaration covers its [skills](/concepts/skill/) as it covers its rules. Your terms should also answer whether teams may run a skill's scripts and copy its files into their repositories, because installing a skill commits a copy of its whole folder in each skill target, such as `.claude/skills/`.

### Skills in a library

The Agent Skills format gives `SKILL.md` an optional `license` field. In a library, that field is informational: the library's declaration in its manifest is what applies, and Trail Guide keeps the field unchanged. When the field differs from the library's `spdxExpression`, or the library declares no license, `trailguide library check` warns:

```text
Warning: skills/build-feature/SKILL.md: license is Apache-2.0, but the library declares MIT. The library's license applies to its skills; make them match.
```

Make the field match the library's expression, or leave it out. A skill whose material needs different terms belongs in a separate library, as a rule's would. A license or notice file inside a skill folder is part of the skill's content, not a declared library file; a library can't declare a file in a skill folder as its license or notice file.

Sync retains the library's license and notice files for skills exactly as it does for rules, at the same generated paths, and `generated/libraries/<source-name>/README.md` lists the source's installed skills with its declared terms. Forking a skill with `trailguide project add skill --from` copies its text into `.trailguide/skills/`, where the library's license still applies to it.

A skill a library [curates](/reference/configuration/#curated-skills) from another repository keeps that repository's terms, not the curating library's: Trail Guide finds them from that repository's own declaration when it's a library, and otherwise as it does for a non-library source, below.

### Skills from a non-library source

A non-library source, a repository without a manifest, has no license declaration. For each skill it installs, Trail Guide looks for terms in this order:

1. The `license` field in the skill's `SKILL.md`, which can name a license or a license file the skill bundles.
2. A file whose name starts with `LICENSE` in the skill's own folder, such as `LICENSE.txt`.
3. Files whose names start with `LICENSE` or `NOTICE` at the repository root.

Trail Guide records what it finds in [provenance](/reference/provenance/), and retains the root files, when present, under `generated/libraries/<source-name>/licenses/`, like a library's declared files. It doesn't infer an SPDX identifier from a file name, or decide whether the terms permit your use. See [Non-library sources](/reference/imports/#non-library-sources).

When a skill has none of these, `trailguide project sync` fails with `license-not-found`, names the source and the skill, and writes nothing:

```text
Error: source pstack: skills/how has no license terms: its SKILL.md has no license field, its folder has no LICENSE file, and the repository has no LICENSE or NOTICE file at its root. Ask the repository's owner to add a license, or exclude the skill. No files were written.
```

A repository's visibility isn't a license. Don't install a skill until you know you may copy it into your repository.

## What a consuming project should understand

Trail Guide leaves the consuming repository's root license unchanged.
Vendored content remains separately identified under its applicable terms.
The index should explain that boundary so the root license does not appear to relicense rules from sources.
Whether a particular combination or redistribution is permitted depends on the actual licenses and use.

When sharing generated rules or installed skills, include their library folders and retained license and notice copies.
Preserve the vendor snapshots when sharing the complete consuming repository.
Copying a rule file by itself can break its license links and omit required notices.

If licensing is undeclared or ambiguous, report it as unspecified and resolve the permissions before redistribution.
Do not infer a license from the repository's visibility, its source name, or another library's terms.
Private libraries can document internal permissions without adopting a public license.

## What the tool can check

The builder validates declared paths, preserves text and notices, and constructs links to the retained files.
It rejects unsafe or unresolved local Markdown references, but does not check whether external URLs are reachable.
It cannot establish ownership, decide legal compatibility, or certify that the chosen terms permit a consumer's intended use.
Vendor snapshots preserve original files. Sync applies changes safely, reports changed files, and persists digests for offline integrity checks.

For source material that does not use this format, follow [Adapt third-party rules](/guides/write-rules/#adapt-third-party-rules). Keep a separate adapted definition with its source citation and retained terms; do not relabel edited content as an unchanged vendor snapshot.
