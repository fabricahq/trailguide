---
title: "Distinguish null from undefined"
whenToRead: "Before writing, changing, or reviewing TypeScript types, function results, request payloads, or database updates where a value can be absent or empty."
impact: "LOW"
impactDescription: "Using null and undefined interchangeably blurs whether a value is deliberately empty or simply not provided, which matters for payloads and database updates."
tags: "typescript, null, undefined, conventions"
attribution:
  - url: https://github.com/mkosir/typescript-style-guide/blob/86bebd58a987e23277dba02028c0ee2d6ffb5073/website/src/pages/index.mdx
    description: "Adapted from mkosir TypeScript Style Guide guidance (distinguish-null-from-undefined; MIT, notice retained in NOTICE.md): restructured to the rule template with examples and the payload and database distinction."
---

## Distinguish null from undefined

Use `null` for a value that is deliberately empty, and `undefined`, or an omitted property, for a value that was not provided.

### Implementation

- Return `null` from a function that looked for something and found nothing, such as `findUser`.
- Use `undefined` or omit the property for optional inputs and fields not set.
- In update payloads and database clients that distinguish the two, such as Prisma, send `null` to clear a field and omit it, or send `undefined`, to leave it unchanged.
- Type fields to say which one applies, such as `middleName: string | null` for a known absence or `nickname?: string` for an optional input.

This is a convention; many projects choose it, and consistency within a codebase matters more than the exact split.

### Rationale

Some APIs and database clients treat the two differently: `null` writes an empty value, while an omitted field keeps the current one.
Mixing them causes updates that erase data or silently do nothing.

### Examples

**Incorrect (counterexample):**

```ts
await updateUser({ id, nickname: form.nickname || null });
```

A user who left the nickname field untouched has their existing nickname erased.

**Correct:**

```ts
await updateUser({ id, nickname: form.nicknameChanged ? form.nickname || null : undefined });
```

The payload clears the nickname only when the user emptied it.

### Validation

Review payload builders and update calls for fields set to `null` when the intent is "leave unchanged".

A codebase that consistently uses only `undefined` and handles clearing another way is not a violation.
