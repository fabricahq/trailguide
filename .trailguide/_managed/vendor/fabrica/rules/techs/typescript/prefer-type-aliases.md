---
title: "Prefer type aliases over interfaces"
whenToRead: "Before declaring or reviewing TypeScript object types, or when choosing between type and interface."
impact: "LOW"
impactDescription: "Mixing type and interface without a rule makes declarations inconsistent, and only type can express unions and other computed types."
tags: "typescript, type-aliases, interfaces, conventions"
attribution:
  - url: https://github.com/mkosir/typescript-style-guide/blob/86bebd58a987e23277dba02028c0ee2d6ffb5073/website/src/pages/index.mdx
    description: "Adapted from mkosir TypeScript Style Guide guidance (prefer-type-aliases; MIT, notice retained in NOTICE.md): restructured to the rule template and replaced an example that did not parse."
---

## Prefer type aliases over interfaces

Declare types with `type` by default.
Use `interface` only when you need declaration merging, such as extending a third-party library's or a global type.

### Implementation

- Use `type` for object shapes, unions, and computed types.
- Use `interface` to augment existing declarations, such as `NodeJS.ProcessEnv` or a library's module types, and disable the lint rule on that line.
- A library meant to be extended by consumers may prefer interfaces; choose deliberately.
- Enforce with `@typescript-eslint/consistent-type-definitions` set to `'type'`.

This is a consistency convention; the two forms are interchangeable for most object types.

### Rationale

`type` can express everything `interface` can for object shapes, plus unions, mapped types, and conditional types, so one form covers every case.
Using one form consistently removes a decision from every declaration.

### Examples

**Incorrect (counterexample):**

```ts
interface UserInfo {
  name: string;
  role: 'admin' | 'guest';
}
type UserRole = UserInfo['role'];
```

**Correct:**

```ts
type UserRole = 'admin' | 'guest';

type UserInfo = {
  name: string;
  role: UserRole;
};
```

### Validation

Run the lint rule, and check that remaining interfaces exist for declaration merging.

An `interface` that augments a global or library type is not a violation.
