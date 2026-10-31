/** @fileoverview Scene 8: a project pulls rules and skills from its organization's library and a public library, adds its own, and excludes what doesn't fit. */
import type { ReactNode } from 'react';
import { ItemRow, LibraryCard, ProjectCard } from '../components/cards';
import { Reveal } from '../components/reveal';
import { SceneFrame } from '../components/scene-frame';
import { Headline } from '../components/text';
import { COLORS } from '../theme';
import { cue, type SceneProps } from './scene-props';

/** A label naming where a group of the project's rows came from. */
function From({ color, children }: { color: string; children: ReactNode }) {
  return <div style={{ fontSize: 20, letterSpacing: '0.12em', color, marginTop: 6 }}>{children}</div>;
}

/** Shows two libraries feeding one project, which also has its own guidance and excludes one rule. */
export function MixAndMatch(props: SceneProps) {
  return (
    <SceneFrame durationInFrames={props.durationInFrames}>
      <Reveal at={0}>
        <Headline style={{ fontSize: 60 }}>Mix and match rules and skills.</Headline>
      </Reveal>
      <div style={{ display: 'grid', gridTemplateColumns: '1fr auto 1.15fr', gap: 32, alignItems: 'center' }}>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 24 }}>
          <Reveal at={cue(props, 0.12)}>
            <LibraryCard name="acme" subtitle="Your organization" visibility="private" />
          </Reveal>
          <Reveal at={cue(props, 0.22)}>
            <LibraryCard name="fabrica" subtitle="A public library" visibility="public" />
          </Reveal>
        </div>
        <Reveal at={cue(props, 0.3)}>
          <div style={{ fontSize: 72, color: COLORS.muted }}>→</div>
        </Reveal>
        <Reveal at={cue(props, 0.3)}>
          <ProjectCard name="acme-checkout" subtitle="Your project">
            <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
              <From color={COLORS.success}>FROM ACME</From>
              <Reveal at={cue(props, 0.36)}>
                <ItemRow kind="skill" id="write-tests" />
              </Reveal>
              <Reveal at={cue(props, 0.4)}>
                <ItemRow kind="rules" id="practices/*" />
              </Reveal>
              <Reveal at={cue(props, 0.78)}>
                <ItemRow
                  kind="rule"
                  id="api/pagination"
                  isExcluded
                  note={<span style={{ color: COLORS.error }}>excluded</span>}
                />
              </Reveal>
              <From color={COLORS.action}>FROM FABRICA</From>
              <Reveal at={cue(props, 0.46)}>
                <ItemRow kind="rules" id="techs/react/*" />
              </Reveal>
              <From color={COLORS.agent}>ITS OWN</From>
              <Reveal at={cue(props, 0.54)}>
                <ItemRow kind="rule" id="refund-once" />
              </Reveal>
            </div>
          </ProjectCard>
        </Reveal>
      </div>
    </SceneFrame>
  );
}
