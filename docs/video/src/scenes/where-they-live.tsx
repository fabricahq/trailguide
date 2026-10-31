/** @fileoverview Scene 11: the new rules can live in the project, the organization's library, or a public library. */
import { LibraryCard, ProjectCard } from '../components/cards';
import { Reveal } from '../components/reveal';
import { SceneFrame } from '../components/scene-frame';
import { Headline } from '../components/text';
import { cue, type SceneProps } from './scene-props';

/** Shows the three places a rule can live. */
export function WhereTheyLive(props: SceneProps) {
  return (
    <SceneFrame durationInFrames={props.durationInFrames}>
      <Reveal at={0}>
        <Headline style={{ fontSize: 64 }}>Put them where they belong.</Headline>
      </Reveal>
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: 28, alignItems: 'stretch' }}>
        <Reveal at={cue(props, 0.05)}>
          <ProjectCard name="This project" subtitle="Just for this codebase" style={{ height: '100%' }} />
        </Reveal>
        <Reveal at={cue(props, 0.3)}>
          <LibraryCard
            name="Your org’s library"
            subtitle="Every project gets them"
            visibility="private"
            style={{ height: '100%' }}
          />
        </Reveal>
        <Reveal at={cue(props, 0.6)}>
          <LibraryCard
            name="A public library"
            subtitle="Versioned, for anyone"
            visibility="public"
            style={{ height: '100%' }}
          />
        </Reveal>
      </div>
    </SceneFrame>
  );
}
