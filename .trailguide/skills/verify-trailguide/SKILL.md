---
name: verify-trailguide
description: Drive the trailguide CLI the way a user does, in a disposable lab with local Git sources, and capture evidence. Use to prove or explore the behavior of project and library commands (init, add source, sync, update, build, check, list, library check) from a checkout or a PR branch.
metadata:
  trailguide-skill-type: playbook
---

# Verify trailguide

Trail Guide's surface is one short-lived Go CLI, `trailguide`, with two command trees: `trailguide project ...` for projects that adopt rules and skills, and `trailguide library ...` for libraries that publish them. A user runs it in a Git repository and points it at other Git repositories, its sources. There is no server to keep alive.

`scripts/tg-lab`, beside this file, is the harness. It builds the CLI from a checkout into a lab with a private `HOME` and Git configuration, so the user's own Git settings, credentials, and signing never apply. In the lab, `https://verify.invalid/NAME` resolves to a local repository that `tg-lab remote NAME` creates, so sources, sync, update, and fetches work offline against repositories you author. Each lab is its own directory, so any number of runs can share a machine.

The feature map in [features/README.md](features/README.md) lists the user-facing features, with a recipe for each. Read it before driving.

## Launch

Run every command below from the trailguide checkout you want to verify. `tg-lab` is at `.trailguide/skills/verify-trailguide/scripts/tg-lab`, beside this file in every installed copy. Below, `tg-lab` means that path.

```sh
tg-lab new
```

`new` runs `go build ./cmd/trailguide` from the checkout's working tree, uncommitted changes included, and prints three lines. The last, `TG_LAB=<lab>`, is the lab. Ready means `new` printed it and exited 0. Every later command needs it in the environment, and each shell call starts fresh, so prefix every call:

```sh
TG_LAB=/tmp/trailguide-verify/run-20261030T050000Z-abcd/lab tg-lab doctor
```

Set `TG_LAB_ROOT` to create runs somewhere other than `${TMPDIR:-/tmp}/trailguide-verify`.

Rebuild with a new `tg-lab new` after you change the code. A lab's binary never changes.

## Doctor

```sh
TG_LAB=<lab> tg-lab doctor
```

It is read-only. It prints the binary's version, the commit and uncommitted path count it was built from, the evidence folder, and the remote mapping, and ends with `doctor: ok`. It exits 1 with `doctor: NOT OK` when the binary is missing, the checkout's HEAD moved since the build (`STALE`), `trailguide` on the lab's PATH isn't the lab's binary, or the remote mapping is missing. Run it first, and again whenever output surprises you.

## Drive

Create the repositories a journey needs, then run commands inside them:

```sh
TG_LAB=<lab> tg-lab remote lib        # prints path=<lab>/remotes/lib and url=https://verify.invalid/lib
TG_LAB=<lab> tg-lab project app       # prints path=<lab>/projects/app
cd <lab>/projects/app
TG_LAB=<lab> tg-lab run init -- trailguide project init --non-interactive
```

- `tg-lab run LABEL -- COMMAND...` runs a command with the lab's environment, prints its output, exits with its code, and saves the command, stdout, stderr, and exit code to the evidence folder as `NNN-LABEL.txt`. Use it for every command whose result is proof.
- `tg-lab exec -- COMMAND...` does the same without saving evidence. Use it for setup and for inspecting files, such as `tg-lab exec -- git status --short`.
- `tg-lab commit DIR MESSAGE` commits everything in a lab repository and prints the commit. Sources are read from commits, so commit a remote after every edit you want a project to see.
- Write source files with ordinary shell tools (`cat > file <<'EOF'`, `mkdir -p`) inside `<lab>/remotes/NAME`. Git commands there must go through `tg-lab exec` or `tg-lab commit`, so they use the lab's identity.

CLI conventions to rely on:

- Human output is the default. `--json` returns one JSON object with `ok`, `value`, and `error`, and never prompts. Assert on JSON where you can; quote human output where the user would read it.
- `--non-interactive` makes `project init`, `project add ...`, and `library ...` commands fail instead of prompting. Interactive prompts need a terminal; this harness doesn't provide one.
- Exit 0 is success, 1 is failure or stale output, 2 is invalid usage.
- A source added with `--ref main` follows a branch, which works for both libraries and non-library skill repositories and needs no release. `trailguide library release` needs a pushed tag and GitHub, so it is out of this harness's reach.
- A library needs `trailguide library init --spdx MIT --license-file FILE --non-interactive`, with FILE an existing license text. A non-library source needs a `license` field in each skill's frontmatter, or a `LICENSE` file.

## Evidence

Evidence goes to the folder `new` and `doctor` print, `<run>/evidence/`, beside the lab, never inside it. Each `tg-lab run` adds one transcript. Proof standards:

- Drive the real user path: the commands a user types, in a real Git repository. Don't call Go functions, edit `lock.json` or `_managed/` by hand to set up state, or use test fixtures, unless a recipe says the user would do the same.
- Capture the action and the resulting state. After a command that writes, capture a second, read-only view: `trailguide project check`, `trailguide project list skills --json`, `git status --short`, or the written file itself (`tg-lab run show-skill -- cat .claude/skills/NAME/SKILL.md`).
- After a failure that promises no writes, prove it: capture `git status --short` and the files that would have changed.
- Record which feature and sub-feature IDs each transcript proves, by its label, such as `refs-rewrite-sync`.
- A recipe step you couldn't reach is a failure with the attempted command and the unmet precondition, never a pass through a different path.

## Cleanup

```sh
TG_LAB=<lab> tg-lab cleanup
```

It deletes the lab, which holds the binary, the private HOME, and every repository, and lists the evidence it kept. The harness starts no background processes, so there is nothing to kill. Confirm the evidence folder still has your transcripts before you report.

## Helpers

- `scripts/tg-lab` (executable). Run it with no arguments for usage. Commands: `new [CHECKOUT]`, `doctor`, `remote NAME`, `project NAME`, `commit DIR MESSAGE`, `exec -- COMMAND...`, `run LABEL -- COMMAND...`, `cleanup`.
