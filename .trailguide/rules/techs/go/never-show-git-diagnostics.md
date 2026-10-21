---
title: Never show what Git, a Git server, or the GitHub CLI printed
whenToRead: Before planning, writing, changing, or reviewing Go code that runs Git or the GitHub CLI through internal/gitexec, or that turns their failures into errors, warnings, or reports, such as fetching a library, pushing a release tag, or creating a GitHub Release.
impact: HIGH
impactDescription: Git and server messages can carry credential-bearing repository addresses and text a hostile server controls, which would leak into terminals, CI logs, and JSON responses.
---

## Never show what Git, a Git server, or the GitHub CLI printed

Treat the diagnostics of Git and the GitHub CLI, such as `gitexec.Result.Diagnostics` or `gh`'s standard error, as private input.
Match them with `gitexec.Mentions` only to choose a fixed message of your own.
Never put their text into an error, warning, log, report, or JSON response.

### Implementation

- Classify with `gitexec.Mentions(diagnostics, phrases...)`, which ignores case, terminal escape sequences, and control characters.
  Reuse the shared phrase lists, such as `gitexec.HTTPSCertificateFailures`, `gitexec.SSHHostKeyFailures`, `gitexec.ConnectionFailures`, and `gitexec.AccessFailures`, in the order their comments require.
- Return a `gitexec.Error`, or the package's alias of it, with a stable code and a static problem that says what to do next.
- For a failure no phrase matches, say that Code Rules doesn't recognize it, and name the Git or `gh` command that shows the message, so the person can read it themselves.
- Build any value you show from what Code Rules already knows. For example, `releasePageURL` constructs a GitHub Release URL instead of showing the one `gh` printed.
- This applies to messages the programs print about their own work. Repository content that Code Rules reads through Git, such as a rule's Markdown, is data: parse and validate it like any other input.

### Rationale

Git's messages can include the repository address with its credentials, such as `https://user:token@host/rules.git`, and a server can print anything after `remote:`, including terminal escape sequences and misleading instructions.
Showing that text leaks credentials into terminals, CI logs, and JSON responses, and lets a server write to the person's terminal as if it were Code Rules.

### Examples

**Incorrect (counterexample):**

```go
if result.Status != 0 {
	return fmt.Errorf("git fetch failed: %s", result.Diagnostics)
}
```

The token in the remote's address and everything the server printed reach the person's terminal and the `--json` response.

**Correct:**

```go
func remoteFailure(diagnostics []byte) error {
	if err := unreachable(diagnostics); err != nil {
		return err
	}
	if gitexec.Mentions(diagnostics, gitexec.AccessFailures...) {
		return fail("not-found-or-no-access", "Could not find the library's repository, or your Git credentials can't read it. Check the repository address and your Git credentials.", nil)
	}
	return fail("git-failed", "Could not read the library's repository for a reason Code Rules doesn't recognize. Run git ls-remote with the repository address to read Git's message.", nil)
}
```

The diagnostics only choose between fixed messages, each with a stable code and a next step.

### Validation

For each changed use of a Git or `gh` result, check that its diagnostics flow only into `gitexec.Mentions` and that every message it can produce is a constant or built from values Code Rules supplied.
A test can make a fake Git or `gh` print a recognizable secret, such as `secret-token`, and assert that neither the human output nor the `--json` response contains it.

Not a violation: generated output that shows a rule's validated Markdown read through Git, or a message naming a commit ID or tag that Code Rules requested.
