/** @fileoverview The tour's scenes and narration, in order; the voiceover script and the composition both read it. */

/** One scene of the tour and the narration spoken over it. */
export type SceneScript = {
  /** Stable ID, used for the scene's component and its voiceover file name. */
  id: SceneId;
  /** What the captions show. */
  narration: string;
  /**
   * What the voice reads, when it differs from the narration: the same words with
   * ElevenLabs audio tags such as `[sharing something new]` and capitals for emphasis.
   */
  speech?: string;
};

/** The IDs of the tour's scenes. */
export type SceneId =
  | 'guidance'
  | 'skills-today'
  | 'enter-trail-guide'
  | 'libraries-and-projects'
  | 'a-library'
  | 'package-manager'
  | 'what-a-rule-is'
  | 'mix-and-match'
  | 'an-insight'
  | 'into-rules'
  | 'where-they-live'
  | 'skills-as-playbooks'
  | 'get-started'
  | 'close';

/** The tour's scenes, in the order they play. */
export const SCENES = [
  {
    id: 'guidance',
    narration:
      'Your agents need guidance to build the right thing. Without it, they guess, and you keep correcting the same mistakes in review.',
    speech:
      '[sharing something new] Your agents need guidance to build the RIGHT thing. Without it, they guess, and you keep correcting the same mistakes in review.',
  },
  {
    id: 'skills-today',
    narration:
      'Today, the most common way to give that guidance is skills. But a skill is coarse-grained: one file bundles dozens of insights, so you can’t adopt one and leave the rest. And skills are versioned only loosely. Most people just take the latest and hope nothing important changed.',
  },
  {
    id: 'enter-trail-guide',
    narration:
      'Enter Trail Guide. Trail Guide gives your agents better guidance in two ways: it’s a package manager for skills, and it introduces a more granular kind of guidance, called a rule.',
  },
  {
    id: 'libraries-and-projects',
    narration:
      'Trail Guide has libraries and projects. A library is a repository of the skills and rules you want others to use. A project is a codebase that uses them.',
  },
  {
    id: 'a-library',
    narration:
      'First, the package manager. Fill your library with skills you write yourself, or curate them from others, even from repositories that don’t use Trail Guide.',
  },
  {
    id: 'package-manager',
    narration:
      'Mark each skill required or recommended, and a project installs them with one command, into every agent’s folder. When the library adds a skill, every project picks it up on its next trailguide project update.',
  },
  {
    id: 'what-a-rule-is',
    narration:
      'Second, rules. A rule is one best practice in one Markdown file: what to do, when it applies, and how to check it, with its own version.',
  },
  {
    id: 'mix-and-match',
    narration:
      'Mix and match rules and skills from many sources: your organization’s library, public libraries, and your own project. Adopt the ones that fit, and exclude the ones that don’t.',
  },
  {
    id: 'an-insight',
    narration:
      'And because a rule is so small, every insight can become one. Say you’ve just read an article on how to build better software factories.',
  },
  {
    id: 'into-rules',
    narration:
      'Ask your agent to turn it into rules. Each recommendation becomes its own rule, with examples your agent can follow.',
  },
  {
    id: 'where-they-live',
    narration:
      'Keep them in your project, add them to your organization’s library so every project gets them, or publish them in a public library, versioned, for anyone to use.',
  },
  {
    id: 'skills-as-playbooks',
    narration:
      'Skills still matter. Trail Guide installs skills from any repository, library or not. But in a Trail Guide library, a skill can declare the rules it uses, so you can improve the playbook and each rule separately, and every skill that uses a rule gets the update.',
  },
  {
    id: 'get-started',
    narration:
      'To get started, tell your agent to install Trail Guide. It installs the command line tool, sets up your project, and adds your first rules and skills.',
  },
  {
    id: 'close',
    narration: 'Trail Guide. The package manager for agent skills and rules.',
  },
] as const satisfies ReadonlyArray<SceneScript>;

/** The text the voice reads for a scene: its speech, or its narration when it has none. */
export function speechFor(scene: SceneScript): string {
  return scene.speech ?? scene.narration;
}
