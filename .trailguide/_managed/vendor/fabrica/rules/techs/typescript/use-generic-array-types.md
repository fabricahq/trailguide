---
title: "Use one array type syntax"
whenToRead: "Before writing or reviewing TypeScript array type annotations."
impact: "LOW"
impactDescription: "Mixing Array<T> and T[] makes array types look different for no reason."
tags: "typescript, arrays, conventions"
attribution:
  - url: https://github.com/mkosir/typescript-style-guide/blob/86bebd58a987e23277dba02028c0ee2d6ffb5073/website/src/pages/index.mdx
    description: "Adapted from mkosir TypeScript Style Guide guidance (use-generic-array-types; MIT, notice retained in NOTICE.md): restructured to the rule template and framed as a consistency convention."
---

## Use one array type syntax

Write array types with the generic syntax, `Array<T>` and `ReadonlyArray<T>`, and enforce it with a lint rule.

### Implementation

- Use `Array<string>` and `ReadonlyArray<string>` rather than `string[]` and `readonly string[]`.
- Enforce with `@typescript-eslint/array-type` set to `{ default: 'generic' }`.

This is a consistency convention; the two syntaxes are equivalent, and a project may choose `T[]` instead as long as it uses one.
The generic form reads the same for mutable and readonly arrays and stays clear with complex element types, such as `Array<string | number>`.

### Examples

**Incorrect (counterexample):**

```ts
const names: string[] = [];
const ids: readonly (string | number)[] = [];
```

**Correct:**

```ts
const names: Array<string> = [];
const ids: ReadonlyArray<string | number> = [];
```

### Validation

Run the lint rule.

A project that consistently uses `T[]` instead is not a violation.
