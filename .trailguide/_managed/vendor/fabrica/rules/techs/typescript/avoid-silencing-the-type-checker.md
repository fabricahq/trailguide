---
title: "Avoid silencing the type checker"
whenToRead: "Before writing, changing, or reviewing TypeScript code that uses type assertions such as as, non-null assertions such as !, or @ts-ignore and @ts-expect-error comments."
impact: "HIGH"
impactDescription: "Assertions and suppression comments tell the compiler to trust a claim it cannot check, so wrong claims become runtime crashes."
tags: "typescript, type-assertions, non-null, ts-expect-error"
attribution:
  - url: https://github.com/mkosir/typescript-style-guide/blob/86bebd58a987e23277dba02028c0ee2d6ffb5073/website/src/pages/index.mdx
    description: "Adapted from mkosir TypeScript Style Guide guidance (avoid-type-and-non-null-assertions, use-ts-expect-error-with-description; MIT, notice retained in NOTICE.md): merged the assertion and ts-expect-error rules into one rule about silencing the type checker, restructured to the rule template, and added the narrowing alternatives."
---

## Avoid silencing the type checker

Prove types to the compiler with checks instead of asserting them.
Use a type assertion, a non-null assertion, or `@ts-expect-error` only as a documented last resort, and never use `@ts-ignore`.

### Implementation

- Replace `value!` with a check that handles the missing case, such as an early return, a thrown error with context, or a default value.
- Replace `value as Type` on data from outside with runtime validation, such as a type guard or schema.
- Where an assertion is truly needed, such as a third-party type that is wrong, keep it in one small adapter and add a comment explaining why.
- Use `@ts-expect-error` with a description of the reason, not `@ts-ignore`; `@ts-expect-error` fails the build when the error it suppresses goes away.
- Enforce with `@typescript-eslint/no-non-null-assertion`, `@typescript-eslint/consistent-type-assertions`, and `@typescript-eslint/ban-ts-comment` with `'ts-expect-error': 'allow-with-description'`.
- `as const` and `satisfies` are not assertions in this sense; they narrow or check types without overriding them.

### Rationale

TypeScript's guarantees hold only where the compiler can check them.
An assertion replaces a check with a claim, and if the claim is wrong the error appears at runtime, often far from the assertion.
`@ts-ignore` hides every error on the next line, including new ones introduced later.

### Examples

#### Application: A value that might be missing

**Incorrect (counterexample):**

```ts
const user = users.find((candidate) => candidate.id === id)!;
renderAvatar(user.avatar);
```

When no user matches, this crashes inside `renderAvatar` with an unhelpful error.

**Correct:**

```ts
const user = users.find((candidate) => candidate.id === id);
if (!user) {
  throw new Error(`User ${id} not found`);
}
renderAvatar(user.avatar);
```

#### Application: Suppressing a compiler error

**Incorrect (counterexample):**

```ts
// @ts-ignore
const newUser = createUser('Gabriel');
```

**Correct:**

```ts
// @ts-expect-error: the library types require an object, but createUser also accepts a name (issue #123).
const newUser = createUser('Gabriel');
```

The comment explains the suppression, and the build fails once the library's types are fixed.

### Validation

Run the lint rules above, and review each remaining assertion or `@ts-expect-error` for a comment that explains why no check is possible.

A documented assertion inside a type guard or adapter, after the checks that justify it, is not a violation.
