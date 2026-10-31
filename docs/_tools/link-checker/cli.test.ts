/** @fileoverview Tests the documentation link checker as a command against isolated built-site fixtures. */

import { expect, test } from 'bun:test';
import {
  mkdtempSync,
  mkdirSync,
  writeFileSync,
  symlinkSync,
  rmSync,
} from 'node:fs';
import { join, dirname } from 'node:path';
import { tmpdir } from 'node:os';
import { spawnSync } from 'node:child_process';

/** Run the Bun command against an isolated built site, then remove all temporary files. */
function checkSite(
  pages: Readonly<Record<string, string>>,
  symlinks: Readonly<Record<string, string>> = {},
): {
  status: number | null;
  stdout: string;
  stderr: string;
} {
  const workspace = mkdtempSync(join(tmpdir(), 'trailguide-links-'));
  try {
    const site = join(workspace, 'dist');
    for (const [path, content] of Object.entries(pages)) {
      const file = join(site, path);
      mkdirSync(dirname(file), { recursive: true });
      writeFileSync(file, content);
    }
    for (const [path, target] of Object.entries(symlinks)) {
      const file = join(site, path);
      mkdirSync(dirname(file), { recursive: true });
      symlinkSync(target, file);
    }
    const result = spawnSync(
      process.execPath,
      [join(import.meta.dir, 'cli.ts'), site],
      { cwd: workspace, encoding: 'utf8' },
    );
    if (result.error !== undefined) throw result.error;
    return {
      status: result.status,
      stdout: result.stdout,
      stderr: result.stderr,
    };
  } finally {
    rmSync(workspace, { recursive: true, force: true });
  }
}

test('should accept page-relative, root-relative, encoded, directory, and same-page links', () => {
  const result = checkSite({
    'index.html':
      '<h1 id="start">Home</h1><a href="#start">self</a><a href="/guide/#section%20one">root</a><a href="guide/">directory</a><a href="a%20file.txt?download=1">asset</a><a href="https://example.com/missing">external</a><a href="//example.com/missing">external</a><a href="mailto:example@example.com">email</a>',
    'guide/index.html':
      '<h1 id="section one">Guide</h1><a href="../#start">up</a><a href="?mode=1#section%20one">self</a>',
    'a file.txt': '',
  });
  expect(result.status).toBe(0);
  expect(result.stdout).toContain(
    'Checked local links and fragments in 2 pages.',
  );
  expect(result.stderr).toBe('');
});

test('should report every broken local link with its source page', () => {
  const result = checkSite({
    'index.html':
      '<a href="missing/">missing</a><a href="#absent">anchor</a><a href="../outside">escape</a>',
  });
  expect(result.status).toBe(1);
  expect(result.stderr).toContain('index.html: missing destination: missing/');
  expect(result.stderr).toContain('index.html: missing anchor: #absent');
  expect(result.stderr).toContain('index.html: outside output: ../outside');
});

test('should fail explicitly when there are no built pages', () => {
  const result = checkSite({});
  expect(result.status).toBe(1);
  expect(result.stderr).toContain(
    'No built pages found. Run bun run docs:build first.',
  );
});

test('should parse HTML attributes and entities without reading markup inside comments or scripts', () => {
  const result = checkSite({
    'index.html':
      '<!-- <a href="missing-comment"> --> <script>const text = \'<a href="missing-script">\';</script><H1 ID="a&b">Target</H1><A HREF="#a&amp;b">entity</A><a\n href=guide.html#ok>unquoted</a>',
    'guide.html': '<p id=ok>Target</p>',
  });
  expect(result.status).toBe(0);
  expect(result.stderr).toBe('');
});

test('should preserve literal plus signs and tolerate literal percent signs in paths', () => {
  const result = checkSite({
    'index.html': '<a href="a+b.txt">plus</a><a href="100%.txt">percent</a>',
    'a+b.txt': '',
    '100%.txt': '',
  });
  expect(result.status).toBe(0);
});

test('should reject encoded traversal beyond the built site', () => {
  const result = checkSite({
    'index.html': '<a href="%2e%2e/outside">escape</a>',
  });
  expect(result.status).toBe(1);
  expect(result.stderr).toContain('index.html: outside output: %2e%2e/outside');
});

test('should reject a symlink destination outside the built site', () => {
  const result = checkSite(
    { 'index.html': '<a href="outside">escape</a>' },
    { outside: tmpdir() },
  );
  expect(result.status).toBe(1);
  expect(result.stderr).toContain('index.html: outside output: outside');
});

test('should reject a directory link when its index page is missing', () => {
  const result = checkSite({
    'index.html': '<a href="empty/">empty</a>',
    'empty/asset.txt': '',
  });
  expect(result.status).toBe(1);
  expect(result.stderr).toContain('index.html: missing destination: empty/');
});

test('should check anchor destinations inside template content', () => {
  const result = checkSite({
    'index.html': '<template><a href="missing-template">link</a></template>',
  });
  expect(result.status).toBe(1);
  expect(result.stderr).toContain(
    'index.html: missing destination: missing-template',
  );
});

test('should ignore external links with URL whitespace', () => {
  const result = checkSite({
    'index.html':
      '<a href=" https://example.com/missing">external</a><a href="https:\n//example.com/missing">external</a>',
  });
  expect(result.status).toBe(0);
});

test('should check absolute same-site URLs against the build including fragments', () => {
  const result = checkSite({
    'index.html':
      '<a href="https://trailguide.fabricahq.com/guide/#ok">good</a><a href="//trailguide.fabricahq.com/absent/">missing</a><a href="http://trailguide.fabricahq.com/guide/#missing">anchor</a>',
    'guide/index.html': '<a name="ok">Named anchor</a>',
  });
  expect(result.status).toBe(1);
  expect(result.stderr).not.toContain('missing anchor: https://');
  expect(result.stderr).toContain(
    'missing destination: //trailguide.fabricahq.com/absent/',
  );
  expect(result.stderr).toContain(
    'missing anchor: http://trailguide.fabricahq.com/guide/#missing',
  );
});
