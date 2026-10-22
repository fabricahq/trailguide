# Release policy

Release Planner's agent reads this file before every release. Treat the public contract below as what users depend on, and flag uncertainty instead of inventing compatibility guarantees.

## Breaking changes

A change is breaking when it changes any of these in a way that forces users to change their configuration, rules, scripts, or workflow:

- Command names, flags, arguments, and exit codes
- JSON output from `--json`, which scripts parse
- Project and library configuration files and their keys
- The rule library format: rule, group, and library files, and the IDs other projects reference
- Library releases: change notes in `changes/`, `release/<number>` tags, and the release record in their tag messages, which other Trail Guide versions read
- The exported API of the `github.com/fabricahq/trailguide/trailguide` Go package, which other tools import to read libraries
- The lockfile (`lock.json`), which projects commit
- Generated files
- The managed project README format (`.trailguide/README.md`); see [Choosing a version](#choosing-a-version)
- Supported platforms, and the release archive names, `SHA256SUMS`, and attestation signer that the standalone installer, the Homebrew tap, and generated library CI workflows depend on

Internal refactors, CI changes, and website-only changes are not part of the contract.

## Choosing a version

Versions follow [SemVer 2.0.0](https://semver.org/), with Git tags `vMAJOR.MINOR.PATCH`. The CLI reports the same version without the `v`. Prereleases use a suffix such as `v0.2.0-rc.1`. Release tags never carry build metadata.

Before 1.0.0:

- Minor: any breaking change or significant new feature. Always document breaking changes; 0.x is not permission to hide them.
- Patch: smaller compatible features and improvements, fixes, documentation, and internal changes

From 1.0.0:

- Major: any breaking change
- Minor: compatible new features
- Patch: compatible fixes, documentation, and internal changes

A change to the managed project README format is always breaking: a minor release before 1.0.0, a major release from 1.0.0. The notes describe the format change, and that `trailguide project build`, `trailguide project sync`, `trailguide project update`, and `trailguide project init` replace an older, unedited guide with the one bundled in the new version but refuse to overwrite a manually edited guide.

## Who reads the release notes

Readers:

- Developers and teams who install the `trailguide` CLI to adopt rules in their projects
- Authors of rule libraries, who care about changes to the library format and authoring commands
- Developers of tools, such as catalogs, that read libraries with the `github.com/fabricahq/trailguide/trailguide` Go package

## Order of the release notes

The notes present changes in this order, leaving out any with nothing to say:

1. New features
2. Improvements
3. Bug fixes
4. Breaking changes

## Always and never

- Always keep new features and improvements high level and concise: one short bullet per point, and leave option names, flags, and edge cases to the linked docs.
- Only open the notes of a major release with one plain sentence on what the release is about and who it matters to most, such as "v2.0.0 is mostly for library authors: it replaces rule groups with tags." Start the notes of every other release with the first heading.
- Always give each new feature its own `###` heading that names what changed for the reader, such as "Sources can follow branches", then summarize it in one bullet. When it helps, start with what was hard before and why it was a problem, using a real example such as Rulemart, then say what readers can now do. End with a link to the feature's docs, and add a short code block when the feature is a command readers will run.
- Always say what readers can now do, with "now", such as "You can now fork a skill into your library" or "`project update --help` now explains how skills update".
- Always give migration steps for a breaking change, with the commands or before-and-after examples readers need. Give each breaking change its own `###` heading that names the change. Until Trail Guide is publicly announced, summarize each breaking change in a sentence instead of migration steps, and say that upgrade instructions are omitted because Trail Guide is in initial development.
- Always credit external contributors by GitHub handle.
- Never present internal refactors, CI changes, or website-only changes as new CLI features.
- Never mention dependency updates unless they fix a security issue.

## Release pipeline

[build-release.yml](../.github/workflows/build-release.yml) builds the release files on the release pull request, and fails when their names differ from the ones the contract above lists.

After a stable release publishes, the `downstream` job runs **Update Trail Guide** (`update-trailguide.yml`) in [fabricahq/homebrew-tap](https://github.com/fabricahq/homebrew-tap). Prereleases don't update the formula. The job authenticates through **Fabrica Homebrew Releaser**, a shared App installed only on the tap with Actions write and Metadata read permissions. Its credentials are the `DOWNSTREAM_APP_CLIENT_ID` variable and `DOWNSTREAM_APP_PRIVATE_KEY` secret in this repository's `downstream` environment. The tap's own publishing App key never leaves the tap. [CR-7](https://linear.app/ohmygoshjosh/issue/CR-7/replace-shared-homebrew-trigger-keys-with-an-oidc-dispatch-service) tracks replacing shared keys with an OIDC dispatch service.

If the dispatch fails, use **Re-run failed jobs** on the Release run. If the tap update fails, fix the reported problem and run the tap's **Update Trail Guide** workflow manually. A tap failure doesn't change the published release.
