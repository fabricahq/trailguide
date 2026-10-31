/** @fileoverview Scene 6: a library sets each skill's level, a project installs them, and a skill the library adds arrives on the next update. */
import { ItemRow, LibraryCard } from '../components/cards';
import { Reveal } from '../components/reveal';
import { SceneFrame } from '../components/scene-frame';
import { Terminal, typingFrames } from '../components/terminal';
import { Check, Muted } from '../components/text';
import { COLORS } from '../theme';
import { cue, type SceneProps } from './scene-props';

const SKILLS = [
  { name: 'write-tests', level: 'required' },
  { name: 'review-change', level: 'required' },
  { name: 'tdd', level: 'recommended' },
] as const;

const NEW_SKILL = 'debug-flaky-tests';
const SYNC_COMMAND = 'trailguide project sync';
const UPDATE_COMMAND = 'trailguide project update';

function Level({ level }: { level: 'required' | 'recommended' }) {
  return <span style={{ color: level === 'required' ? COLORS.success : COLORS.action }}>{level}</span>;
}

/** Shows levels, one sync, then a new library skill arriving through an update. */
export function PackageManager(props: SceneProps) {
  const syncAt = cue(props, 0.24);
  const syncOutputAt = syncAt + typingFrames(SYNC_COMMAND) + 8;
  const newSkillAt = cue(props, 0.6);
  const updateAt = cue(props, 0.72);
  const updateOutputAt = updateAt + typingFrames(UPDATE_COMMAND) + 8;
  return (
    <SceneFrame durationInFrames={props.durationInFrames}>
      <Reveal at={0}>
        <LibraryCard name="acme/.trailguide" subtitle="Your organization’s library" visibility="private">
          {SKILLS.map((skill, index) => (
            <ItemRow
              key={skill.name}
              kind="skill"
              id={skill.name}
              note={
                <Reveal at={cue(props, 0.04 + index * 0.04)}>
                  <Level level={skill.level} />
                </Reveal>
              }
            />
          ))}
          <Reveal at={newSkillAt}>
            <ItemRow
              kind="skill"
              id={NEW_SKILL}
              note={
                <span>
                  <span style={{ color: COLORS.warning, marginRight: 20 }}>new</span>
                  <Level level="required" />
                </span>
              }
            />
          </Reveal>
        </LibraryCard>
      </Reveal>
      <Terminal
        tag="YOUR PROJECT"
        lines={[
          { kind: 'command', text: SYNC_COMMAND, at: syncAt },
          {
            kind: 'output',
            at: syncOutputAt,
            content: (
              <>
                <Check />
                {' 2 required skills  → .claude/skills  .agents/skills'}
              </>
            ),
          },
          {
            kind: 'output',
            at: syncOutputAt + 10,
            content: <Muted>{'  tdd is recommended: include recommended skills to install it'}</Muted>,
          },
          { kind: 'command', text: UPDATE_COMMAND, at: updateAt },
          {
            kind: 'output',
            at: updateOutputAt,
            content: (
              <>
                <span style={{ color: COLORS.success }}>+</span>
                {` ${NEW_SKILL}  `}
                <Muted>new required skill from acme</Muted>
              </>
            ),
          },
        ]}
      />
    </SceneFrame>
  );
}
