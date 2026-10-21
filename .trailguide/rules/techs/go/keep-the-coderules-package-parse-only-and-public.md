---
title: Keep the coderules package parse-only and public
whenToRead: Before planning, writing, changing, or reviewing Go code in the public coderules package or the internal packages it uses, its exports, errors, or tests, or Code Rules code that reads release tags, release records, rule files, group metadata, or the canonical group list.
impact: HIGH
impactDescription: A second parser lets Code Rules and other tools disagree about the same library, and effects, unneeded exports, or leaked internal error types break the public API other tools depend on.
---

## Keep the coderules package parse-only and public

`coderules`, in the `coderules/` directory, is the public package that other tools, such as catalogs, import to read libraries.
Its parsers take bytes or text and return values, without Git, filesystem, or network access, including through the internal packages they use.
Code Rules reads libraries through it, so never parse one of its formats a second way.
Treat its exported API as a public contract: export only what other tools need, and give callers error identity only for decisions they make.

### Implementation

- **Parse only:** every exported function takes bytes or text the caller already read. Its dependencies, such as `internal/decode`, `internal/librarytree`, and `internal/errs`, must stay free of effects too.
- **One parser per format:** library release tags, release records, rule files, group metadata, and the canonical group list are read only through `coderules`, as `internal/library` calls `coderules.ParseGroupMetadata` and `coderules.ParseRule`. Keep helpers that internal parsers share in an internal package, such as `internal/decode`, rather than exporting them.
- **Error identity:** callers can inspect exactly `ErrNotReleaseTag` with `errors.Is`, `*UnsupportedReleaseRecordError` with `errors.As`, and `*coderules.ValidationError` with `errors.As` for any other invalid input. Add another only for a decision callers make.
- **Translate at the boundary:** every exported function passes its error through `translate` in `validation_error.go`, which turns an internal package's validation error into a `*coderules.ValidationError` with the same location, problem, and text. Never wrap an internal error with `%w`, which would expose a type callers can't import.
- **Public contract:** the exported API is part of the public contract in `.release-planner/policy.md`, so changing or removing an export is a breaking change that the release notes must document.
- **Tests:** test through the exported API in package `coderules_test`. Use an internal test, in package `coderules`, only for fixtures of an unexported stage that decoding keeps other input from reaching, as `group_metadata_internal_test.go` does.
- **Name collision:** the repository's root package is also named `coderules` and only embeds the license. In a file that imports both, alias the root package, such as `license "github.com/fabricahq/code-rules"`.

### Rationale

Other tools must read a library exactly as Code Rules does, or a catalog and a project disagree about which rules and versions a library has.
One parser for each format guarantees that, and keeping it parse-only lets tools feed it bytes from any source, such as go-git, without Code Rules' Git or filesystem behavior.
Every exported name and error type is a promise to those tools, so an unneeded export, or an internal error type leaking through `%w`, becomes a contract that's hard to change.

### Examples

#### Application: Returning an internal package's error

**Incorrect (counterexample):**

```go
func ParseGroupMetadata(input []byte, location string) (GroupMetadata, error) {
	_, data, err := decode.YAML(input, location)
	if err != nil {
		return GroupMetadata{}, fmt.Errorf("parse group metadata: %w", err)
	}
	return groupMetadataFields(data, location)
}
```

The error carries a `*decode.ValidationError`, which callers outside Code Rules can't name, so `errors.As` into `*coderules.ValidationError` fails for invalid YAML.

**Correct:**

```go
func ParseGroupMetadata(input []byte, location string) (GroupMetadata, error) {
	_, data, err := decode.YAML(input, location)
	if err != nil {
		return GroupMetadata{}, translate(err)
	}
	return groupMetadataFields(data, location)
}
```

#### Application: Reading a library file inside Code Rules

**Incorrect (counterexample):**

```go
var metadata struct{ Name, Description, WhenToRead string }
if err := yaml.Unmarshal(data, &metadata); err != nil {
	return Group{}, err
}
```

This accepts files `coderules` rejects, such as ones with unknown fields or duplicate keys, so Code Rules and other tools can disagree about the same library.

**Correct:**

```go
metadata, err := coderules.ParseGroupMetadata(data, source+"/"+metadataPath)
if err != nil {
	return Group{}, err
}
```

### Validation

Run `go list -f '{{.ImportPath}}: {{join .Imports " "}}'` on `./coderules` and on each Code Rules package that `go list -deps ./coderules` lists. None may import `os`, `os/exec`, `net/http`, `internal/filetxn`, or `internal/gitexec`.
For each exported function a change adds or edits, check that every error path passes through `translate`, and that a test in `coderules_test` asserts `errors.As` into `*coderules.ValidationError` for invalid input.
For each new export, check that a tool outside Code Rules needs it.
Search Code Rules for decoding of release tags, release records, rule files, group metadata, or the canonical group list outside `coderules`; there should be none.

Not a violation: formats `coderules` doesn't parse, such as project configuration, change notes, and the library manifest's license declaration, parsed in internal packages such as `internal/rules`.
