/** @fileoverview Fetches public documentation URLs while preventing requests into private networks. */

import { lookup } from 'node:dns/promises';
import { request as httpRequest } from 'node:http';
import { request as httpsRequest } from 'node:https';
import { BlockList, isIP } from 'node:net';
import { Readable } from 'node:stream';

const blocked = new BlockList();
for (const [address, prefix] of [
  ['0.0.0.0', 8],
  ['10.0.0.0', 8],
  ['100.64.0.0', 10],
  ['127.0.0.0', 8],
  ['169.254.0.0', 16],
  ['172.16.0.0', 12],
  ['192.0.0.0', 24],
  ['192.0.2.0', 24],
  ['192.88.99.0', 24],
  ['192.168.0.0', 16],
  ['198.18.0.0', 15],
  ['198.51.100.0', 24],
  ['203.0.113.0', 24],
  ['224.0.0.0', 3],
] as const)
  blocked.addSubnet(address, prefix, 'ipv4');
const globalIPv6 = new BlockList();
globalIPv6.addSubnet('2000::', 3, 'ipv6');
for (const [address, prefix] of [
  ['2001::', 23],
  ['2001:db8::', 32],
  ['2002::', 16],
  ['3fff::', 20],
] as const)
  blocked.addSubnet(address, prefix, 'ipv6');

/** Return whether an IP is public unicast; reject private, special-use, mapped, and transition addresses. */
export function isPublicAddress(address: string): boolean {
  if (isIP(address) === 4) return !blocked.check(address, 'ipv4');
  return (
    isIP(address) === 6 &&
    globalIPv6.check(address, 'ipv6') &&
    !blocked.check(address, 'ipv6')
  );
}

/** GET one public URL without redirects or credentials, pinning the verified IP to prevent DNS rebinding. */
export async function readPublicURL(
  url: URL,
  signal: AbortSignal,
): Promise<Response> {
  signal.throwIfAborted();
  const hostname = url.hostname.replace(/^\[|\]$/gu, '');
  let onAbort: () => void = () => {};
  const aborted = new Promise<never>((_resolve, reject) => {
    onAbort = () => reject(signal.reason);
    signal.addEventListener('abort', onAbort, { once: true });
  });
  let addresses;
  try {
    addresses = await Promise.race([lookup(hostname, { all: true }), aborted]);
  } finally {
    signal.removeEventListener('abort', onAbort);
  }
  if (
    !addresses.length ||
    addresses.some(({ address }) => !isPublicAddress(address))
  )
    throw new Error('non-public destination');
  const address =
    addresses.find((entry) => entry.family === 4) ?? addresses[0]!;
  signal.throwIfAborted();
  return new Promise((resolve, reject) => {
    const request = (url.protocol === 'https:' ? httpsRequest : httpRequest)(
      {
        // Connect to the checked address, retaining the original host for HTTP and TLS verification.
        hostname: address.address,
        family: address.family,
        port: url.port || undefined,
        servername: hostname,
        path: `${url.pathname}${url.search}`,
        agent: false,
        signal,
        headers: {
          Host: url.host,
          'User-Agent': 'TrailGuideDocsLinkChecker/1.0',
          Accept: 'text/html,*/*;q=0.8',
        },
      },
      (response) => {
        const headers = new Headers();
        for (const [name, value] of Object.entries(response.headers)) {
          if (typeof value === 'string') headers.set(name, value);
          else if (value) for (const item of value) headers.append(name, item);
        }
        const status = response.statusCode ?? 500;
        const body = [204, 205, 304].includes(status)
          ? null
          : (Readable.toWeb(response) as unknown as ReadableStream<Uint8Array>);
        if (!body) response.resume();
        resolve(new Response(body, { status, headers }));
      },
    );
    request.on('error', reject);
    request.end();
  });
}
