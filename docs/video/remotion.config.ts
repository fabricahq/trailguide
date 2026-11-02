/** @fileoverview Uses lossless browser frame capture for the tour's vector artwork. */
import { Config } from '@remotion/cli/config';

// PNG capture avoids the intermittent tiled frames in the JPEG capture path.
Config.setVideoImageFormat('png');
