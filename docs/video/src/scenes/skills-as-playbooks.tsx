/** @fileoverview Scene 12: a library skill declares the rules it uses, so the playbook and each rule improve separately. */
import { interpolate, useCurrentFrame } from 'remotion';
import { LibraryCard } from '../components/cards';
import { Reveal } from '../components/reveal';
import { SceneFrame } from '../components/scene-frame';
import { Chip, Headline, Muted } from '../components/text';
import { COLORS, FONTS } from '../theme';
import { cue, type SceneProps } from './scene-props';

const STEPS = ['Plan how to check the change', 'Write the tests first', 'Verify before review'] as const;

const RULES = [
  { id: 'give-agents-a-way-to-check-their-work', version: '1.0.0' },
  { id: 'test-real-user-flows', version: '1.2.0' },
  { id: 'keep-test-output-short', version: '1.0.0' },
] as const;

/** Shows a playbook beside the rules it declares, then one rule updating while the playbook stays put. */
export function SkillsAsPlaybooks(props: SceneProps) {
  const frame = useCurrentFrame();
  const updateAt = cue(props, 0.72);
  const glow = interpolate(frame, [updateAt, updateAt + 10, updateAt + 40], [0, 1, 0.35], {
    extrapolateLeft: 'clamp',
    extrapolateRight: 'clamp',
  });
  return (
    <SceneFrame durationInFrames={props.durationInFrames}>
      <Reveal at={0}>
        <Headline style={{ fontSize: 60 }}>
          A playbook <span style={{ color: COLORS.muted }}>declares the rules it uses.</span>
        </Headline>
      </Reveal>
      <Reveal at={cue(props, 0.1)}>
        <LibraryCard name="acme/.trailguide" subtitle="Your organization’s library" visibility="private">
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1.25fr', gap: 36 }}>
            <Chip style={{ fontSize: 28 }}>
              <div style={{ fontWeight: 600, marginBottom: 12 }}>
                verify-changes <Muted>· playbook skill</Muted>
              </div>
              {STEPS.map((step, index) => (
                <div key={step} style={{ fontSize: 26, color: COLORS.muted }}>
                  {index + 1}. {step}
                </div>
              ))}
            </Chip>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
              {RULES.map((rule, index) => {
                const isUpdated = index === 1;
                return (
                  <Reveal key={rule.id} at={cue(props, 0.4 + index * 0.06)}>
                    <Chip
                      style={{
                        display: 'flex',
                        justifyContent: 'space-between',
                        gap: 20,
                        padding: '14px 22px',
                        fontFamily: FONTS.mono,
                        fontSize: 22,
                        borderColor: isUpdated && frame >= updateAt ? COLORS.warning : COLORS.sessionBorder,
                        boxShadow: isUpdated ? `0 0 ${40 * glow}px ${COLORS.warning}88` : 'none',
                      }}
                    >
                      <span>rule {rule.id}</span>
                      <span style={{ color: isUpdated && frame >= updateAt ? COLORS.warning : COLORS.agent }}>
                        {isUpdated && frame >= updateAt ? '1.3.0' : rule.version}
                      </span>
                    </Chip>
                  </Reveal>
                );
              })}
            </div>
          </div>
        </LibraryCard>
      </Reveal>
    </SceneFrame>
  );
}
