# Contributing to Trail Guide

Thanks for helping build Trail Guide. This guide covers building the CLI, validating changes, working on the documentation website, testing PR builds, and releases.
The [README](README.md) introduces the product; [AGENTS.md](AGENTS.md) holds the product context and decisions that guide changes.

## Build the CLI

Install the Go version declared in [go.mod](go.mod). Commands that fetch or publish libraries, such as `trailguide project sync`, `trailguide project update`, and `trailguide library release`, require Git; GitHub Release pages also require the [GitHub CLI](https://cli.github.com/).

```sh
go build -o ./dist/trailguide ./cmd/trailguide
./dist/trailguide --help
```

The executable runs without Node.js or Bun. From a consuming project's root, run `trailguide project init`, create a local group and rule, then `trailguide project build`. To adopt a library or a repository of skills, use `trailguide project add source` with its repository and rule or skill selectors, then run `trailguide project sync`.

Read [project setup](docs/src/content/docs/start-here/set-up-project.md) and [the CLI reference](docs/src/content/docs/reference/cli.md) for the complete workflow. Human output is the default; `--json` returns structured responses and disables prompts. `trailguide project check` reports status and problems without writing files.

## Validate changes

```sh
git ls-files -z '*.go' | xargs -0 gofmt -w
go vet ./...
go run honnef.co/go/tools/cmd/staticcheck@v0.8.2 ./...
go test -race ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 -test ./...
go build ./cmd/trailguide ./cmd/package-binaries
go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12 -shellcheck= -pyflakes=
```

Tests exercise parsers, filesystem safety, Git imports, real CLI processes, generated agent instructions, and installation/upgrade/rollback. Parser regression fixtures live beside their Go tests. This repository's Go conventions, such as output and error ownership, Git safety, and comments, are local rules in the Go group of the [generated rules](.trailguide/_managed/generated/RULES.md).

[Security practices](_engineering/security-practices.md) define dependency pins, Renovate updates, vulnerability scans, and review requirements.

## Documentation website

The Astro website and its JavaScript tooling are independent of the Go executable. Install the Bun version declared in [package.json](package.json), then run:

```sh
bun install --frozen-lockfile
bun run check
bun run docs:dev
```

`bun run check` validates website/tooling formatting, lint, types, tests, the Astro build, and rendered links. It does not replace Go validation. See [docs/README.md](docs/README.md) for site development and the [CSS and Tailwind guidelines](docs/_internal/css.md) for styling conventions.

TypeScript stays on 6.0.3 because the current Astro checker and ESLint parser require its compiler API. [TypeScript 7 does not yet provide that API](https://devblogs.microsoft.com/typescript/announcing-typescript-7-0/#running-side-by-side-with-typescript-6-0); revisit this pin when both tools support it.

## Test a PR build

The [PR packaging workflow](.github/workflows/package-binaries.yml) builds preview executables for every PR. After a successful build, PRs opened from a branch in this repository by someone with write or admin access automatically receive a comment with preview links. Forks, including maintainer-owned forks, and other contributors need a maintainer to approve the exact commit:

1. Review the current PR commit, including source, dependencies, tests, and build workflow changes, for isolated testing.
2. Find its successful **Package CLI binaries** run. Copy the numeric run ID from its URL and the PR's full 40-character commit SHA.
3. In Actions, select **CLI preview downloads**, then **Run workflow** from the default branch. Enter that run ID and commit SHA.

A new commit needs a new approval.

**Warning: These executables run code from the PR. Use a disposable test environment without credentials or private files. Even `--help` executes the program.**

The comment offers direct executable downloads for macOS (Apple Silicon or Intel) and Linux (ARM or Intel/AMD), and a one-line command that uses the signed-in GitHub CLI to download the file for your computer as `./trailguide` in the current directory, replacing any existing file there. The command runs [`_tools/install-preview.sh`](_tools/install-preview.sh) from the default branch's workflow commit, never from the PR. To download by hand instead, rename the file to `trailguide`, then run:

```sh
chmod +x trailguide
./trailguide --help
```

Each preview command prints this warning to stderr before command output, including help, version, and JSON commands:

```text
WARNING: Unreleased preview from commit <full SHA>. For testing only; not for production use.
```

JSON output on stdout remains unchanged. The warning is a reminder, not proof of authenticity: someone modifying the binary could remove it. Preview binaries are not publisher-signed or attested by Trail Guide.

No extraction or global installation is needed. The comment also links to the build results and license. GitHub sign-in is required; downloads expire after seven days, whether the PR is open, closed, or merged.

### Candidate archives

To build unreleased archives locally, commit your changes first: packaging builds committed source. Release builds take an explicit version; candidate versions default to a source commit identifier.

```sh
go run ./cmd/package-binaries --candidate --output /tmp/trailguide-artifacts
```

Use a new output directory for each attempt. Verify archive checksums against a trusted manifest before extraction. Describe these builds as unpublished review candidates, and claim platform compatibility only where testing supports it. Read the [packaging command](cmd/package-binaries/) for options.

## Releases

[Release Planner](https://release-planner.fabricahq.com) publishes releases: a release PR supplies editable notes and the version in `_releases/v<version>.md`, and builds and tests the release assets; merging it publishes them. [Make a release](https://release-planner.fabricahq.com/start-here/release/) describes the procedure, and the [release policy](.release-planner/policy.md) owns versions, breaking changes, and this repository's release pipeline.

## Implementation map

- `trailguide`: the public Go package for reading libraries; the root package of the same name only embeds the license for the CLI. It parses library release tags, release records, library manifests, rule files, group metadata, and the canonical group list, and places library files in their repository, for Trail Guide and for other tools such as catalogs, without Git, filesystem, or network access. Its exports are a public API that follows Trail Guide's version, so keep them deliberate.
- `internal/errs`: error contracts several packages share, such as the `ValidationError` interface that each package's own validation error satisfies.
- `internal/decode`: the strict rules every format shares for decoding YAML and JSON text into generic values, before the format's own parser reads them.
- `internal/librarytree`: the structure of files and directories in a library, and the group and rule IDs derived from positions in that tree.
- `internal/skills`: skill IDs, install names, the `SKILL.md` fields generated guidance shows, and the marker line of an installed copy.
- `internal/rules`: validated configuration, change notes, license declarations, Git references, links, and rendering of authored rules and groups.
- `internal/library`: initialize, author, check, load, and publish rule libraries.
- `internal/imports`: choose rule versions from library release tags, import libraries with verified Git provenance, and read published rule versions for forks.
- `internal/releasetag`: list a repository's `release/<number>` tags and read their release notes and records within the release tag limits, for both library authoring and imports.
- `internal/gitexec`: run Git, and other trusted programs such as the GitHub CLI, with bounded output and cancellation, isolated for other people's repositories or honoring the user's configuration in their own, and classify what they print privately, so callers explain failures with their own messages and never show that text.
- `internal/build`: generate complete output from validated inputs through `Generate`.
- `internal/project`: initialize and author consuming projects, sync libraries, build offline, and check complete project freshness.
- `internal/filetxn`: bounded filesystem reads, writer ownership, safe publication, and recovery shared by both owners.
- `internal/cli`: collect inputs and present operation results.
- `internal/distribution`: package and verify executable archives.
- `internal/test/acceptance`: exercise complete CLI workflows through real processes.
- `internal/test/gitfixture`: supply disposable Git repositories that tests can fetch from and push to.
- `internal/test/ghfixture`: stand in for the GitHub CLI with declared responses and recorded calls.
- `internal/test/terminalfixture`: exercise interactive CLI prompts in isolated terminals.

`internal/test/` groups these four independent packages; it contains no Go package of its own. Unit tests remain beside the code they test.

Engineering policies belong to independently owned libraries. This repository supplies the formats, tools, and public authoring guidance.
