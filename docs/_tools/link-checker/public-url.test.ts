/** @fileoverview Tests public-address boundaries and rejection of private redirect destinations. */

import { expect, test } from 'bun:test';
import { isPublicAddress, readPublicURL } from './public-url';
import { checkExternalLinks } from './external';
import { pageLinks } from './html';

test('should reject private and special-use IPv4 and IPv6 addresses', () => {
  for (const address of [
    '0.0.0.0',
    '127.0.0.1',
    '10.1.2.3',
    '172.16.1.1',
    '192.168.1.1',
    '169.254.169.254',
    '100.64.0.1',
    '192.0.2.1',
    '198.18.1.1',
    '224.0.0.1',
    '255.255.255.255',
    '::1',
    '::',
    '::ffff:127.0.0.1',
    'fc00::1',
    'fe80::1',
    'ff02::1',
    '2001:db8::1',
    '2002:7f00:1::1',
    'not-an-ip',
  ])
    expect(isPublicAddress(address)).toBe(false);
  for (const address of ['1.1.1.1', '8.8.8.8', '2606:4700:4700::1111'])
    expect(isPublicAddress(address)).toBe(true);
});

test('should revalidate a redirect before contacting a private destination', async () => {
  let requests = 0;
  const server = Bun.serve({
    hostname: '127.0.0.1',
    port: 0,
    fetch: () => {
      requests++;
      return new Response('private');
    },
  });
  try {
    const pages = new Map([
      [
        '/site/index.html',
        pageLinks('<a href="https://example.com/redirect">link</a>'),
      ],
    ]);
    const errors = await checkExternalLinks('/site', pages, (url, signal) =>
      url.hostname === 'example.com'
        ? Promise.resolve(Response.redirect(server.url, 302))
        : readPublicURL(url, signal),
    );
    expect(errors.join('\n')).toContain('non-public destination');
    expect(requests).toBe(0);
  } finally {
    server.stop(true);
  }
});
