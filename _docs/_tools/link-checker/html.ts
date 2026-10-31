/** @fileoverview Extracts hyperlink destinations and fragment targets from static HTML. */

import { parse } from 'parse5';
import type { DefaultTreeAdapterTypes } from 'parse5';

/** IDs and anchor destinations found in a built page, after HTML entity decoding. */
export type PageLinks = {
  readonly ids: ReadonlySet<string>;
  readonly links: ReadonlyArray<string>;
};
/** Collect IDs and anchor href values; comments and script text are not treated as markup. */
export function pageLinks(html: string): PageLinks {
  const ids = new Set<string>();
  const links: Array<string> = [];
  /** Visit parsed elements and their children in document order. */
  function visit(node: DefaultTreeAdapterTypes.Node): void {
    if ('tagName' in node) {
      const id = node.attrs.find((attribute) => attribute.name === 'id');
      if (id !== undefined) ids.add(id.value);
      const name = node.attrs.find((attribute) => attribute.name === 'name');
      if (node.tagName === 'a' && name !== undefined) ids.add(name.value);
      if (node.tagName === 'a') {
        const href = node.attrs.find((attribute) => attribute.name === 'href');
        if (href !== undefined) links.push(href.value);
      }
    }
    if ('content' in node) visit(node.content);
    if ('childNodes' in node) for (const child of node.childNodes) visit(child);
  }
  visit(parse(html));
  return { ids, links };
}
