import React from 'react'
import { Composition } from 'remotion'
import { Promo } from './Promo'

export const RemotionRoot: React.FC = () => {
  return (
    <Composition
      id="SmartDocPromo"
      component={Promo}
      durationInFrames={1200}
      fps={30}
      width={1920}
      height={1080}
    />
  )
}
