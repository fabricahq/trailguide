/**
 * @fileoverview Cards for Trail Guide's two kinds of repository, drawn differently so a
 * viewer can tell them apart at a glance: a library is a bound volume with a colored
 * spine, and a project is a folder with a tab.
 */
import type { CSSProperties, ReactNode } from 'react';
import { COLORS, FONTS } from '../theme';

/** Who a library is shared with, which sets its spine color and badge. */
export type LibraryVisibility = 'private' | 'public';

const LIBRARY_COLORS = { private: COLORS.success, public: COLORS.action } as const satisfies Record<
  LibraryVisibility,
  string
>;

function Badge({ color, children }: { color: string; children: ReactNode }) {
  return (
    <span
      style={{
        padding: '6px 14px',
        borderRadius: 999,
        border: `1.5px solid ${color}`,
        color,
        fontSize: 18,
        letterSpacing: '0.12em',
        fontWeight: 600,
        whiteSpace: 'nowrap',
      }}
    >
      {children}
    </span>
  );
}

function BooksIcon({ color }: { color: string }) {
  return (
    <svg width={40} height={40} viewBox="0 0 24 24" aria-hidden="true">
      <path
        d="M4 19.5V5a2 2 0 0 1 2-2h3v18H6a2 2 0 0 1-2-1.5Z M9 3h4v18H9 M14.5 4.2l3.6-1 3.4 16.6-3.6 1Z"
        fill="none"
        stroke={color}
        strokeWidth={1.7}
        strokeLinejoin="round"
      />
    </svg>
  );
}

function FolderIcon({ color }: { color: string }) {
  return (
    <svg width={40} height={40} viewBox="0 0 24 24" aria-hidden="true">
      <path
        d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2Z"
        fill="none"
        stroke={color}
        strokeWidth={1.7}
        strokeLinejoin="round"
      />
    </svg>
  );
}

/** Props for LibraryCard and ProjectCard. */
type CardProps = {
  /** The repository's name, such as `acme/.trailguide`. */
  name: string;
  /** A short line under the name, such as `Your organization`. */
  subtitle?: string;
  children?: ReactNode;
  style?: CSSProperties;
};

function CardHeader({
  icon,
  name,
  subtitle,
  badge,
}: {
  icon: ReactNode;
  name: string;
  subtitle?: string;
  badge: ReactNode;
}) {
  // The badge sits above the name, so the header fits a narrow card without wrapping the name.
  return (
    <div style={{ display: 'flex', alignItems: 'flex-start', gap: 18 }}>
      <div style={{ marginTop: 44 }}>{icon}</div>
      <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'flex-start', gap: 10, minWidth: 0 }}>
        {badge}
        <div style={{ fontSize: 34, fontWeight: 600, letterSpacing: '-0.02em', color: COLORS.ink }}>{name}</div>
        {subtitle === undefined ? null : <div style={{ fontSize: 24, color: COLORS.muted }}>{subtitle}</div>}
      </div>
    </div>
  );
}

/** A library: a card with a thick colored spine down its left edge. */
export function LibraryCard({
  name,
  subtitle,
  visibility,
  children,
  style,
}: CardProps & { visibility: LibraryVisibility }) {
  const color = LIBRARY_COLORS[visibility];
  return (
    <div
      style={{
        display: 'flex',
        flexDirection: 'column',
        gap: 22,
        padding: '28px 32px 28px 40px',
        borderRadius: '10px 22px 22px 10px',
        borderLeft: `14px solid ${color}`,
        background: `linear-gradient(90deg, ${color}1f, ${COLORS.sessionSurface} 40%)`,
        boxShadow: `0 30px 90px -50px ${color}66`,
        fontFamily: FONTS.sans,
        ...style,
      }}
    >
      <CardHeader
        icon={<BooksIcon color={color} />}
        name={name}
        {...(subtitle === undefined ? {} : { subtitle })}
        badge={<Badge color={color}>{visibility === 'private' ? 'PRIVATE LIBRARY' : 'PUBLIC LIBRARY'}</Badge>}
      />
      {children}
    </div>
  );
}

/** A project: a folder-shaped card with a tab on its top edge. */
export function ProjectCard({ name, subtitle, children, style }: CardProps) {
  const color = COLORS.agent;
  return (
    <div style={{ display: 'flex', flexDirection: 'column', fontFamily: FONTS.sans, ...style }}>
      <div
        style={{
          width: 180,
          height: 22,
          borderRadius: '14px 14px 0 0',
          background: `${color}40`,
          border: `1.5px solid ${color}88`,
          borderBottom: 'none',
        }}
      />
      <div
        style={{
          flex: 1,
          display: 'flex',
          flexDirection: 'column',
          gap: 22,
          padding: '26px 32px',
          borderRadius: '0 22px 22px 22px',
          border: `1.5px solid ${color}88`,
          background: `${color}14`,
        }}
      >
        <CardHeader
          icon={<FolderIcon color={color} />}
          name={name}
          {...(subtitle === undefined ? {} : { subtitle })}
          badge={<Badge color={color}>PROJECT</Badge>}
        />
        {children}
      </div>
    </div>
  );
}

/** Props for ItemRow. */
export type ItemRowProps = {
  kind: 'skill' | 'skills' | 'rule' | 'rules';
  id: string;
  /** Text at the row's right, such as a level or where the skill came from. */
  note?: ReactNode;
  isExcluded?: boolean;
};

/** One skill or rule in a card: a kind label, its ID, and an optional note. */
export function ItemRow({ kind, id, note, isExcluded = false }: ItemRowProps) {
  return (
    <div style={{ display: 'flex', alignItems: 'baseline', gap: 18, fontSize: 28 }}>
      <span
        style={{ width: 82, color: COLORS.muted, fontSize: 22, letterSpacing: '0.08em', textTransform: 'uppercase' }}
      >
        {kind}
      </span>
      <span
        style={{
          fontFamily: FONTS.mono,
          color: isExcluded ? COLORS.sessionMuted : COLORS.ink,
          textDecoration: isExcluded ? 'line-through' : 'none',
        }}
      >
        {id}
      </span>
      {note === undefined ? null : <span style={{ marginLeft: 'auto', paddingLeft: 24 }}>{note}</span>}
    </div>
  );
}
