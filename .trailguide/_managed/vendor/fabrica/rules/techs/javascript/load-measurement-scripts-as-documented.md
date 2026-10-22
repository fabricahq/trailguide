---
title: "Load measurement scripts the way their vendors document"
whenToRead: "Before planning, writing, changing, or reviewing how a page loads a page-view analytics or real user monitoring script, such as Cloudflare Web Analytics or Plausible, including performance work that delays third-party scripts."
impact: "LOW-MEDIUM"
impactDescription: "A delayed measurement script never reports visits that end before it runs and misses early interactions, so analytics undercount visits and skew performance data."
tags: "javascript, browser, analytics, real user monitoring, third-party scripts"
---

## Load measurement scripts the way their vendors document

Load page-view analytics and real user monitoring scripts in the non-blocking form their vendor documents, such as an `async`, `defer`, or `type="module"` script tag. Do not delay them further with idle callbacks, timers, `load` listeners, or interaction triggers.

This rule covers scripts whose job is to measure visits or page performance. Other third-party scripts, such as chat widgets, ads, and embeds, are outside it.

### Implementation

- Use the vendor's snippet. When code must insert the script, such as to load it only on the production host, insert it as soon as that code runs during page load.
- When the visitor's consent is required, load the script as soon as consent is given.
- Calls your code makes through the script, such as custom events, are your own work and can be deferred.

### Rationale

These scripts measure the page load themselves and report when the page is hidden.
They start observing and register that report only when they run, so a delayed script loses visits that end first and misses interactions that happened before it started observing.
The vendor's non-blocking form keeps the script from blocking HTML parsing. Running it still takes main-thread time, but a measurement script is small, and delaying it costs data on every short visit. If a script's own execution measurably delays rendering, choose a lighter script instead of delaying it.

### Examples

**Incorrect (counterexample):**

```ts
requestIdleCallback(() => {
  const beacon = document.createElement('script');
  beacon.src = 'https://static.cloudflareinsights.com/beacon.min.js';
  beacon.dataset.cfBeacon = JSON.stringify({ token: SITE_TOKEN });
  document.head.append(beacon);
}, { timeout: 2000 });
```

A visitor who leaves before idle time is never counted, and the beacon misses interactions that happen before it loads.

**Correct:**

```html
<script type="module" src="https://static.cloudflareinsights.com/beacon.min.js" data-cf-beacon='{"token": "SITE_TOKEN"}'></script>
```

This is Cloudflare's documented snippet. The module script does not block parsing, and the beacon runs as soon as the page is parsed.

### Validation

Check that the page loads the script in its vendor's documented form, or inserts it from code that runs during page load, and that no idle callback, timer, `load` listener, or interaction gates it.

Waiting for required consent is not a violation. A script in its documented non-blocking form that runs after the first paint is not a violation.
