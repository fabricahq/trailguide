---
title: "Declare constants with as const, and satisfies when a type exists"
whenToRead: "Before writing, changing, or reviewing TypeScript constants, such as configuration objects, lists of allowed values, or lookup tables."
impact: "MEDIUM"
impactDescription: "Constants declared without as const widen to general types and stay mutable, and constants without satisfies can drift from the type they must match."
tags: "typescript, as-const, satisfies, constants"
attribution:
  - url: https://github.com/mkosir/typescript-style-guide/blob/86bebd58a987e23277dba02028c0ee2d6ffb5073/website/src/pages/index.mdx
    description: "Adapted from mkosir TypeScript Style Guide guidance (use-const-assertions-for-constants, use-as-const-satisfies-for-typed-constants; MIT, notice retained in NOTICE.md): merged the as const and as const satisfies rules, restructured to the rule template, and corrected the inferred types shown in the examples."
---

## Declare constants with as const, and satisfies when a type exists

Declare constant objects and arrays with `as const`, so their values keep literal types and become readonly.
When the constant must match an existing type, add `satisfies` to check it without widening.

### Implementation

- Use `as const` on objects and arrays that never change, such as allowed roles or default settings.
- Use `as const satisfies Type` when a type describes what the constant must contain, such as a union of allowed roles or a generated API type.
- Prefer `satisfies` over a type annotation for constants; an annotation widens the value to the annotated type and loses the literal types.
- Derive types from the constant when the constant is the source of truth, such as `type Role = (typeof ROLES)[number]`.

### Rationale

Without `as const`, `['admin', 'editor']` is `string[]`, so the compiler cannot tell which values are allowed, and code can push to it.
An annotation such as `: ReadonlyArray<UserRole>` checks the values but widens the type to the whole union.
`as const satisfies` keeps the exact values, makes them readonly, and still checks them against the type.

### Examples

These examples use the following types:

```ts
type UserRole = 'admin' | 'editor' | 'viewer';
type OrderStatus = { pending: 'pending' | 'idle'; fulfilled: boolean };
```

**Incorrect (counterexample):**

```ts
const DASHBOARD_ROLES = ['admin', 'editor'];
const DEFAULT_ORDER: OrderStatus = { pending: 'idle', fulfilled: true };
```

`DASHBOARD_ROLES` is `string[]` and mutable, and `DEFAULT_ORDER.pending` is widened to `'pending' | 'idle'`.

**Correct:**

```ts
const DASHBOARD_ROLES = ['admin', 'editor'] as const satisfies ReadonlyArray<UserRole>;
// readonly ['admin', 'editor']

const DEFAULT_ORDER = { pending: 'idle', fulfilled: true } as const satisfies OrderStatus;
// { readonly pending: 'idle'; readonly fulfilled: true }
```

A typo such as `'editr'` fails the `satisfies` check.

### Validation

Hover over constants in the editor and check that they have literal, readonly types.
Check that constants meant to match a type use `satisfies` rather than an annotation.

A mutable value that is deliberately changed at runtime is not a constant and needs no `as const`.
