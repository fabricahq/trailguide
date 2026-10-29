# Trail Guide documentation

The [Fabrica Trail Guide](https://trailguide.fabricahq.com) website documents the Go CLI and its file formats.

The site uses Astro and Starlight, with human-facing guides, a reference, and a dedicated “For agents” section.
Its visual direction follows [FM Linear's documentation](https://github.com/josh-padnick/fm-linear/tree/main/docs): monochrome typography, generous spacing, and rounded example panels.
The starting design reference is commit `bbb6bb3f1b94d0506120277179128066270f2e3b`.
Shared theme tokens and the accessible-callout transform are adapted from that project.
Homepage content and illustrations are original Trail Guide examples; no FM Linear brand imagery is included.

## Run locally

From the repository root:

```sh
bun install --frozen-lockfile
bun run docs:dev
```

Use the local URL printed by Astro.

```sh
bun run docs:check
bun run docs:build
bun run docs:links
bun run docs:preview
```

The output is `docs/dist/`.
The rendered-link checker lives in `_tools/link-checker/`. `cli.ts` owns command arguments and exit status; `check-site.ts` coordinates validation. HTML parsing, external requests, public-network restrictions, and their tests stay inside that folder.
The root TypeScript, lint, and test commands include these Bun scripts; Astro checks the site code.
`bun run docs:links` uses Bun and parse5 to check local destinations and fragments in the built HTML.
For a different build directory, run `bun docs/_tools/link-checker/cli.ts <directory>` from the repository root.
The production build includes the search index.
These commands do not publish the site.

## Publishing

The Documentation workflow publishes the site, including `install.sh`, to GitHub Pages at <https://trailguide.fabricahq.com> after a push to `main` passes every docs check, including the external link check. Pull requests and other branches only run the checks.
The site is served at the domain root, and page links are root-relative; do not set Astro's `base`.
GitHub Pages settings, the custom domain, and its DNS record are managed in Fabrica's infrastructure repository, not here.
Every documentation page loads Cloudflare Web Analytics, which counts visits without cookies, only when served from trailguide.fabricahq.com, so local builds and tests report nothing. `src/analytics.mjs` builds that script, and `astro.config.mjs` holds the public site token from the Cloudflare dashboard. The footer links to Fabrica's privacy policy, which describes the analytics.

## Maintain the docs

Write pages under `src/content/docs/` in folders matching the sidebar sections: `start-here/`, `concepts/`, `guides/`, `reference/`, and `for-agents/`.
Keep the homepage at `index.mdx` and navigation in `astro.config.mjs` aligned with page slugs. When moving a public page, update links and add a redirect from its old URL.
Homepage composition lives in `src/components/HomePage.astro` and its illustration components.

### Brand assets

The Trailguide mark is a folded map with two rule lines in its center panel.
The [brand reference](src/content/docs/reference/brand.mdx) explains how to use it and links the downloadable logo exports in `public/brand/`.
The header uses a text-only product name. The homepage hero uses the approved compact horizontal SVGs, switching between dark and light ink by theme, and `src/components/BrandAsset.astro` renders the reference page's previews.
The site's favicons at the root of `public/` (`favicon.svg`, `favicon.ico`, and `apple-touch-icon.png`) come from the brand package; its favicon and app icon exports are deliberately not published for download because they identify Trail Guide itself.
Pages link only `favicon.svg`, which switches ink with the browser's color scheme. Don't link `favicon.ico` too: Chrome prefers an ICO that lists its sizes and shows its dark tile in light mode. `favicon.ico` stays at the site root for browsers and tools that request it by convention.

### Styling

Follow the [CSS and Tailwind guidelines](_internal/css.md) for styling ownership, responsive breakpoints, design tokens, and reuse.

### Product documentation

Use the docs to explain the product experience and its contracts.
Keep implementation tasks in the internal planning material.
Keep release availability accurate in the installation guide.

Before handing off visual changes, inspect desktop and narrow layouts, light and dark themes, keyboard navigation, and search in a production preview.
Run `bun run check` to verify website/tooling formatting, lint, types, tests, the static build, and links between built pages. It doesn't run Go tests: `go test ./internal/build` renders the copyable rule examples through the current resolver and renderer.

### Link checks

`bun run check` checks links and anchors within the built site, including absolute URLs to `trailguide.fabricahq.com`. To also check external links, run `bun run docs:build && bun run docs:links:all` from the repository root.

The Documentation workflow runs both checks on every push and pull request. A broken link fails the workflow, even when the change does not touch a Markdown file.

The external check follows redirects and verifies anchors in static HTML. It permits only public network destinations, including after redirects. It fetches each URL once, retries temporary failures, and exits with an error listing the source pages and failing links. HTTP errors (including access denials), timeouts, and oversized HTML responses fail the check; they are never silently treated as valid links.

Checks cover rendered hyperlinks, including linked downloads. URLs in code examples are not hyperlinks. Email links and other non-HTTP schemes are skipped. Fragments in non-HTML downloads are not checked; anchors created only by client-side JavaScript cannot be verified. Do not add broad exclusions to hide a failed check: fix the link, use a stable destination, or retry if the site is temporarily unavailable.
