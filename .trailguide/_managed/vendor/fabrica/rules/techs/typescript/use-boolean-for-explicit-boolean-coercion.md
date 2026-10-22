---
title: "Use Boolean() for explicit boolean coercion"
whenToRead: "Before writing, changing, or reviewing TypeScript code that converts a value to a boolean, such as !!value, or tests a value's truthiness."
impact: "LOW"
impactDescription: "Double negation is easy to misread, and truthiness checks hide which absent or empty values a condition excludes."
tags: "typescript, booleans, readability, conventions"
---

## Use Boolean() for explicit boolean coercion

When code intentionally converts a value to a boolean from its truthiness, write `Boolean(value)` instead of `!!value`.
When the condition has a specific meaning, such as "present" or "non-empty", write that comparison instead of coercing.

### Implementation

- Replace `!!value` with `Boolean(value)`.
- Compare against the specific absent value, such as `value !== undefined`, when you need TypeScript to narrow the variable afterward; `Boolean(value)` in a separate variable does not narrow it.
- Keep domain comparisons, such as `count > 0`, `name.trim() !== ''`, or `status === 'ready'`; they say which condition matters.

This is a readability convention.

### Rationale

`Boolean(value)` names the intent and is easy to search for, while `!!` is a symbol pair that readers can overlook.
Truthiness also treats `0`, `''`, and `NaN` as false, which is often not what a condition means; an explicit comparison makes the excluded cases visible.

### Examples

**Incorrect (counterexample):**

```ts
const hasAccount = !!accountContext;
```

**Correct:**

```ts
const hasAccount = Boolean(accountContext);
const canView = accountContext !== undefined && sharedFile.allowedAccountIds.includes(accountContext.id);
```

### Validation

Search for `!!` and replace intentional coercions with `Boolean()` or a specific comparison.

A specific comparison, such as `count > 0`, is not a violation.
