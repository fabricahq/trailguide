/** @fileoverview Scene 3: Trail Guide's two answers, a package manager for skills and rules. */
import { Reveal } from '../components/reveal';
import { SceneFrame } from '../components/scene-frame';
import { Chip, Muted } from '../components/text';
import { Wordmark } from '../components/wordmark';
import { COLORS } from '../theme';
import { cue, type SceneProps } from './scene-props';

const ANSWERS = [
  { number: '1', title: 'A package manager for skills', detail: 'Known sources and versions', at: 0.4 },
  { number: '2', title: 'Rules', detail: 'A more granular kind of guidance', at: 0.68 },
] as const;

/** Introduces Trail Guide and its two ideas. */
export function EnterTrailGuide(props: SceneProps) {
  return (
    <SceneFrame durationInFrames={props.durationInFrames}>
      <Reveal at={0} style={{ textAlign: 'center' }}>
        <Wordmark fontSize={120} />
      </Reveal>
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 32 }}>
        {ANSWERS.map((answer) => (
          <Reveal key={answer.number} at={cue(props, answer.at)}>
            <Chip style={{ padding: '36px 40px', fontSize: 40 }}>
              <span style={{ color: COLORS.success, marginRight: 18 }}>{answer.number}</span>
              {answer.title}
              <div style={{ fontSize: 30, marginTop: 10 }}>
                <Muted>{answer.detail}</Muted>
              </div>
            </Chip>
          </Reveal>
        ))}
      </div>
    </SceneFrame>
  );
}
