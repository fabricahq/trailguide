/** @fileoverview Tests documentation preview routing, site validation, the preview stack, and the workflow's API scripts. */

import { afterAll, beforeAll, describe, expect, test } from 'bun:test';
import { execFile } from 'node:child_process';
import {
  existsSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  symlinkSync,
  writeFileSync,
} from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { promisify } from 'node:util';
import { runInNewContext } from 'node:vm';
import { parse } from 'yaml';
import {
  parsePullRequest,
  previewPort,
  pullRequestForStateDir,
  resolveSitePath,
  routeRequest,
  type PathKind,
} from './docs-preview-routing';
import { validateSite } from './docs-preview-stack';

const run = promisify(execFile);

// Creates a small static site shaped like the Astro build.
function makeSite(): string {
  const root = mkdtempSync(join(tmpdir(), 'docs-preview-site-'));
  writeFileSync(join(root, 'index.html'), '<title>Code Rules</title>');
  writeFileSync(join(root, '404.html'), '<title>Not found</title>');
  mkdirSync(join(root, 'guides', 'write-rules'), { recursive: true });
  writeFileSync(join(root, 'guides', 'write-rules', 'index.html'), 'guide');
  return root;
}

describe('preview configuration', () => {
  test('accepts only positive PR numbers', () => {
    expect(parsePullRequest('57')).toBe(57);
    for (const bad of ['', '0', '-1', '1.5', '57abc', undefined]) {
      expect(() => parsePullRequest(bad)).toThrow();
    }
  });

  test('gives each PR a fixed port in the preview range', () => {
    expect(previewPort(57)).toBe(26057);
    expect(previewPort(499)).toBe(26499);
    expect(previewPort(500)).toBe(26000);
  });

  test('recognizes only preview state directories', () => {
    expect(pullRequestForStateDir('pr-57')).toBe(57);
    expect(pullRequestForStateDir('pr-57.bak')).toBeNull();
    expect(pullRequestForStateDir('.DS_Store')).toBeNull();
  });

  test('keeps request paths inside the site', () => {
    expect(resolveSitePath('/srv/site', '/a/b.css')).toBe('/srv/site/a/b.css');
    expect(resolveSitePath('/srv/site', '/a/../b.css')).toBe('/srv/site/b.css');
    expect(resolveSitePath('/srv/site', '/../secret')).toBeNull();
    expect(resolveSitePath('/srv/site', '/%2e%2e/secret')).toBeNull();
    expect(resolveSitePath('/srv/site', '/..%2fsite-secrets/x')).toBeNull();
    expect(resolveSitePath('/srv/site', '/%E0%A4%A')).toBeNull();
    expect(resolveSitePath('/srv/site', '/a%00b')).toBeNull();
  });
});

describe('preview routing matches GitHub Pages', () => {
  const tree: Record<string, PathKind> = {
    '/site': 'directory',
    '/site/index.html': 'file',
    '/site/guides': 'directory',
    '/site/guides/index.html': 'file',
    '/site/about.html': 'file',
    '/site/empty': 'directory',
  };
  const route = (path: string) =>
    routeRequest(
      '/site',
      new URL(path, 'http://preview'),
      (p) => tree[p] ?? null,
    );

  test('serves files and directory indexes', () => {
    expect(route('/')).toEqual({ status: 200, file: '/site/index.html' });
    expect(route('/guides/')).toEqual({
      status: 200,
      file: '/site/guides/index.html',
    });
  });

  test('redirects directories to a trailing slash, keeping the query', () => {
    expect(route('/guides?x=1')).toEqual({
      status: 301,
      location: '/guides/?x=1',
    });
  });

  test('serves extensionless paths from .html files', () => {
    expect(route('/about')).toEqual({ status: 200, file: '/site/about.html' });
    expect(route('/about/')).toEqual({ status: 404 });
  });

  test('returns 404 for missing pages and index-less directories', () => {
    expect(route('/missing')).toEqual({ status: 404 });
    expect(route('/empty/')).toEqual({ status: 404 });
  });
});

describe('site validation', () => {
  test('accepts a site of plain files', () => {
    const site = makeSite();
    expect(() => validateSite(site)).not.toThrow();
    rmSync(site, { recursive: true, force: true });
  });

  test('rejects symlinks anywhere in the site', () => {
    const site = makeSite();
    symlinkSync('/etc/hosts', join(site, 'guides', 'hosts'));
    expect(() => validateSite(site)).toThrow('only files and directories');
    rmSync(site, { recursive: true, force: true });
  });

  test('requires an index page', () => {
    const site = makeSite();
    rmSync(join(site, 'index.html'));
    expect(() => validateSite(site)).toThrow('no index.html');
    rmSync(site, { recursive: true, force: true });
  });
});

describe('preview stack', () => {
  // PR 499 maps to the top of the range, clear of low-numbered live previews.
  const origin = 'http://127.0.0.1:26499';
  const previewsDir = mkdtempSync(join(tmpdir(), 'docs-previews-'));
  const site = makeSite();
  const env = {
    ...process.env,
    PREVIEWS_DIR: previewsDir,
    PREVIEW_PR: '499',
    PREVIEW_SHA: 'abc1234def',
    PREVIEW_SITE: site,
    GITHUB_OUTPUT: join(previewsDir, 'github-output'),
  };
  const stack = (command: string, overrides: Record<string, string> = {}) =>
    run(process.execPath, ['_tools/docs-preview-stack.ts', command], {
      env: { ...env, ...overrides },
    });

  beforeAll(() => stack('start'));
  afterAll(async () => {
    await stack('stop').catch(() => {});
    rmSync(previewsDir, { recursive: true, force: true });
    rmSync(site, { recursive: true, force: true });
  });

  test('serves the site with no caching', async () => {
    const response = await fetch(`${origin}/`);
    expect(response.status).toBe(200);
    expect(response.headers.get('content-type')).toStartWith('text/html');
    expect(response.headers.get('cache-control')).toBe('no-store');
    expect(await response.text()).toContain('<title>Code Rules</title>');
  });

  test('reports the URL and the pid of the server that answers it', async () => {
    const pid = (await fetch(`${origin}/`)).headers.get(
      'x-code-rules-preview-pid',
    );
    expect(readFileSync(env.GITHUB_OUTPUT, 'utf8')).toBe(
      `url=http://127.0.0.1:26499/\npid=${pid}\n`,
    );
  });

  test('routes like GitHub Pages, serving 404.html for missing pages', async () => {
    const guide = await fetch(`${origin}/guides/write-rules`, {
      redirect: 'manual',
    });
    expect(guide.status).toBe(301);
    expect(guide.headers.get('location')).toBe('/guides/write-rules/');
    expect((await fetch(`${origin}/guides/write-rules/`)).status).toBe(200);
    const missing = await fetch(`${origin}/missing/`);
    expect(missing.status).toBe(404);
    expect(await missing.text()).toContain('<title>Not found</title>');
  });

  test('refuses a site with a symlink and removes its state', async () => {
    const unsafe = makeSite();
    symlinkSync('/etc/hosts', join(unsafe, 'hosts'));
    await expect(
      stack('start', { PREVIEW_PR: '498', PREVIEW_SITE: unsafe }),
    ).rejects.toThrow('only files and directories');
    expect(existsSync(join(previewsDir, 'pr-498'))).toBe(false);
    rmSync(unsafe, { recursive: true, force: true });
  });

  test('restarting replaces the running server', async () => {
    const before = (await fetch(`${origin}/`)).headers.get(
      'x-code-rules-preview-pid',
    );
    await stack('start');
    const state = JSON.parse(
      readFileSync(join(previewsDir, 'pr-499', 'state.json'), 'utf8'),
    );
    const current = (await fetch(`${origin}/`)).headers.get(
      'x-code-rules-preview-pid',
    );
    expect(current).not.toBe(before);
    expect(current).toBe(String(state.pid));
  });

  test('stopping shuts the server down and removes its state', async () => {
    await stack('stop');
    expect(existsSync(join(previewsDir, 'pr-499'))).toBe(false);
    await expect(fetch(`${origin}/`)).rejects.toThrow();
  });
});

const workflow = parse(
  readFileSync('.github/workflows/docs-preview.yml', 'utf8'),
);
const resolveScript: string = workflow.jobs.resolve.steps[0].with.script;
const commentScript: string = workflow.jobs.publish.steps[1].with.script;
const sha = 'a'.repeat(40);

type ResolveOptions = {
  trigger?: string;
  ref?: string;
  sameRepository?: boolean;
  authorPermission?: string;
  actorPermission?: string;
  runID?: string;
  approvedCommit?: string;
  path?: string;
  event?: string;
  conclusion?: string;
  runRepository?: number;
  artifacts?: object[];
  state?: string;
  head?: string;
  closedBaseRepository?: number;
};

// Runs the shipped resolve script against fixtures and returns its outputs.
async function resolvePreview(options: ResolveOptions = {}) {
  const outputs: Record<string, string> = {};
  const notices: string[] = [];
  const pr = {
    number: 38,
    user: { login: 'contributor' },
    state: options.state ?? 'open',
    head: {
      sha: options.head ?? sha,
      repo: { id: options.sameRepository === false ? 99 : 1 },
    },
    base: { repo: { id: 1 } },
  };
  const workflowRun = {
    id: 500,
    head_sha: sha,
    path: options.path ?? '.github/workflows/docs.yml',
    event: options.event ?? 'pull_request',
    status: 'completed',
    conclusion: options.conclusion ?? 'success',
    repository: { id: options.runRepository ?? 1 },
  };
  const responses: Record<string, object[]> = {
    artifacts: options.artifacts ?? [
      { name: 'docs-site', expired: false, size_in_bytes: 100 },
    ],
    pulls: [pr],
  };
  const github = {
    paginate: async (route: string) => responses[route],
    rest: {
      actions: {
        listWorkflowRunArtifacts: 'artifacts',
        getWorkflowRun: async () => ({ data: workflowRun }),
      },
      repos: {
        listPullRequestsAssociatedWithCommit: 'pulls',
        getCollaboratorPermissionLevel: async (args: { username: string }) => ({
          data: {
            permission:
              args.username === 'maintainer'
                ? (options.actorPermission ?? 'write')
                : (options.authorPermission ?? 'write'),
          },
        }),
      },
      pulls: { get: async () => ({ data: pr }) },
    },
  };
  await runInNewContext(`(async () => { ${resolveScript}\n })()`, {
    github,
    core: {
      setOutput: (name: string, value: string) => {
        outputs[name] = value;
      },
      notice: (message: string) => notices.push(message),
    },
    context: {
      repo: { owner: 'fabricahq', repo: 'code-rules' },
      actor: 'maintainer',
      eventName: options.trigger ?? 'workflow_run',
      ref: options.ref ?? 'refs/heads/main',
      payload: {
        action: 'closed',
        repository: { id: 1, default_branch: 'main' },
        workflow_run: workflowRun,
        pull_request: {
          number: 38,
          base: { repo: { id: options.closedBaseRepository ?? 1 } },
        },
        inputs: {
          run_id: options.runID ?? '500',
          commit: options.approvedCommit ?? sha,
        },
      },
    },
  });
  return { outputs, notices };
}

describe('preview resolution', () => {
  test('deploys a same-repository PR from an author with write access', async () => {
    const { outputs } = await resolvePreview();
    expect(outputs).toEqual({
      action: 'deploy',
      pr: '38',
      run_id: '500',
      sha,
    });
  });

  test('holds fork PRs for manual approval', async () => {
    const { outputs, notices } = await resolvePreview({
      sameRepository: false,
    });
    expect(outputs.action).toBe('skip');
    expect(notices.join()).toContain('need manual approval');
  });

  test('holds same-repository PRs from authors without write access', async () => {
    const { outputs } = await resolvePreview({ authorPermission: 'read' });
    expect(outputs.action).toBe('skip');
  });

  test('deploys a fork PR a maintainer approves by run and commit', async () => {
    const { outputs } = await resolvePreview({
      trigger: 'workflow_dispatch',
      sameRepository: false,
    });
    expect(outputs.action).toBe('deploy');
  });

  test('rejects approval from someone without write access', async () => {
    await expect(
      resolvePreview({ trigger: 'workflow_dispatch', actorPermission: 'read' }),
    ).rejects.toThrow('requires repository write access');
  });

  test('rejects an approval whose commit differs from the run', async () => {
    await expect(
      resolvePreview({
        trigger: 'workflow_dispatch',
        approvedCommit: 'b'.repeat(40),
      }),
    ).rejects.toThrow('does not match the approved commit');
  });

  test('rejects malformed approval inputs', async () => {
    await expect(
      resolvePreview({ trigger: 'workflow_dispatch', runID: '5; rm' }),
    ).rejects.toThrow('full 40-character commit SHA');
  });

  test('runs only from the default branch', async () => {
    await expect(resolvePreview({ ref: 'refs/pull/38/merge' })).rejects.toThrow(
      'default-branch workflow',
    );
  });

  for (const [name, options] of [
    ['another workflow', { path: '.github/workflows/go.yml' }],
    ['a push build', { event: 'push' }],
    ['a failed build', { conclusion: 'failure' }],
    ['another repository', { runRepository: 2 }],
    ['a missing artifact', { artifacts: [] }],
    [
      'an expired artifact',
      {
        artifacts: [{ name: 'docs-site', expired: true, size_in_bytes: 100 }],
      },
    ],
    ['a closed PR', { state: 'closed' }],
    ['a superseded commit', { head: 'b'.repeat(40) }],
  ] as Array<[string, ResolveOptions]>) {
    test(`skips ${name}`, async () => {
      const { outputs } = await resolvePreview(options);
      expect(outputs.action).toBe('skip');
    });
  }

  test('stops the preview when a PR closes', async () => {
    const { outputs } = await resolvePreview({
      trigger: 'pull_request_target',
    });
    expect(outputs).toEqual({ action: 'stop', pr: '38' });
  });

  test('ignores closed PRs targeting another repository', async () => {
    const { outputs } = await resolvePreview({
      trigger: 'pull_request_target',
      closedBaseRepository: 2,
    });
    expect(outputs.action).toBe('skip');
  });
});

// Runs the shipped comment script against fixtures and returns its writes.
async function comment(
  options: {
    live?: boolean;
    runId?: number;
    previous?: { id: number; user: { login: string }; body: string };
  } = {},
) {
  const writes: Array<Record<string, unknown>> = [];
  const github = {
    paginate: async () => (options.previous ? [options.previous] : []),
    rest: {
      issues: {
        listComments: 'comments',
        createComment: async (args: Record<string, unknown>) => {
          writes.push({ method: 'create', ...args });
        },
        updateComment: async (args: Record<string, unknown>) => {
          writes.push({ method: 'update', ...args });
        },
      },
    },
  };
  await runInNewContext(`(async () => { ${commentScript}\n })()`, {
    github,
    process: {
      env: {
        PREVIEW_PR: '38',
        PREVIEW_SHA: sha,
        PREVIEW_URL: 'http://preview-host:26038/',
        PREVIEW_LIVE: String(options.live ?? true),
      },
    },
    context: {
      repo: { owner: 'fabricahq', repo: 'code-rules' },
      serverUrl: 'https://github.com',
      runId: options.runId ?? 700,
      runAttempt: 1,
    },
  });
  return writes;
}

describe('preview comment', () => {
  const marker = '<!-- code-rules-docs-preview -->';

  test('posts the preview link for a live preview', async () => {
    const writes = await comment();
    expect(writes).toHaveLength(1);
    expect(writes[0]).toMatchObject({ method: 'create', issue_number: 38 });
    expect(writes[0]!.body).toStartWith(marker);
    expect(writes[0]!.body).toContain(
      '**[Open preview](http://preview-host:26038/)**',
    );
  });

  test('links the run when the preview failed', async () => {
    const writes = await comment({ live: false });
    expect(writes[0]!.body).toContain('failed to deploy');
    expect(writes[0]!.body).toContain(
      'https://github.com/fabricahq/code-rules/actions/runs/700',
    );
  });

  test('updates its own comment instead of posting another', async () => {
    const writes = await comment({
      previous: {
        id: 8,
        user: { login: 'github-actions[bot]' },
        body: `${marker}\n<!-- run:699 attempt:1 -->`,
      },
    });
    expect(writes).toEqual([
      expect.objectContaining({ method: 'update', comment_id: 8 }),
    ]);
  });

  test('does not edit a human comment containing the marker', async () => {
    const writes = await comment({
      previous: { id: 8, user: { login: 'reviewer' }, body: marker },
    });
    expect(writes[0]!.method).toBe('create');
  });

  test('never replaces a newer run status with an older one', async () => {
    const writes = await comment({
      previous: {
        id: 8,
        user: { login: 'github-actions[bot]' },
        body: `${marker}\n<!-- run:701 attempt:1 -->`,
      },
    });
    expect(writes).toHaveLength(0);
  });
});
