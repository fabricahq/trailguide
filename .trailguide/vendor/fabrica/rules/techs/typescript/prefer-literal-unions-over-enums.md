---
title: "Prefer literal unions over enums"
whenToRead: "Before writing, changing, or reviewing a TypeScript type for a fixed set of values, such as roles, statuses, or modes, or code that declares an enum."
impact: "MEDIUM"
impactDescription: "Enums generate runtime code, behave differently from the rest of TypeScript's structural types, and do not work with type-stripping tools."
tags: "typescript, enums, literal-unions, as-const"
attribution:
  - url: https://github.com/mkosir/typescript-style-guide/blob/86bebd58a987e23277dba02028c0ee2d6ffb5073/website/src/pages/index.mdx
    description: "Adapted from mkosir TypeScript Style Guide guidance (prefer-literal-unions-over-enums; MIT, notice retained in NOTICE.md): restructured to the rule template, fixed an example that did not parse, and added the type-stripping rationale."
---

## Prefer literal unions over enums

Represent a fixed set of values with a union of string literals.
When code also needs the values at runtime, derive the union from an `as const` array or object.

### Implementation

- Use `type Role = 'guest' | 'moderator' | 'admin'` when only the type is needed.
- Use `const ROLES = ['guest', 'moderator', 'admin'] as const` and `type Role = (typeof ROLES)[number]` when code iterates over the values.
- Use an `as const` object when names map to different values, such as color names to hex codes.
- Enable the `erasableSyntaxOnly` compiler option, or ban `TSEnumDeclaration` with `no-restricted-syntax`, to prevent new enums.

### Rationale

An enum compiles to a runtime object and is nominal: a plain string `'admin'` is not assignable to `Role.Admin`.
Enums are also not erasable syntax, so tools that run TypeScript by stripping types, such as Node's built-in type stripping, reject them; TypeScript 5.8 added `erasableSyntaxOnly` to flag them.
Literal unions have no runtime cost and accept plain values that match.

### Examples

**Incorrect (counterexample):**

```ts
enum UserRole {
  Guest = 'guest',
  Admin = 'admin',
}

setRole('admin'); // error: string is not assignable to UserRole
```

**Correct:**

```ts
const USER_ROLES = ['guest', 'admin'] as const;
type UserRole = (typeof USER_ROLES)[number];

setRole('admin');

for (const role of USER_ROLES) {
  seedRole(role);
}
```

### Validation

Run the type checker with `erasableSyntaxOnly`, or the lint rule, and check that no enums remain in authored code.

Enums in generated code or third-party types are not a violation.
