/** @fileoverview A terminal panel that types commands and reveals their output on cue. */
import type { ReactNode } from 'react';
import { interpolate, useCurrentFrame } from 'remotion';
import { COLORS, FONTS } from '../theme';
import { Panel } from './panel';
import { Reveal } from './reveal';

/** Characters typed per frame, about 48 per second. */
const CHARACTERS_PER_FRAME = 1.6;

/** A line of terminal content and the frame at which it appears. */
export type TerminalLine =
  { kind: 'command'; text: string; at: number } | { kind: 'output'; content: ReactNode; at: number };

/** Props for Terminal. */
export type TerminalProps = {
  lines: ReadonlyArray<TerminalLine>;
  /** Text to emphasize inside commands once typed, such as a newly added flag. */
  highlight?: string;
  /** Small uppercase label at the header's right, such as `YOUR PROJECT`. */
  tag?: string;
};

/** Frames a command takes to type, so callers can cue its output. */
export function typingFrames(text: string): number {
  return Math.ceil(text.length / CHARACTERS_PER_FRAME);
}

function TypedCommand({ text, at, highlight }: { text: string; at: number; highlight: string | undefined }) {
  const frame = useCurrentFrame();
  const shown = Math.floor(Math.max(0, frame - at) * CHARACTERS_PER_FRAME);
  const isTyping = shown < text.length;
  const typed = text.slice(0, shown);
  const cursorVisible = isTyping || Math.floor(frame / 15) % 2 === 0;
  const parts = highlight !== undefined && !isTyping ? typed.split(highlight) : [typed];
  return (
    <div style={{ whiteSpace: 'pre' }}>
      <span style={{ color: COLORS.sessionMuted }}>$ </span>
      {parts.map((part, index) => (
        <span key={index}>
          {index > 0 ? <span style={{ color: COLORS.warning }}>{highlight}</span> : null}
          {part}
        </span>
      ))}
      {isTyping || frame - at < typingFrames(text) + 20 ? (
        <span style={{ opacity: cursorVisible ? 1 : 0, color: COLORS.action }}>▍</span>
      ) : null}
    </div>
  );
}

function OutputLine({ content, at }: { content: ReactNode; at: number }) {
  const frame = useCurrentFrame();
  const opacity = interpolate(frame, [at, at + 6], [0, 1], { extrapolateLeft: 'clamp', extrapolateRight: 'clamp' });
  return <div style={{ whiteSpace: 'pre', opacity }}>{content}</div>;
}

/** Shows each line from its frame on: commands type out, output fades in. */
export function Terminal({ lines, highlight, tag }: TerminalProps) {
  const frame = useCurrentFrame();
  // Bring the window in just before its first line, so it never sits empty.
  const firstAt = Math.min(...lines.map((line) => line.at));
  return (
    <Reveal at={Math.max(0, firstAt - 8)}>
      <Panel title="Terminal · acme-checkout" tag={tag}>
        <div style={{ fontFamily: FONTS.mono, fontSize: 30, lineHeight: 1.65, minHeight: 120 }}>
          {lines
            .filter((line) => frame >= line.at)
            .map((line, index) =>
              line.kind === 'command' ? (
                <TypedCommand key={index} text={line.text} at={line.at} highlight={highlight} />
              ) : (
                <OutputLine key={index} content={line.content} at={line.at} />
              ),
            )}
        </div>
      </Panel>
    </Reveal>
  );
}
