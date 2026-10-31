/** @fileoverview Runs documentation link checks and reports failures with a nonzero exit status. */

import { join } from 'node:path';
import { checkSite } from './check-site';

if (import.meta.main) {
  try {
    const args = process.argv.slice(2);
    const external = args.includes('--external');
    const paths = args.filter((arg) => arg !== '--external');
    if (paths.length > 1 || paths.some((arg) => arg.startsWith('--')))
      throw new Error('Usage: cli.ts [output-directory] [--external]');
    const root = paths[0] ?? join(import.meta.dir, '../../dist');
    console.log(await checkSite(root, external));
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    process.exitCode = 1;
  }
}
