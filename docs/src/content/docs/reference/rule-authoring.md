---
title: "Rule rubric and template"
description: "How to write a rule agents can understand and check it against the shared authoring standard."
---

A **rule** is a Markdown file that tells an agent what to do, when the instruction applies, and how to check its work. A useful rule gives enough context and examples for the agent to follow it without guessing what you meant.

Use this page when writing a new rule, improving an existing one, or reviewing a proposed rule. The **template** gives you a starting structure. The **rubric** is a checklist for deciding whether the rule is clear, useful, and verifiable.

The same template and rubric apply to rules in shared libraries and rules written for one project. This page explains how to complete the template and review the result. For the steps to create a rule, see [Write a rule](/guides/write-rules/). For the fields and files Trail Guide accepts, see [Rule and rule group format](/reference/rule-format/).

## What goes in a rule

A rule file has two parts:

- **Metadata:** the fields between the opening `---` lines, also called YAML frontmatter. They give the rule a title, explain when to read it, and describe why it matters.
- **Body:** the Markdown below those fields. It states the instruction, explains its scope and exceptions, and provides examples and ways to check the result.

The metadata helps agents find relevant guidance. The body tells them what following that guidance means.

## When to read, implement, and validate

Before writing, answer three questions:

| Question | Where to answer it |
| --- | --- |
| When should an agent read this? | Use the required `whenToRead` field to describe relevant work, including planning before any code exists. |
| What should the agent do? | State the instruction in the body. Add implementation steps or decisions when they help the agent follow it. |
| How can someone check the result? | Describe observable evidence, checks, and exceptions in the body. Add a Validation section when it contributes useful detail. |

Agents should read the complete rule, including its exceptions, before applying it. Implementation guidance helps with planning and changes; validation guidance helps with review, testing, and diagnosis. A task may need both.

A relevant rule is not evidence of a violation. Its full instruction and exceptions determine whether anything needs to change.

## Markdown template

Copy the template below and replace the placeholders. The [rule creation command](/start-here/set-up-project/) uses this same starting structure.

Keep sections that add useful information and remove unused prompts. You do not need a separate Implementation or Validation section if the instruction and examples already answer those questions.

````md
---
title: <Short action-oriented title>
whenToRead: 'Before [relevant activities] involving [specific behavior or artifact], such as [representative cases, if helpful].'
impact: <Level matching the credible consequence within this rule's scope>
impactDescription: <Specific consequence the rule helps prevent, supporting the impact level>
---

## <Short action-oriented title>

<State one concrete obligation.>
<Name version, runtime, or surrounding-contract prerequisites when they affect the advice.>

### Implementation

<Describe decisions or steps that help an agent write compliant code.>
<Include relevant exceptions and boundaries.>

### Rationale

<Explain the failure or tradeoff this rule addresses.>
<Explain the mechanism connecting the action to that consequence; cite supporting contracts where needed.>

### Examples

<Repeat the application block for each distinct known use of the rule.>
<Cover differences in behavior, constraints, or implementation choices; combine cases when the same pair explains them without losing useful guidance.>

#### Application: <Name the situation>

<Explain when this application occurs and which conditions matter.>

**Incorrect (counterexample):**

<Show a plausible mistake that violates the rule in this situation.>
<Explain what goes wrong.>

**Correct:**

<Show the preferred approach in the same situation, keeping unrelated details consistent.>
<Explain how it satisfies the rule and clarify illustrative choices.>
<When overapplication is likely, also show a similar-looking valid case and explain why it needs no change.>

### Validation

<Describe observable evidence or checks that establish compliance.>
<Identify plausible situations that are insufficient evidence of a violation.>
<Name any surrounding code or contracts the reviewer must inspect before deciding.>
````

The [rule metadata reference](/reference/rule-format/#rule-metadata) defines the required fields and accepted values. For a filled-in template, see the [complete rule example](/guides/write-rules/#example-rule).

You can add optional `tags` for search terms, such as `tags: [testing, cancellation]`. Tags do not determine whether a rule applies, and Trail Guide does not use them to filter rules. Omit tags that add no useful search terms.

### Keep narrow rules short

Size a rule to its obligation. A narrow rule, such as one API choice or one performance technique, can be complete with a sentence or two stating the obligation, a sentence on when it does not apply, one counterexample paired with the correct approach, and a short check. It does not need every template heading or several applications.

Add sections and applications when they carry information an agent would otherwise miss: a rationale that is not obvious, a context that changes how to follow the rule, or an exception that is easy to overlook. Length is not evidence of quality. A rule that repeats the same lesson in more words is harder to apply, and large rule groups are summarized in generated guidance, so each rule's title and `whenToRead` cue must stand on their own.

## Write whenToRead guidance that helps selection

The `whenToRead` field helps an agent decide whether to open a rule before it knows the rule's full contents. Describe work the agent can recognize from its task, rather than a defect it must first discover.

For example, this cue requires the agent to spot a problem before reading the advice:

> When orchestration is obscured by parsing.

A more useful cue describes the work itself:

> Before planning, writing, changing, or reviewing a function that coordinates multiple steps, such as parsing input, validating it, calling another operation, or constructing a result.

The second cue also applies to a function that is already well structured. Reading the full rule determines whether the agent should make a change.

### Distinguish rule group guidance from rule guidance

A rule group's `whenToRead` field describes an area of work. A rule's field describes the specific situations that warrant reading that rule.

For example, a code design practice group could say:

> Before planning, writing, changing, or reviewing how code is organized, how responsibilities are divided, or how functions and modules work together.

Describe the rule group's intended scope even if it contains only one rule. After opening a rule group, agents use each rule's cue to decide what to read next. Opening the rule group does not mean every rule applies.

### Make the cue recognizable

- **Name the work and its subject.** Identify the behavior, artifact, interface, or technology involved. “When coding” is too broad to distinguish relevant work.
- **Include work before implementation.** Mention planning or writing when relevant. Do not require existing code or a known defect before the cue can match.
- **Use examples to clarify scope.** “Such as parsing input or validating it” makes “multiple steps” concrete without turning the examples into an exhaustive list.
- **Cover distinct situations.** Include review and diagnosis when relevant. Describe behavior as well as files: testing guidance can matter even when no test files change.
- **Name important boundaries.** Mention a technology or context restriction when it prevents a likely selection mistake. Put detailed obligations and exceptions in the body.
- **Keep useful detail and remove repetition.** Prefer a focused sentence or two. There is no fixed word limit; the cue needs to distinguish relevant tasks.

Replace the template's activity and scope placeholders. Omit the “such as” clause when examples would add nothing.

### Check the cue against real tasks

Try representative tasks before accepting a cue:

1. A planned change within scope, with no code written yet, should lead to reading the rule.
2. A relevant review or diagnosis should also match, even when an expected test or other artifact is missing.
3. A relevant task phrased differently should still match. Examples should not become an exhaustive checklist.
4. A nearby task outside the rule's scope should be distinguishable. Changing only a color token would not match the function-design cue above.

Check rule selection separately from compliance. A well-structured function with several steps can warrant reading the rule without warranting a review finding.

When trying cues with an agent, record both missed relevant rules and unnecessary selections. Revise the cue using those cases. This is an authoring practice, not another metadata field or proof that every agent will select correctly.

We adapted these recommendations from the [Agent Skills specification](https://agentskills.io/specification#description-field) and Anthropic's [authoring](https://platform.claude.com/docs/en/agents-and-tools/agent-skills/best-practices#writing-effective-descriptions) and [evaluation](https://platform.claude.com/docs/en/agents-and-tools/agent-skills/enterprise#evaluating-skills-before-deployment) guidance. Those sources discuss skill discovery. Applying them to rule selection is a design inference, not a measured improvement in Trail Guide compliance.

## Describe impact through consequences

The `impact` field describes how significant the consequence is that a rule helps prevent. Choose a level based on credible consequences within the rule's scope, rather than an imaginable worst case.

Use `impactDescription` to explain that consequence in a focused sentence. Generated pages show it as **Why it matters**.

| Impact | Consequence the rule addresses |
| --- | --- |
| CRITICAL | Severe harm, such as irreversible data loss or a major security breach. |
| HIGH | Substantial correctness, reliability, or maintainability problems. |
| MEDIUM | Meaningful but bounded defects or recurring development friction. |
| LOW | Local clarity or consistency improvements with limited consequences. |

`MEDIUM-HIGH` and `LOW-MEDIUM` sit between the adjacent levels.

The description must support the chosen level. “Important for quality” does not explain a consequence. For a rule about expressing operations as meaningful steps, a useful description is:

> Mixing orchestration with low-level details can hide important decisions and make behavior harder to verify or change.

Impact is not a filter for which rules to follow. Agents should follow every applicable rule, including its exceptions, regardless of impact.

Impact also does not resolve conflicting rules or set the severity of a review finding. Judge a finding using the evidence and consequences of that specific violation. A high-impact design rule does not make every readability issue severe or require extracting every function that performs several steps.

## Write examples that explain the difference

Examples should help an agent recognize both a plausible mistake and the preferred approach. Use code consistent with the relevant technology and project conventions.

### Cover distinct applications

Identify the situations the rule covers before choosing examples. For each distinct known application, aim to show an incorrect/correct pair addressing the same situation.

Add another pair when a different context changes how to follow the rule, what can go wrong, or which exception matters. For example, preserving errors in return values and preserving them across asynchronous callbacks may need different examples.

One pair is enough when it explains multiple applications without hiding meaningful differences. Renaming variables or repeating the same lesson in another language does not by itself require another pair.

There is no fixed example count, and you do not need to invent every possible future use. Unlisted situations can still fall within the rule's stated scope. Add coverage when a new application reveals a gap.

### Make counterexamples useful

A **counterexample** shows a plausible violation and explains what goes wrong. Put the corrected approach nearby and keep unrelated details consistent so the important difference is clear.

Label choices that are only illustrative. For example, a testing practice demonstrated in Go does not automatically apply only to Go.

When a rule is easy to overapply, also show a similar-looking case that is already valid. Explain why it needs no change; not every rule needs this third example.

For a rule about meaningful operation steps, a short, cohesive function can remain inline while performing several steps. The issue is whether low-level details hide the operation, not the number of actions or lines.

## Explain how to check the rule

Name observable evidence or a specific check command. “Verify that this rule is followed” and “review the code” do not tell a reviewer what to inspect.

A rule can govern plans, code, tests, documentation, or another engineering artifact. Match the check to the instruction; not every rule needs an automated test.

### Check the advice and its prerequisites

Use these questions to find missing context. They do not require additional metadata or separate body sections.

- **Why does the advice work?** Explain how the recommended action prevents the stated consequence. Distinguish a team's preference from a general correctness claim.
- **When is the advice true?** State relevant versions, runtime modes, framework behavior, and caller contracts. A dependency elsewhere in the repository does not prove those conditions hold here.
- **What else must a reviewer inspect?** Identify surrounding code, configuration, or contracts needed before deciding that a rule was violated.
- **What similar-looking case is valid?** Explain the boundary that prevents an unnecessary change.

Keep exceptions separate from uncertainty. “This case is allowed” differs from “the available evidence cannot establish a violation.” A tool's inability to inspect a wrapper or file does not make the code exempt from the written rule.

Put prerequisites and instructions in the body. Mention them in `whenToRead` only when they help agents decide to read the rule.

## Supporting material

Keep the instruction and its exceptions in the rule itself. Put optional explanations, images, and sample data in the adjacent `assets/<rule-name>/` directory. Put material shared by several rules in the library's `.trailguide/assets/` directory.

Link to supporting files using ordinary Markdown. Markdown in an asset directory is supporting text, not another rule.

Each rule must remain independently selectable. Do not link to another rule document on disk, another rule's private assets, or arbitrary repository documents. This restriction also applies to links inside attachments, regardless of which rules a project selects or excludes. Move shared supporting explanations into shared assets.

See [Supporting assets](/reference/rule-format/#supporting-assets) for the required layout and examples.

When adapting someone else's material, preserve source attribution and required notices. Declare one license for the whole library in its manifest; license overrides on a single rule or rule group are unsupported. Use optional [structured attribution](/reference/rule-format/#rule-attribution) for source credits. Follow [Adapt third-party rules](/guides/write-rules/#adapt-third-party-rules) for material that does not already use the Trail Guide format.

## Record changes to a library rule

After the first library release, every edit to a rule needs a [change note](/reference/rule-versions/#change-notes) that says how much the obligation changed. Projects use it to decide whether they can update without changing their code. See [Choose a version change](/reference/rule-versions/#choose-a-version-change) for choosing major, minor, or patch and writing the summary, and [Version your rules](/guides/version-rules/#change-rules-after-the-first-library-release) for the commands.

## Authoring rubric

Use this checklist to review every rule. A completed template is a starting point; headings alone do not establish that the guidance is useful.

| Criterion | What to look for |
| --- | --- |
| One obligation | State one expectation a project can adopt, review, or replace independently. Split unrelated obligations into separate rules. |
| Clear action | Tell the agent what to do. Replace vague advice such as “use good error handling” with observable behavior. |
| Discoverable relevance | Use [recognizable selection guidance](#write-whentoread-guidance-that-helps-selection), with useful examples and checks against relevant and out-of-scope tasks. |
| Explicit scope | State conditions and exceptions in the body. An example's language does not silently restrict a practice rule to that language. |
| Supported impact | Choose a level and name a [credible consequence](#describe-impact-through-consequences). Keep review-finding severity dependent on evidence. |
| Useful rationale | Explain the failure or tradeoff the instruction addresses. Keep background context from introducing hidden requirements. |
| Concrete examples | Cover distinct known applications with incorrect/correct pairs. Explain the situation and decisive difference, and label illustrative choices. |
| Counterexamples | Show plausible mistakes and explain what goes wrong. Pair each with a clearly labeled correct approach to the same situation. |
| Verifiable compliance | Name the code, behavior, test, or other evidence that demonstrates compliance. Match the check to the instruction. |
| Honest claims | Support factual claims and retain source attribution where needed. Distinguish team preferences from universal requirements. |
| Complete meaning | Make the instruction understandable with its stated context. Keep the obligation, scope, and checks visible instead of hiding them behind unexplained references. |

Evaluate every criterion and revise missing or unclear guidance where it applies. Add headings only when they help convey that guidance.

## Review a proposed rule

For each unmet criterion:

1. Identify the passage that needs work.
2. Explain what is ambiguous, missing, or unsupported.
3. Suggest a concrete revision.

For example, if a Validation section only says “review the code,” identify the behavior or surrounding contract the reviewer should check.

The rubric evaluates the quality of the instruction. The library owner chooses the engineering policy. Keep writing-quality findings separate from disagreements with that policy, and ask the author when the intended policy is unclear.
