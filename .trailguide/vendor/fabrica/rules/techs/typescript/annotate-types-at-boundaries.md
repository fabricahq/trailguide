---
title: "Annotate types at module boundaries and where they narrow"
whenToRead: "Before writing, changing, or reviewing TypeScript type annotations on exported functions, variables, state, or collections, or deciding whether to rely on inference."
impact: "MEDIUM"
impactDescription: "Missing annotations let exported contracts change by accident and let values widen to any or string, while redundant annotations add noise that can drift from the code."
tags: "typescript, type-annotations, inference, return-types"
attribution:
  - url: https://github.com/mkosir/typescript-style-guide/blob/86bebd58a987e23277dba02028c0ee2d6ffb5073/website/src/pages/index.mdx
    description: "Adapted from mkosir TypeScript Style Guide guidance (annotate-public-return-types, prefer-inference-unless-annotation-narrows; MIT, notice retained in NOTICE.md): merged the public-return-type and inference rules, restructured to the rule template, and corrected the lint recommendation to module-boundary types."
---

## Annotate types at module boundaries and where they narrow

Annotate the types of exported functions and values, and annotate anywhere inference would produce a wider type than you mean.
Let TypeScript infer everything else.

### Implementation

- Give exported functions explicit parameter and return types, so a change inside the function cannot silently change what callers receive.
- Annotate when inference widens: an empty `new Map()` becomes `Map<any, any>`, `useState('admin')` becomes `string` rather than a union, and `[]` needs its element type.
- Do not annotate what inference already gets exactly right, such as `const count = 0` or `useState(false)`.
- Enforce boundary annotations with `@typescript-eslint/explicit-module-boundary-types`, which checks exports only.
  `explicit-function-return-type` checks every function and is usually too strict for internal code.
- The `isolatedDeclarations` compiler option, which lets tools emit declaration files per file, requires these boundary annotations.

### Rationale

An exported function's return type is a contract with every caller.
When it is inferred, an internal change, such as returning `null` on a new path, changes the contract without any error at the function, and the failure appears at a distant call site.
Inside a module, inferred types follow the code automatically, so annotations there mostly add noise, except where inference picks a wider type than intended.

### Examples

#### Application: An exported function

**Incorrect (counterexample):**

```ts
export function findUser(users: ReadonlyArray<User>, id: string) {
  return users.find((user) => user.id === id) ?? null;
}
```

The return type is inferred as `User | null`, and a later edit that returns `undefined` changes it without an error here.

**Correct:**

```ts
export function findUser(users: ReadonlyArray<User>, id: string): User | null {
  return users.find((user) => user.id === id) ?? null;
}
```

#### Application: A value that inference widens

**Incorrect (counterexample):**

```ts
const ages = new Map();
const [role, setRole] = useState('admin');
```

`ages` is `Map<any, any>`, and `role` is `string`, so any string can be set.

**Correct:**

```ts
const ages = new Map<string, number>();
const [role, setRole] = useState<UserRole>('admin');
```

### Validation

Run the boundary-types lint rule and the type checker.
Review new annotations inside modules and remove those that repeat an exact inferred type.

An annotation that documents intent where inference is already exact is not a violation.
