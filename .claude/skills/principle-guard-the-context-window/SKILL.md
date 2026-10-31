---
name: principle-guard-the-context-window
description: "Apply when context is filling up: large outputs, long files, repeated reads, fan-out planning. Route bulk to subagents; keep summaries in the main thread, not raw payloads."
disable-model-invocation: true
---
<!-- trailguide:installed fabrica:pstack:pstack/skills/principle-guard-the-context-window 699e1e7488c53c05ca13409f40da83cd37682091. Do not edit; trailguide project sync replaces this copy. -->

# Guard the Context Window

**Why:** Context overflow degrades reasoning quality, creates compression artifacts, and halts progress.

**Pattern:**
- **Size phases and cap scope.** Limit files per phase, set turn budgets, account for mechanism costs.
