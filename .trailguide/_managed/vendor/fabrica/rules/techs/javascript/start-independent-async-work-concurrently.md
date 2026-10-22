---
title: "Start independent asynchronous work concurrently"
whenToRead: "Before planning, writing, changing, or reviewing code that awaits several asynchronous operations, such as request handlers, server functions, or data loaders that call multiple services or queries."
impact: "HIGH"
impactDescription: "Awaiting independent operations one after another adds their latencies together instead of overlapping them."
tags: "javascript, async, promises, concurrency, latency"
attribution:
  - url: https://github.com/vercel-labs/agent-skills/tree/4ec6f84b61cd3c931046c3e6e398f3ae7de372f7/skills/react-best-practices/rules
    description: "Adapted from four Vercel Agent Skills rules (async-parallel, async-api-routes, async-dependencies, and server-parallel-nested-fetching): merged four rules on Promise.all, API route waterfalls, dependency-based parallelization, and nested fetching; restructured to the rule template; and added failure, rejection, and concurrency-limit guidance."
---

## Start independent asynchronous work concurrently

Start asynchronous operations that do not depend on each other at the same time, and await them together.
When some operations depend on others, start each one as soon as its own inputs are ready.

### Implementation

- Await independent operations together with `Promise.all`.
- Use `Promise.allSettled` when you need every result even if some fail, such as optional dashboard widgets.
- For partial dependencies, chain each dependent operation onto the promise it needs, such as `userPromise.then((user) => fetchProfile(user.id))`, and await everything at the end.
- For a list of items with nested lookups, chain each item's lookups inside its own promise, so one slow item does not delay the others.
- When you start a promise early and might return or throw before awaiting it, attach a rejection handler or await it on every path; otherwise its failure becomes an unhandled rejection.
- `Promise.all` rejects on the first failure but does not cancel the other operations.
  Pass an `AbortSignal` to operations that should stop when the group fails.
- Keep operations sequential when order matters: side effects that must happen in sequence, queries inside one database transaction, or calls to a service that limits concurrent requests.
- Limit concurrency when mapping over many items, such as by processing them in batches or with a concurrency-limiting helper, instead of starting thousands of requests at once.

### Rationale

Each `await` waits for its operation to finish before the next line runs.
Awaiting three independent 100 ms requests in sequence takes about 300 ms; starting them together takes about 100 ms.
Starting work as soon as its inputs are ready keeps a slow operation from delaying unrelated ones.

### Examples

#### Application: Independent operations

**Incorrect (counterexample):**

```ts
const user = await fetchUser();
const posts = await fetchPosts();
const comments = await fetchComments();
```

**Correct:**

```ts
const [user, posts, comments] = await Promise.all([fetchUser(), fetchPosts(), fetchComments()]);
```

#### Application: Partial dependencies

**Incorrect (counterexample):**

```ts
const [user, config] = await Promise.all([fetchUser(), fetchConfig()]);
const profile = await fetchProfile(user.id);
```

`fetchProfile` waits for `fetchConfig`, although it needs only the user.

**Correct:**

```ts
const userPromise = fetchUser();
const profilePromise = userPromise.then((user) => fetchProfile(user.id));

const [user, config, profile] = await Promise.all([userPromise, fetchConfig(), profilePromise]);
```

#### Application: Nested lookups for many items

**Incorrect (counterexample):**

```ts
const chats = await Promise.all(chatIds.map((id) => getChat(id)));
const authors = await Promise.all(chats.map((chat) => getUser(chat.authorId)));
```

One slow `getChat` delays every author lookup.

**Correct:**

```ts
const authors = await Promise.all(chatIds.map((id) => getChat(id).then((chat) => getUser(chat.authorId))));
```

### Validation

Trace the operation's requests with timing or logs, and check that independent operations overlap.
Check that promises started early are awaited or handled on every path, including early returns and thrown errors.

Sequential awaits where each operation needs the previous result, or where order or a transaction requires it, are not violations.
