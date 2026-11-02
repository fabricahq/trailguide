/** @fileoverview Scene 1: agents need guidance, and without it review repeats the same corrections. */
import { Reveal } from '../components/reveal';
import { SceneFrame } from '../components/scene-frame';
import { Chip, Headline } from '../components/text';
import { COLORS } from '../theme';
import { cue, type SceneProps } from './scene-props';

const REVIEW_COMMENTS = [
  'These tests mock the database again.',
  'There’s no end-to-end test for checkout.',
  'Same note as last week.',
] as const;

/** Opens on the need for guidance, over review comments that repeat. */
export function Guidance(props: SceneProps) {
  return (
    <SceneFrame durationInFrames={props.durationInFrames}>
      <Reveal at={0}>
        <Headline>Your agents need guidance to build the right thing.</Headline>
      </Reveal>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 18, alignItems: 'flex-start' }}>
        {REVIEW_COMMENTS.map((comment, index) => (
          <Reveal key={comment} at={cue(props, 0.45 + index * 0.12)}>
            <Chip style={{ borderLeft: `6px solid ${COLORS.error}` }}>{comment}</Chip>
          </Reveal>
        ))}
      </div>
    </SceneFrame>
  );
}
