---
title: "Defer non-critical browser work to idle time"
whenToRead: "Before planning, writing, changing, or reviewing browser code that runs work the user is not waiting for after an interaction or during page load, such as sending custom analytics events, saving drafts, prefetching, or processing large data."
impact: "MEDIUM"
impactDescription: "Secondary work that runs before the response is painted delays it, making the interface feel slow."
tags: "javascript, browser, scheduling, requestIdleCallback, performance"
attribution:
  - url: https://github.com/vercel-labs/agent-skills/blob/4ec6f84b61cd3c931046c3e6e398f3ae7de372f7/skills/react-best-practices/rules/js-request-idle-callback.md
    description: "Adapted from the Vercel Agent Skills rule js-request-idle-callback: moved from the React group, restructured to the rule template, and generalized the browser support guidance."
---

## Defer non-critical browser work to idle time

Run work your code does that the user is not waiting for in idle time after the browser paints the response.

This rule covers work your own code runs. It does not govern when a third-party script loads; the rule "Load measurement scripts the way their vendors document" covers analytics and monitoring scripts.

### Implementation

- After an interaction, schedule the work with `requestIdleCallback`.
- During page load, wait for the `load` event and the next animation frame before calling `requestIdleCallback`. Before the first paint, the browser can be idle while it waits for resources, so `requestIdleCallback` alone can run the work before anything is on screen. An animation frame callback runs just before that frame renders, so an idle callback or timer requested from it runs after the render.
- Pass a `timeout` when the work must run even if the browser stays busy.
- Fall back to `setTimeout` in browsers without `requestIdleCallback`; check the project's target browsers.
- Split large jobs into chunks that check `deadline.timeRemaining()` and reschedule themselves.
- Idle callbacks may never run if the user leaves; flush work that must not be lost, such as a draft, when the page is hidden.

### Rationale

JavaScript runs on the same thread that handles input and rendering, so secondary work that runs first delays the frame that shows the result.
Idle callbacks run when the browser has nothing more urgent to do, but during page load those gaps can come before the first paint.

### Examples

#### Application: Work after an interaction

**Incorrect (counterexample):**

```ts
function handleSearch(query: string) {
  setResults(searchItems(query));
  analytics.track('search', { query });
  saveToRecentSearches(query);
}
```

Tracking and saving run before the browser can paint the results.

**Correct:**

```ts
const scheduleIdle: (callback: () => void) => void =
  typeof requestIdleCallback === 'function'
    ? (callback) => requestIdleCallback(callback, { timeout: 2000 })
    : (callback) => setTimeout(callback, 1);

function handleSearch(query: string) {
  setResults(searchItems(query));
  scheduleIdle(() => {
    analytics.track('search', { query });
    saveToRecentSearches(query);
  });
}
```

#### Application: Work that starts during page load

**Incorrect (counterexample):**

```ts
scheduleIdle(prefetchLikelyRoutes);
```

The callback can run while the browser waits for resources, before any content is painted.

**Correct:**

```ts
function scheduleAfterPaint(callback: () => void) {
  const afterNextFrame = () => requestAnimationFrame(() => scheduleIdle(callback));
  if (document.readyState === 'complete') afterNextFrame();
  else addEventListener('load', afterNextFrame, { once: true });
}

scheduleAfterPaint(prefetchLikelyRoutes);
```

The prefetch waits until the loaded page paints a frame, then for idle time. Hidden tabs run no animation frames, so it waits until the tab is shown.

### Validation

Record a performance profile of the interaction or page load and check that the deferred work runs after the result is painted. For page-load work, compare against the `paintTime` of the `first-contentful-paint` entry, which Chrome and Firefox provide. In those browsers `startTime` is when the frame reached the screen, which can be a few milliseconds after the render, so work that runs between the two is not a violation.
Check that a fallback exists for browsers without `requestIdleCallback`.

Running work immediately is not a violation when the user is waiting for its result.
