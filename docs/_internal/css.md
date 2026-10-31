# CSS and Tailwind guidelines

This internal guide owns styling conventions for the documentation site and homepage.
Keep contributor guidance here; public documentation lives in `src/content/docs/`.
Paths below are relative to `docs/`.

## Use Tailwind for component styling

Use Tailwind utilities for layout, spacing, typography, and interaction states on markup we own.
The site uses Tailwind v4 through its Vite plugin and Starlight’s Tailwind integration.

- Prefer Tailwind’s spacing and type scales. Use arbitrary values for specific illustration details when the scale does not fit.
- Use semantic colors such as `text-ink`, `text-muted`, and `bg-surface`. They follow Starlight’s saved theme and automatic system preference.
- Keep complete utility names in source so Tailwind can find them. Select between complete strings instead of constructing partial class names.
- Include keyboard focus styles alongside hover styles. Respect reduced-motion preferences for transitions and animations.
- Keep one-off treatments in the component. Extract a shared class only for an established, repeated visual treatment, such as our buttons or asides.
- Use component CSS for generated markup and state-dependent illustration behavior that utilities cannot express clearly.

## Style ownership and cascade

| Location | Responsibility |
| --- | --- |
| [tailwind.css](../src/styles/tailwind.css) | CSS layer order, utility scanning, shared breakpoints, token aliases, and shared button, section-label, and aside styles |
| [custom.css](../src/styles/custom.css) | Light/dark theme values and overrides for generated Starlight navigation and Markdown content |
| [home.css](../src/styles/home.css) | Generated Starlight homepage shell and rule-example code markup |
| Component styles | Private illustration palettes, playback visibility, and generated syntax highlighting |

Starlight owns the reset. Do not add Tailwind Preflight alongside it.
The layer order is `base, starlight, theme, components, utilities`.
Put shared utility-based treatments in the `components` layer so utilities can override them.
Unlayered CSS takes precedence over normal layered declarations; keep framework overrides targeted rather than adding broad global selectors.

## Responsive layouts

Use the shared responsive variants instead of inline viewport queries.
Our names use Starlight’s cutoffs, not Tailwind’s default breakpoint values:

| Variant | Minimum width | Starlight alignment |
| --- | --- | --- |
| `sm:` | 30rem (480px) | Smaller file-tree cutoff |
| `md:` | 50rem (800px) | Desktop navigation and sidebar |
| `lg:` | 72rem (1152px) | Wide layout and right sidebar |

The definitions live together in `src/styles/tailwind.css`; pixel equivalents assume the default browser font size.
Start with the narrow layout, then add variants for wider screens.
Use `max-sm:` and `max-md:` for smaller layouts, or stack variants such as `sm:max-md:` for a range.
Tailwind’s default breakpoint scale is disabled to avoid mixing conventions.

Starlight hardcodes its own media queries.
When upgrading it, check our definitions against the installed `style/util.css` and `user-components/FileTree.astro`.

## Design tokens

A token represents a design decision that should change consistently across its consumers.
Choose it by purpose, not by its current hex value. Equal values alone do not justify sharing a token.

### Shared site tokens

`src/styles/custom.css` owns light/dark values. `src/styles/tailwind.css` exposes reusable roles as utilities.

| Token / utility suffix | Purpose |
| --- | --- |
| `paper` | Page background and surfaces that match the page |
| `surface` | Subtle background for grouped content and hover states |
| `surface-header` | Stronger background for illustration title bars, distinct from their content surfaces |
| `ink` | Primary foreground, including text and icons |
| `muted` | Secondary foreground, including captions and metadata |
| `border` | Normal component boundaries |
| `border-subtle` | Low-emphasis section separators |
| `border-strong` | Stronger boundaries, such as outlined actions |
| `primary-action` / `on-primary-action` | Filled primary action and its contrasting foreground; use together |
| `rounded-small` | Small-radius geometry for compact controls and badges (6px) |
| `rounded-card` | Content-card geometry (12px) |
| `rounded-panel` | Large-panel geometry (20px) |

Examples: `bg-paper`, `text-muted`, `border-border-subtle`, and `bg-primary-action text-on-primary-action`.
Radius names describe geometry or surface scale, not a mandatory component type. A badge and a compact button may share `rounded-small`; a pill-shaped action uses `rounded-full`.

The global fallback focus ring (`--cr-focus-ring`) and current-page marker (`--cr-current-page-indicator`) have independent roles even though their colors currently match. Component focus treatments may use a stronger foreground outline.
Starlight's `--sl-*` names are a framework API: retain those names at the integration boundary.
Use Tailwind's spacing and type scale for new component layout; avoid introducing a parallel global scale for one-off values.
The retained `--cr-text-sm`, `--cr-text-body`, and `--cr-line-height-body` values style generated documentation markup. `--cr-font-mono` styles examples; Starlight's inline-code font remains separate deliberately.

### Component-private tokens

These variables are implementation details of their owning components. They are not exported in the global Tailwind color theme.

- **AgentSetup:** `--setup-agent` identifies the agent and `--setup-rule-path` emphasizes the path in its instructions.
- **CodeSample:** `--sample-token-*` names follow Shiki's syntax roles. Each tone owns its complete palette. Syntax colors must not borrow command-success, error, or button colors. Callers may set `--sample-font-size` and `--sample-padding` on their wrapper; syntax variables stay inside the renderer.

### Adding or reusing a token

1. Reuse an existing token when both consumers represent the same role and should change together in both themes.
2. Keep a component-specific decision local. Use a component prefix and a role name, such as `--setup-rule-path`, rather than a hue such as `--setup-blue`.
3. Promote a local role only when multiple independent components need the same design contract. Define both theme values, document its consumers, then expose a Tailwind alias.
4. Keep equal-valued roles separate when they need to evolve independently. A green string literal, passing test, and added diff line are three different meanings.
5. Remove unused tokens rather than leaving speculative APIs. Do not add a global token for every literal spacing, color mix, or illustration detail.
6. Verify changes in both themes and at the shared breakpoints. A naming-only refactor should preserve computed styles and layout.


## Validate styling changes

Run `bun run check` from the repository root for formatting, lint, types, tests, the static build, and rendered links.
See [site development](../README.md#run-locally) for the production preview commands.

For visual changes:

- Inspect the homepage and affected documentation pages in light and dark themes.
- Check narrow screens and widths immediately below and at each affected breakpoint.
- Verify keyboard navigation, visible focus, search, and any changed interactive states.
- Check for unintended text wrapping, horizontal overflow, and nested scroll areas.
- Capture before/after screenshots at matching widths and states. For token renames, also compare computed styles to confirm appearance is unchanged.
