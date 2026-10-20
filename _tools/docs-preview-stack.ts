/**
 * @fileoverview Starts, stops, and reaps pull request documentation previews on the preview runner host.
 * The site comes from a PR's build artifact, so it is untrusted data: it is copied, validated
 * as plain files, and served statically; nothing in it is executed. The server must outlive
 * the Actions job and later jobs replace the workspace, so the site and server are copied
 * into ~/.code-rules-docs-previews/pr-<N>/ and the server runs detached from there.
 *
 * Usage: bun _tools/docs-preview-stack.ts <start|stop|reap>
 * Environment: PREVIEW_PR (start, stop), PREVIEW_SITE, PREVIEW_HOST, and PREVIEW_SHA (start),
 * GITHUB_REPOSITORY and GH_TOKEN (reap), and PREVIEWS_DIR to override the state root.
 */

import { execFileSync, spawn, type ChildProcess } from 'node:child_process';
import {
  appendFileSync,
  copyFileSync,
  cpSync,
  existsSync,
  lstatSync,
  mkdirSync,
  openSync,
  readdirSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from 'node:fs';
import { homedir } from 'node:os';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import {
  parsePullRequest,
  previewPort,
  previewStateDir,
  previewURL,
  pullRequestForStateDir,
} from './docs-preview-routing';

const here = dirname(fileURLToPath(import.meta.url));
const previewsDir =
  process.env.PREVIEWS_DIR || join(homedir(), '.code-rules-docs-previews');
const readyTimeoutMs = 15_000;
const stopTimeoutMs = 5_000;
// The built site is about 10 MB in 120 files; these bounds leave ample headroom.
const maxSiteBytes = 200 * 1024 * 1024;
const maxSiteFiles = 10_000;

const sleep = (ms: number) => new Promise<void>((done) => setTimeout(done, ms));

/**
 * Checks that a copied site holds only regular files and directories within the size
 * bounds, with an index page. Symlinks and special files are rejected rather than served.
 */
export function validateSite(root: string): void {
  let bytes = 0;
  let files = 0;
  const pending = [root];
  while (pending.length > 0) {
    const directory = pending.pop()!;
    for (const name of readdirSync(directory)) {
      const path = join(directory, name);
      const stats = lstatSync(path);
      if (stats.isDirectory()) {
        pending.push(path);
      } else if (stats.isFile()) {
        bytes += stats.size;
        files += 1;
      } else {
        throw new Error(
          `Preview site may contain only files and directories: ${path}`,
        );
      }
      if (bytes > maxSiteBytes || files > maxSiteFiles) {
        throw new Error('Preview site exceeds the size or file-count limit');
      }
    }
  }
  if (
    !lstatSync(join(root, 'index.html'), { throwIfNoEntry: false })?.isFile()
  ) {
    throw new Error('Preview site has no index.html');
  }
}

// Returns the server pid recorded for a preview, or null.
function recordedPid(stateDir: string): number | null {
  try {
    const pid = Number(readFileSync(join(stateDir, 'pid'), 'utf8').trim());
    return Number.isInteger(pid) && pid > 0 ? pid : null;
  } catch {
    return null;
  }
}

// Checking the command line keeps a stale pid file, after a reboot for example,
// from killing an unrelated process that reused the pid.
function isPreviewServer(pid: number, stateDir: string): boolean {
  try {
    const command = execFileSync('ps', ['-p', String(pid), '-o', 'command='], {
      encoding: 'utf8',
    });
    return command.includes(join(stateDir, 'docs-preview-server.ts'));
  } catch {
    return false;
  }
}

// Stops a preview's server if it is running, then deletes its state directory.
async function stopPreview(stateDir: string): Promise<void> {
  const pid = recordedPid(stateDir);
  if (pid !== null && isPreviewServer(pid, stateDir)) {
    process.kill(pid, 'SIGTERM');
    const deadline = Date.now() + stopTimeoutMs;
    while (isPreviewServer(pid, stateDir) && Date.now() < deadline) {
      await sleep(100);
    }
    if (isPreviewServer(pid, stateDir)) {
      console.log(`docs preview: pid ${pid} ignored SIGTERM; killing`);
      process.kill(pid, 'SIGKILL');
    }
  }
  rmSync(stateDir, { recursive: true, force: true });
}

// Waits until the spawned server answers, failing if another process holds the port.
async function waitUntilReady(
  child: ChildProcess,
  port: number,
  logFile: string,
): Promise<void> {
  let exited = false;
  child.once('exit', () => {
    exited = true;
  });
  const deadline = Date.now() + readyTimeoutMs;
  let lastProblem = 'no response';
  while (Date.now() < deadline && !exited) {
    try {
      const response = await fetch(`http://127.0.0.1:${port}/`);
      const servedBy = response.headers.get('x-code-rules-preview-pid');
      if (response.ok && servedBy === String(child.pid)) return;
      lastProblem = `status ${response.status} from pid ${servedBy ?? 'unknown'}`;
    } catch (error) {
      lastProblem = error instanceof Error ? error.message : String(error);
    }
    await sleep(200);
  }
  const log = existsSync(logFile) ? readFileSync(logFile, 'utf8').trim() : '';
  throw new Error(
    `Preview server on port ${port} did not become ready (${exited ? 'server exited' : lastProblem})${log ? `\n${log}` : ''}`,
  );
}

// Copies a PR's site into its state directory, validates the copy, and serves it detached.
async function start(): Promise<void> {
  const pr = parsePullRequest(process.env.PREVIEW_PR);
  const source = process.env.PREVIEW_SITE;
  if (!source || !existsSync(source)) {
    throw new Error('PREVIEW_SITE must name the downloaded site directory');
  }
  const host = process.env.PREVIEW_HOST || '127.0.0.1';
  const sha = process.env.PREVIEW_SHA || 'unknown';
  const stateDir = previewStateDir(previewsDir, pr);
  const port = previewPort(pr);
  const url = previewURL(host, pr);

  await stopPreview(stateDir);
  const site = join(stateDir, 'site');
  mkdirSync(stateDir, { recursive: true });
  // Copy symlinks as links so validation sees and rejects them.
  cpSync(source, site, { recursive: true, verbatimSymlinks: true });
  try {
    validateSite(site);
  } catch (error) {
    rmSync(stateDir, { recursive: true, force: true });
    throw error;
  }
  for (const file of ['docs-preview-server.ts', 'docs-preview-routing.ts']) {
    copyFileSync(join(here, file), join(stateDir, file));
  }

  const logFile = join(stateDir, 'server.log');
  const log = openSync(logFile, 'a');
  const child = spawn(
    process.execPath,
    [join(stateDir, 'docs-preview-server.ts'), site, String(port)],
    {
      cwd: stateDir,
      detached: true,
      // The Actions runner kills every process tagged with the job's tracking id when the
      // job ends. Clearing it lets the server outlive the job.
      env: { ...process.env, RUNNER_TRACKING_ID: '' },
      stdio: ['ignore', log, log],
    },
  );
  child.unref();
  writeFileSync(join(stateDir, 'pid'), `${child.pid}\n`);
  writeFileSync(
    join(stateDir, 'state.json'),
    `${JSON.stringify({ pr, sha, port, url, pid: child.pid, startedAt: new Date().toISOString() }, null, 2)}\n`,
  );
  await waitUntilReady(child, port, logFile);

  console.log(`docs preview: PR #${pr} (${sha.slice(0, 7)}) is live at ${url}`);
  if (process.env.GITHUB_OUTPUT) {
    appendFileSync(process.env.GITHUB_OUTPUT, `url=${url}\npid=${child.pid}\n`);
  }
}

// Tears down one PR's preview.
async function stop(): Promise<void> {
  const pr = parsePullRequest(process.env.PREVIEW_PR);
  await stopPreview(previewStateDir(previewsDir, pr));
  console.log(`docs preview: PR #${pr} stopped`);
}

// Tears down previews whose PRs are no longer open, catching close events the runner
// missed while offline. A failed lookup keeps the preview, so an API outage never
// deletes a live one.
async function reap(): Promise<void> {
  const repository = process.env.GITHUB_REPOSITORY;
  const token = process.env.GH_TOKEN;
  if (!repository || !token) {
    throw new Error('reap needs GITHUB_REPOSITORY and GH_TOKEN');
  }
  const entries = existsSync(previewsDir) ? readdirSync(previewsDir) : [];
  for (const name of entries) {
    const pr = pullRequestForStateDir(name);
    if (pr === null) continue;
    const response = await fetch(
      `https://api.github.com/repos/${repository}/pulls/${pr}`,
      {
        headers: {
          Accept: 'application/vnd.github+json',
          Authorization: `Bearer ${token}`,
          'X-GitHub-Api-Version': '2022-11-28',
        },
      },
    );
    if (!response.ok) {
      console.log(
        `docs preview: keeping PR #${pr}; lookup returned ${response.status}`,
      );
      continue;
    }
    const { state } = (await response.json()) as { state: string };
    if (state !== 'open') {
      await stopPreview(previewStateDir(previewsDir, pr));
      console.log(`docs preview: reaped PR #${pr} (${state})`);
    }
  }
}

if (import.meta.main) {
  const commands: Record<string, () => Promise<void>> = { start, stop, reap };
  const command = commands[process.argv[2] ?? ''];
  if (!command) {
    console.error('usage: bun _tools/docs-preview-stack.ts <start|stop|reap>');
    process.exit(2);
  }
  await command();
}
