/**
 * @fileoverview Pure decisions for pull request documentation previews: ports, state paths, and request routing.
 * The preview server copies this file beside itself, so it imports only Node built-ins.
 */

import { extname, join, resolve, sep } from 'node:path';

// Previews listen on 26000-26499, clear of the app and website preview ranges on
// the same runner host. Two open PRs collide only when their numbers differ by 500.
const portBase = 26000;
const portRange = 500;

/** Parses a positive PR number, rejecting anything else. */
export function parsePullRequest(raw: string | undefined): number {
  const text = raw ?? '';
  const pr = Number(text);
  if (!/^\d+$/.test(text) || !Number.isSafeInteger(pr) || pr <= 0) {
    throw new Error(`PREVIEW_PR must be a positive PR number, got "${text}"`);
  }
  return pr;
}

/** Returns the fixed port for a PR, so every job for it agrees without shared state. */
export function previewPort(pr: number): number {
  return portBase + (pr % portRange);
}

/** Returns the directory that holds one PR's copied site, pid, and log. */
export function previewStateDir(previewsDir: string, pr: number): string {
  return join(previewsDir, `pr-${pr}`);
}

/** Returns the PR number for a state directory name, or null for other entries. */
export function pullRequestForStateDir(name: string): number | null {
  const match = /^pr-(\d+)$/.exec(name);
  return match ? Number(match[1]) : null;
}

/** Builds the URL reviewers open from the tailnet. */
export function previewURL(host: string, pr: number): string {
  return `http://${host}:${previewPort(pr)}/`;
}

const contentTypes: Record<string, string> = {
  '.css': 'text/css; charset=utf-8',
  '.html': 'text/html; charset=utf-8',
  '.ico': 'image/x-icon',
  '.jpg': 'image/jpeg',
  '.js': 'text/javascript; charset=utf-8',
  '.json': 'application/json; charset=utf-8',
  '.mjs': 'text/javascript; charset=utf-8',
  '.png': 'image/png',
  '.sh': 'text/plain; charset=utf-8',
  '.svg': 'image/svg+xml',
  '.txt': 'text/plain; charset=utf-8',
  '.wasm': 'application/wasm',
  '.webp': 'image/webp',
  '.woff2': 'font/woff2',
  '.xml': 'application/xml; charset=utf-8',
};

/** Returns the Content-Type for a served file. */
export function contentType(path: string): string {
  return (
    contentTypes[extname(path).toLowerCase()] ?? 'application/octet-stream'
  );
}

/**
 * Maps a request path into the site root, or returns null when the decoded path escapes it.
 * Traversal, malformed encodings, NUL bytes, and sibling directories that only share the
 * root as a string prefix are all rejected.
 */
export function resolveSitePath(
  root: string,
  rawPathname: string,
): string | null {
  let pathname: string;
  try {
    pathname = decodeURIComponent(rawPathname);
  } catch {
    return null;
  }
  if (pathname.includes('\0')) return null;
  const resolvedRoot = resolve(root);
  const candidate = resolve(resolvedRoot, `.${pathname}`);
  if (candidate !== resolvedRoot && !candidate.startsWith(resolvedRoot + sep)) {
    return null;
  }
  return candidate;
}

/** A path's type on disk, or null when it does not exist. */
export type PathKind = 'file' | 'directory' | null;

/** How the server answers one request. */
export type Route =
  | { status: 200; file: string }
  | { status: 301; location: string }
  | { status: 404 };

/**
 * Decides how to answer a request the way GitHub Pages does, so previews match production:
 * directories redirect to a trailing slash and serve index.html, extensionless paths fall
 * back to .html, and anything else is a 404.
 */
export function routeRequest(
  root: string,
  url: URL,
  kindOf: (path: string) => PathKind,
): Route {
  const path = resolveSitePath(root, url.pathname);
  if (path === null) return { status: 404 };
  const kind = kindOf(path);
  if (kind === 'file') return { status: 200, file: path };
  if (kind === 'directory') {
    if (!url.pathname.endsWith('/')) {
      return { status: 301, location: `${url.pathname}/${url.search}` };
    }
    const index = join(path, 'index.html');
    return kindOf(index) === 'file'
      ? { status: 200, file: index }
      : { status: 404 };
  }
  if (!url.pathname.endsWith('/') && kindOf(`${path}.html`) === 'file') {
    return { status: 200, file: `${path}.html` };
  }
  return { status: 404 };
}
