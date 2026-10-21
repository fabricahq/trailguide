/** @fileoverview Configures the Trail Guide documentation site, navigation, and Markdown rendering. */

import { defineConfig } from 'astro/config';
import { unified } from '@astrojs/markdown-remark';
import starlight from '@astrojs/starlight';
import tailwindcss from '@tailwindcss/vite';
import accessibleAsideTitles from './src/plugins/accessible-aside-titles.mjs';
import { cloudflareWebAnalytics } from './src/analytics.mjs';

// Served at the domain root; page links are root-relative.
const site = 'https://trailguide.fabricahq.com';

export default defineConfig({
  site,
  devToolbar: { enabled: false },
  // Keep native bindings outside the SSR bundle: https://vite.dev/config/ssr-options.html#ssr-external
  // Satteri's installed index.js resolves platform binaries relative to its package.
  vite: { plugins: [tailwindcss()], ssr: { external: ['satteri'] } },
  redirects: {
    '/guides/customize/': '/guides/select-rules/',
    '/overview/': '/start-here/overview/',
    '/status/': '/start-here/install/',
    '/guides/install/': '/start-here/install/',
    '/guides/set-up-project/': '/start-here/set-up-project/',
    '/guides/use-rules/': '/start-here/set-up-project/',
    '/start-here/use-rules/': '/start-here/set-up-project/',
    '/guides/create-library/': '/start-here/create-library/',
    '/reference/rule-library-format/': '/reference/rule-format/',
  },
  markdown: { processor: unified({ rehypePlugins: [accessibleAsideTitles] }) },
  integrations: [starlight({
    title: 'Trail Guide',
    description: 'The package manager for your agent skills and rules',
    favicon: '/favicon.svg',
    head: [
      { tag: 'link', attrs: { rel: 'icon', href: '/favicon.ico', sizes: '32x32' } },
      { tag: 'link', attrs: { rel: 'apple-touch-icon', href: '/apple-touch-icon.png' } },
      // The Web Analytics site token is public and grants no access.
      cloudflareWebAnalytics({ host: new URL(site).hostname, token: 'ad5e47e228f942f7a16b047a95854176' }),
    ],
    disable404Route: true,
    customCss: ['./src/styles/tailwind.css', './src/styles/custom.css', './src/styles/home.css'],
    components: {
      Hero: './src/components/HomePage.astro',
      SiteTitle: './src/components/SiteTitle.astro',
      SocialIcons: './src/components/NavLinks.astro',
      ThemeSelect: './src/components/ThemeSelect.astro',
      Footer: './src/components/Footer.astro',
    },
    // Open the active section initially; Starlight remembers subsequent toggles in this tab.
    sidebar: [
      { label: 'Start here', collapsed: true, items: [
        { label: 'What is Trail Guide?', slug: 'start-here/overview' },
        { label: 'Install Trail Guide', slug: 'start-here/install' },
        { label: 'Set up your first project', slug: 'start-here/set-up-project' },
        { label: 'Create your first library', slug: 'start-here/create-library' },
      ] },
      { label: 'Concepts', collapsed: true, items: [
        { label: 'Rule', slug: 'concepts/rule' },
        { label: 'Rule group', slug: 'concepts/groups' },
        { label: 'Skill', slug: 'concepts/skill' },
        { label: 'Library', slug: 'concepts/libraries' },
        { label: 'Skill', slug: 'concepts/skill' },
        { label: 'Project', slug: 'concepts/project' },
      ] },
      { label: 'Guides', collapsed: true, items: [
        { label: 'Write a rule', slug: 'guides/write-rules' },
        { label: 'Write a skill', slug: 'guides/write-skills' },
        { label: 'Projects', collapsed: true, items: [
          { label: 'Include rules', slug: 'guides/select-rules' },
          { label: 'Install skills', slug: 'guides/install-skills' },
          { label: 'Fork a skill', slug: 'guides/fork-skills' },
          { label: 'Update rules and skills', slug: 'guides/update' },
          { label: 'Resolve conflicting rules', slug: 'guides/conflicting-guidance' },
        ] },
        { label: 'Libraries', collapsed: true, items: [
          { label: 'Version your rules', slug: 'guides/version-rules' },
          { label: 'License rules', slug: 'guides/license-rules' },
          { label: 'Adapt a third-party rule', slug: 'guides/adapt-rules' },
        ] },
      ] },
      { label: 'Reference', collapsed: true, items: [
        { label: 'Projects', collapsed: true, items: [
          { label: 'Project files', slug: 'reference/files' },
          { label: 'Configuration', slug: 'reference/configuration' },
          { label: 'How sources work', slug: 'reference/imports' },
          { label: 'Sync and recovery', slug: 'reference/sync' },
          { label: 'Provenance', slug: 'reference/provenance' },
        ] },
        { label: 'Rules, skills, and libraries', collapsed: true, items: [
          { label: 'Rule and rule group format', slug: 'reference/rule-format' },
          { label: 'Rule rubric and template', slug: 'reference/rule-authoring' },
          { label: 'Skill format', slug: 'reference/skill-format' },
          { label: 'Library format', slug: 'reference/library-format' },
          { label: 'Rule and skill versions', slug: 'reference/rule-versions' },
        ] },
        { label: 'CLI commands', slug: 'reference/cli' },
      { label: 'Brand', slug: 'reference/brand' },
      ] },
      { label: 'For agents', collapsed: true, items: [{ label: 'Plan, write, and review', slug: 'for-agents' }] },
    ],
    tableOfContents: { minHeadingLevel: 2, maxHeadingLevel: 3 },
  })],
});
