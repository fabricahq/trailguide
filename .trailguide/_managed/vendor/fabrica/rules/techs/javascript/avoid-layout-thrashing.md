---
title: "Avoid layout thrashing"
whenToRead: "Before writing, changing, reviewing, or diagnosing browser code that changes element styles and also reads layout, such as measuring elements with getBoundingClientRect, offsetWidth, or getComputedStyle, including code in UI framework Effects."
impact: "MEDIUM"
impactDescription: "Reading layout after each style change forces the browser to recalculate layout repeatedly, causing jank."
tags: "javascript, browser, dom, layout, performance"
attribution:
  - url: https://github.com/vercel-labs/agent-skills/blob/4ec6f84b61cd3c931046c3e6e398f3ae7de372f7/skills/react-best-practices/rules/js-batch-dom-css.md
    description: "Adapted from the Vercel Agent Skills rule js-batch-dom-css: moved from the React group, restructured to the rule template, and shortened the examples."
---

## Avoid layout thrashing

Do not interleave DOM style writes with layout reads.
Read all the layout values you need first, then make all the style changes, or change styles through a CSS class.

### Implementation

- Group layout reads, such as `getBoundingClientRect()`, `offsetWidth`, `scrollTop`, and `getComputedStyle()`, before any writes in the same task.
- Group style writes after the reads, or apply them with a class change.
- When many elements are measured and updated, such as in a list, read all of them, then write all of them.
- Schedule visual writes for the next frame with `requestAnimationFrame` when reads and writes come from different code paths.

### Rationale

After a style change, the browser marks layout as stale.
Reading a layout property then forces it to recalculate layout immediately and synchronously.
Alternating writes and reads repeats that recalculation for every pair, which can take far longer than one recalculation at the end.

### Examples

**Incorrect (counterexample):**

```ts
function resizeCards(cards: ReadonlyArray<HTMLElement>) {
  for (const card of cards) {
    card.style.width = `${card.parentElement!.offsetWidth / 2}px`;
  }
}
```

Each iteration reads a layout property after the previous iteration's write, forcing a layout per card.

**Correct:**

```ts
function resizeCards(cards: ReadonlyArray<HTMLElement>) {
  const widths = cards.map((card) => card.parentElement!.offsetWidth / 2);
  cards.forEach((card, index) => {
    card.style.width = `${widths[index]}px`;
  });
}
```

All reads happen before any write, so layout is calculated once.

### Validation

Record a performance profile of the interaction and look for repeated "Layout" entries flagged as forced reflow.
Review loops and Effects that both write styles and read layout.

A single read after all writes is not a violation.
