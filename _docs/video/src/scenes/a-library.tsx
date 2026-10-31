/** @fileoverview Scene 5: a library holds skills you write and skills you curate, even from repositories that don't use Trail Guide. */
import { ItemRow, LibraryCard } from '../components/cards';
import { Reveal } from '../components/reveal';
import { SceneFrame } from '../components/scene-frame';
import { Headline } from '../components/text';
import { COLORS } from '../theme';
import { cue, type SceneProps } from './scene-props';

const SKILLS = [
  { name: 'write-tests', origin: 'written by your team', isCurated: false },
  { name: 'review-change', origin: 'written by your team', isCurated: false },
  { name: 'release-checklist', origin: 'written by your team', isCurated: false },
  { name: 'tdd', origin: 'curated from mattpocock/skills', isCurated: true },
] as const;

/** Shows a library's skills and where each came from. */
export function ALibrary(props: SceneProps) {
  return (
    <SceneFrame durationInFrames={props.durationInFrames}>
      <Reveal at={0}>
        <Headline style={{ fontSize: 64 }}>
          Write skills, <span style={{ color: COLORS.muted }}>or curate them.</span>
        </Headline>
      </Reveal>
      <Reveal at={cue(props, 0.12)}>
        <LibraryCard name="acme/.trailguide" subtitle="Your organization’s library" visibility="private">
          {SKILLS.map((skill) => (
            <Reveal key={skill.name} at={skill.isCurated ? cue(props, 0.55) : cue(props, 0.3)}>
              <ItemRow
                kind="skill"
                id={skill.name}
                note={<span style={{ color: skill.isCurated ? COLORS.agent : COLORS.muted }}>{skill.origin}</span>}
              />
            </Reveal>
          ))}
        </LibraryCard>
      </Reveal>
      <Reveal at={cue(props, 0.75)} style={{ fontSize: 32, color: COLORS.muted }}>
        mattpocock/skills doesn’t use Trail Guide. It doesn’t need to.
      </Reveal>
    </SceneFrame>
  );
}
