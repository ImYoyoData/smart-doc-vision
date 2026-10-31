import React from 'react'
import { Composition } from 'remotion'
import { Promo } from './Promo'
import { LaunchComposition } from './Launch'
import { CinematicComposition } from './Cinematic'

export const RemotionRoot: React.FC = () => {
  return (
    <>
      <Composition
        id="SmartDocPromo"
        component={Promo}
        durationInFrames={1200}
        fps={30}
        width={1920}
        height={1080}
      />
      <Composition
        id="SmartDocLaunch"
        component={LaunchComposition}
        durationInFrames={1200}
        fps={30}
        width={1920}
        height={1080}
      />
      <Composition
        id="SmartDocCinematic"
        component={CinematicComposition}
        durationInFrames={1200}
        fps={30}
        width={1920}
        height={1080}
      />
    </>
  )
}
