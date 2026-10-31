/** @fileoverview The Trail Guide logo: the symbol beside the name, in one color, as the docs homepage sets it. */
import { COLORS } from '../theme';
import { BrandMark } from './brand-mark';

/** Props for Wordmark. */
export type WordmarkProps = { fontSize: number };

/** Draws the symbol and "Trail Guide", sized from the name's font size in pixels. */
export function Wordmark({ fontSize }: WordmarkProps) {
  return (
    <span style={{ display: 'inline-flex', alignItems: 'center', gap: fontSize * 0.28 }}>
      <BrandMark size={Math.round(fontSize * 1.2)} />
      <span style={{ fontSize, fontWeight: 650, letterSpacing: '-0.04em', lineHeight: 1, color: COLORS.ink }}>
        Trail Guide
      </span>
    </span>
  );
}
