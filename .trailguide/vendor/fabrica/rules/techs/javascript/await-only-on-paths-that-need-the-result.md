---
title: "Await only on paths that need the result"
whenToRead: "Before planning, writing, changing, or reviewing asynchronous functions with branches or early returns, such as handlers that check a flag, a cheap condition, or a cached value before doing more work."
impact: "MEDIUM"
impactDescription: "Awaiting a result before a branch that may not use it adds that operation's latency and cost to every path."
tags: "javascript, async, latency, branching"
attribution:
  - url: https://github.com/vercel-labs/agent-skills/tree/4ec6f84b61cd3c931046c3e6e398f3ae7de372f7/skills/react-best-practices/rules
    description: "Adapted from two Vercel Agent Skills rules (async-defer-await and async-cheap-condition-before-await): merged two rules on deferring await and checking cheap conditions first, restructured to the rule template, and added the rule that authorization checks keep their order."
---

## Await only on paths that need the result

Move an `await` into the branch that uses its result, and check cheap synchronous conditions before awaiting a remote value that is combined with them.

### Implementation

- When some branches return without using an awaited value, move the `await` below those returns.
- For a condition such as `flag && cheapCondition`, evaluate the cheap synchronous condition first and await the flag only when it is true.
- Keep the original order when the awaited check guards what follows.
  In particular, check authorization before revealing whether a resource exists, so callers without access cannot tell "not found" from "forbidden".
- Keep the original order when the operation has a side effect that must happen on every path, or when the cheap condition depends on the awaited value.

### Rationale

An `await` pauses the function until the operation completes.
Awaiting before a branch makes every path pay the operation's latency and load, including paths that return without using it.
When the skipped path is common, such as a disabled feature, the saving is large.

### Examples

#### Application: An early return

**Incorrect (counterexample):**

```ts
async function handleRequest(userId: string, skipProcessing: boolean) {
  const userData = await fetchUserData(userId);

  if (skipProcessing) {
    return { skipped: true };
  }

  return processUserData(userData);
}
```

**Correct:**

```ts
async function handleRequest(userId: string, skipProcessing: boolean) {
  if (skipProcessing) {
    return { skipped: true };
  }

  const userData = await fetchUserData(userId);
  return processUserData(userData);
}
```

#### Application: A cheap condition combined with a remote flag

**Incorrect (counterexample):**

```ts
const betaEnabled = await getFeatureFlag('beta-editor');
if (betaEnabled && user.isInternal) {
  // ...
}
```

Every request calls the flag service, even for users who are not internal.

**Correct:**

```ts
if (user.isInternal && (await getFeatureFlag('beta-editor'))) {
  // ...
}
```

#### Application: An authorization check that must stay first

**Incorrect (counterexample):**

```ts
async function updateResource(resourceId: string, userId: string) {
  const resource = await getResource(resourceId);
  if (!resource) return { error: 'Not found' };

  const permissions = await fetchPermissions(userId);
  if (!permissions.canEdit) return { error: 'Forbidden' };
  // ...
}
```

Deferring the permission check lets any caller learn which resource ids exist.

**Correct:**

```ts
async function updateResource(resourceId: string, userId: string) {
  const permissions = await fetchPermissions(userId);
  if (!permissions.canEdit) return { error: 'Not found' };

  const resource = await getResource(resourceId);
  if (!resource) return { error: 'Not found' };
  // ...
}
```

### Validation

For each `await` above a branch, check whether every path after it uses the result.
Check that reordering did not move an authorization check after a lookup that reveals information.

An await that must precede a branch because it guards access or has a required side effect is not a violation.
