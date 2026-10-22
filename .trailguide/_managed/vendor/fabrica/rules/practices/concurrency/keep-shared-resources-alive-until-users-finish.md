---
title: "Keep shared resources alive until their users finish"
whenToRead: "When planning, implementing, changing, or reviewing shared resources that can be replaced, closed, or invalidated during concurrent use."
impact: "HIGH"
impactDescription: "Prevents resource replacement and cleanup from breaking in-flight operations."
tags: "concurrency, resources, lifecycle"
---

## Keep shared resources alive until their users finish

Keep a shared resource usable until every operation entitled to use it has finished. Replacing the current resource must not invalidate an earlier user's handle.

### Implementation

Apply this rule when concurrent operations share a resource with an explicit lifetime, such as a connection pool, file handle, client, or loaded snapshot.

Protect the whole period of use, including the interval between obtaining a handle and starting work with it. A lock around pointer lookup and replacement alone does not protect that period.

Choose a lifecycle contract that fits the resource. Options include acquiring a lease before exposing the handle, retiring old resources until their users finish, or serializing use and cleanup. A resource's own concurrency guarantees may already provide the needed protection; verify what they cover.

If operations deliberately run serially, enforce that contract at the owning boundary. Resources that remain valid for the process lifetime need no replacement protocol.

An explicit shutdown or cancellation contract may cancel in-flight work. Coordinate that transition so callers receive the promised outcome and cannot continue using an invalid resource.

### Rationale

An operation can retain a valid-looking handle after another operation closes its underlying resource. The failure depends on scheduling, so ordinary sequential tests can miss it.

Resource ownership must explain who can use a resource, when that permission ends, and when cleanup becomes safe.

### Examples

#### Application: Replacing a pool during concurrent use

One request obtains the current pool while another request replaces it.

**Incorrect (counterexample):**

1. Request A reads the current pool under a lock, then releases the lock.
2. Request A pauses before acquiring a connection.
3. Request B replaces the current pool and closes the old pool.
4. Request A resumes and attempts to acquire a connection from the closed pool.

The lock protects pointer lookup, but the pool's lifetime does not cover its user.

**Correct:**

1. The owner gives request A a lease on the current pool before exposing its handle. A pauses before acquiring a connection.
2. Request B publishes a replacement pool and retires the old pool. New requests obtain leases on the replacement.
3. Request A resumes, acquires a connection, completes its work, and releases its lease after returning the connection.
4. The owner closes the retired pool after its last lease ends.

The lease covers both the wait before acquisition and the subsequent use. Serialization or a verified resource-native guarantee can satisfy the same obligation.

### Validation

For resources that support replacement during use, test controlled interleavings rather than relying on sleeps or repeated runs:

- Pause a user after obtaining its handle but before starting resource-dependent work. Replace the resource, resume the user, and verify the promised operation succeeds.
- Repeat with a user that has already acquired a subordinate resource, such as a borrowed connection.
- Verify retired resources close only after their users finish, including error exits, and that new users reach the replacement.

For a serial contract, verify the owning boundary prevents use and cleanup from overlapping.

Before reporting a violation, inspect the resource's ownership and shutdown contracts, including any guarantees from the resource itself. A replaceable shared pointer alone does not establish that cleanup can interrupt a user.
