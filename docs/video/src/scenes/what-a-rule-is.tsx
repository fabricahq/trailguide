/** @fileoverview Scene 7: a rule is one best practice in one versioned Markdown file, shown with a real rule from Fabrica's public library. */
import { Panel } from '../components/panel';
import { Reveal } from '../components/reveal';
import { SceneFrame } from '../components/scene-frame';
import { Headline, Muted, Provenance } from '../components/text';
import { COLORS, FONTS } from '../theme';
import { cue, type SceneProps } from './scene-props';

/** Shows the rule's title, when to read it, and its incorrect and correct examples. */
export function WhatARuleIs(props: SceneProps) {
  return (
    <SceneFrame durationInFrames={props.durationInFrames}>
      <Reveal at={0}>
        <Headline style={{ fontSize: 60 }}>
          A rule: <span style={{ color: COLORS.muted }}>one best practice, one file, its own version.</span>
        </Headline>
      </Reveal>
      <Reveal at={cue(props, 0.18)}>
        <Panel
          title={
            <span style={{ display: 'flex', justifyContent: 'space-between' }}>
              <span>techs/react/rendering-conditional-render.md</span>
              <Reveal at={cue(props, 0.8)}>
                <Provenance>1.0.0</Provenance>
              </Reveal>
            </span>
          }
        >
          <div style={{ fontFamily: FONTS.mono, fontSize: 28, lineHeight: 1.6, whiteSpace: 'pre' }}>
            <div>
              <span style={{ color: COLORS.action }}>title</span>: Use a boolean condition for conditional rendering
            </div>
            <Reveal at={cue(props, 0.45)}>
              <span style={{ color: COLORS.action }}>whenToRead</span>: Before writing JSX that renders with && on a
              non-boolean.
            </Reveal>
            <Reveal at={cue(props, 0.55)} style={{ marginTop: 18 }}>
              <span style={{ color: COLORS.error }}>Incorrect:</span>
              {' {count && <Badge count={count} />}   '}
              <Muted>renders a stray “0”</Muted>
            </Reveal>
            <Reveal at={cue(props, 0.65)}>
              <span style={{ color: COLORS.success }}>Correct:</span>
              {'   {count > 0 ? <Badge count={count} /> : null}'}
            </Reveal>
          </div>
        </Panel>
      </Reveal>
    </SceneFrame>
  );
}
