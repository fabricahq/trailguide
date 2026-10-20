/**
 * @fileoverview Serves one PR's documentation build on the preview runner host.
 * The stack copies this file and docs-preview-routing.ts beside the site and starts it
 * detached, so it imports only its sibling and Node built-ins.
 * Usage: bun docs-preview-server.ts <site-dir> <port>
 */

import { createReadStream, lstatSync } from 'node:fs';
import { createServer } from 'node:http';
import { join } from 'node:path';
import {
  contentType,
  routeRequest,
  type PathKind,
} from './docs-preview-routing';

const [root, rawPort] = process.argv.slice(2);
const port = Number(rawPort);
if (!root || !Number.isInteger(port)) {
  console.error('usage: bun docs-preview-server.ts <site-dir> <port>');
  process.exit(2);
}
const site = root;

// lstat, not stat: the stack rejects symlinks before serving, and this keeps any that
// appear later from being followed out of the site.
function kindOf(path: string): PathKind {
  try {
    const stats = lstatSync(path);
    if (stats.isFile()) return 'file';
    return stats.isDirectory() ? 'directory' : null;
  } catch {
    return null;
  }
}

const server = createServer((request, response) => {
  // The stack and publish job check this header to confirm they reached this process,
  // not a stale server or another PR's preview holding the same port.
  response.setHeader('X-Code-Rules-Preview-Pid', String(process.pid));
  // Every push replaces the site, so browsers must not reuse old files.
  response.setHeader('Cache-Control', 'no-store');
  response.setHeader('X-Content-Type-Options', 'nosniff');
  if (request.method !== 'GET' && request.method !== 'HEAD') {
    response.writeHead(405, { Allow: 'GET, HEAD' }).end();
    return;
  }
  const route = routeRequest(
    site,
    new URL(request.url ?? '/', 'http://preview'),
    kindOf,
  );
  if (route.status === 301) {
    response.writeHead(301, { Location: route.location }).end();
    return;
  }
  const file = route.status === 200 ? route.file : join(site, '404.html');
  if (route.status === 404 && kindOf(file) !== 'file') {
    response.writeHead(404, { 'Content-Type': 'text/plain; charset=utf-8' });
    response.end(request.method === 'HEAD' ? undefined : 'Not found\n');
    return;
  }
  response.writeHead(route.status, { 'Content-Type': contentType(file) });
  if (request.method === 'HEAD') {
    response.end();
    return;
  }
  createReadStream(file)
    .on('error', () => response.destroy())
    .pipe(response);
});

server.listen(port, '0.0.0.0', () => {
  console.log(
    `docs preview: serving ${site} on port ${port} (pid ${process.pid})`,
  );
});
for (const signal of ['SIGTERM', 'SIGINT'] as const) {
  process.on(signal, () => {
    server.close(() => process.exit(0));
    server.closeAllConnections();
  });
}
