---
title: Follow the project write protocol
whenToRead: Before planning, writing, changing, or reviewing Go code that writes project or library files, such as sync, update, build, init, fork, or authoring commands, including commands that ask a person for input first, or a change that adds a target, an input, or a check to such a write.
impact: HIGH
impactDescription: Waiting for a person while holding the writer locks every other command out of the project, and a check that fails after the first write leaves imported, generated, and configuration files that disagree.
---

## Follow the project write protocol

Every operation that writes project or library files follows one protocol:

1. **Plan and read.** Read and check what the operation needs. A plan may take the writer briefly, as `project.PlanUpdate` does to recover an interrupted command and read the project.
2. **Release the writer** before anything waits for a person.
3. **Gather human input** in `internal/cli`. `--json` and `--non-interactive` never prompt.
4. **Retake the writer** with `filetxn.WithWriter` or `filetxn.Edit`.
5. **Recheck assumptions.** Fail, writing nothing, when what the plan read changed, as `UpdatePlan.Apply` does with `concurrent-change`.
6. **Validate and render every target in memory**, including network reads such as reading a fork's version.
7. **Apply everything in one transaction:** one `Writer.Apply` call, whose `assertUnchanged` callback rereads the inputs, or one `filetxn.Edit`.

Two things are forbidden: waiting for a person while holding the writer, and any check that can fail after the first write.

### Rationale

The writer is an exclusive lock: while one command holds it, every other command that writes the project fails with `busy`, and a person can leave a prompt open for minutes.
`Writer.Apply` stages every target, rechecks the inputs, and writes a journal before it replaces live files, so an interrupted write can be recovered or rolled back as a whole.
A check that fails between two writes defeats that, leaving new vendor snapshots beside old generated output, which the next build rejects or agents read as valid guidance.

### Examples

#### Application: Asking a person

**Incorrect (counterexample):**

```go
// Apply asks for confirmation while it holds the writer.
err = filetxn.WithWriter(ctx, root, func(w *filetxn.Writer) error {
	before, err := readProject(ctx, root)
	if err != nil {
		return err
	}
	if !confirm(preview) {
		return nil
	}
	changes, err = install(ctx, root, w, before, in)
	return err
})
```

The project stays locked for as long as the preview sits unanswered.

**Correct:**

```go
// PlanUpdate reads the project under the writer and releases it before returning.
plan, err := project.PlanUpdate(cmd.Context(), project.Options{Directory: directory, ToolVersion: options.Version}, options.Git, targets, decisions)
// ...
answer, err := askChoice(f, "Apply the update?", [2]string{"yes", "no"})
// ...
// Apply retakes the writer and fails with concurrent-change if the project changed after the preview.
result, err := plan.Apply(cmd.Context(), decisions)
```

#### Application: Writing the result

**Incorrect (counterexample):**

```go
for name, files := range vendor {
	if err := writeVendorSource(root, name, files); err != nil {
		return FileChanges{}, err
	}
}
output, err := renderProject(ctx, state, libraries, in.options)
if err != nil {
	return FileChanges{}, err
}
```

An invalid local rule makes rendering fail after `vendor/` already holds the new snapshots.

**Correct:**

```go
output, err := renderProject(ctx, state, libraries, in.options)
if err != nil {
	return FileChanges{}, err
}
targets := map[filetxn.Target]map[string][]byte{filetxn.Vendor: vendor, filetxn.Generated: output.Files}
// ...
err = w.Apply(targets, func() error {
	if err := requireUnchanged(ctx, root, before); err != nil {
		return err
	}
	return requireGuideUnchanged(ctx, root, in.guide)
})
```

### Validation

For each `filetxn.WithWriter` or `filetxn.Edit` callback a change adds or edits, check that nothing inside it reads standard input or calls back into `internal/cli`, and that every target reaches the same `Apply` or `Edit` call after all validation, rendering, and network reads.
Check that tests change the project between plan and commit, and make a late step fail, and assert in both cases that no file changed.
