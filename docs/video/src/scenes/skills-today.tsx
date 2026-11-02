/** @fileoverview Scene 2: skills are the common way to guide agents, but they're coarse-grained and loosely versioned. */
import { interpolate, useCurrentFrame } from 'remotion';
import { Panel } from '../components/panel';
import { Reveal } from '../components/reveal';
import { SceneFrame } from '../components/scene-frame';
import { Chip } from '../components/text';
import { COLORS, FONTS } from '../theme';
import { cue, type SceneProps } from './scene-props';

const INSIGHTS = [
  'Prefer end-to-end tests for user flows',
  'Use property tests for invariants',
  'Never mock the database',
  'Quarantine flaky tests',
  'Keep test output short',
] as const;

/** Shows one skill bundling many insights, with no version anyone checks. */
export function SkillsToday(props: SceneProps) {
  const frame = useCurrentFrame();
  // The rest of the skill lands with a pop, as the narration says it bundles dozens of insights.
  const moreAt = cue(props, 0.22);
  const moreScale = interpolate(frame, [moreAt, moreAt + 14], [1.3, 1], {
    extrapolateLeft: 'clamp',
    extrapolateRight: 'clamp',
  });
  return (
    <SceneFrame durationInFrames={props.durationInFrames}>
      <Reveal at={0}>
        <Panel
          title={
            <span style={{ display: 'flex', justifyContent: 'space-between' }}>
              <span>skills/testing/SKILL.md</span>
              <Reveal at={cue(props, 0.68)}>
                <span style={{ color: COLORS.warning }}>version: latest?</span>
              </Reveal>
            </span>
          }
        >
          <div style={{ fontFamily: FONTS.mono, fontSize: 30, lineHeight: 1.65 }}>
            <div># Testing</div>
            {INSIGHTS.map((insight) => (
              <div key={insight}>- {insight}</div>
            ))}
            <Reveal at={moreAt}>
              <div
                style={{
                  marginTop: 8,
                  fontSize: 44,
                  fontWeight: 600,
                  color: COLORS.warning,
                  transform: `scale(${moreScale})`,
                  transformOrigin: 'left center',
                }}
              >
                … 40 more insights
              </div>
            </Reveal>
          </div>
        </Panel>
      </Reveal>
      <div style={{ display: 'flex', gap: 24 }}>
        <Reveal at={cue(props, 0.4)}>
          <Chip>Coarse-grained: all or nothing</Chip>
        </Reveal>
        <Reveal at={cue(props, 0.7)}>
          <Chip>Loosely versioned: take the latest</Chip>
        </Reveal>
      </div>
    </SceneFrame>
  );
}
