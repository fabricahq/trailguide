---
title: "Import types with import type"
whenToRead: "Before writing, changing, or reviewing TypeScript imports of types, interfaces, or other declarations used only in type positions."
impact: "MEDIUM"
impactDescription: "Under per-file transpilers and verbatimModuleSyntax, a type imported as a value is kept in the emitted JavaScript and can fail at runtime or load a module for no reason."
tags: "typescript, imports, verbatimModuleSyntax, isolatedModules"
attribution:
  - url: https://github.com/mkosir/typescript-style-guide/blob/86bebd58a987e23277dba02028c0ee2d6ffb5073/website/src/pages/index.mdx
    description: "Adapted from mkosir TypeScript Style Guide guidance (separate-type-imports; MIT, notice retained in NOTICE.md): restructured to the rule template and corrected the rationale from bundle size to emit correctness under verbatimModuleSyntax and per-file transpilers."
---

## Import types with import type

Import anything used only as a type with `import type`, or with an inline `type` modifier, so the import is removed from the emitted JavaScript.

### Implementation

- Write `import type { User } from './user'` for type-only imports.
- Write `import { createUser, type User } from './user'` when a module provides both values and types.
- Enable `verbatimModuleSyntax`, which requires type-only imports to be marked, and `@typescript-eslint/consistent-type-imports` to fix them automatically.

### Rationale

Tools that transpile one file at a time, such as esbuild, SWC, and Babel, cannot always tell whether an imported name is a type, so they may keep the import.
With `verbatimModuleSyntax`, TypeScript itself keeps every import not marked as a type.
A kept import of a type-only export fails at runtime or loads the module, with its side effects, for nothing.
Marking type imports makes the emitted code predictable and shows at a glance which imports are needed at runtime.

### Examples

**Incorrect (counterexample):**

```ts
import { User } from './user';

export function greet(user: User): string {
  return `Hello, ${user.name}`;
}
```

If `./user` exports `User` only as a type, a per-file transpiler may emit an import of a name that does not exist at runtime.

**Correct:**

```ts
import type { User } from './user';

export function greet(user: User): string {
  return `Hello, ${user.name}`;
}
```

### Validation

Enable `verbatimModuleSyntax` or run the lint rule, and check that the type checker reports no unmarked type-only imports.

An import used both as a value and a type needs no `type` marker.
