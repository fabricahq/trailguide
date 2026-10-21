---
title: Run Git with gitexec.Isolated for libraries and gitexec.Owned for the user's repository
whenToRead: Before planning, writing, changing, or reviewing Go code that creates a Git runner or runs Git, such as fetching or reading a library someone else publishes, or reading, tagging, or pushing the user's own library repository.
impact: HIGH
impactDescription: Honoring the user's hooks and transports in a repository someone else controls lets its address or content run commands, while isolating the user's own repository skips the hooks their pushes rely on.
---

## Run Git with gitexec.Isolated for libraries and gitexec.Owned for the user's repository

Choose the Git runner by who owns the repository.
Use `gitexec.Isolated` for a repository someone else controls, such as a library a project imports or forks from.
Use `gitexec.Owned` only for the user's own repository, such as the library repository that `code-rules library release` tags and pushes.

### Implementation

- `gitexec.Isolated` disables hooks and allows only HTTPS and SSH transports, whatever the user's Git configuration says. Use it whenever the repository address comes from project configuration or another library, including when Code Rules fetches into a temporary repository it created.
- `gitexec.Owned` honors the user's Git configuration, credentials, and hooks, so a push runs the same pre-push hooks as `git push` would. Use it only in a repository the user is working in, such as the library root a `code-rules library` command resolved.
- Use `gitexec.Command` for another trusted program, such as `gh`.
- Take `gitexec.Options` from the caller's trusted settings, such as `cli.Options.Git`, never from configuration or repository content.
- When you can't tell whether a repository is the user's own, use `gitexec.Isolated`.

### Rationale

A library's address comes from configuration that someone else may have written.
With the user's configuration, Git would accept any transport the user allowed for their own work, such as local paths or `ext::`, which runs a command, and the user's global hooks would run in repositories that hold content Code Rules fetched.
The user's own repository is the opposite case: their hooks and configuration are part of how they work, and skipping them would publish a release tag their own checks never saw.

### Examples

#### Application: Reading a library

**Incorrect (counterexample):**

```go
runner, err := gitexec.Owned(gitexec.Options{GitPath: options.GitPath, Environment: options.Environment})
```

In `internal/imports`, this fetches a library whose address came from `config.yaml` with every transport and hook the user's Git configuration allows.

**Correct:**

```go
runner, err := gitexec.Isolated(gitexec.Options{GitPath: options.GitPath, Environment: options.Environment})
```

#### Application: Releasing the user's library

**Correct:**

```go
// openLibraryGit returns nil when the library root isn't a Git repository's root, so it has no release history.
runner, err := gitexec.Owned(options)
```

The library commands work in the user's own repository, so pushing a release tag runs their pre-push hooks.

### Validation

For each call to `gitexec.Isolated` or `gitexec.Owned` that a change adds or moves, identify whose repository the runner reads or writes, and check that the choice matches.
A runner that reads any address from configuration or a library must be `gitexec.Isolated`.
