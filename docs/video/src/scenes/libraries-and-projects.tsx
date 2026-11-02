/** @fileoverview Scene 4: Trail Guide has libraries, which share skills and rules, and projects, which use them */
import { ItemRow, LibraryCard, ProjectCard } from '../components/cards';
import { Reveal } from '../components/reveal';
import { SceneFrame } from '../components/scene-frame';
import { Headline } from '../components/text';
import { COLORS } from '../theme';
import { cue, type SceneProps } from './scene-props';

/** Shows one library feeding a project. */
export function LibrariesAndProjects(props: SceneProps) {
  return (
    <SceneFrame durationInFrames={props.durationInFrames}>
      <Reveal at={0}>
        <Headline style={{ fontSize: 64 }}>
          Libraries <span style={{ color: COLORS.muted }}>and</span> projects
        </Headline>
      </Reveal>
      <div style={{ display: 'grid', gridTemplateColumns: '1fr auto 1fr', gap: 40, alignItems: 'start' }}>
        <Reveal at={cue(props, 0.15)} style={{ display: 'flex', flexDirection: 'column', gap: 24 }}>
          <LibraryCard name="acme/.trailguide" subtitle="Skills and rules to share" visibility="private">
            <ItemRow kind="skills" id="write-tests, review-change" />
            <ItemRow kind="rules" id="practices/*" />
          </LibraryCard>
        </Reveal>
        <Reveal at={cue(props, 0.58)}>
          <div style={{ fontSize: 72, color: COLORS.muted, marginTop: 70 }}>→</div>
        </Reveal>
        <Reveal at={cue(props, 0.6)} style={{ display: 'flex', flexDirection: 'column', gap: 24 }}>
          <ProjectCard name="acme-checkout" subtitle="A codebase that uses them">
            <ItemRow kind="skills" id=".claude/skills/" />
          </ProjectCard>
        </Reveal>
      </div>
    </SceneFrame>
  );
}
