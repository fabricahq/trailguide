/** @fileoverview Scene 14: the Trail Guide name, tagline, and site. */
import { Img } from 'remotion';
import { Reveal } from '../components/reveal';
import { SceneFrame } from '../components/scene-frame';
import { Headline } from '../components/text';
import { COLORS, FONTS } from '../theme';
import type { SceneProps } from './scene-props';

/** Ends on the name and where to learn more. */
export function Close(props: SceneProps) {
  return (
    <SceneFrame durationInFrames={props.durationInFrames}>
      <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 40, textAlign: 'center' }}>
        <Reveal at={0}>
          <Img
            src={new URL('../../../docs/public/brand/logos/trailguide-horizontal-dark.svg', import.meta.url).href}
            alt="Trailguide"
            style={{ display: 'block', width: 680, height: (680 * 72) / 302 }}
          />
        </Reveal>
        <Reveal at={12}>
          <Headline style={{ fontSize: 64 }}>The package manager for agent skills and rules</Headline>
        </Reveal>
        <Reveal at={24}>
          <div style={{ fontFamily: FONTS.mono, fontSize: 32, color: COLORS.muted }}>trailguide.fabricahq.com</div>
        </Reveal>
      </div>
    </SceneFrame>
  );
}
