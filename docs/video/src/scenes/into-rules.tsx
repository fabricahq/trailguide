/** @fileoverview Scene 10: the agent turns the article into one rule per recommendation. */
import { AgentLine, AgentSession, UserMessage } from '../components/chat';
import { Reveal } from '../components/reveal';
import { SceneFrame } from '../components/scene-frame';
import { Check } from '../components/text';
import { FONTS } from '../theme';
import { cue, type SceneProps } from './scene-props';

const RULES = [
  'give-agents-a-way-to-check-their-work',
  'test-real-user-flows',
  'property-test-what-must-never-happen',
  'diff-old-and-new-during-rewrites',
  'keep-tests-fast-and-deterministic',
  'quarantine-flaky-tests',
  'keep-test-output-short',
  'turn-repeat-mistakes-into-checks',
] as const;

/** Shows the request and the eight rules it produces. */
export function IntoRules(props: SceneProps) {
  return (
    <SceneFrame durationInFrames={props.durationInFrames}>
      <AgentSession>
        <UserMessage at={0} text="Turn this article into Trail Guide rules." />
        <AgentLine at={cue(props, 0.3)}>Wrote 8 rules in practices/verification:</AgentLine>
        <div
          style={{
            display: 'grid',
            gridTemplateColumns: '1fr 1fr',
            columnGap: 40,
            rowGap: 8,
            fontFamily: FONTS.mono,
            fontSize: 25,
          }}
        >
          {RULES.map((rule, index) => (
            <Reveal key={rule} at={cue(props, 0.36 + index * 0.05)}>
              <Check /> {rule}
            </Reveal>
          ))}
        </div>
      </AgentSession>
    </SceneFrame>
  );
}
