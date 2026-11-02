/** @fileoverview Registers the tour composition with Remotion. */
import { Composition } from 'remotion';
import { VIDEO } from './theme';
import { calculateTourMetadata, Tour, type TourProps } from './tour';

const DEFAULT_PROPS: TourProps = { scenes: [] };

/** Lists the compositions Remotion can render. */
export function RemotionRoot() {
  return (
    <Composition
      id="TrailGuideTour"
      component={Tour}
      width={VIDEO.width}
      height={VIDEO.height}
      fps={VIDEO.fps}
      durationInFrames={1}
      defaultProps={DEFAULT_PROPS}
      calculateMetadata={calculateTourMetadata}
    />
  );
}
