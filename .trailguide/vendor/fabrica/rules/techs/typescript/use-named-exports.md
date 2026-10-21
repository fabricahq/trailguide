---
title: "Use named exports"
whenToRead: "Before writing, changing, or reviewing TypeScript module exports and imports."
impact: "LOW"
impactDescription: "Default exports let each importer choose a different name, so the same thing appears under several names and renames miss imports."
tags: "typescript, exports, modules, conventions"
attribution:
  - url: https://github.com/mkosir/typescript-style-guide/blob/86bebd58a987e23277dba02028c0ee2d6ffb5073/website/src/pages/index.mdx
    description: "Adapted from mkosir TypeScript Style Guide guidance (use-named-exports; MIT, notice retained in NOTICE.md): restructured to the rule template and listed the framework exceptions."
---

## Use named exports

Export values and types by name, and avoid default exports except where a framework requires them.

### Implementation

- Write `export function formatCurrency()` and import it as `import { formatCurrency } from './format-currency'`.
- Allow default exports only where a framework requires them, such as some file-based routing pages or configuration files, through a lint override for those paths.
- Enforce with `import/no-default-export`.

This is a consistency convention.

### Rationale

A named export has one name that every import must use, so searching finds every use, and a rename tool updates them all.
A default export can be imported under any name, which scatters different names for the same thing, and a missing default export is not always caught.

### Examples

**Incorrect (counterexample):**

```ts
export default function formatCurrency(amount: number): string {
  return amount.toFixed(2);
}

import formatMoney from './format-currency';
```

**Correct:**

```ts
export function formatCurrency(amount: number): string {
  return amount.toFixed(2);
}

import { formatCurrency } from './format-currency';
```

### Validation

Run the lint rule, and check that its overrides cover only paths where a framework requires default exports.

A default export required by a framework is not a violation.
