/** @fileoverview Reads the voiceover manifest the voiceover script writes, and times each scene from its narration. */
import { SCENES, type SceneId } from './script';

/** A spoken word and when it starts and ends, in seconds from the start of its scene's clip. */
export type TimedWord = { text: string; start: number; end: number };

/** One scene's generated narration clip. */
export type VoiceoverClip = {
  /** Path under `public/`, such as `voiceover/sync.mp3`. */
  file: string;
  /** The speech the clip was generated from; a mismatch means the clip is stale. */
  text: string;
  /**
   * The neighboring scenes' speech the clip was generated with, which shapes its intonation;
   * a mismatch means the clip is stale. Absent from clips generated before it was recorded.
   */
  context?: string;
  words: ReadonlyArray<TimedWord>;
};

/** The manifest at `public/voiceover/manifest.json`. */
export type VoiceoverManifest = {
  voiceId: string;
  modelId: string;
  clips: Partial<Record<SceneId, VoiceoverClip>>;
};

/** Where the manifest is published, relative to `public/`. */
export const MANIFEST_PATH = 'voiceover/manifest.json';

/** Speaking rate used to estimate a scene's length before it has a clip. */
const ESTIMATED_WORDS_PER_SECOND = 2.5;

function isSceneId(value: string): value is SceneId {
  return SCENES.some((scene) => scene.id === value);
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function parseWord(value: unknown): TimedWord {
  if (
    !isRecord(value) ||
    typeof value.text !== 'string' ||
    typeof value.start !== 'number' ||
    typeof value.end !== 'number' ||
    value.end < value.start
  ) {
    throw new Error(`Invalid word in ${MANIFEST_PATH}: ${JSON.stringify(value)}`);
  }
  return { text: value.text, start: value.start, end: value.end };
}

function parseClip(value: unknown): VoiceoverClip {
  if (
    !isRecord(value) ||
    typeof value.file !== 'string' ||
    typeof value.text !== 'string' ||
    !Array.isArray(value.words)
  ) {
    throw new Error(`Invalid clip in ${MANIFEST_PATH}: expected file, text, and words`);
  }
  if (value.context !== undefined && typeof value.context !== 'string') {
    throw new Error(`Invalid clip in ${MANIFEST_PATH}: expected context to be a string`);
  }
  const clip: VoiceoverClip = { file: value.file, text: value.text, words: value.words.map(parseWord) };
  if (typeof value.context === 'string') clip.context = value.context;
  return clip;
}

/** Validates parsed manifest JSON, failing with the field that's wrong. */
export function parseManifest(value: unknown): VoiceoverManifest {
  if (
    !isRecord(value) ||
    typeof value.voiceId !== 'string' ||
    typeof value.modelId !== 'string' ||
    !isRecord(value.clips)
  ) {
    throw new Error(`Invalid ${MANIFEST_PATH}: expected voiceId, modelId, and clips`);
  }
  const clips: Partial<Record<SceneId, VoiceoverClip>> = {};
  for (const [id, clip] of Object.entries(value.clips)) {
    if (!isSceneId(id)) {
      throw new Error(`Invalid ${MANIFEST_PATH}: no scene has the ID ${id}`);
    }
    clips[id] = parseClip(clip);
  }
  return { voiceId: value.voiceId, modelId: value.modelId, clips };
}

/**
 * Spreads a narration's words across a duration by character count, for captions
 * on a scene that has no generated clip yet.
 */
export function estimateWords({
  text,
  durationInSeconds,
}: {
  text: string;
  durationInSeconds: number;
}): Array<TimedWord> {
  const words = text.split(/\s+/).filter((word) => word.length > 0);
  const totalCharacters = words.reduce((sum, word) => sum + word.length + 1, 0);
  let elapsed = 0;
  return words.map((word) => {
    const length = ((word.length + 1) / totalCharacters) * durationInSeconds;
    const timed = { text: word, start: elapsed, end: elapsed + length };
    elapsed += length;
    return timed;
  });
}

/**
 * Times the caption words from a clip's spoken words. Audio tags such as
 * `[sharing something new]` are dropped, and each remaining word takes the
 * narration's spelling, so emphasis capitals don't show. If the words don't line
 * up with the narration, they're spread across the clip instead.
 */
export function captionWords({
  spoken,
  narration,
  durationInSeconds,
}: {
  spoken: ReadonlyArray<TimedWord>;
  narration: string;
  durationInSeconds: number;
}): Array<TimedWord> {
  const narrationWords = narration.split(/\s+/).filter((word) => word.length > 0);
  const said: Array<TimedWord> = [];
  let isInTag = false;
  for (const word of spoken) {
    if (word.text.startsWith('[')) isInTag = true;
    if (!isInTag) said.push(word);
    if (word.text.endsWith(']')) isInTag = false;
  }
  if (said.length !== narrationWords.length) return estimateWords({ text: narration, durationInSeconds });
  return said.map((word, index) => ({ ...word, text: narrationWords[index] ?? word.text }));
}

/** How long a narration takes to say at an ordinary pace, for scenes with no clip yet. */
export function estimateNarrationSeconds(text: string): number {
  return text.split(/\s+/).length / ESTIMATED_WORDS_PER_SECOND;
}
