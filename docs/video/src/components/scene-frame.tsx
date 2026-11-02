/** @fileoverview The full-frame backdrop every scene sits in, fading in at its start and out at its end. */
import type { ReactNode } from 'react';
import { AbsoluteFill, interpolate, useCurrentFrame } from 'remotion';
import { COLORS, FADE_FRAMES, FONTS } from '../theme';

/** Props for SceneFrame. */
export type SceneFrameProps = {
  /** The scene's length in frames, so it can fade out before the next one. */
  durationInFrames: number;
  children: ReactNode;
};

/** Lays out a scene's content above the caption band, with fades at both ends. */
export function SceneFrame({ durationInFrames, children }: SceneFrameProps) {
  const frame = useCurrentFrame();
  const opacity = interpolate(frame, [0, FADE_FRAMES, durationInFrames - FADE_FRAMES, durationInFrames], [0, 1, 1, 0], {
    extrapolateLeft: 'clamp',
    extrapolateRight: 'clamp',
  });
  return (
    <AbsoluteFill
      style={{
        opacity,
        padding: '110px 160px 230px',
        display: 'flex',
        flexDirection: 'column',
        justifyContent: 'center',
        gap: 36,
        color: COLORS.ink,
        fontFamily: FONTS.sans,
      }}
    >
      {children}
    </AbsoluteFill>
  );
}
