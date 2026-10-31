/** @fileoverview Scene 13: tell your agent to install Trail Guide, and where the getting-started guide is. */
import { AgentLine, AgentSession, UserMessage } from '../components/chat';
import { Reveal } from '../components/reveal';
import { SceneFrame } from '../components/scene-frame';
import { Accent, Check, Muted } from '../components/text';
import { COLORS, FONTS } from '../theme';
import { cue, type SceneProps } from './scene-props';

const STEPS = [
  { text: 'Installed Trail Guide', detail: null },
  { text: 'Set up the project', detail: 'trailguide project init' },
  { text: 'Added your first rules and skills', detail: 'from acme' },
] as const;

/** Shows the agent installing Trail Guide, then the guide's address. */
export function GetStarted(props: SceneProps) {
  return (
    <SceneFrame durationInFrames={props.durationInFrames}>
      <AgentSession>
        <UserMessage at={0} text="Install Trail Guide in this project." />
        {STEPS.map((step, index) => (
          <AgentLine key={step.text} at={cue(props, 0.4 + index * 0.12)}>
            <Check /> {step.text}
            {step.detail === null ? null : (
              <Muted>
                <span style={{ fontFamily: FONTS.mono, fontSize: 26 }}> · {step.detail}</span>
              </Muted>
            )}
          </AgentLine>
        ))}
      </AgentSession>
      <Reveal at={cue(props, 0.8)} style={{ textAlign: 'center', fontSize: 36, color: COLORS.muted }}>
        Get started at <Accent>trailguide.fabricahq.com</Accent>
      </Reveal>
    </SceneFrame>
  );
}
