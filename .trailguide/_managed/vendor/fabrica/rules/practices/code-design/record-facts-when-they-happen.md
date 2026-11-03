---
title: "Record facts when they happen"
whenToRead: "Before planning, writing, changing, or reviewing code that needs to know something that happened earlier, such as which question a reply answered, which job delivered a result, or whether a payment was refunded, and before migrating records that don't say it."
impact: "HIGH"
impactDescription: "Code that works out a past event from timestamps, ordering, or which records exist gets it wrong whenever history takes a shape its author didn't foresee, and each patch to the guess invites the next."
tags: "code-design, state, events, data-modeling, migrations"
---

## Record facts when they happen

When your code needs to know something that happened earlier, write it down at the moment it happens, and read it back later.
Don't work it out afterwards from clues such as timestamps, the order of records, or which records exist.

The moment something happens is the only moment the code knows for certain what happened.
Later, all it has are clues, and clues can be read more than one way.

### Implementation

- Say the fact as a plain sentence, such as "this reply answers that question" or "this refund reverses that payment."
  If nothing stored says that sentence, add a field or a record that does.
- Write it in the same operation, and the same transaction, as the event itself, with the ids that link the two, such as the question a reply answers.
  Then the event and its record can't disagree.
- Make every later decision read that record: a payment is refunded when a refund record names it, and a question is open while no reply names it.
- Records written before you added the fact don't have it.
  Don't guess it for them, in a backfill or in a fallback when reading.
  Give them one explicit outcome instead, usually the behavior they had at the time, and mark them as settled without the fact.
  Record the fact for everything that happens from the change on.

These comply:

- Deriving state from an event log whose events state the fact; the log is the record.
- Asking the system that owns the fact, such as a payment provider's refund API.
- Using patterns in history for estimates that drive no behavior, such as a trend on a dashboard.

### Rationale

A guess about the past holds only for the histories its author pictured.
Retries, failures, duplicates, edits, and new features produce histories nobody pictured, and the guess then gives a wrong answer without raising an error.
Each wrong answer gets patched with one more condition, and each condition creates a new case to get wrong, so the code never settles.
A recorded fact doesn't depend on what happens afterwards, because the code that knew the answer wrote it down.

Guessing the fact for old records brings the guess back, and it is usually harder, because old history was never shaped to answer the question.
Settling old records with one explicit outcome is a single decision a reviewer can check once.

### Examples

These examples are in TypeScript; the practice applies in any language and with any storage.

#### Application: Deciding whether a payment was refunded

A cancellation flow refunds an order's payment unless it was already refunded.
The order's payment entries include charges, refunds, chargebacks, and manual corrections, each stored as a signed amount.

**Incorrect (counterexample):**

```ts
function alreadyRefunded(order: Order): boolean {
  return order.payments.some((entry) => entry.amount < 0);
}
```

A negative entry can be a chargeback or a correction, so the flow skips a refund the customer is owed.
A refund the provider hasn't settled yet has no negative entry, so cancelling twice refunds twice.
Patches such as checking the entry's memo or comparing its timestamp with the cancellation's each add a condition that the next history breaks.

**Correct:**

```ts
type Refund = { readonly paymentId: string; readonly amount: number };

/** Record the refund and queue it for the provider in one transaction. */
async function refundPayment(tx: Transaction, payment: Payment): Promise<void> {
  await tx.insert('refunds', { paymentId: payment.id, amount: payment.amount });
  await tx.insert('provider_requests', { kind: 'refund', paymentId: payment.id });
}

/** True when a refund record names the payment; payment entries aren't consulted. */
function alreadyRefunded(refunds: ReadonlyArray<Refund>, payment: Payment): boolean {
  return refunds.some((refund) => refund.paymentId === payment.id);
}
```

The refund record exists from the moment the flow decides to refund, so a second cancellation sees it while the provider is still processing.
A unique constraint on `refunds.paymentId` covers two cancellations racing in parallel: the second insert fails instead of queuing a second refund.
A chargeback writes no refund record, so it can't be mistaken for one.
The queue table is illustrative; what matters is that the record commits together with the event it describes.

#### Application: Payments from before the record existed

Payments taken before the refund records were added have none.
Their refunds were issued by hand, by support staff in the provider's dashboard.

**Incorrect (counterexample):**

```ts
// Backfill: treat a negative entry within 30 days of a charge as its refund.
for (const order of ordersBeforeRefundRecords) {
  for (const charge of order.payments.filter((entry) => entry.amount > 0)) {
    const refund = order.payments.find(
      (entry) => entry.amount === -charge.amount && withinDays(charge.createdAt, entry.createdAt, 30),
    );
    if (refund) await db.insert('refunds', { paymentId: charge.id, amount: charge.amount });
  }
}
```

The backfill is the same inference in a new place.
Its records look observed but are guesses, and the first mismatch gets another condition.

**Correct:**

```ts
// Refunds before the refund records were manual, so automation leaves these payments to support.
await db.update('payments', { createdBefore: REFUND_RECORDS_START }, { refundHandling: 'manual' });

function canRefundAutomatically(payment: Payment): boolean {
  return payment.refundHandling !== 'manual';
}
```

Old payments keep the behavior they had: support handles their refunds.
The `manual` marker is the settlement, stated once for the payments from before the change, so no one mistakes it for observed history.
Every new refund is recorded when it happens.

### Validation

For each decision that depends on an earlier event, find the field or record that states it, and check that it is written together with the event and that the decision reads it.

Look for decisions computed from the first or latest record, timestamps compared across records, counts, or which records exist, and for fallbacks that handle records missing the fact.
Each is a likely guess.
Check for a test where the clues and the fact disagree, such as a chargeback that isn't a refund or a job that ran twice, and that asserts the behavior follows the record.
For a migration, check that old records get an explicit, labeled outcome and that no code reconstructs the fact for them.

These are not by themselves violations: sorting records by time for display; reading the latest record when "latest" is what's meant, such as the current setting being the most recent edit; deriving state from an event log whose events state the fact; and reading the fact from the system that owns it.
