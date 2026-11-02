/** @fileoverview The Trail Guide symbol, the same checked-box path as the docs site's logo files. */
import { COLORS } from '../theme';

/** Props for BrandMark. */
export type BrandMarkProps = { size: number };

/** Draws the Trail Guide symbol at a square size in pixels. */
export function BrandMark({ size }: BrandMarkProps) {
  return (
    <svg width={size} height={size} viewBox="0 0 64 64" aria-hidden="true">
      <path
        d="M36 14H17a3 3 0 0 0-3 3v30a3 3 0 0 0 3 3h30a3 3 0 0 0 3-3V35 M27 25l9 9 17-20"
        fill="none"
        stroke={COLORS.ink}
        strokeWidth={3.3}
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}
