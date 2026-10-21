---
title: "Comment the role, the result, and the hidden constraint"
whenToRead: "Before writing, changing, or reviewing TypeScript, JavaScript, or Astro files in this repository's website and tools, their exported functions, types, and components, or code whose purpose, behavior, or constraints are not obvious from names and types."
impact: "MEDIUM"
impactDescription: "Missing or narrating comments force readers and agents to trace implementations to learn what files and functions do, and guessed rationales become false specifications."
tags: "typescript, comments, tsdoc, documentation"
attribution:
  - url: https://github.com/mkosir/typescript-style-guide/blob/86bebd58a987e23277dba02028c0ee2d6ffb5073/website/src/pages/index.mdx
    description: "Adapted from mkosir TypeScript Style Guide guidance (comment-role-result-and-constraints; MIT, notice retained in NOTICE.md): restructured to the rule template; the guidance and examples are otherwise preserved."
  - url: https://github.com/fabricahq/public-library/blob/568e32f46cc3bf557ab90e82b8df6d661b839112/techs/typescript/comment-role-result-and-constraints.md
    description: Forked from version 1.0.0 of techs/typescript/comment-role-result-and-constraints, published in library release 1 at commit 568e32f46cc3bf557ab90e82b8df6d661b839112.
---

## Comment the role, the result, and the hidden constraint

ESLint already requires a `/** @fileoverview ... */` header on every source file and a description on every exported function, type, and class, so this rule is about what those comments say.
A file header states the file's role, and what it isn't for when that's surprising.
An export's comment states what callers receive or can rely on that the signature hides, such as ordering, units, mutation, empty input, and failure modes.
A body comment explains only a constraint the code can't show.
Never narrate the code, and never invent a reason you don't know.

### Implementation

- In an Astro component, put the overview at the top of the frontmatter and describe what the component renders, because its export is implicit.
- Drop `@param` and `@returns` tags that repeat names and types; keep `@throws` only when it names the condition.
- Comment a private helper only when its behavior isn't clear from its name and signature. ESLint doesn't require or forbid these comments, and neither does this rule.
- When the reason for a value isn't known, name the value and leave the reason out, or point to the issue that will settle it.

### Rationale

Presence checks can't tell a useful comment from one that repeats the name, and agent-written code tends to satisfy them with exactly that.
A file header lets a reader decide whether to open the file, and an export comment tells a caller what they get without reading the body.
A guessed rationale is worse than none, because the next reader or agent treats it as a requirement.

### Examples

#### Application: File role and exported contract

**Incorrect (counterexample):**

```ts
/** @fileoverview Transactions. */
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

Both comments pass ESLint, but they repeat names and hide what the function accepts and how it fails.

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

Verify these details against `splitRows` and `parseRow`; don't infer them from the function's name.

#### Application: A hidden constraint

**Incorrect (counterexample):**

```ts
// Wait 250 milliseconds.
const VENDOR_MIN_CALL_INTERVAL_MS = 250;
```

**Correct:**

```ts
// The vendor sandbox returns HTTP 429 for calls closer together than this.
const VENDOR_MIN_CALL_INTERVAL_MS = 250;
```

### Validation

Run `bun run lint` for presence.
Then check that each header states the file's role, each export's comment states its observable contract, and each body comment explains a constraint rather than narrating.

Not a violation: a private helper without a comment whose name and signature make its behavior clear.
