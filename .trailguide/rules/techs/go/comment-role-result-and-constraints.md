---
title: Comment the role, the result, and the hidden constraint in Go
whenToRead: 'Before writing, changing, or reviewing Go files in this repository, including tests: their header comments, doc comments on exported and unexported declarations, and comments on values or lines whose reason the code doesn''t show.'
impact: MEDIUM
impactDescription: Missing headers and contract comments force readers and agents to trace implementations to learn what a file is for, who owns a result, and what nil means, and guessed rationales become false specifications.
---

## Comment the role, the result, and the hidden constraint in Go

Start every Go file, including tests, with one to three lines stating the file's role; in the file that holds the package comment, that comment states it.
Give every exported declaration a doc comment that says what the signature hides: who owns or must close a result, what nil or empty means, and how it fails.
Comment a value or line only for a constraint the code can't show, such as a protocol quirk or a security boundary.
Never narrate the code, and never invent a reason you don't know: name the value and leave the reason out instead.

### Rationale

Every Go file here follows the role-header convention, and nothing else checks it.
The contracts that matter most in this codebase are invisible in signatures, such as who owns an `*os.Root` or whether a missing directory is nil or an error.
A guessed rationale is worse than none, because the next reader or agent treats it as a requirement.

### Examples

**Incorrect (counterexample):**

```go
package filetxn

// ReadTree reads a tree.
func ReadTree(ctx context.Context, root *os.Root, name string) (*Tree, error) {
```

The file has no header, and the doc comment repeats the name while hiding that the caller still owns `root` and that a missing directory returns nil.

**Correct:**

```go
// Read bounded project trees and compare their exact bytes, including empty directories.

package filetxn

// ReadTree returns a contained, bounded tree without following observed links or accepting hard links.
// The caller owns root. Missing directories return nil; failures return no partial tree.
func ReadTree(ctx context.Context, root *os.Root, name string) (*Tree, error) {
```

### Validation

Check that every Go file a change adds or edits starts with a comment line.
For each exported declaration a change adds or edits, check that its doc comment states ownership, nil or empty meanings, and failure behavior wherever the signature leaves them open.

Not a violation: an unexported helper without a comment whose name and signature make its behavior clear.
