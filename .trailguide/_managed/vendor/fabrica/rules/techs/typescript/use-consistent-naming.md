---
title: "Follow consistent naming conventions"
whenToRead: "Before naming or reviewing names of TypeScript variables, constants, functions, types, generic parameters, React components, props, event handlers, or Hooks."
impact: "LOW"
impactDescription: "Inconsistent naming makes readers stop to decode each identifier and hides whether a name refers to a value, type, constant, or handler."
tags: "typescript, naming, conventions, react"
attribution:
  - url: https://github.com/mkosir/typescript-style-guide/blob/86bebd58a987e23277dba02028c0ee2d6ffb5073/website/src/pages/index.mdx
    description: "Adapted from mkosir TypeScript Style Guide guidance (use-consistent-naming; MIT, notice retained in NOTICE.md): restructured to the rule template, fixed examples that did not parse, and removed the requirement that custom Hooks return objects."
---

## Follow consistent naming conventions

Name identifiers by their kind, using one convention per kind across the codebase.

### Implementation

- **Variables and functions:** camelCase, such as `productsFiltered` and `formatCurrency`.
- **Booleans:** a question prefix, such as `isDisabled`, `hasProduct`, or `canEdit`.
- **Module-level constants:** UPPER_SNAKE_CASE, such as `FEATURED_PRODUCT_ID`, including `as const` objects and arrays.
- **Types:** PascalCase, such as `OrderStatus`.
- **Generic parameters:** `T` followed by a descriptive name, such as `TRequest`, when a function has more than one parameter or the meaning is not obvious; a single `T` is fine for a simple generic.
- **Acronyms:** treat them as words, such as `generateUserUrl` and `FaqItem`, and avoid abbreviations that are not widely known.
- **React:** PascalCase components; `ComponentNameProps` for prop types; `on*` for callback props and `handle*` for the functions that implement them; `[value, setValue]` for `useState`.
- Enforce what tooling can check with `@typescript-eslint/naming-convention`, `react/jsx-handler-names`, and `react/hook-use-state`.

This is a set of conventions; a project may choose different ones, as long as each kind has one.

### Rationale

Consistent names let readers tell at a glance what an identifier is: a value, a type, a constant, or an event handler.
Prefixed generic names also avoid shadowing real types: in `<Request extends Request>`, the parameter refers to itself and the declaration does not compile.

### Examples

**Incorrect (counterexample):**

```tsx
type FAQItem = { question: string };
const generateUserURL = (id: string) => `/users/${id}`;
const handle = <Request extends Request>(req: Request) => {};
const [userName, setUser] = useState('');
<Button click={actionClick} />;
```

**Correct:**

```tsx
type FaqItem = { question: string };
const generateUserUrl = (id: string) => `/users/${id}`;
const handle = <TRequest extends Request>(req: TRequest) => {};
const [userName, setUserName] = useState('');
<Button onClick={handleClick} />;
```

### Validation

Run the naming lint rules, and review new identifiers against the conventions above.

A name required by an external API or framework, such as a library's option key, is not a violation.
