---
title: "Generate service types from their contracts"
whenToRead: "Before planning, writing, changing, or reviewing TypeScript types for external APIs, message formats, or database schemas, such as REST, GraphQL, or queue payloads."
impact: "HIGH"
impactDescription: "Hand-written types for external services drift from the real contract, so the compiler approves code that fails against the actual service."
tags: "typescript, codegen, openapi, graphql, contracts"
attribution:
  - url: https://github.com/mkosir/typescript-style-guide/blob/86bebd58a987e23277dba02028c0ee2d6ffb5073/website/src/pages/index.mdx
    description: "Adapted from mkosir TypeScript Style Guide guidance (generate-service-types-from-contracts; MIT, notice retained in NOTICE.md): restructured to the rule template with an example and runtime-validation note."
---

## Generate service types from their contracts

Generate TypeScript types from each external service's machine-readable contract, such as an OpenAPI document, GraphQL schema, or database schema, instead of writing them by hand.

### Implementation

- Use a generator for the contract format, such as `openapi-typescript` for OpenAPI, GraphQL Code Generator for GraphQL, or the database client's type generation.
- Regenerate as part of the build or a checked-in script, and fail CI when the generated types are out of date.
- Do not edit generated files; change the contract or the generator configuration.
- Write types by hand only when no contract is available, and note where the shape came from.
- Generated types describe what the service promises; still validate untrusted responses at runtime where a wrong shape would cause damage.

### Rationale

A hand-written type is a copy of the contract made once and updated by memory.
When the service changes, the copy stays the same, and TypeScript approves code that fails at runtime.
Generated types change with the contract, so a breaking service change becomes a compile error.

### Examples

**Incorrect (counterexample):**

```ts
// Written by hand from the API docs last year.
type Order = { id: string; total: number };
```

The API now returns `total` as an object with `amount` and `currency`, and nothing flags the code that treats it as a number.

**Correct:**

```ts
import type { components } from './generated/api-types';

type Order = components['schemas']['Order'];
```

The type is regenerated from the OpenAPI document, so the change to `total` fails to compile wherever it matters.

### Validation

Check that types for external services come from generated files, and that CI checks the generated files are current.

A hand-written type for a service with no available contract, with a note about its source, is not a violation.
