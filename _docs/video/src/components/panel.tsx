/** @fileoverview A window in the style of the docs site's example sessions: a titled header over a dark body. */
import type { CSSProperties, ReactNode } from 'react';
import { COLORS, FONTS } from '../theme';

/** Props for Panel. */
export type PanelProps = {
  /** Header text, such as a file path or `Terminal`. */
  title: ReactNode;
  /** Small uppercase label at the header's right, such as `EXAMPLE SESSION`. */
  tag?: string | undefined;
  children: ReactNode;
  style?: CSSProperties;
};

/** Frames content as a session window with a header. */
export function Panel({ title, tag, children, style }: PanelProps) {
  return (
    <div
      style={{
        border: `1.5px solid ${COLORS.sessionBorder}`,
        borderRadius: 22,
        overflow: 'hidden',
        background: COLORS.sessionSurface,
        color: COLORS.sessionText,
        boxShadow: '0 40px 120px -50px #547cba66',
        ...style,
      }}
    >
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 20,
          padding: '20px 32px',
          background: COLORS.sessionHeader,
          borderBottom: `1.5px solid ${COLORS.sessionBorder}`,
          fontFamily: FONTS.mono,
          fontSize: 24,
          color: COLORS.sessionMuted,
        }}
      >
        <span style={{ flex: 1 }}>{title}</span>
        {tag === undefined ? null : <span style={{ fontSize: 18, letterSpacing: '0.14em' }}>{tag}</span>}
      </div>
      <div style={{ padding: '28px 36px' }}>{children}</div>
    </div>
  );
}
