/** @fileoverview Exercises external link validation through real HTTP responses and the checker command. */

import { expect, test } from 'bun:test';
import { mkdtempSync, writeFileSync, rmSync } from 'node:fs';
import { join } from 'node:path';
import { tmpdir } from 'node:os';
import { checkExternalLinks } from './external';
import { pageLinks } from './html';

test('should check redirects, fragments, failures, retries, and duplicate URLs over HTTP', async () => {
  const requests = new Map<string, number>();
  const server = Bun.serve({
    hostname: '127.0.0.1',
    port: 0,
    fetch(request) {
      const path = new URL(request.url).pathname;
      const count = (requests.get(path) ?? 0) + 1;
      requests.set(path, count);
      expect(request.headers.get('authorization')).toBeNull();
      expect(request.headers.get('cookie')).toBeNull();
      if (path === '/redirect')
        return Response.redirect(new URL('/page', request.url), 302);
      if (path === '/loop') return Response.redirect(request.url, 302);
      if (path === '/missing') return new Response('', { status: 404 });
      if (path === '/denied') return new Response('', { status: 403 });
      if (path === '/unavailable' || (path === '/retry' && count === 1))
        return new Response('', { status: 503 });
      if (path === '/file')
        return new Response('file', {
          headers: { 'Content-Type': 'application/pdf' },
        });
      return new Response(
        '<h1 id="a&b">Title</h1><a name="legacy">Old anchor</a>',
        { headers: { 'Content-Type': 'text/html' } },
      );
    },
  });
  try {
    const base = server.url.href;
    const links = [
      'page#a%26b',
      'page#absent',
      'page#legacy',
      'redirect#a%26b',
      'missing',
      'denied',
      'retry',
      'unavailable',
      'loop',
      'file#page=2',
    ];
    const pages = new Map([
      [
        '/site/index.html',
        pageLinks(
          links.map((path) => `<a href="${base}${path}">link</a>`).join(''),
        ),
      ],
      [
        '/site/other.html',
        pageLinks(`<a href="${base}page#a%26b">duplicate</a>`),
      ],
    ]);
    const errors = await checkExternalLinks('/site', pages, (url, signal) =>
      fetch(url, { signal, redirect: 'manual' }),
    );
    expect(errors).toHaveLength(5);
    expect(errors.join('\n')).toContain('index.html: missing external anchor:');
    expect(errors.join('\n')).toContain('HTTP 404');
    expect(errors.join('\n')).toContain('HTTP 403');
    expect(errors.join('\n')).toContain('HTTP 503');
    expect(errors.join('\n')).toContain('excessive redirects');
    expect(requests.get('/page')).toBe(2); // One direct request and one redirect, regardless of references/fragments.
    expect(requests.get('/missing')).toBe(1);
    expect(requests.get('/retry')).toBe(2);
    expect(requests.get('/unavailable')).toBe(3);
  } finally {
    server.stop(true);
  }
}, 10_000);

test('should report malformed URLs and reject embedded credentials', async () => {
  const pages = new Map([
    [
      '/site/index.html',
      pageLinks(
        '<a href="https://[invalid">bad</a><a href="https://user:password@example.com/">credentials</a>',
      ),
    ],
  ]);
  const errors = await checkExternalLinks('/site', pages);
  expect(errors).toHaveLength(2);
  expect(errors.join('\n')).toContain('invalid URL');
  expect(errors.join('\n')).toContain('embedded credentials');
});

test('should reject private destinations in the command and succeed after the link is fixed', async () => {
  const root = mkdtempSync(join(tmpdir(), 'docs-http-links-'));
  const server = Bun.serve({
    hostname: '127.0.0.1',
    port: 0,
    fetch: () => new Response('', { status: 404 }),
  });
  try {
    writeFileSync(
      join(root, 'index.html'),
      `<a href="${server.url}missing">broken</a>`,
    );
    const command = [
      process.execPath,
      join(import.meta.dir, 'cli.ts'),
      root,
      '--external',
    ];
    const broken = Bun.spawn(command, { stdout: 'pipe', stderr: 'pipe' });
    expect(await broken.exited).toBe(1);
    expect(await new Response(broken.stderr).text()).toContain(
      'index.html: non-public destination:',
    );
    writeFileSync(
      join(root, 'index.html'),
      '<h1 id="ok">Title</h1><a href="#ok">fixed</a>',
    );
    const fixed = Bun.spawn(command, { stdout: 'pipe', stderr: 'pipe' });
    expect(await fixed.exited).toBe(0);
  } finally {
    server.stop(true);
    rmSync(root, { recursive: true, force: true });
  }
});
