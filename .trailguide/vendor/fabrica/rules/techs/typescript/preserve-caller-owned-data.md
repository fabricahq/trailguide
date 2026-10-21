---
title: "Preserve caller-owned data"
whenToRead: "Before planning, writing, changing, or reviewing TypeScript functions that transform arrays, objects, maps, or sets received from a caller or from shared state, such as sorting, filtering, or updating fields."
impact: "MEDIUM"
impactDescription: "Prevents hidden mutation from changing another caller's behavior."
tags: "typescript"
attribution: [{"url":"https://github.com/mkosir/typescript-style-guide/blob/86bebd58a987e23277dba02028c0ee2d6ffb5073/website/src/pages/index.mdx","description":"Underlying TypeScript Style Guide material by mkosir, adapted under MIT; copyright and permission notice retained in NOTICE.md."}]
---

## Preserve caller-owned data

When transforming data owned by a caller or shared state, return a new value instead of mutating the input.
Declare such inputs as readonly so that the compiler rejects accidental mutation.

### Implementation

- Type array inputs as `ReadonlyArray<T>` and object inputs with `Readonly<T>` or `readonly` properties when the function should not change them.
- Watch for methods that mutate in place, such as `sort`, `reverse`, `splice`, `push`, `fill`, and `Object.assign` with the input as its target.
  Copy first, or use a non-mutating alternative such as `toSorted`, `toReversed`, `toSpliced`, or `with`.
  Those alternatives need an ES2023 runtime and a TypeScript `lib` setting that includes ES2023.
- Readonly types and shallow copies do not make nested objects immutable.
  When changing nested data, copy each level along the path to the change, or follow the project's ownership convention.

Mutation is appropriate for data the function newly created and owns, such as a local array it builds and returns.
It is also appropriate in an explicitly mutating API whose name and contract tell callers the input changes, such as `sortInPlace`.

### Rationale

A caller that passes an array or object usually keeps using it.
If the callee mutates it, the change appears in unrelated code that holds the same reference, such as a cached value or rendered state, and the defect shows up far from the function that caused it.
Readonly input types move that mistake to compile time, and returning new values makes each function's effect visible in its return type.

### Examples

#### Application: Removing an item

**Incorrect (counterexample):**

```ts
function withoutFirst(values: Array<string>): Array<string> {
  values.splice(0, 1);
  return values;
}
```

Removing the item also removes it from the caller's array.

**Correct:**

```ts
function withoutFirst(values: ReadonlyArray<string>): Array<string> {
  return values.slice(1);
}
```

The input is readonly and the result is a separate array.

#### Application: Sorting

**Incorrect (counterexample):**

```ts
function byName(users: Array<User>): Array<User> {
  return users.sort((a, b) => a.name.localeCompare(b.name));
}
```

`sort` reorders the caller's array in place and returns the same reference.

**Correct:**

```ts
function byName(users: ReadonlyArray<User>): Array<User> {
  return [...users].sort((a, b) => a.name.localeCompare(b.name));
}
```

With an ES2023 runtime and `lib` setting, `users.toSorted(...)` is equivalent.

#### Application: Updating nested data

**Incorrect (counterexample):**

```ts
function renameCity(user: Readonly<User>, city: string): User {
  const copy = { ...user };
  copy.address.city = city;
  return copy;
}
```

The spread copies only the top level, so `copy.address` is the caller's object and its city changes too.

**Correct:**

```ts
function renameCity(user: Readonly<User>, city: string): User {
  return { ...user, address: { ...user.address, city } };
}
```

Each level along the path to the change is copied.

### Validation

Test that the original input remains unchanged and that the returned value contains the intended transformation.
When a change reaches nested data, check that nested references in the input are unchanged.

Do not demand a deep clone when no nested data changes.
Mutating data the function created, or mutating through an API whose contract states that it mutates, is not a violation.
