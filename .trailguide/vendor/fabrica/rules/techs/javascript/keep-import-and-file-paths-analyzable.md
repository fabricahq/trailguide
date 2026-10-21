---
title: "Keep dynamic import and file paths statically analyzable"
whenToRead: "Before planning, writing, changing, or reviewing code that chooses modules with dynamic import() or builds file-system paths from variables in code a bundler or file tracer processes, such as pages, plugins, or server functions."
impact: "MEDIUM"
impactDescription: "Paths that build tools cannot analyze make them include broad sets of files or fail to include needed ones, enlarging bundles and slowing builds and cold starts."
tags: "javascript, bundling, dynamic-import, file-tracing"
attribution:
  - url: https://github.com/vercel-labs/agent-skills/blob/4ec6f84b61cd3c931046c3e6e398f3ae7de372f7/skills/react-best-practices/rules/bundle-analyzable-paths.md
    description: "Adapted from the Vercel Agent Skills rule bundle-analyzable-paths: moved from the React group and restructured to the rule template."
---

## Keep dynamic import and file paths statically analyzable

When code chooses among modules or files at runtime, list each possible path literally, such as in a map of import functions, instead of computing the path from a variable.

### Implementation

- Map keys to functions that each call `import()` with a literal path.
- Build file-system paths from literal strings at each branch, rather than concatenating a variable into the path.
- When the set of files is genuinely open-ended, such as user-provided plugins, load them outside the bundle through the runtime's own module loading, and configure the build tool accordingly.

### Rationale

Bundlers and file tracers decide what to include by reading import and file-system calls at build time.
A path computed from a variable hides the real targets, so the tool either includes every file that might match, warns, or misses files the program needs at runtime.

### Examples

#### Application: Choosing a module

**Incorrect (counterexample):**

```ts
const pageModules = { home: './pages/home', settings: './pages/settings' } as const;
const page = await import(pageModules[pageName]);
```

The bundler cannot tell which modules `import()` may load.

**Correct:**

```ts
const pageModules = {
  home: () => import('./pages/home'),
  settings: () => import('./pages/settings'),
} as const;
const page = await pageModules[pageName]();
```

#### Application: Choosing a directory

**Incorrect (counterexample):**

```ts
const baseDir = path.join(process.cwd(), `content/${contentKind}`);
```

A file tracer may include the whole `content` directory, or miss the files used at runtime.

**Correct:**

```ts
const baseDir =
  contentKind === 'blog' ? path.join(process.cwd(), 'content/blog') : path.join(process.cwd(), 'content/docs');
```

### Validation

Search for `import()` and file-system calls whose path argument is not a literal.
Inspect the build output or traced files to confirm that only the intended modules and files are included.

A dynamic path deliberately excluded from bundling and loaded at runtime is not a violation.
