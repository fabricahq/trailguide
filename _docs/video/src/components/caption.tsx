/** @fileoverview Subtitles for the narration: the current phrase, with spoken words brightened as the voice reaches them. */
import { useCurrentFrame, useVideoConfig } from 'remotion';
import { COLORS, FONTS } from '../theme';
import type { TimedWord } from '../voiceover';

/** The most words a caption shows at once. */
const MAX_WORDS_PER_PHRASE = 11;

/** Splits words into groups that each end with one of the given punctuation marks, or at the end. */
function splitAfter<T extends { text: string }>(words: ReadonlyArray<T>, pattern: RegExp): Array<Array<T>> {
  const groups: Array<Array<T>> = [];
  let current: Array<T> = [];
  for (const word of words) {
    current.push(word);
    if (pattern.test(word.text)) {
      groups.push(current);
      current = [];
    }
  }
  if (current.length > 0) groups.push(current);
  return groups;
}

/** Splits a clause too long for one caption into even chunks, so no chunk is a stray word or two. */
function splitEvenly<T>(clause: ReadonlyArray<T>): Array<Array<T>> {
  const count = Math.ceil(clause.length / MAX_WORDS_PER_PHRASE);
  const size = Math.ceil(clause.length / count);
  const chunks: Array<Array<T>> = [];
  for (let index = 0; index < clause.length; index += size) chunks.push(clause.slice(index, index + size));
  return chunks;
}

/**
 * Splits words into captions: each sentence on its own, its clauses joined while
 * they fit on one line, and a clause too long for a line split evenly.
 */
export function toPhrases(words: ReadonlyArray<TimedWord>): Array<Array<TimedWord>> {
  const phrases: Array<Array<TimedWord>> = [];
  for (const sentence of splitAfter(words, /[.?!]$/)) {
    let current: Array<TimedWord> = [];
    for (const clause of splitAfter(sentence, /[,:;]$/).flatMap(splitEvenly)) {
      if (current.length > 0 && current.length + clause.length > MAX_WORDS_PER_PHRASE) {
        phrases.push(current);
        current = [];
      }
      current.push(...clause);
    }
    if (current.length > 0) phrases.push(current);
  }
  return phrases;
}

/** Props for Caption. */
export type CaptionProps = { words: ReadonlyArray<TimedWord> };

/** Shows the phrase being spoken at the current frame. */
export function Caption({ words }: CaptionProps) {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  const seconds = frame / fps;
  const phrases = toPhrases(words);
  const phrase = phrases.find((candidate) => (candidate.at(-1)?.end ?? 0) >= seconds) ?? phrases.at(-1);
  if (phrase === undefined) return null;
  return (
    <div
      style={{
        position: 'absolute',
        left: 160,
        right: 160,
        bottom: 70,
        display: 'flex',
        justifyContent: 'center',
      }}
    >
      <div
        style={{
          padding: '18px 34px',
          borderRadius: 16,
          background: '#0d0d0dd9',
          border: `1.5px solid ${COLORS.border}`,
          fontFamily: FONTS.sans,
          fontSize: 40,
          lineHeight: 1.35,
          letterSpacing: '-0.01em',
          textAlign: 'center',
        }}
      >
        {phrase.map((word, index) => (
          <span key={index} style={{ color: word.start <= seconds ? COLORS.ink : COLORS.muted }}>
            {index > 0 ? ' ' : ''}
            {word.text}
          </span>
        ))}
      </div>
    </div>
  );
}
