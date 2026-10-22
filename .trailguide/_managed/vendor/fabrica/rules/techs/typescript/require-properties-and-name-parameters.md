---
title: "Make properties and parameters required, and name them"
whenToRead: "Before planning, writing, changing, or reviewing TypeScript object types or function signatures, especially ones with many optional properties or several positional parameters."
impact: "MEDIUM-HIGH"
impactDescription: "Optional-heavy types and long positional parameter lists hide which inputs are needed and let calls pass the wrong value in the wrong place."
tags: "typescript, function-signatures, optional-properties, options-object"
attribution:
  - url: https://github.com/mkosir/typescript-style-guide/blob/86bebd58a987e23277dba02028c0ee2d6ffb5073/website/src/pages/index.mdx
    description: "Adapted from mkosir TypeScript Style Guide guidance (prefer-required-object-properties, keep-function-args-mostly-required, prefer-single-object-function-args; MIT, notice retained in NOTICE.md): merged the required-properties, required-arguments, and single-object-argument rules, restructured to the rule template, and pointed variant modeling to the discriminated union rule."
---

## Make properties and parameters required, and name them

Make object properties and function parameters required unless their absence has a meaning.
When a function takes several parameters, especially booleans or values of the same type, take one options object so each value is named at the call site.

### Implementation

- Mark a property or parameter optional only when leaving it out is a valid, meaningful case, such as an optional filter.
- When optional fields exist because an object represents several states, model the states as a discriminated union instead.
- When a function accumulates optional parameters for different use cases, split it into separate functions.
- Use a single options object when a function takes more than two or three parameters, or any boolean or same-typed parameters that are easy to swap.
- Keep simple positional parameters for one or two obviously distinct values, such as `isNumber(value)` or `clamp(value, min, max)`.

### Rationale

Each optional field is a question every reader and caller must answer, and TypeScript cannot flag a caller that forgets a field that should have been required.
Positional parameters depend on order; `transform('client', false, 60, 120, null, true, 2000)` compiles even when two numbers are swapped, and the call site does not say what any value means.

### Examples

#### Application: Positional parameters

**Incorrect (counterexample):**

```ts
transformUserInput('client', false, 60, 120, null, true, 2000);
```

**Correct:**

```ts
transformUserInput({
  method: 'client',
  isValidated: false,
  minLines: 60,
  maxLines: 120,
  defaultInput: null,
  shouldLog: true,
  timeoutMs: 2000,
});
```

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

Review each optional property or parameter and check that omitting it is a meaningful case.
Review calls with several positional arguments and check that each value's meaning is obvious without reading the function.

An optional property whose absence is meaningful, or a short positional list of distinct values, is not a violation.
