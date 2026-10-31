/** @fileoverview A conversation with a coding agent, styled like the docs site's example sessions. */
import type { ReactNode } from 'react';
import { useCurrentFrame } from 'remotion';
import { COLORS, FONTS } from '../theme';
import { Panel } from './panel';
import { Reveal } from './reveal';

/** Characters of a request typed per frame. */
const CHARACTERS_PER_FRAME = 1.4;

/** Props for UserMessage. */
export type UserMessageProps = { text: string; at: number };

/** The developer's request, typed into a right-aligned bubble from frame `at`. */
export function UserMessage({ text, at }: UserMessageProps) {
  const frame = useCurrentFrame();
  const shown = Math.floor(Math.max(0, frame - at) * CHARACTERS_PER_FRAME);
  return (
    <div style={{ display: 'flex', justifyContent: 'flex-end' }}>
      <div
        style={{
          maxWidth: '78%',
          padding: '22px 30px',
          borderRadius: 22,
          background: COLORS.sessionSubtle,
          border: `1.5px solid ${COLORS.sessionBorder}`,
          fontSize: 38,
          lineHeight: 1.4,
          // Reserve the full text's height so the bubble doesn't grow while typing.
          display: 'grid',
        }}
      >
        <span style={{ gridArea: '1 / 1', visibility: 'hidden' }}>{text}</span>
        <span style={{ gridArea: '1 / 1' }}>{text.slice(0, shown)}</span>
      </div>
    </div>
  );
}

/** Props for AgentLine. */
export type AgentLineProps = { at: number; children: ReactNode };

/** One line of the agent's reply, marked with its glyph, revealed from frame `at`. */
export function AgentLine({ at, children }: AgentLineProps) {
  return (
    <Reveal at={at} style={{ display: 'flex', gap: 22, fontSize: 34, lineHeight: 1.45 }}>
      <span style={{ color: COLORS.agent }}>✳︎</span>
      <span>{children}</span>
    </Reveal>
  );
}

/** Props for AgentSession. */
export type AgentSessionProps = { children: ReactNode };

/** The session window that holds a conversation. */
export function AgentSession({ children }: AgentSessionProps) {
  return (
    <Panel
      title={
        <span style={{ fontFamily: FONTS.sans, color: COLORS.sessionText, fontSize: 28 }}>
          <span style={{ color: COLORS.agent, marginRight: 16 }}>✳︎</span>Your coding agent
          <span style={{ fontFamily: FONTS.mono, color: COLORS.sessionMuted, fontSize: 22, marginLeft: 18 }}>
            acme-checkout
          </span>
        </span>
      }
      tag="EXAMPLE SESSION"
    >
      <div style={{ display: 'flex', flexDirection: 'column', gap: 30, fontFamily: FONTS.sans }}>{children}</div>
    </Panel>
  );
}
