---
title: "Use template literal types for patterned strings"
whenToRead: "Before writing, changing, or reviewing TypeScript types for strings that follow a pattern, such as API paths, translation keys, CSS class or color tokens, or event names."
impact: "LOW-MEDIUM"
impactDescription: "Typing patterned strings as string lets typos and invalid combinations compile and fail at runtime."
tags: "typescript, template-literal-types, strings"
attribution:
  - url: https://github.com/mkosir/typescript-style-guide/blob/86bebd58a987e23277dba02028c0ee2d6ffb5073/website/src/pages/index.mdx
    description: "Adapted from mkosir TypeScript Style Guide guidance (use-template-literal-types-for-patterned-strings; MIT, notice retained in NOTICE.md): restructured to the rule template, trimmed the examples, and noted that template literal types check shape only."
---

## Use template literal types for patterned strings

When a string must follow a known pattern built from a fixed set of parts, type it with a template literal type instead of `string`.

### Implementation

- Build the type from existing unions, such as ``type ApiEndpoint = `/api/${ApiRoute}` ``.
- Use it for API paths, translation keys, design tokens, and event names that code constructs or accepts.
- Template literal types check the string's shape, not whether the value is otherwise valid; `${number}` accepts any numeric text.
  Validate strings from outside the program at runtime.
- Avoid deeply computed template types, such as modeling SQL queries; they slow the compiler and are hard to read.

### Rationale

A `string` accepts any typo, such as `'/api/usersss'`, and the error appears only when the request fails.
A template literal type turns the typo into a compile error and gives editor completion for the valid values.

### Examples

**Incorrect (counterexample):**

```ts
const userEndpoint: string = '/api/usersss';
```

**Correct:**

```ts
type ApiRoute = 'users' | 'posts' | 'comments';
type ApiEndpoint = `/api/${ApiRoute}`;

const userEndpoint: ApiEndpoint = '/api/users';
```

`'/api/usersss'` no longer compiles.

### Validation

Review string parameters and fields that must follow a pattern, and check whether a template literal type can express it.

A free-form string, such as a user's display name, should stay `string`.
