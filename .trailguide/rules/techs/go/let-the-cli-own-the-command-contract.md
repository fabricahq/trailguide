---
title: Let the CLI own the command contract
whenToRead: Before planning, writing, changing, or reviewing Go code that reports a result, warning, or failure, that turns one into human output, a --json response, a prompt, or an exit status, or that tests what a code-rules command prints, returns, or exits with.
impact: HIGH
impactDescription: Output, prompts, or exits outside internal/cli corrupt the --json response and exit statuses that agents and scripts depend on, and tests below the CLI boundary pass while the assembled command breaks them.
---

## Let the CLI own the command contract

`internal/cli` alone writes standard output and standard error, produces the `--json` response, prompts, and chooses the exit status, and only `main` exits the process.
Command output, `--json`, and exit codes are part of the public contract in `.release-planner/policy.md`.
Domain packages return results and errors; they never print, prompt, or exit.

### Implementation

- Return what a person or agent needs to see as data, such as `project.FileChanges.Warnings`, or as an error with a stable code, such as a `*gitexec.Error`.
- `--json` writes exactly one response to standard output, including for failures, and never prompts. Report an expected failure once, as the command's result, not also as a log line.
- Invalid input crosses package boundaries only through the `errs.ValidationError` interface.
  Recognize it with `errors.As` into a variable of that interface type, never into another package's concrete error type, and report a new problem with your own package's `ValidationError`.
- Test a change to the contract at the CLI boundary: call `cli.Run` with test `Streams`, as `invokeReport` does, and run the compiled binary, as `buildCLI` and `runCLI` do, for behavior only a process has, such as exit status from `main`, terminal prompts, interrupts, and Git configuration.

### Rationale

Agents and scripts parse the `--json` response and branch on exit statuses.
Text a domain package prints is missing from that response and appears even when an embedder passes its own streams, and a domain package that exits skips deferred cleanup, such as releasing the project writer.
A check for one package's concrete validation error misses invalid input from every other package, which then reaches agents without a location.
These behaviors come from Cobra, `internal/cli`, and the domain operation together, so a helper's unit test can pass while the command breaks them.

### Examples

#### Application: Reporting a warning

**Incorrect (counterexample):**

```go
// In internal/project, sync announces a warning itself.
for _, warning := range in.imported[source.Name].Warnings {
	fmt.Fprintln(os.Stderr, "Warning:", warning)
}
```

**Correct:**

```go
// In internal/project, install collects warnings into the report it returns.
warnings = append(warnings, item.Warnings...)
// ...
changes.Warnings = warnings
return changes, nil
```

`internal/cli` prints each warning in human output, and `--json` includes them in `value.warnings`.

#### Application: Recognizing invalid input

**Incorrect (counterexample):**

```go
var validation *project.ValidationError
if errors.As(err, &validation) {
	result.Kind = "validation"
}
```

Invalid input that `internal/imports` or `coderules` reported comes back as an operation failure without a location.

**Correct:**

```go
var validation errs.ValidationError
// ...
case errors.As(err, &validation):
	result.Kind = "validation"
	result.Location = validation.ValidationLocation()
```

#### Application: Testing a usage error

**Incorrect (counterexample):**

```go
if got := classifyError(usage(errors.New("unknown flag: --bad"))); got.Kind != "usage" {
	t.Fatalf("kind %q", got.Kind)
}
```

This passes even if `Run` exits with `1` instead of `2` or writes to standard error in `--json` mode.

**Correct:**

```go
out, diagnostic, code := invokeReport(t, ctx, directory, "project", "build", "--bad", "--json")
var result response
if err := json.Unmarshal([]byte(out), &result); err != nil {
	t.Fatal(err, out)
}
if code != 2 || result.OK || diagnostic != "" || result.Error.Kind != "usage" || result.Error.Code != "invalid-arguments" {
	t.Fatal(code, out, diagnostic)
}
```

### Validation

Search non-test Go files under `internal/` outside `internal/cli` for `os.Exit`, `os.Stdout`, `os.Stderr`, `os.Stdin`, `fmt.Print`, and the `log` package; there should be no matches.
Search changed code for `errors.As` into another package's `*ValidationError`, or construction of another package's `ValidationError`.
For a change to a command's output, error codes, prompts, or exit status, check that a test reaches it through `cli.Run` or the compiled binary in each output mode it affects.

Not a violation: a separate `main` package, such as `cmd/package-binaries`, printing its own output, or a package's tests asserting on its own concrete `*ValidationError`.
