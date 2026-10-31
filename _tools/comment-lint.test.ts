/** @fileoverview Checks that comment linting enforces exports and file roles without requiring private-helper documentation. */

import { expect, test } from 'bun:test';
import { ESLint } from 'eslint';

const eslint = new ESLint();
const header =
  '/** @fileoverview Example module for comment-policy checks. */\n\n';

test.each([
  {
    name: 'file header',
    code: 'const value = 1;',
    rule: 'jsdoc/require-file-overview',
  },
  {
    name: 'exported function comment',
    code: `${header}export function value() { return 1; }`,
    rule: 'jsdoc/require-jsdoc',
  },
  {
    name: 'exported type comment',
    code: `${header}export type Value = string;`,
    rule: 'jsdoc/require-jsdoc',
  },
  {
    name: 'exported interface comment',
    code: `${header}export interface Value { text: string; }`,
    rule: 'jsdoc/require-jsdoc',
  },
  {
    name: 'exported arrow comment',
    code: `${header}export const value = () => 1;`,
    rule: 'jsdoc/require-jsdoc',
  },
  {
    name: 'export description',
    code: `${header}/** @param input */\nexport function value(input: string) { return input; }`,
    rule: 'jsdoc/require-description',
  },
])(
  'should report a missing $name when linting TypeScript',
  async ({ code, rule }) => {
    const results = await eslint.lintText(code, {
      filePath: 'docs/src/comment-policy-example.ts',
    });
    expect(
      results.flatMap((result) =>
        result.messages.map((message) => message.ruleId),
      ),
    ).toContain(rule);
  },
);

test('should allow private helpers with or without useful comments when exports have descriptions', async () => {
  const results = await eslint.lintText(
    `${header}
function double(value: number) { return value * 2; }
/** Return zero for an absent value. */
function defaultValue(value: number | undefined) { return value ?? 0; }
/** Return twice the supplied value, or zero when absent. */
export function calculate(value: number | undefined) { return double(defaultValue(value)); }
`,
    { filePath: 'docs/src/comment-policy-example.ts' },
  );
  expect(results.flatMap((result) => result.messages)).toEqual([]);
});

test('should enforce the role header when an Astro component has no explicit export', async () => {
  const missing = await eslint.lintText('<p>Example</p>', {
    filePath: 'docs/src/components/CommentPolicyExample.astro',
  });
  expect(
    missing.flatMap((result) =>
      result.messages.map((message) => message.ruleId),
    ),
  ).toContain('jsdoc/require-file-overview');
  const documented = await eslint.lintText(
    `---\n${header}---\n<p>Example</p>`,
    { filePath: 'docs/src/components/CommentPolicyExample.astro' },
  );
  expect(documented.flatMap((result) => result.messages)).toEqual([]);
});

test('should require a contract when an Astro page exports route generation', async () => {
  const results = await eslint.lintText(
    `---\n${header}export function getStaticPaths() { return []; }\n---\n<p>Example</p>`,
    { filePath: 'docs/src/pages/comment-policy-example.astro' },
  );
  expect(
    results.flatMap((result) =>
      result.messages.map((message) => message.ruleId),
    ),
  ).toContain('jsdoc/require-jsdoc');
});
