/** @fileoverview Scene 9: because rules are small, every insight can become one, starting from an article. */
import { Panel } from '../components/panel';
import { Reveal } from '../components/reveal';
import { SceneFrame } from '../components/scene-frame';
import { Headline, Muted } from '../components/text';
import { COLORS } from '../theme';
import { cue, type SceneProps } from './scene-props';

/** Placeholder bars for the article's body text, as fractions of the column width. */
const TEXT_LINES = [0.96, 0.9, 0.94, 0.62] as const;

/** Shows the article the next scene turns into rules. */
export function AnInsight(props: SceneProps) {
  return (
    <SceneFrame durationInFrames={props.durationInFrames}>
      <Reveal at={0}>
        <Headline style={{ fontSize: 64 }}>Every insight can become a rule.</Headline>
      </Reveal>
      <Reveal at={cue(props, 0.45)} style={{ width: 1200, alignSelf: 'center' }}>
        <Panel title="addyo.substack.com/p/agentic-verification">
          <div style={{ display: 'flex', flexDirection: 'column', gap: 20 }}>
            <div style={{ fontSize: 60, fontWeight: 650, letterSpacing: '-0.03em' }}>Agentic Verification</div>
            <div style={{ fontSize: 30 }}>
              <Muted>Addy Osmani · Elevate</Muted>
            </div>
            {TEXT_LINES.map((width, index) => (
              <div
                key={index}
                style={{ height: 18, width: `${width * 100}%`, borderRadius: 9, background: COLORS.sessionBorder }}
              />
            ))}
          </div>
        </Panel>
      </Reveal>
    </SceneFrame>
  );
}
