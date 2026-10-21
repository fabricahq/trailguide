---
title: "Version and minimize data in browser storage"
whenToRead: "Before planning, writing, changing, or reviewing code that stores data in localStorage or sessionStorage, or that changes the shape of data already stored there."
impact: "MEDIUM"
impactDescription: "Unversioned stored data breaks when its shape changes, storing whole objects can persist sensitive fields, and unguarded storage calls throw."
tags: "javascript, browser, localStorage, persistence"
attribution:
  - url: https://github.com/vercel-labs/agent-skills/blob/4ec6f84b61cd3c931046c3e6e398f3ae7de372f7/skills/react-best-practices/rules/client-localstorage-schema.md
    description: "Adapted from the Vercel Agent Skills rule client-localstorage-schema: moved from the React group, restructured to the rule template, and added validation of parsed data."
---

## Version and minimize data in browser storage

Store only the fields the client needs under a versioned key, validate what you read back, and handle storage errors.

### Implementation

- Include a version in the key, such as `userConfig:v2`, and bump it when the stored shape changes.
- Migrate or discard data stored under older versions when the app starts.
- Store only the fields the UI needs, never tokens, personal data, or internal flags copied along with a larger object.
- Wrap reads and writes in `try` / `catch`; storage can be unavailable, full, or disabled, depending on the browser and privacy settings.
- Validate parsed data before using it; stored values may come from an older version or be edited by the user.

### Rationale

Stored data outlives the code that wrote it.
When a new release reads data written by an old one, a changed shape causes errors unless the key or data carries a version.
Storing whole server objects persists more than intended, and browser storage is readable by any script on the page.

### Examples

**Incorrect (counterexample):**

```ts
localStorage.setItem('userConfig', JSON.stringify(fullUserObject));
const config = JSON.parse(localStorage.getItem('userConfig')!);
```

The whole user object is stored, the key has no version, and a missing value or unavailable storage throws.

**Correct:**

```ts
const CONFIG_KEY = 'userConfig:v2';

function saveConfig(config: { theme: string; language: string }) {
  try {
    localStorage.setItem(CONFIG_KEY, JSON.stringify(config));
  } catch {
    // Storage is unavailable or full; the app continues with in-memory settings.
  }
}

function loadConfig(): { theme: string; language: string } | null {
  try {
    const stored = localStorage.getItem(CONFIG_KEY);
    const value: unknown = stored ? JSON.parse(stored) : null;
    return isConfig(value) ? value : null;
  } catch {
    return null;
  }
}
```

`isConfig` is a type guard that checks both fields, so data written by an older version or edited by hand is ignored instead of crashing the app.

### Validation

Check each storage key for a version, each write for the minimum fields, and each read for error handling and validation.

Storing a single primitive preference, such as a theme name, under a stable key is not a violation.
