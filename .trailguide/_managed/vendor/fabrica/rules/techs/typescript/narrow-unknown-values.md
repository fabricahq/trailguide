---
title: "Narrow unknown values before use"
whenToRead: "Before planning, writing, changing, or reviewing TypeScript code that receives data whose shape the compiler cannot guarantee, such as parsed JSON, network responses, storage reads, message events, or caught errors."
impact: "HIGH"
impactDescription: "Prevents unchecked data from bypassing TypeScript and failing later at runtime."
tags: "typescript"
attribution: [{"url":"https://github.com/mkosir/typescript-style-guide/blob/86bebd58a987e23277dba02028c0ee2d6ffb5073/website/src/pages/index.mdx","description":"Underlying TypeScript Style Guide material by mkosir, adapted under MIT; copyright and permission notice retained in NOTICE.md."}]
---

## Narrow unknown values before use

Type data whose shape has not been established as `unknown`, then validate it at runtime before reading its properties or assigning it to a narrower type.

### Implementation

- Assign values from untyped sources to `unknown` immediately.
  Several standard APIs return `any`, such as `JSON.parse` and the `json()` method of a fetch `Response`, so their results need an explicit `unknown` annotation.
- Establish the type with runtime checks, a type-guard function, or a schema validator that owns the boundary.
  A type assertion such as `as Order` changes only what the compiler believes; it does not check the value.
- Validate domain constraints as well as types.
  A number can still be invalid as a count, and a string can still be invalid as an identifier.
- Treat caught errors as `unknown`, which is the default under the `strict` compiler option since TypeScript 4.4, and check them before reading properties such as `message`.
- Avoid `any` at these boundaries, because it disables the checks that would require validation.
  When an external library forces `any`, contain it in an adapter that returns validated types.

### Rationale

TypeScript types are erased at runtime, so a type annotation on external data is only a claim.
If the claim is wrong, the program fails later and farther from the source, such as when a missing field is read deep in rendering or business logic.
Validating once at the boundary turns an unexpected shape into an immediate, specific error, and lets the rest of the code rely on the types it sees.

### Examples

#### Application: A single value from parsed data

**Incorrect (counterexample):**

```ts
const value: unknown = JSON.parse(responseText);
const count = value as number;
```

The cast changes the compiler's belief without validating the value, so a string or `null` flows on as a "number".

**Correct:**

```ts
const value: unknown = JSON.parse(responseText);
if (typeof value !== 'number' || !Number.isInteger(value) || value < 0) {
  throw new Error('Expected a non-negative integer count');
}
const count = value;
```

The runtime check establishes both the type and the domain constraint before use.

#### Application: An object from a network response

**Incorrect (counterexample):**

```ts
const order = (await response.json()) as { id: string; total: number };
renderTotal(order.total);
```

If the server omits `total` or sends it as a string, the error appears inside `renderTotal`, far from the response.

**Correct:**

```ts
function isOrder(value: unknown): value is { id: string; total: number } {
  return (
    typeof value === 'object' &&
    value !== null &&
    'id' in value &&
    typeof value.id === 'string' &&
    'total' in value &&
    typeof value.total === 'number'
  );
}

const body: unknown = await response.json();
if (!isOrder(body)) {
  throw new Error('Unexpected order response');
}
renderTotal(body.total);
```

The type guard checks each field the code relies on.
A schema validator that the project already uses at this boundary is an equally good choice.

#### Application: A caught error

**Incorrect (counterexample):**

```ts
try {
  await save();
} catch (error: any) {
  showMessage(error.message);
}
```

A thrown string or object without `message` produces `undefined` in the user interface.

**Correct:**

```ts
try {
  await save();
} catch (error: unknown) {
  showMessage(error instanceof Error ? error.message : String(error));
}
```

### Validation

Inspect each data entry point and check that property access and narrow typing follow a runtime check.
Look for tests that pass malformed input and assert the boundary's error.

Replacing `any` with an unchecked type assertion does not satisfy this rule.
A type assertion inside a type-guard function or schema validator, after the checks that justify it, is not a violation.
