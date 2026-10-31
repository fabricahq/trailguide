/** @fileoverview Enforces source headers and export descriptions while leaving comment quality to review. */

import tsParser from '@typescript-eslint/parser';
import * as astroParser from 'astro-eslint-parser';
import jsdoc from 'eslint-plugin-jsdoc';

const sourceFiles = [
  '_tools/**/*.ts',
  'docs/_tools/**/*.ts',
  'docs/src/**/*.{ts,astro,mjs}',
  'docs/*.mjs',
  'eslint.config.mjs',
];

export default [
  { ignores: ['**/node_modules/**', '**/dist/**', '**/.astro/**'] },
  {
    files: sourceFiles,
    languageOptions: { parser: tsParser },
    plugins: { jsdoc },
    settings: {
      jsdoc: {
        mode: 'typescript',
        // A blank line separates file overviews from the first declaration; export docs must be adjacent.
        maxLines: 1,
        tagNamePreference: { file: 'fileoverview' },
      },
    },
    rules: {
      'jsdoc/require-file-overview': 'error',
      'jsdoc/require-jsdoc': [
        'error',
        {
          publicOnly: true,
          enableFixer: false,
          contexts: ['TSTypeAliasDeclaration', 'TSInterfaceDeclaration'],
          require: {
            FunctionDeclaration: true,
            ArrowFunctionExpression: true,
            FunctionExpression: true,
            ClassDeclaration: true,
          },
        },
      ],
      'jsdoc/require-description': [
        'error',
        {
          contexts: [
            'FunctionDeclaration',
            'FunctionExpression',
            'ArrowFunctionExpression',
            'ClassDeclaration',
            'TSTypeAliasDeclaration',
            'TSInterfaceDeclaration',
          ],
        },
      ],
      'jsdoc/require-param': 'off',
      'jsdoc/require-returns': 'off',
    },
  },
  {
    files: ['docs/src/**/*.astro'],
    // Astro components have implicit exports; their frontmatter overview also describes the rendered result.
    languageOptions: {
      parser: astroParser,
      parserOptions: { parser: tsParser, extraFileExtensions: ['.astro'] },
    },
  },
];
