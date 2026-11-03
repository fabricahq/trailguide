---
title: "Hide implementation decisions behind meaningful interfaces"
whenToRead: "Before designing, changing, or reviewing how functions, classes, packages, or services work together. Read when choosing parameters, defaults, or configuration, passing dependencies, returning internal data, defining a format that more than one module reads or writes, or deciding which module performs a step."
impact: "HIGH"
impactDescription: "Callers that repeat a module's internal steps, formats, or settings can miss required work and must change whenever those details change."
tags: "code-design, interfaces, information-hiding, ownership"
attribution:
  - url: https://web.stanford.edu/~ouster/cgi-bin/book.php
    description: "Informed by modules and interfaces, information hiding and leakage, temporal decomposition, pass-through methods and variables, configuration parameters, and pulling complexity downward in John Ousterhout's A Philosophy of Software Design, second edition, chapters 4, 5, 7, and 8. Wording and examples are original."
---

## Hide implementation decisions behind meaningful interfaces

Hide implementation decisions so callers neither need to understand them nor depend on them.

A module can be a function, class, package, or service.
Its interface is everything a caller must know to use it correctly.
That includes arguments, return values, call order, who closes a resource, and what happens if a step fails, whether or not the type system states it.

Give each implementation decision one owning module.
Any code that would have to change if the decision changed belongs in that module.
Implementation decisions include how stored data is arranged, which format encodes it, which algorithm to use, and which steps must happen in what order.
The owning module makes those decisions and carries them out, so callers do not repeat them.

Information hiding succeeds when callers can use the module correctly without knowing those decisions.
You should be able to change a hidden decision without changing callers, as long as the module's behavior stays the same for them.
For example, callers of a document store should not need to know whether it stores documents in one file or several.

Making a field private is not enough.
If callers still need to understand its contents through getters, setters, or configuration, that information remains exposed.
A decision can also leak without appearing in any interface: if two modules both build the same cache key or parse the same file layout, each depends on that decision.

Hide only what callers can safely ignore.
If callers need a fact to use the module correctly, such as whether a write survives a crash, put that fact in the interface or its documentation.

Let callers ask for a result and choose the things that matter to their task, such as which document to save.
Keep the decisions about how to produce that result inside the module.

### Implementation

- Draw module boundaries around knowledge, not execution order.
  Splitting code into a reader, a transformer, and a writer because they run in that order often gives two modules the same knowledge of a format.
  Put the code that reads and the code that writes a format, key layout, or protocol in the module that owns it, even when they run at different times.
- Move work into a module only when it belongs to that module's job, simplifies its callers, and keeps its interface simple.
  Moving one caller's policy into a general module, such as putting a screen's keyboard shortcuts into a text buffer, leaks that caller's decisions into the module instead.
- Make each module save its callers from knowing or doing something.
  If a wrapper forwards the same arguments and returns the same result, check what else it provides, such as authorization, retries, or a choice among implementations.
  If it adds nothing, remove it or move the work so each module has a clear job.
  Functions with the same signature are fine when each does distinct work, such as several implementations of one interface.
- Shape the interface around what callers do, not around how the module stores data.
  Return the value a caller asked for, such as `request.parameter('id')`, rather than the internal map that holds every parameter.
  A text buffer stored as lines can still let callers insert text at any position, so they do not split and join lines themselves.
- Ask callers only for choices they can make better than the module.
  Keep choices such as a destination, a payment provider, or a transaction that several operations share explicit, along with lower-level operations that callers combine into a larger task.
  If the module can determine a value itself, such as a retry interval from measured latency or a response format from the request it answers, let it do so.
  When a setting is still needed, give it a default that suits most callers, and keep rarely used overrides out of the common call.
- Build and connect dependencies where the code already knows which implementations to use.
  Give a value that only a deep component needs to that component when it is constructed, instead of adding the value to every function signature on the way down.
  A module's tests can supply substitutes to its factory without adding those parameters to every caller above it.
- Judge an interface by what callers must understand, not by how many arguments it takes.
  Putting five implementation details in one options object still requires callers to know all five, and writing the fields explicitly or deriving them from another type does not remove that knowledge.
  Shared types are fine when they describe the same data: `Pick<User, 'id' | 'name'>` need not become a separate handwritten type.

### Rationale

When every caller repeats the steps for an operation, each can omit a step or run them in the wrong order.
Changing those steps then requires finding and updating every caller.
Putting the steps in one module means fixing them once, while callers keep asking for the same result.

Leakage that appears in no interface is harder to find than leakage through one.
When a writer and a reader each encode the same format, nothing in their signatures shows the dependency, so a change to one can silently break the other.

A setting or required argument that the module could have determined moves the decision to every caller.
Each caller must learn what the setting means and pick a value, often with less information than the module has.

### Examples

These examples use TypeScript, but the practice applies in any language.
They show what callers use; the implementation must still do the work correctly.

#### Application: Saving a document

An editor needs to save a document while other readers see either the complete old version or the complete new version.

**Incorrect (counterexample):**

```ts
const temporary = await documents.writeTemporary(path, contents);
await documents.replace(path, temporary);
```

Every caller must know to write a temporary file before replacing the original.
Each must also handle leftover files if replacement fails.
Putting `temporary` in an options object would leave those responsibilities with the caller.

**Correct:**

```ts
await documents.save(path, contents);
```

The document module creates the temporary file, replaces the original, and cleans up after failure.
The caller supplies the destination and contents.
Changing how replacement works does not require editing callers.
A successful save in this example does not guarantee that the contents survive a machine crash; because callers may need to know that, the module's documentation should say so.

#### Application: Writing and reading a shared format

One module stores a user's profile in a cache after loading it, and another module reads it back later.

**Incorrect (counterexample):**

```ts
// profile-loader.ts
await cache.set(`profile:${userId}:v2`, JSON.stringify(profile));

// profile-page.ts
const stored = await cache.get(`profile:${userId}:v2`);
const profile = stored === undefined ? undefined : decodeProfile(stored);
```

Both modules independently encode knowledge of the same storage format.
Changing that format requires updating both together.
If the loader moves to a `v3` key and the page is not updated, the page silently misses every cached profile.
The code was split by when it runs, so the format ended up in both places.

**Correct:**

```ts
// profile-cache.ts
const key = (userId: UserId) => `profile:${userId}:v2`;

export async function getCachedProfile(userId: UserId): Promise<Profile | undefined> {
  const stored = await cache.get(key(userId));
  return stored === undefined ? undefined : decodeProfile(stored);
}

export async function cacheProfile(userId: UserId, profile: Profile): Promise<void> {
  await cache.set(key(userId), JSON.stringify(profile));
}
```

The loader calls `cacheProfile` and the page calls `getCachedProfile`.
The key layout and encoding now live in one module, so changing either edits one file.
`decodeProfile` stands for whatever validation the stored value needs.
A format that is an intentional contract between separately deployed systems, such as a published event schema, is shared on purpose: define it once where both sides can import it, and change it as a contract.

#### Application: Constructing an application

An entry point chooses the API mode and whether users must sign in.
The application already has fixed functions for fetching sessions, logging out, and clearing account state.

**Incorrect (counterexample):**

```ts
type ApplicationConfig = Omit<SessionDependencies, 'clearCache'>;

const app = createApplication({
  apiMode,
  authRequired,
  fetchSession,
  logOutServer,
  resetAccountState,
});
```

The entry point must import those functions and know where to pass them.
Because the implementations are fixed, that is unnecessary exposure; passing them would be appropriate if the entry point intentionally chose implementations, such as one per platform.
Adding a function to `SessionDependencies` changes what the entry point must supply, even if it has no new decision to make.
Replacing `Omit` with an explicit list of the same fields makes the type easier to read but leaves that problem in place.

**Correct:**

```ts
type ApplicationConfig = {
  apiMode: ApiMode;
  authRequired: boolean;
};

const app = createApplication({ apiMode, authRequired });
```

`createApplication` selects the session functions and connects them to account and cache cleanup.
The session module's factory still accepts dependencies, so its tests can supply controlled implementations.
Adding or replacing an internal session dependency does not change what the entry point must supply.
If callers need to choose an authentication provider, expose that choice explicitly.

#### Application: Sharing a transaction across operations

A caller needs to save an order and reserve stock in one transaction: both changes must succeed, or neither should take effect.

**Incorrect (counterexample):**

Make `saveOrder(order)` and `reserveStock(items)` each start and commit its own transaction to avoid passing a transaction argument.
The order can then save successfully even if reserving stock fails.

**Correct:**

```ts
await database.transaction(async (tx) => {
  await orders.save(tx, order);
  await inventory.reserve(tx, items);
});
```

The caller starts the transaction because it knows the two changes must happen together.
Each module uses that transaction but keeps its queries and table layout private.
Removing the transaction argument would take away control the caller needs.

### Validation

Read the module's interface and a few places that call it.
For each argument, callback, configuration setting, or required sequence, ask:

- Does the caller need to choose this, such as the destination file or a shared transaction?
- Or does the caller supply it only because the module requires help with an internal step?
- Could the module determine a value as good as the caller's, or provide a default that most callers would accept?

Then search for the same decision encoded outside its owner: key templates, file or message layouts, magic constants, and ordering assumptions that appear in more than one module.

Try a plausible internal change: add a cleanup step, change a table layout or cache key, or replace a helper.
Which modules would need edits, and why?
If their requested result stays the same, those modules should normally stay unchanged.
A change to results, accepted inputs, or an intentionally shared data format can require caller changes.

Before reporting a problem, name what the caller unnecessarily needs to know and which module could handle it instead.
Neither argument counts, `Omit`, dependency injection, configuration settings, nor shared types prove a problem on their own.

After refactoring, test the results callers depend on, including what happens if a step fails.
Check that callers can still make necessary choices, that facts they rely on, such as durability, are still documented, and that work moved into a module belongs to its job.
