/** @fileoverview Checks local destinations and fragments in built documentation using parsed HTML. */

import {
  existsSync,
  readFileSync,
  readdirSync,
  realpathSync,
  statSync,
} from 'node:fs';
import {
  basename,
  dirname,
  isAbsolute,
  join,
  relative,
  resolve,
  sep,
} from 'node:path';
import { unescape as decodePercentEscapes } from 'node:querystring';
import { pageLinks } from './html';
import type { PageLinks } from './html';
import { checkExternalLinks } from './external';

/** A local URL's path and fragment, with query parameters omitted. */
type LocalReference = { readonly path: string; readonly fragment: string };

/** Discover HTML files without following symlinked directories outside the built site. */
function htmlFiles(root: string): ReadonlyArray<string> {
  const paths: Array<string> = [];
  /** Walk physical subdirectories only; symlink destinations are checked when referenced by a link. */
  function visit(directory: string): void {
    for (const entry of readdirSync(directory, { withFileTypes: true })) {
      if (entry.isSymbolicLink()) continue;
      const path = join(directory, entry.name);
      if (entry.isDirectory()) visit(path);
      else if (entry.isFile() && entry.name.endsWith('.html')) paths.push(path);
    }
  }
  if (existsSync(root)) visit(root);
  return paths.sort();
}

/** Load built HTML pages in path order, returning no pages when the build directory is absent. */
function builtPages(root: string): ReadonlyMap<string, PageLinks> {
  return new Map(
    htmlFiles(root).map((path) => [
      path,
      pageLinks(readFileSync(path, 'utf8')),
    ]),
  );
}

/** Split local references without URL normalization hiding traversal outside the output root. */
function localReference(rawHref: string): LocalReference | undefined {
  let href = rawHref.trim().replace(/[\t\r\n]/gu, '');
  // Check absolute links to this site against this build, never the deployed version.
  if (/^(https?:)?\/\//iu.test(href)) {
    try {
      const url = new URL(href, 'https://trailguide.fabricahq.com');
      if (url.hostname === 'trailguide.fabricahq.com')
        href = `${url.pathname}${url.search}${url.hash}`;
    } catch {
      return undefined; // The external check reports malformed HTTP URLs.
    }
  }
  if (/^[a-z][a-z0-9+.-]*:/iu.test(href) || href.startsWith('//'))
    return undefined;
  const hash = href.indexOf('#');
  const beforeHash = hash < 0 ? href : href.slice(0, hash);
  const query = beforeHash.indexOf('?');
  return {
    path: query < 0 ? beforeHash : beforeHash.slice(0, query),
    fragment: hash < 0 ? '' : href.slice(hash + 1),
  };
}

/** Resolve existing symlinks while retaining the unresolved suffix of a missing destination. */
function resolvedPath(path: string): string {
  if (existsSync(path)) return realpathSync(path);
  const parent = dirname(path);
  if (parent === path) return path;
  return join(resolvedPath(parent), basename(path));
}

/** Resolve a local URL from its page or the site root, including directory index files. */
function localTarget(root: string, source: string, pathname: string): string {
  // Unlike form decoding, URL paths retain literal plus signs; malformed percent escapes remain literal.
  const destination = decodePercentEscapes(pathname);
  let target: string;
  if (destination === '') target = source;
  else if (destination.startsWith('/'))
    target = resolve(root, `.${destination}`);
  else target = resolve(dirname(source), destination);
  const canonical = resolvedPath(target);
  return existsSync(canonical) && statSync(canonical).isDirectory()
    ? resolvedPath(join(canonical, 'index.html'))
    : canonical;
}

/** Report the first failure for a local link, or return nothing for a valid or external link. */
function checkLink(
  root: string,
  source: string,
  href: string,
  pages: ReadonlyMap<string, PageLinks>,
): string | undefined {
  const link = localReference(href);
  if (link === undefined) return undefined;
  const target = localTarget(root, source, link.path);
  const relativeTarget = relative(root, target);
  const location = relative(root, source);
  if (
    relativeTarget === '..' ||
    relativeTarget.startsWith(`..${sep}`) ||
    isAbsolute(relativeTarget)
  )
    return `${location}: outside output: ${href}`;
  if (!existsSync(target)) return `${location}: missing destination: ${href}`;
  const targetPage = pages.get(target);
  if (
    link.fragment &&
    targetPage !== undefined &&
    !targetPage.ids.has(decodePercentEscapes(link.fragment))
  )
    return `${location}: missing anchor: ${href}`;
  return undefined;
}

/** Check a built site and optionally public HTTP links; return a summary or throw all link diagnostics. */
export async function checkSite(
  directory: string,
  external: boolean,
): Promise<string> {
  const root = resolvedPath(resolve(directory));
  const pages = builtPages(root);
  if (pages.size === 0)
    throw new Error('No built pages found. Run bun run docs:build first.');
  const errors: Array<string> = [];
  for (const [source, page] of pages) {
    for (const href of page.links) {
      const error = checkLink(root, source, href, pages);
      if (error !== undefined) errors.push(error);
    }
  }
  if (external) errors.push(...(await checkExternalLinks(root, pages)));
  if (errors.length) throw new Error(errors.join('\n'));
  return `Checked ${external ? 'local and external links' : 'local links'} and fragments in ${pages.size} pages.`;
}
