# Rule library

This repository contains a [Fabrica Trail Guide](https://trailguide.fabricahq.com) library: a collection of engineering rules that projects can adopt from a Git repository.
The library section of `.trailguide/config.yaml` makes the repository a library. Its groups and rules live in `.trailguide/rules/`, shared assets in `.trailguide/assets/`, and change notes in `.trailguide/changes/`.
Use this README to author and maintain the library.

## Core concepts

- **Rule:** One engineering practice in a Markdown file, with guidance, exceptions, and a cue explaining when to read it.
- **Group:** Related rules for a technology (`techs/go`) or practice (`practices/testing`). Each group has a description and a reading cue.
- **Library:** The groups and rules published together in this repository. Each rule has its own version, and library releases publish new versions. Consuming projects choose the groups to import and when to adopt new versions.

Library groups supply shared guidance. A consuming project's local groups belong to that project. When group IDs match, local group descriptions and reading cues take precedence. Local rules supplement imported rules; replacing or excluding an imported rule requires an explicit source-specific configuration entry.

## Instructions for agents

### Managing rules

1. Run the commands below from the folder containing this README. From elsewhere, pass `--directory` with this library's path.
2. Replace example metadata and guidance with the publisher's intended engineering practices.
3. Before adding or revising a rule, read the [rule authoring rubric](https://github.com/fabricahq/trailguide/blob/main/docs/src/content/docs/reference/rule-authoring.md).
4. After the library's first library release, record every rule change in a change note with `trailguide library change`, in the same commit as the change.
5. After editing, run `trailguide library check`. Fix every error and review licensing warnings before reporting the library ready.

Human-readable output is the default. Add `--json` for structured results, or `--help` to inspect a command's options.

#### Add a group

Choose a technology or practice group path and supply its scope and reading cue:

```sh
trailguide library add group techs/go \
  --name 'Go' \
  --description 'Engineering practices for Go code.' \
  --when-to-read 'When writing or reviewing Go code.'
```

Confirm the metadata in `.trailguide/rules/techs/go/_group.yaml`. Read `.trailguide/rules/techs/go/README.md` before authoring rules in that group.

#### Add a rule

Create the group first with `trailguide library add group`. Rule creation errors if the group is missing. Create a complete Markdown body, then add its discovery metadata. This example refuses to overwrite an existing body file:

```sh
set -C
cat > return-errors.body.md <<'RULE_BODY'
# Return errors to the caller

Return a descriptive error when an operation fails. Let the caller decide whether to retry, report, or stop.
RULE_BODY
trailguide library add rule techs/go/return-errors \
  --title 'Return errors to the caller' \
  --impact HIGH \
  --impact-description 'Keep failures visible so callers can respond.' \
  --when-to-read 'When calling fallible operations.' \
  --body-file return-errors.body.md
```

Inspect `.trailguide/rules/techs/go/return-errors.md`; make future edits there. The body file is only an authoring input.
If you omit `--body-file`, open the created Markdown file in your editor. Keep its metadata between the `---` lines; below it, write the instructions, rationale, correct and incorrect examples, and validation steps. Replace template placeholders, remove unused sections, and remove the `<!-- trailguide:draft -->` marker when the rule is complete. Then run `trailguide library check`.

#### Validate and share

```sh
trailguide library check
```

Exit 0 means the library passes input validation; it does not prove that application code follows its rules.
An undeclared license produces a warning. Confirm the publisher's license terms before sharing.
Use one license declaration for the library in the library section of `.trailguide/config.yaml`, with the actual license text and any notice files.
The result previews the next library release: each rule's change, and its current and next version.

`.github/workflows/trailguide.yml` runs the same check on every pull request, with the Trail Guide version that wrote it. Commit it with the library. To move it to a newer Trail Guide version, delete it and run `trailguide library init` again.

Review the diff, commit the library, and push it to the remote's default branch. Then publish a library release with `trailguide library release`, which tags that commit, pushes the tag, and gives each changed rule its new version. On GitHub.com it also creates a GitHub Release page with the signed-in GitHub CLI, `gh`, unless you pass `--no-github-release`. The first library release gives every rule version `1.0.0`.
Give consumers the repository address and group IDs.
Consumers run `trailguide project add source` in their project, then `trailguide project sync` to import the selected guidance.

#### Record changes after the first library release

After the first library release, every change to a rule needs a change note in `.trailguide/changes/`. Record it with `trailguide library change` and the rule's ID:

- For a rule that has a version, pass `--bump major`, `--bump minor`, or `--bump patch`. Choose `major` when work that complied with the previous version could fail the new one.
- For a new rule, leave out `--bump`; it starts at version `1.0.0`.
- To retire a rule, delete its Markdown file and asset directory, then pass `--retire`, and `--replaced-by` with the rule that replaces it, if any.

Pass `--summary` with one line for project maintainers deciding whether to update. `trailguide library check` fails when a changed rule has no note, or a note doesn't match a change. [Version your rules](https://trailguide.fabricahq.com/guides/version-rules/) explains change levels and library releases.

You may customize this README for the library. Re-running `trailguide library init` preserves an existing README.
