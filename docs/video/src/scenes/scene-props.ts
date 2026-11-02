/** @fileoverview The props every scene takes, and a helper to place cues within a scene's length. */

/** Props every scene component receives. */
export type SceneProps = {
  /** The scene's length in frames, which follows its narration clip. */
  durationInFrames: number;
};

/** The frame at a fraction of the way through a scene, so cues stay in step when narration length changes. */
export function cue({ durationInFrames }: SceneProps, fraction: number): number {
  return Math.round(durationInFrames * fraction);
}
