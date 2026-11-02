/** @fileoverview Colors, fonts, and frame sizes shared by every scene, matching the docs site's dark example sessions. */
import '@fontsource-variable/inter';
import '@fontsource/jetbrains-mono/400.css';
import '@fontsource/jetbrains-mono/500.css';

/** The video's palette: the docs site's page colors plus its example-session accents. */
export const COLORS = {
  page: '#151515',
  ink: '#f2f2f2',
  muted: '#b8b8b8',
  border: '#444444',
  sessionSurface: '#121c2b',
  sessionSubtle: '#172336',
  sessionHeader: '#1b293e',
  sessionBorder: '#2c3a50',
  sessionText: '#e1e9f5',
  sessionMuted: '#99aac2',
  action: '#7fc5ff',
  agent: '#c5a4ff',
  success: '#80d9b2',
  error: '#ff9c9d',
  warning: '#f2c46d',
} as const;

/** Font stacks, with the bundled faces first. */
export const FONTS = {
  sans: "'Inter Variable', Inter, sans-serif",
  mono: "'JetBrains Mono', ui-monospace, monospace",
} as const;

/** Frame size and rate for every render. */
export const VIDEO = { width: 1920, height: 1080, fps: 30 } as const;

/** Frames each scene spends fading in and out. */
export const FADE_FRAMES = 10;
