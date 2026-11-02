---
title: "Install Trail Guide"
description: "Install Trail Guide with the shell script or Homebrew."
---

Trail Guide is a standalone binary for macOS and Linux. [Using Windows?](#installing-on-windows)

## Use the standalone installer

```sh
curl -fsSL https://trailguide.fabricahq.com/install.sh | sh
```

This installs the latest stable release into `~/.local/bin` and sets up your shell's `PATH`. No `sudo` needed. [View the installer script](/install.sh).

<span id="set-up-your-path"></span>

Open a new terminal, then verify the installation:

```sh
trailguide --version
```

To use your current terminal, run the command the installer prints instead.

**Next:** [Set up your first project](/start-here/set-up-project/) or [create your first library](/start-here/create-library/).

## Upgrade

Rerun the installer. Then, from each existing project root, refresh and check its generated files:

```sh
trailguide project build
trailguide project check
```

A library's check workflow installs the release its `minTrailguideVersion` names, which `trailguide project update` raises to the version you run. A workflow written by Trail Guide 0.8.0 or earlier pins the version that created it instead; to replace it, delete `.github/workflows/trailguide.yml` from the library and run `trailguide library init` again; see [Check changes in CI](/guides/version-rules/#check-changes-in-ci). A library created with Code Rules has `.github/workflows/code-rules.yml` instead; follow [Upgrade from Code Rules](#upgrade-from-code-rules).

## Upgrade from Code Rules

Trail Guide was called Code Rules. Its command was `code-rules`, and projects and libraries kept their files in `.code-rules/`. After you install Trail Guide, remove the old `code-rules` executable, then move each repository to the new name:

1. From the repository root, rename the directory, keeping everything in it:

   ```sh
   git mv .code-rules .trailguide
   ```

   In a project that isn't in a Git repository, use `mv` instead of `git mv`.

2. In `AGENTS.md`, `CLAUDE.md`, or the instruction file your agent reads, delete the section that points to `.code-rules/generated/RULES.md`. The next step adds a section to `AGENTS.md` that Trail Guide keeps current. If your agent reads `CLAUDE.md`, add the line `@AGENTS.md` to it.
3. Rebuild and check the project:

   ```sh
   trailguide project build
   trailguide project check
   ```

4. In a library, delete `.github/workflows/code-rules.yml` and run `trailguide library init` to write `.github/workflows/trailguide.yml`.

Until you rename it, project commands keep reading and updating `.code-rules/` and print a warning; a later version will stop reading it. Library commands refuse until the directory is renamed. Library releases published from `.code-rules/` stay readable, so the projects that import a library keep their rule versions and need no changes when the library moves.

## Install with Homebrew

With [Homebrew](https://brew.sh/) installed on macOS or Linux:

```sh
brew install fabricahq/tap/trailguide
trailguide --version
```

To upgrade or remove it:

```sh
brew upgrade trailguide
brew uninstall trailguide
```

## Other options

<details id="choose-a-version-or-installation-directory">
<summary>Choose a version or installation directory</summary>

Add `--version` to install an older or specific [release](https://github.com/fabricahq/trailguide/releases), or `--install-dir` to choose an absolute, writable directory:

```sh
curl -fsSL https://trailguide.fabricahq.com/install.sh |
  sh -s -- --version 0.1.0 --install-dir "$HOME/bin"
```

Use the same custom directory when upgrading. Pass `--no-update-path` if you prefer to manage your shell configuration yourself.

</details>

<details id="download-a-release-manually">
<summary>Download a release manually</summary>

Download the archive for your OS and processor, plus `SHA256SUMS`, from [GitHub Releases](https://github.com/fabricahq/trailguide/releases). If you have the GitHub CLI, first verify the checksum manifest came from our release workflow:

```sh
gh attestation verify SHA256SUMS --repo fabricahq/trailguide \
  --signer-workflow \
    fabricahq/trailguide/.github/workflows/release-planner.yml \
  --source-ref refs/heads/main
```

Releases 0.3.0 and earlier were published as Code Rules, before the repository was renamed, so their attestations name the old repository. To verify one of them, use `fabricahq/code-rules` in place of `fabricahq/trailguide` in both places in the command above.

Stop if verification fails. Before extracting, compare the archive checksum with the matching entry using `shasum -a 256 <archive>` on macOS or `sha256sum <archive>` on Linux.

Extract the archive and move the executable into a directory on your `PATH`.

To build from source instead, follow the contributing guide in the [Trail Guide repository](https://github.com/fabricahq/trailguide).

</details>

<details id="installing-on-windows">
<summary>Installing on Windows</summary>

Native Windows is not supported yet. The Linux version is expected to work in [WSL 2](https://learn.microsoft.com/en-us/windows/wsl/about), but we have not tested it end to end. Run the installer above in your WSL 2 terminal.

</details>

<details id="upgrade-roll-back-or-remove">
<summary>Uninstall</summary>

```sh
rm "$HOME/.local/bin/trailguide"
```

Adjust the path if you chose a custom installation directory. Your project's rules are left in place.

</details>

For version compatibility and managed guide changes, see the [release version policy](https://github.com/fabricahq/trailguide/blob/main/.release-planner/policy.md#choosing-a-version).
