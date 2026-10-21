---
title: Parse and render in memory; leave all I/O to the operation
whenToRead: Before planning, writing, changing, or reviewing Go code that parses Code Rules formats, such as configuration, rules, group metadata, or release records, that renders generated output, or that reads the files those steps need.
impact: MEDIUM-HIGH
impactDescription: A parser or renderer that reads files itself escapes the operation's containment, size limits, cancellation, and concurrent-change check.
---

## Parse and render in memory; leave all I/O to the operation

Parsers and renderers consume bytes and values and return values; they never open files, run Git, or use the network.
The operation that calls them, such as `project.Sync`, `project.Build`, or a library command, does all I/O, because it owns what makes I/O safe here: `*os.Root` containment, the bounded readers in `internal/filetxn`, cancellation, the snapshot that its concurrent-change check compares, and write ownership.

### Rationale

`project.Sync` and `UpdatePlan.Apply` record every file they read and compare that record before they write.
A renderer that reads a file itself escapes that check, the root's containment, and the read limits, so a concurrent edit or an oversized or linked file reaches generated output unnoticed.

### Examples

**Incorrect (counterexample):**

```go
// Generate reads the project's local rules itself.
func Generate(config rules.Configuration, libraries map[string]Library, directory string, options Options) (Output, error) {
	localFiles, err := readLocalRules(filepath.Join(directory, ".code-rules", "local"))
	// ...
}
```

**Correct:**

```go
// In internal/build: inputs arrive as bytes, and output is returned, not written.
func Generate(config rules.Configuration, libraries map[string]Library, localFiles map[string][]byte, options Options) (Output, error)

// In internal/project: the operation reads once, under its root and limits, then renders.
tree, err := filetxn.ReadTree(ctx, root, item.name)
// ...
output, err := build.Generate(state.config, libraries, treeFiles(state.local), build.Options{ToolVersion: version})
```

### Validation

Run `go list -f '{{.ImportPath}}: {{join .Imports " "}}'` on the parsing and rendering packages, such as `./coderules`, `./internal/rules`, `./internal/decode`, and `./internal/build`; none may import `os`, `os/exec`, `net/http`, `internal/filetxn`, or `internal/gitexec`.
Check that a new parser or renderer takes bytes or values, not paths.

Not a violation: `io/fs` used only for path validation, such as `fs.ValidPath`.
