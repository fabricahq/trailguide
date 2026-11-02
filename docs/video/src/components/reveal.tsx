/** @fileoverview Fades and lifts its children into view from a given frame. */
import type { CSSProperties, ReactNode } from 'react';
import { interpolate, useCurrentFrame } from 'remotion';

/** Frames a reveal takes to finish. */
const REVEAL_FRAMES = 12;

/** Props for Reveal. */
export type RevealProps = {
  /** The frame, within the current scene, at which the reveal starts. */
  at: number;
  children: ReactNode;
  style?: CSSProperties;
};

/** Shows its children from frame `at`, easing them up and in. */
export function Reveal({ at, children, style }: RevealProps) {
  const frame = useCurrentFrame();
  const progress = interpolate(frame, [at, at + REVEAL_FRAMES], [0, 1], {
    extrapolateLeft: 'clamp',
    extrapolateRight: 'clamp',
  });
  return (
    <div style={{ ...style, opacity: progress, transform: `translateY(${(1 - progress) * 18}px)` }}>{children}</div>
  );
}
