/**
 * @fileoverview Generates each scene's narration with ElevenLabs and records it in the voiceover manifest.
 *
 * Run with `bun run voiceover`. It needs ELEVENLABS_API_KEY and ELEVENLABS_VOICE_ID,
 * and regenerates only scenes whose narration, voice, or model changed, so edits
 * to one line don't spend credits on the rest.
 */
import { existsSync } from 'node:fs';
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { SCENES, speechFor } from '../src/script';
import {
  MANIFEST_PATH,
  parseManifest,
  type TimedWord,
  type VoiceoverClip,
  type VoiceoverManifest,
} from '../src/voiceover';

const PUBLIC_DIR = join(import.meta.dir, '..', 'public');
const DEFAULT_MODEL_ID = 'eleven_multilingual_v2';
const OUTPUT_FORMAT = 'mp3_44100_128';

/** ElevenLabs' character-level timing for a clip. */
type Alignment = {
  characters: Array<string>;
  starts: Array<number>;
  ends: Array<number>;
};

function requireEnv(name: string): string {
  const value = process.env[name];
  if (value === undefined || value === '') {
    throw new Error(`Set ${name} to generate the voiceover. See docs/video/README.md.`);
  }
  return value;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function isNumberArray(value: unknown): value is Array<number> {
  return Array.isArray(value) && value.every((item) => typeof item === 'number');
}

function isStringArray(value: unknown): value is Array<string> {
  return Array.isArray(value) && value.every((item) => typeof item === 'string');
}

/** Validates the with-timestamps response: base64 audio plus per-character timing. */
function parseSpeech(value: unknown): { audio: Buffer; alignment: Alignment } {
  if (!isRecord(value) || typeof value.audio_base64 !== 'string' || !isRecord(value.alignment)) {
    throw new Error('ElevenLabs returned no audio_base64 or alignment');
  }
  const { characters, character_start_times_seconds, character_end_times_seconds } = value.alignment;
  if (
    !isStringArray(characters) ||
    !isNumberArray(character_start_times_seconds) ||
    !isNumberArray(character_end_times_seconds) ||
    characters.length !== character_start_times_seconds.length ||
    characters.length !== character_end_times_seconds.length
  ) {
    throw new Error('ElevenLabs returned an alignment whose character and timing lists differ');
  }
  return {
    audio: Buffer.from(value.audio_base64, 'base64'),
    alignment: { characters, starts: character_start_times_seconds, ends: character_end_times_seconds },
  };
}

/** Groups character timings into words, splitting on whitespace. */
function toWords(alignment: Alignment): Array<TimedWord> {
  const words: Array<TimedWord> = [];
  let current: TimedWord | null = null;
  for (const [index, character] of alignment.characters.entries()) {
    const start = alignment.starts[index] ?? 0;
    const end = alignment.ends[index] ?? start;
    if (/\s/.test(character)) {
      if (current !== null) words.push(current);
      current = null;
    } else if (current === null) {
      current = { text: character, start, end };
    } else {
      current = { text: current.text + character, start: current.start, end };
    }
  }
  if (current !== null) words.push(current);
  return words;
}

async function readManifest(): Promise<VoiceoverManifest | null> {
  const path = join(PUBLIC_DIR, MANIFEST_PATH);
  if (!existsSync(path)) return null;
  const json: unknown = JSON.parse(await readFile(path, 'utf8'));
  return parseManifest(json);
}

async function synthesize({
  apiKey,
  voiceId,
  modelId,
  text,
  previousText,
  nextText,
}: {
  apiKey: string;
  voiceId: string;
  modelId: string;
  text: string;
  previousText: string | undefined;
  nextText: string | undefined;
}): Promise<{ audio: Buffer; alignment: Alignment }> {
  const url = `https://api.elevenlabs.io/v1/text-to-speech/${encodeURIComponent(voiceId)}/with-timestamps?output_format=${OUTPUT_FORMAT}`;
  const response = await fetch(url, {
    method: 'POST',
    headers: { 'xi-api-key': apiKey, 'Content-Type': 'application/json' },
    // The neighboring lines keep intonation continuous across separately generated clips.
    body: JSON.stringify({ text, model_id: modelId, previous_text: previousText, next_text: nextText }),
  });
  if (!response.ok) {
    throw new Error(`ElevenLabs returned ${response.status}: ${await response.text()}`);
  }
  const json: unknown = await response.json();
  return parseSpeech(json);
}

async function main(): Promise<void> {
  const apiKey = requireEnv('ELEVENLABS_API_KEY');
  const voiceId = requireEnv('ELEVENLABS_VOICE_ID');
  const modelId = process.env.ELEVENLABS_MODEL_ID ?? DEFAULT_MODEL_ID;
  const previous = await readManifest();
  const isSameVoice = previous?.voiceId === voiceId && previous.modelId === modelId;
  const clips: Partial<Record<(typeof SCENES)[number]['id'], VoiceoverClip>> = {};
  await mkdir(join(PUBLIC_DIR, 'voiceover'), { recursive: true });

  for (const [index, scene] of SCENES.entries()) {
    const existing = isSameVoice ? previous.clips[scene.id] : undefined;
    const speech = speechFor(scene);
    const previousScene = SCENES[index - 1];
    const nextScene = SCENES[index + 1];
    const previousText = previousScene === undefined ? undefined : speechFor(previousScene);
    const nextText = nextScene === undefined ? undefined : speechFor(nextScene);
    const context = JSON.stringify([previousText ?? null, nextText ?? null]);
    // A clip from before contexts were recorded is kept and given the current context.
    const isSameContext = existing?.context === undefined || existing.context === context;
    if (existing?.text === speech && isSameContext && existsSync(join(PUBLIC_DIR, existing.file))) {
      clips[scene.id] = { ...existing, context };
      console.log(`unchanged  ${scene.id}`);
      continue;
    }
    const result = await synthesize({ apiKey, voiceId, modelId, text: speech, previousText, nextText });
    const file = `voiceover/${scene.id}.mp3`;
    await writeFile(join(PUBLIC_DIR, file), result.audio);
    clips[scene.id] = { file, text: speech, context, words: toWords(result.alignment) };
    console.log(`generated  ${scene.id}`);
  }

  const manifest: VoiceoverManifest = { voiceId, modelId, clips };
  await writeFile(join(PUBLIC_DIR, MANIFEST_PATH), `${JSON.stringify(manifest, null, 2)}\n`);
  console.log(`wrote      public/${MANIFEST_PATH}`);
}

await main();
