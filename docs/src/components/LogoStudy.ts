/** @fileoverview Resolves development logo-study routes into complete presentation data. */

import { seededLogos } from './SeededLogoStudies';
import { tinkerLogos } from './TinkerLogoStudies';
import { smallLogos } from './SmallLogoStudies';
import { socketLogos } from './SocketLogoStudies';
import { vectorLogos } from './VectorLogoStudies';

/** Visual properties shared by generated and vector logo concepts. */
type Logo = Pick<
  (typeof seededLogos)[number],
  'id' | 'name' | 'description' | 'src' | 'scale' | 'position'
>;
/** All route-specific content and display decisions needed by the comparison template. */
type Study = {
  readonly id: string;
  readonly title: string;
  readonly pageTitle: string;
  readonly eyebrow: string;
  readonly description: string;
  readonly next: { readonly href: string; readonly label: string };
  readonly sectionTitle: string;
  readonly showRecipe: boolean;
  readonly smallSamples: boolean;
  readonly reference: Logo | null;
  readonly concepts: ReadonlyArray<
    Logo & { readonly combination: (typeof seededLogos)[number] | null }
  >;
};

/** Find a required comparison reference by identity, failing early when the study inventory is incomplete. */
function referenceLogo(logos: ReadonlyArray<Logo>, id: string): Logo {
  const logo = logos.find((item) => item.id === id);
  if (logo === undefined)
    throw new Error(`Missing logo-study reference: ${id}`);
  return logo;
}

/** Adapt visual concepts that do not have a seeded combination recipe. */
function visualConcepts(logos: ReadonlyArray<Logo>): Study['concepts'] {
  return logos.map((item) => ({ ...item, combination: null }));
}

const seededStudy: Study = {
  id: 'seeded',
  title: 'Ten different directions.',
  pageTitle: 'Ten new directions',
  eyebrow: 'SEEDED EXPLORATION / TEN NEW CONCEPTS',
  description:
    'A wider search, from a pixel arch to a workshop creature. Your two favorites stay here as reference points.',
  next: { href: '/logo-study/tinker/', label: 'Explore Tinker variations ↗' },
  sectionTitle: 'The new concepts',
  showRecipe: true,
  smallSamples: false,
  reference: null,
  concepts: seededLogos.map((item) => ({ ...item, combination: item })),
};

/** Development study routes and their presentation data, in exploration order. */
export const logoStudies: ReadonlyArray<Study> = [
  seededStudy,
  {
    id: 'tinker',
    title: 'A little more Tinker.',
    pageTitle: 'Tinker variations',
    eyebrow: 'TINKER / THREE VARIATIONS',
    description:
      'The same curious workshop companion, with simpler geometry and more room to breathe at 16px.',
    next: {
      href: '/logo-study/small/',
      label: 'Explore smaller cube variations ↗',
    },
    sectionTitle: 'Reference + three variations',
    showRecipe: false,
    smallSamples: false,
    reference: null,
    concepts: visualConcepts([
      { ...referenceLogo(seededLogos, 'tinker'), name: 'Tinker Original' },
      ...tinkerLogos,
    ]),
  },
  {
    id: 'small',
    title: 'A cube that reads small.',
    pageTitle: 'Tinker variations',
    eyebrow: 'TINKER / SMALL SIZE EXPLORATION',
    description:
      'Larger dots, simpler outlines. Compare each at 16px, 12px and 8px before judging it enlarged.',
    next: {
      href: '/logo-study/socket/',
      label: 'Explore friendlier Socket expressions ↗',
    },
    sectionTitle: 'Reference + three variations',
    showRecipe: false,
    smallSamples: true,
    reference: null,
    concepts: visualConcepts([
      { ...referenceLogo(tinkerLogos, 'tinker-hex'), name: 'Previous Hex' },
      ...smallLogos,
    ]),
  },
  {
    id: 'socket',
    title: 'A friendlier Socket.',
    pageTitle: 'Socket expressions',
    eyebrow: 'SOCKET / THREE EXPRESSIONS',
    description:
      'Keep the cube. Give the little maker a warmer expression. The original stays here for comparison at the same sizes.',
    next: {
      href: '/logo-study/vector/',
      label: 'Explore four small vector marks ↗',
    },
    sectionTitle: 'Reference + three variations',
    showRecipe: false,
    smallSamples: true,
    reference: null,
    concepts: visualConcepts([
      {
        ...referenceLogo(smallLogos, 'tinker-socket'),
        name: 'Socket Original',
      },
      ...socketLogos,
    ]),
  },
  {
    id: 'vector',
    title: 'Less face. More character.',
    pageTitle: 'Small vector studies',
    eyebrow: 'FABRICA / FOUR SMALL MARKS',
    description:
      'Closed eyes, no smile. Four quieter expressions, drawn around the space available in a tiny header.',
    next: {
      href: '/logo-study/socket/',
      label: '← Previous Socket expressions',
    },
    sectionTitle: 'Four new variations',
    showRecipe: false,
    smallSamples: true,
    reference: referenceLogo(socketLogos, 'tinker-beam'),
    concepts: visualConcepts(vectorLogos),
  },
];

/** Resolve known routes, retaining the seeded study as the fallback for absent or unknown route values. */
export function logoStudy(route: string | undefined): Study {
  return logoStudies.find((study) => study.id === route) ?? seededStudy;
}
