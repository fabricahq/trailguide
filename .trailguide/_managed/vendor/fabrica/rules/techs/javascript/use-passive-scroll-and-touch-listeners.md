---
title: "Mark scroll-related listeners passive when they never cancel scrolling"
whenToRead: "Before writing, changing, or reviewing browser code that adds touchstart, touchmove, wheel, or mousewheel listeners, or diagnosing delayed scrolling on touch devices."
impact: "MEDIUM"
impactDescription: "A non-passive touch or wheel listener makes the browser wait for it before scrolling, which delays scrolling."
tags: "javascript, browser, events, scrolling, performance"
attribution:
  - url: https://github.com/vercel-labs/agent-skills/blob/4ec6f84b61cd3c931046c3e6e398f3ae7de372f7/skills/react-best-practices/rules/client-passive-event-listeners.md
    description: "Adapted from the Vercel Agent Skills rule client-passive-event-listeners: moved from the React group, restructured to the rule template, and corrected to reflect browsers' passive defaults for document-level targets."
---

## Mark scroll-related listeners passive when they never cancel scrolling

When a `touchstart`, `touchmove`, `wheel`, or `mousewheel` listener never calls `preventDefault()`, register it with `{ passive: true }`.

### Implementation

- Add `{ passive: true }` to listeners that only observe, such as analytics or position tracking.
- Keep a listener non-passive, and set `{ passive: false }` explicitly, when it must call `preventDefault()`, such as for a custom swipe gesture or zoom control.
- Pass the option explicitly even on `window`, `document`, or `document.body`.
  Browsers other than Safari already treat those targets as passive by default, but element-level listeners and Safari do not, and the explicit option documents intent.

### Rationale

A touch or wheel listener can cancel scrolling by calling `preventDefault()`, so the browser must wait for it to run before it scrolls.
Marking the listener passive promises it will not cancel, so the browser scrolls immediately.

### Examples

**Incorrect (counterexample):**

```ts
scrollContainer.addEventListener('touchstart', (event) => {
  trackTouch(event.touches[0].clientX);
});
```

The browser waits for this listener before scrolling the container, although it never cancels.

**Correct:**

```ts
scrollContainer.addEventListener(
  'touchstart',
  (event) => {
    trackTouch(event.touches[0].clientX);
  },
  { passive: true },
);
```

### Validation

Check each touch and wheel listener for `preventDefault()` calls; listeners without one should be passive.
Browser developer tools can report listeners that delay scrolling.

A non-passive listener that calls `preventDefault()` is not a violation.
