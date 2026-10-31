/** @fileoverview Provides the seeded logo study choices and their reproducible selection recipe. */

import toggle from '../assets/fabrica-logo-study/seeded/toggle.png';
import arcade from '../assets/fabrica-logo-study/seeded/arcade.png';
import carve from '../assets/fabrica-logo-study/seeded/carve.png';
import ember from '../assets/fabrica-logo-study/seeded/ember.png';
import ligature from '../assets/fabrica-logo-study/seeded/ligature.png';
import tinker from '../assets/fabrica-logo-study/seeded/tinker.png';
import graft from '../assets/fabrica-logo-study/seeded/graft.png';
import resonance from '../assets/fabrica-logo-study/seeded/resonance.png';
import jig from '../assets/fabrica-logo-study/seeded/jig.png';
import column from '../assets/fabrica-logo-study/seeded/column.png';
export const seed = "f9e5d10460fc680107a2bb9998587bf94ac84d62f6a92a65697b2888c0a61d43";
export const mappingMethod = "Read ten consecutive six-character chunks as three bytes. For each dimension select byte modulo remaining list length, then remove that choice, so each concept uses a different combination. The final four characters select a shared finishing constraint modulo four.";
export const seededLogos = [
  { ...{"id": "toggle", "name": "Toggle", "description": "A switch with a little personality.", "metaphor": "a switch connecting energy", "form": "a compact vertical totem", "character": "unexpected friendly personality", "chunk": "f9e5d1", "scale": 123.0438, "position": "50.0000% 50.0000%"}, src: toggle.src },
  { ...{"id": "arcade", "name": "Arcade", "description": "A pixel arch from idea to something built.", "metaphor": "an architectural arch", "form": "a stepped pixel-grid silhouette", "character": "radical retro-computing charm", "chunk": "0460fc", "scale": 130.828, "position": "50.0000% 49.8308%"}, src: arcade.src },
  { ...{"id": "carve", "name": "Carve", "description": "Two pieces defined by the cut between them.", "metaphor": "a chisel carving material", "form": "two unequal complementary pieces", "character": "quiet order and simplicity", "chunk": "680107", "scale": 121.7727, "position": "50.0000% 49.7770%"}, src: carve.src },
  { ...{"id": "ember", "name": "Ember", "description": "A soft mass ready to release its energy.", "metaphor": "a comet at ignition", "form": "a soft monolithic form", "character": "kinetic tension and release", "chunk": "a2bb99", "scale": 116.7089, "position": "53.8990% 59.1906%"}, src: ember.src },
  { ...{"id": "ligature", "name": "Ligature", "description": "A useful knot becomes an unfamiliar glyph.", "metaphor": "a tightly tied useful knot", "form": "a continuous broad ribbon", "character": "futuristic experimental typography without letters", "chunk": "98587b", "scale": 129.6766, "position": "53.3103% 52.4392%"}, src: ligature.src },
  { ...{"id": "tinker", "name": "Tinker", "description": "A geometric frame with a creature's curiosity.", "metaphor": "a friendly abstract workshop creature", "form": "an open geometric frame", "character": "Swiss precision and restraint", "chunk": "f94ac8", "scale": 139.4982, "position": "54.0838% 54.6471%"}, src: tinker.src },
  { ...{"id": "graft", "name": "Graft", "description": "Separate forms growing toward each other.", "metaphor": "magnetic attraction", "form": "an expressive asymmetric shape", "character": "organic growth and curiosity", "chunk": "4d62f6", "scale": 124.2107, "position": "50.2046% 55.1140%"}, src: graft.src },
  { ...{"id": "resonance", "name": "Resonance", "description": "A tuning fork with architectural weight.", "metaphor": "a musical tuning fork", "form": "one bold compact silhouette", "character": "brutalist industrial confidence", "chunk": "a92a65", "scale": 132.5939, "position": "50.0000% 49.8378%"}, src: resonance.src },
  { ...{"id": "jig", "name": "Jig", "description": "An eccentric cut in a solid maker's stamp.", "metaphor": "a precision tooling jig", "form": "a block with one large negative-space cut", "character": "playful rebellious workshop energy", "chunk": "697b28", "scale": 132.2963, "position": "50.1633% 48.6934%"}, src: jig.src },
  { ...{"id": "column", "name": "Column", "description": "Three solid fins bringing ideas into alignment.", "metaphor": "a lens focusing light", "form": "three repeated solid elements", "character": "architectural weight and permanence", "chunk": "88c0a6", "scale": 114.8889, "position": "50.0000% 49.6923%"}, src: column.src },
];
