/** @fileoverview Checks public HTTP links and static HTML fragments without sending credentials. */

import { readPublicURL } from './public-url';

import { relative } from 'node:path';
import { pageLinks } from './html';
import type { PageLinks } from './html';

/** An HTTP destination and each built page that links to it. */
type Destination = {
  url: URL;
  references: Array<{ source: string; href: string; fragment: string }>;
};

/** Fetch a destination with bounded redirects, retries, time, and HTML body size. */
async function readDestination(
  url: URL,
  readURL: typeof readPublicURL,
): Promise<ReadonlySet<string> | undefined> {
  for (let attempt = 0; ; attempt++) {
    try {
      const signal = AbortSignal.timeout(15_000);
      let target = url;
      for (let redirects = 0; ; redirects++) {
        if (
          !['http:', 'https:'].includes(target.protocol) ||
          target.username ||
          target.password
        )
          throw new Error('unsupported URL or embedded credentials');
        const response = await readURL(target, signal);
        if ([301, 302, 303, 307, 308].includes(response.status)) {
          await response.body?.cancel();
          const location = response.headers.get('location');
          if (!location || redirects >= 5)
            throw new Error('invalid or excessive redirects');
          target = new URL(location, target);
          continue;
        }
        if (!response.ok) {
          await response.body?.cancel();
          throw new Error(`HTTP ${response.status}`);
        }
        if (!response.headers.get('content-type')?.includes('text/html')) {
          await response.body?.cancel();
          return undefined;
        }
        const chunks: Array<Uint8Array> = [];
        let size = 0;
        if (response.body) {
          for await (const chunk of response.body) {
            size += chunk.byteLength;
            if (size > 5_000_000)
              throw new Error('HTML exceeds 5 MB check limit');
            chunks.push(chunk);
          }
        }
        return pageLinks(Buffer.concat(chunks).toString('utf8')).ids;
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      if (
        attempt >= 2 ||
        /HTTP (?!429|5\d\d)\d+|non-public|unsupported|redirects|exceeds/u.test(
          message,
        )
      )
        throw error;
      await Bun.sleep(1000 * (attempt + 1));
    }
  }
}

/** Check HTTP URLs once each with four workers; return all failures with source pages in stable order. */
export async function checkExternalLinks(
  root: string,
  pages: ReadonlyMap<string, PageLinks>,
  readURL: typeof readPublicURL = readPublicURL,
): Promise<Array<string>> {
  const errors: Array<string> = [];
  const destinations = new Map<string, Destination>();
  for (const [source, page] of pages) {
    for (const href of page.links) {
      const normalized = href.trim().replace(/[\t\r\n]/gu, '');
      if (!/^(https?:|\/\/)/iu.test(normalized)) continue;
      try {
        const url = new URL(normalized, 'https://trailguide.fabricahq.com');
        if (url.hostname === 'trailguide.fabricahq.com') continue;
        const fragment = decodeURIComponent(url.hash.slice(1));
        url.hash = '';
        const destination = destinations.get(url.href) ?? {
          url,
          references: [],
        };
        destination.references.push({ source, href, fragment });
        destinations.set(url.href, destination);
      } catch {
        errors.push(`${relative(root, source)}: invalid URL: ${href}`);
      }
    }
  }
  const queue = [...destinations.values()];
  async function worker(): Promise<void> {
    for (
      let destination = queue.shift();
      destination;
      destination = queue.shift()
    ) {
      let ids: ReadonlySet<string> | undefined;
      let failure: string | undefined;
      try {
        ids = await readDestination(destination.url, readURL);
      } catch (error) {
        failure = error instanceof Error ? error.message : String(error);
      }
      for (const { source, href, fragment } of destination.references) {
        // GitHub prefixes user-authored heading IDs to avoid collisions with its own UI.
        const githubAnchor =
          destination.url.hostname === 'github.com' &&
          ids?.has(`user-content-${fragment}`);
        const missing = fragment && ids && !ids.has(fragment) && !githubAnchor;
        if (failure || missing)
          errors.push(
            `${relative(root, source)}: ${failure ?? 'missing external anchor'}: ${href}`,
          );
      }
    }
  }
  await Promise.all(Array.from({ length: 4 }, () => worker()));
  console.log(`Checked ${destinations.size} unique external URLs.`);
  return errors.sort();
}
