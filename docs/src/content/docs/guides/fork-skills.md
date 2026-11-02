---
title: "Fork a skill"
description: "Customize a skill you install from a source, keep your edits, and merge the source's later improvements into your copy."
---

A skill you install from a library or another repository rarely fits your project exactly. Your team may fix bugs behind a feature flag, review migrations differently, or skip a step that doesn't apply to you. You can't just edit the installed copy in `.claude/skills/`: the next sync replaces it, and `trailguide project check` reports the difference.

**Forking a skill** gives your project its own copy to edit. Agents get your fork instead of the source's skill. The source keeps improving the original, though, and you probably want those improvements without losing your edits. Trail Guide records which copy your fork started from, its **`basedOn`** commit, so each update can show what the source changed since then and merge those changes into your fork.

In this guide, you'll fork a skill, edit it, and bring in the source's later changes, including changes that conflict with your edits. Start with a project that already installs the skill; see [Install skills](/guides/install-skills/).

## Fork the skill

Fork `skills/fix-bug` from the `fabrica` source, and say why:

```sh
trailguide project add skill \
  --from fabrica:skills/fix-bug \
  --reason 'We fix bugs behind a feature flag first.' \
  --non-interactive
```

The command copies the skill your project installs into `.trailguide/skills/fabrica/fix-bug/`, in the folder named for the source it came from, and records the fork under the source in `.trailguide/config.yaml`:

```yaml
sources:
  fabrica:
    skills:
      forks:
        skills/fix-bug:
          as: skills/fix-bug
          basedOn: "9f2c4e1a7b3d5f60817a2b4c6d8e0f1a3b5c7d9e"
          reason: We fix bugs behind a feature flag first.
```

- `as` is your skill, `skills/<name>`, which agents get in place of the source's skill. Its folder is `.trailguide/skills/<source>/<name>/`, so anyone browsing `.trailguide/skills/` sees that it's a fork and where it came from.
- `basedOn` is the commit the fork copied. It's how Trail Guide tells your edits apart from the source's later changes.
- `reason` tells the next person why your project doesn't use the source's skill as it is.

A fork doesn't need a `skills.include` entry: forking the skill is enough to keep tracking it. You can't fork a [required skill](/reference/configuration/#skill-levels), which your organization or library expects every project to use as it is. The source's license still applies to the copied files; see [License rules](/guides/license-rules/#license-skills).

The fork rewrites the skill's citations of other skills and rules to project IDs, such as `fabrica:practices/testing/verify-retry-limits`. When it can't, because the skill's frontmatter is written in a way Trail Guide can't edit, the fork is left as a draft, and the command lists what to fix. See [Fork a skill](/reference/cli/#fork-a-skill) for every step.

## Edit and install your fork

Edit `.trailguide/skills/fabrica/fix-bug/SKILL.md` and any other files in its folder, then install the result:

```sh
trailguide project build
```

Build installs your fork into each skill target, such as `.claude/skills/fix-bug/`, where the source's skill used to be. If the fork is a draft, build refuses it until you fix what the fork command listed and remove the `<!-- trailguide:draft -->` line. Commit the fork and `config.yaml` together.

## See what the source changed

When you update the source, the preview lists your fork as `replaced` if the source changed the skill since `basedOn`:

```sh
trailguide project update fabrica
```

```text
fabrica
  replaced  skills/fix-bug  commit 9f2c4e1a7b3d -> 4b8e0c2d6f1a
            Your skill: skills/fix-bug, based on commit 9f2c4e1a7b3d.
            Changed since commit 9f2c4e1a7b3d, the copy your skill is based on:
              SKILL.md
              scripts/reproduce.sh
            Merging them into your skill finds no text conflicts with your edits. To merge
            them and set basedOn to commit 4b8e0c2d6f1a, pass --merge-fork fabrica:skills/fix-bug.
```

The update never changes your fork unless you ask. If you confirm the rest of the update without merging, your fork stays as it is, and later previews keep listing the source's changes until you take them in.

## Merge the source's changes

To merge, answer `merge` when a terminal update asks about the skill, or pass `--merge-fork` in a script:

```sh
trailguide project update fabrica --yes --merge-fork fabrica:skills/fix-bug
```

Trail Guide compares three copies of each file: the source's copy at `basedOn`, your fork, and the source's newer copy. A file only you changed keeps your version. A file only the source changed takes the source's version. A file you both changed is merged line by line, the way Git merges a branch. The merged fork, the new `basedOn`, and the installed copies are written together, so an interrupted update leaves either all of them old or all of them new.

"No text conflicts" means your edits and the source's changes touch different lines. It doesn't mean they agree. If you wrote "skip the regression test for UI bugs" and the source added "always add a regression test" ten lines away, both survive the merge and contradict each other. Read the merged `SKILL.md`, and check the diff before you commit it.

## Resolve conflicting changes

When you and the source changed the same lines, Trail Guide can't decide which version you want, so it doesn't merge. The preview lists the files that conflict:

```text
fabrica
  replaced  skills/fix-bug  commit 9f2c4e1a7b3d -> 4b8e0c2d6f1a
            Your skill: skills/fix-bug, based on commit 9f2c4e1a7b3d.
            Changed since commit 9f2c4e1a7b3d, the copy your skill is based on:
              SKILL.md
            Your edits and these changes touch the same lines of:
              SKILL.md
            Merge them into your skill by hand, then set sources.fabrica.skills.forks.skills/fix-bug.basedOn
            to "4b8e0c2d6f1a3e5c7b9d0f2a4c6e8b0d2f4a6c8e".
```

`--merge-fork` refuses such a skill and writes nothing. To bring the changes in yourself:

1. Compare the source's copy with yours. The source's copy at the new commit is in `.trailguide/_managed/vendor/fabrica/` after you apply the rest of the update, or in the source's repository at that commit.
2. Edit your fork to take the changes you want.
3. Set `basedOn` to the commit the preview names, so later updates list only newer changes.
4. Run `trailguide project build`.

## When the source deletes the skill

If the source stops publishing the skill, your fork keeps working: sync and build install it as before, and the update preview says the source no longer has the skill. There's nothing left to merge, so the fork is entirely yours from then on.

## Skill forks and rule forks

Rules can be [forked](/guides/select-rules/#replace-a-rule) too, but their updates work differently, and the flags keep the two apart:

| | Skill fork | Rule fork |
| --- | --- | --- |
| Flag | `--merge-fork SOURCE:SKILL` | `--update-fork SOURCE:RULE` |
| Your edits | Kept. The source's changes are merged in. | Discarded. Your rule becomes a fresh fork of the newest version. |
| `basedOn` | The commit merged. | The version forked. |

`--update-fork` refuses a forked skill and `--merge-fork` refuses a rule, each naming the other flag, so you don't discard edits by reaching for the wrong one. See [Rules you replaced with your own](/guides/update/#rules-you-replaced-with-your-own).

## Fork a skill in a library

A library can fork a skill from one of its own sources, such as a third party's skill it publishes with changes, with `trailguide library add skill --from`. When you move that source's `ref`, `trailguide library check` lists what the source changed since `basedOn`. A library merges those changes by hand, then sets `basedOn`. See [Write a skill](/guides/write-skills/#in-a-library).

## Next steps

- If you want every option and refusal of the fork and merge commands, see [project add skill](/reference/cli/#fork-a-skill) and [Merge a source's changes into a forked skill](/reference/cli/#merge-a-sources-changes-into-a-forked-skill).
- If you want the full `skills.forks` format, see [Configuration](/reference/configuration/#fork-a-skill).
