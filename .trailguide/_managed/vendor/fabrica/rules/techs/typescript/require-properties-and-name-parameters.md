---
title: "Require caller-owned inputs and document defaults"
whenToRead: "Before planning, writing, changing, or reviewing TypeScript object types or function signatures, especially ones with many optional properties or several positional parameters."
impact: "MEDIUM-HIGH"
impactDescription: "Optional-heavy types and long positional parameter lists hide which inputs are needed and let calls pass the wrong value in the wrong place."
tags: "typescript, function-signatures, optional-properties, options-object"
attribution:
  - url: https://github.com/mkosir/typescript-style-guide/blob/86bebd58a987e23277dba02028c0ee2d6ffb5073/website/src/pages/index.mdx
    description: "Adapted from mkosir TypeScript Style Guide guidance (prefer-required-object-properties, keep-function-args-mostly-required, prefer-single-object-function-args; MIT, notice retained in NOTICE.md): merged the required-properties, required-arguments, and single-object-argument rules, restructured to the rule template, and pointed variant modeling to the discriminated union rule."
---

## Require caller-owned inputs and document defaults

Require inputs that the caller must supply; make absence explicit when it has a meaning, including selecting a documented default.
Keep implementation choices inside the module when it can determine them itself.
When a function takes several parameters, especially booleans or values of the same type, take one options object so each value is named at the call site.

### Implementation

- Before adding an input, identify the information or policy the caller owns.
  Compute implementation choices inside the module when it has the information it needs.
- Mark a property or parameter optional only when leaving it out is a valid, meaningful case, such as an optional filter or a documented default.
  Keep caller-specific requirements explicit, such as which account to use or whether the user consented; a default must not guess them.
- When optional fields exist because an object represents several states, model the states as a discriminated union instead.
- Split a function only when its options select different operations or allow combinations that can't be valid together.
  Keep cohesive variations of one operation together, such as a search's filters, pagination, and sort order.
- Use a single options object when a function takes more than two or three parameters, or any boolean or same-typed parameters that are easy to swap.
- Keep simple positional parameters for one or two obviously distinct values, such as `isNumber(value)` or `formatAmount(amount, currency)`.

### Rationale

Required inputs let TypeScript flag missing caller-owned information.
Documented defaults let ordinary callers omit choices the module can make, while explicit optionality distinguishes valid absence from forgotten data.
Positional parameters depend on order; swapping a reminder's user ID and message can compile because both are strings.
Named inputs make that mistake easier to see at the call site.

### Examples

#### Application: Positional parameters

**Incorrect (counterexample):**

```ts
scheduleReminder(userId, remindAt, message);
```

**Correct:**

```ts
scheduleReminder({ userId, remindAt, message });
```

The user ID and message are both strings; named inputs make their roles visible at the call site.
All three values describe the reminder the caller wants.

#### Application: Defaults for ordinary use

The document search API can choose a default page size; callers may override it for a different display.
Workspace and query identify what to search and must come from the caller.

**Incorrect (counterexample):**

```ts
searchDocuments({ workspaceId, query, pageSize: 50, cursor: undefined });
```

Every first-page caller has to repeat the same page size and explicitly pass an absent cursor because the API requires both properties.
Naming these values makes the call readable but does not justify making callers choose them.

**Correct:**

```ts
searchDocuments({ workspaceId, query });
searchDocuments({ workspaceId, query, pageSize: 20, cursor: nextCursor });
```

The contract documents a default page size of 50 and that an omitted cursor starts at the first page.
Those numbers are illustrative; the module chooses defaults appropriate to its contract and validates explicit overrides.

#### Application: Optional properties

**Incorrect (counterexample):**

```ts
type User = {
  id?: number;
  email?: string;
  adminPermissions?: ReadonlyArray<string>;
  temporaryToken?: string;
};
```

Every consumer must guess which combination of fields a given user has.

**Correct:**

```ts
type User =
  | { role: 'admin'; id: number; email: string; adminPermissions: ReadonlyArray<string> }
  | { role: 'guest'; temporaryToken: string };
```

### Validation

Review required inputs for information or policy the caller owns, and optional inputs for a meaningful omission or documented default.
Walk a common call and check that it does not repeat implementation choices the module could determine.
Review calls with several positional arguments and check that each value's meaning is obvious without reading the function.

An optional property whose absence is meaningful, a cohesive set of options for one operation, or a short positional list of distinct values is not a violation.
