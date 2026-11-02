/** @fileoverview Small text pieces scenes share: colored spans, a headline, and chips. */
import type { CSSProperties, ReactNode } from 'react';
import { COLORS, FONTS } from '../theme';

/** Props for the colored span helpers. */
export type SpanProps = { children: ReactNode };

/** Secondary text, such as a path prefix or a note. */
export function Muted({ children }: SpanProps) {
  return <span style={{ color: COLORS.sessionMuted }}>{children}</span>;
}

/** A source and version tag, such as `payments · release 12`. */
export function Provenance({ children }: SpanProps) {
  return <span style={{ color: COLORS.agent }}>{children}</span>;
}

/** A success check mark. */
export function Check() {
  return <span style={{ color: COLORS.success }}>✓</span>;
}

/** A Trail Guide command or skill name called out in prose. */
export function Accent({ children }: SpanProps) {
  return <span style={{ color: COLORS.action }}>{children}</span>;
}

/** Props for Headline. */
export type HeadlineProps = { children: ReactNode; style?: CSSProperties };

/** A large statement in the docs site's display style. */
export function Headline({ children, style }: HeadlineProps) {
  return (
    <div style={{ fontSize: 76, fontWeight: 450, letterSpacing: '-0.05em', lineHeight: 1.08, ...style }}>
      {children}
    </div>
  );
}

/** Props for Chip. */
export type ChipProps = { children: ReactNode; isDimmed?: boolean; style?: CSSProperties };

/** A bordered label, such as one feature of a library. */
export function Chip({ children, isDimmed = false, style }: ChipProps) {
  return (
    <div
      style={{
        padding: '20px 28px',
        borderRadius: 16,
        border: `1.5px solid ${COLORS.sessionBorder}`,
        background: COLORS.sessionSubtle,
        color: isDimmed ? COLORS.sessionMuted : COLORS.sessionText,
        fontFamily: FONTS.sans,
        fontSize: 32,
        ...style,
      }}
    >
      {children}
    </div>
  );
}
