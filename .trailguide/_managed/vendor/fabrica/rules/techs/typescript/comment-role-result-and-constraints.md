---
title: "Comment the role, the result, and the hidden constraint"
whenToRead: "Before writing, changing, or reviewing TypeScript files, exported functions, types, components, or code whose purpose, behavior, or constraints are not obvious from names and types."
impact: "MEDIUM"
impactDescription: "Missing or narrating comments force readers and agents to trace implementations to learn what files and functions do, and guessed rationales become false specifications."
tags: "typescript, comments, tsdoc, documentation"
attribution:
  - url: https://github.com/mkosir/typescript-style-guide/blob/86bebd58a987e23277dba02028c0ee2d6ffb5073/website/src/pages/index.mdx
    description: "Adapted from mkosir TypeScript Style Guide guidance (comment-role-result-and-constraints; MIT, notice retained in NOTICE.md): restructured to the rule template; the guidance and examples are otherwise preserved."
---

## Comment the role, the result, and the hidden constraint

Write comments that save the reader work: a file comment stating the file's role, an export comment stating what callers receive, and a body comment only for a constraint that names and types cannot express.
Do not narrate the code, and do not invent reasons you do not know.

### Implementation

- **File role:** start every source file with a 1-3 line comment saying what the file is for, and what it is not for when that is surprising.
  Write it as a `/** @fileoverview */` block or `//` lines; a bare `/** */` before the first declaration documents that declaration instead.
  One file, one role: a helper the header cannot account for belongs elsewhere.
  Add a header to any file a change touches that lacks one.
  Generated files are exempt.
- **Exported contract:** give every exported function, type, and component a doc comment stating what callers receive or can rely on, including rules the signature hides, such as ordering, case, locale, units, mutation, empty input, and failure modes.
  Describe behavior in plain language rather than naming the mechanism.
  Drop `@param` and `@returns` tags that repeat names and types; keep `@throws` only when it names the condition.
- **Private helpers:** prefer names and signatures that make purpose clear, and add a short comment only when behavior or constraints would otherwise require reading the implementation.
- **Hidden constraints:** comment a body line only for a vendor quirk, legal requirement, workaround, or invariant the types cannot express, next to the value or operation it explains.
  When the constraint belongs to a constant, name the constant and put the comment there.
- **Unknown reasons:** when the reason is not known, name the value and leave the rationale out, or point to the issue that will settle it.
  Point workarounds and TODOs at an issue, pull request, or upstream document.
- **Durability:** prefer comments that stay true if the implementation changes while keeping its contract, and remove scratch and change-history comments before merging; version control owns history.

### Rationale

A comment is an index entry, not a narration of the next line.
A file comment lets a reader decide whether to open a file, and an export comment tells a caller what they get without reading the body.
Agent-written code tends to fail this by omission: files without headers, exports whose `@param` tags repeat the signature, and unexplained magic numbers.
A guessed rationale is worse than none, because the next reader or agent treats it as a requirement.

### Examples

#### Application: File role and exported contract

**Incorrect (counterexample):**

```ts
import { splitRows } from './split-rows';

/**
 * Parses transactions.
 * @param csv - The CSV string to parse
 * @returns The parsed transactions
 */
export function parseTransactions(csv: string): Array<Transaction> {
  return splitRows(csv).map(parseRow);
}
```

The file has no header, and the export comment repeats the name and signature.

**Correct:**

```ts
/**
 * @fileoverview Ingests the ledger's `id,posted_at,amount` CSV export.
 * Not a general CSV parser: fields are never quoted or comma-embedded.
 */
import { splitRows } from './split-rows';

/**
 * Parse the ledger export into transactions with amounts in integer cents.
 * Tolerates a leading UTF-8 BOM, CRLF line endings, and blank lines.
 * Throws on a row with the wrong field count, an unparsable timestamp,
 * or an amount that is not a two-decimal number.
 */
export function parseTransactions(csv: string): Array<Transaction> {
  return splitRows(csv).map(parseRow);
}
```

These contract details must be verified against `splitRows` and `parseRow`, not inferred from the function's name.

#### Application: A hidden constraint

**Incorrect (counterexample):**

```ts
// Wait 250 milliseconds.
const VENDOR_MIN_CALL_INTERVAL_MS = 250;

// Try three times.
const MAX_ATTEMPTS = 3;
```

The comments narrate the values and omit why the interval exists.

**Correct:**

```ts
// The vendor sandbox returns HTTP 429 for calls closer together than this.
const VENDOR_MIN_CALL_INTERVAL_MS = 250;

const MAX_ATTEMPTS = 3;
```

The attempt count stays named but unexplained, because no reason for choosing three has been recorded.

#### Application: An observable result

**Incorrect (counterexample):**

```ts
/** Sorts folders by name. */
export function sortFolders(folders: ReadonlyArray<Folder>): Array<Folder> {
  return [...folders].sort((a, b) => a.name.localeCompare(b.name, 'en-US', { sensitivity: 'base' }));
}
```

**Correct:**

```ts
/** Return a new array ordered by name using en-US collation, ignoring case and accents. */
export function sortFolders(folders: ReadonlyArray<Folder>): Array<Folder> {
  return [...folders].sort((a, b) => a.name.localeCompare(b.name, 'en-US', { sensitivity: 'base' }));
}
```

### Validation

Check that each touched file has a role header, that each export's comment states its observable contract, and that body comments explain constraints rather than narrate.
Lint for presence, not content, with `eslint-plugin-jsdoc`: enable `require-file-overview`, `require-jsdoc` with `publicOnly`, and `require-description`, and disable `require-param` and `require-returns`.
`require-file-overview` recognizes only the `@file` block form, so a project using `//` headers needs its own check.
Review judges whether comments are accurate and useful.

A private helper without a comment is not a violation when its name and signature make its behavior clear.
