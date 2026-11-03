---
title: "Remove unnecessary error cases through contract design"
whenToRead: "Before planning, writing, changing, or reviewing an operation's success and failure semantics, such as deletion, unsubscribe, empty collections, retries, or callers handling an already-satisfied request."
impact: "MEDIUM-HIGH"
impactDescription: "Unnecessary failures spread special-case handling across callers, while suppressing meaningful failures makes success reports unreliable."
tags: "code-design, errors, contracts, idempotence"
attribution:
  - url: https://web.stanford.edu/~ouster/cgi-bin/book.php
    description: "Original guidance informed by chapter 10 of John Ousterhout's A Philosophy of Software Design, second edition; examples and wording are original."
---

## Remove unnecessary error cases through contract design

Define ordinary cases as successful outcomes when they satisfy the operation's promised result, and preserve failures that prevent that result or carry information callers need.
Make this choice in the contract before deciding which errors to handle internally.

### Implementation

- State the promised outcome and inspect each proposed error against it.
  An already-absent entry can satisfy an operation that ensures absence; an empty collection can be a valid query result.
- Handle an expected condition once in the operation that owns its meaning, rather than requiring every caller to catch it or perform a separate existence check.
  Avoid check-then-act sequences whose precondition can change before the operation runs.
- Preserve meaningful distinctions, such as invalid input, authorization failure, conflicting content, and inability to confirm a write.
  A successful response must still mean the documented outcome was achieved.
- Keep required input validation and authorization checks even when the requested state already holds.
  Scope any internal error handling to the specific condition whose semantics permit success.
- Inspect existing callers before changing a published contract.
  When callers need to distinguish an actual change from an already-satisfied request, preserve that distinction in the result or retain the failure contract.
- Evaluate retries against the whole operation, including messages, billing, and audit effects.
  A repeated request is not harmless merely because the stored value ends up the same.

### Rationale

If every caller catches the same error and proceeds normally, the operation may be exporting a special case it could resolve itself.
A clearer success contract removes that repeated handling.
Catching all failures has the opposite effect: callers can no longer tell whether the requested work happened.

### Examples

#### Application: Ensuring a subscription is absent

The operation promises that the authenticated user will have no subscription to a topic after it succeeds.
Removing the subscription is the only effect in this example.

**Incorrect (counterexample):**

Each caller checks whether the subscription exists, then calls a removal method that fails when it is absent.
Concurrent requests can both pass the check and then race to remove the same subscription.
Every caller needs a second handler for this otherwise successful outcome.

**Correct:**

The unsubscribe operation validates the request, checks authorization, and deletes the matching subscription directly.
Deleting zero matching rows is success because the promised state already holds.
Database failures still fail the operation; callers need no separate existence check.

#### Application: Hiding a failure that prevents success

**Incorrect (counterexample):**

A save operation catches every storage exception and returns success so callers do not need error handling.
Users can be told their changes are saved when storage rejected the write.

**Correct:**

The operation reports a failed or unconfirmed save through its documented failure contract.
It can translate storage-specific errors into that contract, but it preserves the fact that persistence was not confirmed.

#### Application: A caller needs the distinction

**Correct:**

An operation that requires replacing an existing record reports a missing record as a failure.
That differs from ensuring a desired state: existence is part of this operation's promise.
Likewise, a caller that needs to know whether unsubscribe changed anything can receive an explicit `removed` or `alreadyAbsent` result without treating either as a failure.

### Validation

For each condition converted from failure to success, demonstrate that the promised outcome holds and that callers lose no required information.
Test both the normal case and the already-satisfied or empty case through the operation's interface.
Also exercise a meaningful failure and verify it remains observable.
For retryable mutations, inspect all effects, not only final stored state.

An explicit error is not a violation when it protects a required distinction or signals an unmet outcome.
