import React from 'react'
import {
  AbsoluteFill,
  Audio,
  Img,
  interpolate,
  spring,
  staticFile,
  useCurrentFrame,
} from 'remotion'

const FONT = '"Microsoft YaHei", "PingFang SC", "Noto Sans SC", "Segoe UI", sans-serif'
const MONO = '"Cascadia Code", "JetBrains Mono", Consolas, monospace'
const BEAT = 15
const CYAN = '#22d3ee'
const GREEN = '#34d399'
const AMBER = '#fbbf24'
const RED = '#f87171'
const BLUE = '#60a5fa'

const clamp = {
  extrapolateLeft: 'clamp' as const,
  extrapolateRight: 'clamp' as const,
}

const kick = (frame: number) => Math.exp(-((frame % BEAT) / 3.2))
const easeOut = (value: number) => 1 - Math.pow(1 - value, 3)
const saturate = (value: number) => Math.min(1, Math.max(0, value))

function snareFlash(frame: number) {
  const local = frame % 60
  const distance = Math.min(Math.abs(local - BEAT), Math.abs(local - BEAT * 3))
  return distance < 4 ? (1 - distance / 4) * 0.2 : 0
}

const impactFrames = [0, 480, 840, 1080]
function impactEnergy(frame: number) {
  let value = 0
  for (const hit of impactFrames) {
    const distance = frame - hit
    if (distance >= 0 && distance < 36) value = Math.max(value, 1 - distance / 36)
  }
  return value
}

const Logo: React.FC<{ size?: number }> = ({ size = 96 }) => (
  <div
    style={{
      width: size,
      height: size,
      borderRadius: size * 0.27,
      background: 'linear-gradient(150deg, #7dd3fc, #2563eb 48%, #0ea5e9)',
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      border: '1px solid rgba(191,219,254,.55)',
      boxShadow: '0 0 70px rgba(37,99,235,.65), inset 0 0 24px rgba(255,255,255,.18)',
    }}
  >
    <svg width={size * 0.52} height={size * 0.52} viewBox="0 0 24 24" fill="none">
      <path d="M7 3.8h7l4 4v12.4H7z" stroke="#fff" strokeWidth="1.5" strokeLinejoin="round" />
      <path d="M14 3.8v4h4" stroke="#fff" strokeWidth="1.5" strokeLinejoin="round" />
      <path d="M9.4 13.4l1.9 1.9 3.5-3.7" stroke="#e0f2fe" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  </div>
)

const SectionHeader: React.FC<{
  index: string
  kicker: string
  title: string
  subtitle: string
  local: number
  accent?: string
}> = ({ index, kicker, title, subtitle, local, accent = CYAN }) => {
  const enter = interpolate(local, [0, 16], [0, 1], clamp)
  return (
    <div
      style={{
        position: 'absolute',
        left: 110,
        top: 76,
        opacity: enter,
        transform: `translateY(${(1 - enter) * 26}px)`,
      }}
    >
      <div style={{ display: 'flex', alignItems: 'center', gap: 16 }}>
        <span style={{ fontFamily: MONO, fontSize: 20, color: accent, letterSpacing: 4 }}>{index}</span>
        <span style={{ width: 58, height: 3, background: accent, boxShadow: `0 0 18px ${accent}` }} />
        <span style={{ fontFamily: MONO, fontSize: 17, color: '#93c5fd', letterSpacing: 6 }}>{kicker}</span>
      </div>
      <div style={{ marginTop: 18, fontSize: 62, fontWeight: 800, color: '#f8fafc', letterSpacing: 4 }}>
        {title}
      </div>
      <div style={{ marginTop: 10, display: 'flex', alignItems: 'center', gap: 10, fontSize: 23, color: '#94a3b8', letterSpacing: 1 }}>
        <span style={{ color: accent, fontSize: 19 }}>✓</span>
        {subtitle}
      </div>
    </div>
  )
}

type TransitionKind = 'wipe' | 'slices' | 'zoom' | 'streak'

const TRANSITIONS: { at: number; kind: TransitionKind }[] = [
  { at: 120, kind: 'slices' },
  { at: 180, kind: 'wipe' },
  { at: 960, kind: 'slices' },
  { at: 1080, kind: 'zoom' },
]

const Wipe: React.FC<{ progress: number }> = ({ progress }) => {
  const barX = interpolate(progress, [0, 1], [-1600, 2100])
  const secondX = interpolate(progress, [0, 1], [-2100, 1500])
  const glow = Math.sin(progress * Math.PI)
  return (
    <AbsoluteFill>
      <AbsoluteFill style={{ background: '#020617', opacity: glow * 0.32 }} />
      <div
        style={{
          position: 'absolute',
          top: -420,
          left: barX,
          width: 1000,
          height: 1900,
          transform: 'skewX(-16deg)',
          background:
            'linear-gradient(90deg, rgba(34,211,238,0) 0%, rgba(34,211,238,.45) 34%, rgba(255,255,255,.95) 49%, rgba(96,165,250,.7) 58%, rgba(59,130,246,0) 100%)',
          filter: 'blur(1px)',
          opacity: glow,
        }}
      />
      <div
        style={{
          position: 'absolute',
          top: -420,
          left: secondX,
          width: 420,
          height: 1900,
          transform: 'skewX(-16deg)',
          background: 'linear-gradient(90deg, transparent, rgba(96,165,250,.35), transparent)',
          filter: 'blur(2px)',
        }}
      />
      <div
        style={{
          position: 'absolute',
          left: 0,
          right: 0,
          top: '50%',
          height: 2,
          background: 'rgba(226,232,240,.75)',
          opacity: glow * 0.7,
          boxShadow: '0 0 26px rgba(34,211,238,.95)',
        }}
      />
    </AbsoluteFill>
  )
}

const Slices: React.FC<{ progress: number }> = ({ progress }) => {
  const flash = Math.sin(progress * Math.PI)
  return (
    <AbsoluteFill>
      <AbsoluteFill style={{ background: 'rgba(2,6,23,.55)', opacity: flash }} />
      {Array.from({ length: 10 }).map((_, index) => {
        const direction = index % 2 === 0 ? 1 : -1
        const stagger = 1 - Math.min(0.32, index * 0.028)
        const cover =
          progress < 0.5
            ? easeOut(Math.min(1, progress * 2 * stagger))
            : 1 - easeOut(Math.min(1, (progress - 0.5) * 2 * stagger))
        const shift = direction * (1 - cover) * 2200
        const seam = index % 3 === 0 ? 'rgba(34,211,238,.55)' : 'rgba(96,165,250,.3)'
        return (
          <div
            key={index}
            style={{
              position: 'absolute',
              left: 0,
              width: 1920,
              top: `${index * 10}%`,
              height: '10.2%',
              transform: `translateX(${shift}px)`,
              background:
                index % 3 === 0
                  ? 'linear-gradient(90deg, #030712, #0b1e3f 58%, #061225)'
                  : index % 2 === 0
                    ? 'linear-gradient(90deg, #020617, #0a1730 55%, #040b18)'
                    : 'linear-gradient(90deg, #040b18, #0c1a2e 46%, #020617)',
              borderTop: `1px solid ${seam}`,
              boxShadow: `inset 0 1px 18px rgba(34,211,238,${0.05 + (index % 3) * 0.03})`,
            }}
          >
            <div
              style={{
                position: 'absolute',
                left: `${(index * 37) % 82}%`,
                top: 0,
                width: 160 + (index % 4) * 90,
                height: '100%',
                background: `linear-gradient(90deg, transparent, ${seam}, transparent)`,
                opacity: flash * 0.24,
              }}
            />
          </div>
        )
      })}
      <AbsoluteFill style={{ background: '#fff', opacity: Math.max(0, flash - 0.9) * 0.5, mixBlendMode: 'screen' }} />
    </AbsoluteFill>
  )
}

const Zoom: React.FC<{ progress: number }> = ({ progress }) => {
  const flash = Math.sin(progress * Math.PI)
  const scale = interpolate(progress, [0, 1], [0.4, 2.7])
  return (
    <AbsoluteFill>
      <div
        style={{
          position: 'absolute',
          left: '50%',
          top: '50%',
          width: 900,
          height: 900,
          marginLeft: -450,
          marginTop: -450,
          borderRadius: '50%',
          border: `${10 * (1 - progress) + 2}px solid rgba(147,197,253,${flash * 0.8})`,
          transform: `scale(${scale})`,
          boxShadow: `0 0 120px rgba(34,211,238,${flash * 0.8})`,
        }}
      />
      <div
        style={{
          position: 'absolute',
          left: '50%',
          top: '50%',
          width: 320,
          height: 320,
          marginLeft: -160,
          marginTop: -160,
          borderRadius: '50%',
          background: `radial-gradient(circle, rgba(255,255,255,${flash * 0.95}), rgba(96,165,250,${flash * 0.4}) 45%, transparent 72%)`,
        }}
      />
      {Array.from({ length: 12 }).map((_, index) => (
        <div
          key={index}
          style={{
            position: 'absolute',
            left: '50%',
            top: '50%',
            width: 2,
            height: 560,
            background: 'linear-gradient(180deg, transparent, rgba(191,219,254,.8), transparent)',
            transform: `rotate(${index * 30}deg) translateY(-280px)`,
            opacity: flash * 0.5,
          }}
        />
      ))}
      <AbsoluteFill style={{ background: '#020617', opacity: flash * 0.22 }} />
    </AbsoluteFill>
  )
}

const Streak: React.FC<{ progress: number }> = ({ progress }) => {
  const flash = Math.sin(progress * Math.PI)
  return (
    <AbsoluteFill>
      <AbsoluteFill style={{ background: '#020617', opacity: flash * 0.4 }} />
      {Array.from({ length: 22 }).map((_, index) => {
        const x = ((index * 173) % 1980) - 30
        const direction = index % 2 === 0 ? 1 : -1
        const travel = interpolate(progress, [0, 1], [-1400 * direction, 1400 * direction])
        const height = 120 + ((index * 97) % 420)
        return (
          <div
            key={index}
            style={{
              position: 'absolute',
              left: x,
              top: travel,
              width: 2 + (index % 3),
              height,
              transform: 'skewX(-12deg)',
              background:
                index % 4 === 0
                  ? 'rgba(255,255,255,.9)'
                  : index % 3 === 0
                    ? 'rgba(34,211,238,.8)'
                    : 'rgba(96,165,250,.7)',
              boxShadow: '0 0 18px rgba(96,165,250,.8)',
              opacity: flash * 0.9,
            }}
          />
        )
      })}
    </AbsoluteFill>
  )
}

const TransitionOverlay: React.FC = () => {
  const frame = useCurrentFrame()
  return (
    <AbsoluteFill style={{ pointerEvents: 'none' }}>
      {TRANSITIONS.map(({ at, kind }) => {
        const local = frame - (at - 7)
        if (local < 0 || local > 14) return null
        const progress = local / 14
        if (kind === 'wipe') return <Wipe key={at} progress={progress} />
        if (kind === 'slices') return <Slices key={at} progress={progress} />
        if (kind === 'zoom') return <Zoom key={at} progress={progress} />
        return <Streak key={at} progress={progress} />
      })}
    </AbsoluteFill>
  )
}

type ChapterCard = {
  at: number
  index: string
  kicker: string
  title: string
  sub: string
  chips: string[]
  accent: string
  reverse?: boolean
}

const CHAPTER_CARDS: ChapterCard[] = [
  {
    at: 240,
    index: '01',
    kicker: 'MULTIMODAL',
    title: '图文联防',
    sub: '多段文本 + 多张图片，一次判定',
    chips: ['联合判定', '漏判更少'],
    accent: CYAN,
  },
  {
    at: 360,
    index: '02',
    kicker: 'RISK ENGINE',
    title: '判定有据',
    sub: '权重、阈值、依据，全部可追溯',
    chips: ['可解释', '可调优'],
    accent: AMBER,
    reverse: true,
  },
  {
    at: 480,
    index: '03',
    kicker: 'RESULT ACTIONS',
    title: '一次判定，全部触发',
    sub: 'HTTP · Redis · JS 并行执行',
    chips: ['并行', '自动重试', '故障隔离'],
    accent: BLUE,
  },
  {
    at: 600,
    index: '04',
    kicker: 'CUSTOM SCRIPT',
    title: '复杂逻辑，脚本兜底',
    sub: '内置 HTTP / 数据库 / 加密能力',
    chips: ['沙盒隔离', '密钥不出服务端'],
    accent: '#a78bfa',
    reverse: true,
  },
  {
    at: 720,
    index: '05',
    kicker: 'MODEL ROUTING',
    title: '故障不断审',
    sub: '优先级调度 + 自动熔断回落',
    chips: ['1.2s 接管', '审核不中断'],
    accent: GREEN,
  },
  {
    at: 840,
    index: '06',
    kicker: 'OBSERVABILITY',
    title: '每一笔，都算得清',
    sub: 'Token · 延迟 · 成本全量可见',
    chips: ['可计量', '可复盘'],
    accent: CYAN,
    reverse: true,
  },
]

const CHAPTER_CUT = 22
const CHAPTER_LEAD = 11

const ChapterCut: React.FC = () => {
  const frame = useCurrentFrame()
  return (
    <AbsoluteFill style={{ pointerEvents: 'none', overflow: 'hidden' }}>
      {CHAPTER_CARDS.map((card, cardIndex) => {
        const local = frame - (card.at - CHAPTER_LEAD)
        if (local < 0 || local > CHAPTER_CUT) return null
        const p = local / CHAPTER_CUT
        const enter = easeOut(saturate(p / 0.4))
        const exit = easeOut(saturate((p - 0.68) / 0.32))
        const direction = card.reverse ? -1 : 1
        const x = direction * ((1 - enter) * 118 - exit * 122)
        const textIn = interpolate(p, [0.06, 0.24], [0, 1], clamp)
        const textOut = interpolate(p, [0.84, 0.98], [1, 0], clamp)
        const textOpacity = textIn * textOut
        const titleScale = interpolate(p, [0.2, 0.5], [0.94, 1], clamp)
        const flash = Math.max(0, 1 - Math.abs(p - 0.47) / 0.16)
        return (
          <React.Fragment key={card.at}>
            <div
              style={{
                position: 'absolute',
                inset: 0,
                transform: `translateX(${x}%)`,
                background: 'linear-gradient(118deg, #030711 0%, #05162b 46%, #082742 100%)',
                boxShadow: '0 0 140px rgba(2,6,23,.95)',
              }}
            >
              <AbsoluteFill
                style={{
                  backgroundImage:
                    'linear-gradient(rgba(148,163,184,.06) 1px, transparent 1px), linear-gradient(90deg, rgba(148,163,184,.06) 1px, transparent 1px)',
                  backgroundSize: '72px 72px',
                  opacity: 0.7,
                }}
              />
              <AbsoluteFill
                style={{
                  background: `radial-gradient(circle at 70% 42%, ${card.accent}24, transparent 52%), radial-gradient(circle at 12% 86%, rgba(37,99,235,.3), transparent 58%)`,
                }}
              />
              <div
                style={{
                  position: 'absolute',
                  left: -260,
                  top: -300,
                  width: 1000,
                  height: 1700,
                  transform: 'skewX(-16deg)',
                  background: `linear-gradient(90deg, transparent, ${card.accent}10 45%, transparent)`,
                }}
              />
              <div style={{ position: 'absolute', right: 60, top: 206, width: 560, height: 560 }}>
                {[0, 1].map((ring) => (
                  <div
                    key={ring}
                    style={{
                      position: 'absolute',
                      inset: ring * 54,
                      borderRadius: '50%',
                      border: `1px ${ring === 0 ? 'solid' : 'dashed'} ${card.accent}26`,
                      transform: `rotate(${frame * (ring === 0 ? 0.5 : -0.35)}deg)`,
                    }}
                  />
                ))}
                <div
                  style={{
                    position: 'absolute',
                    inset: 0,
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    fontFamily: MONO,
                    fontSize: 420,
                    fontWeight: 800,
                    color: 'transparent',
                    WebkitTextStroke: `2px ${card.accent}3d`,
                    letterSpacing: -24,
                    lineHeight: 1,
                    transform: `translateY(${Math.sin(frame * 0.06) * 10}px)`,
                  }}
                >
                  {card.index}
                </div>
              </div>
              <div style={{ position: 'absolute', left: 168, top: 292, opacity: textOpacity }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 18, transform: `translateY(${(1 - textIn) * 22}px)` }}>
                  <span style={{ fontFamily: MONO, fontSize: 17, color: card.accent, letterSpacing: 6 }}>
                    CHAPTER {card.index}
                  </span>
                  <span style={{ width: 54, height: 2, background: card.accent }} />
                  <span style={{ fontFamily: MONO, fontSize: 15, color: '#94a3b8', letterSpacing: 5 }}>{card.kicker}</span>
                </div>
                <div
                  style={{
                    marginTop: 28,
                    fontSize: 116,
                    fontWeight: 900,
                    color: '#f8fafc',
                    letterSpacing: 5,
                    lineHeight: 1.02,
                    textShadow: `0 0 80px ${card.accent}66`,
                    transform: `scale(${titleScale})`,
                    transformOrigin: 'left center',
                    whiteSpace: 'nowrap',
                  }}
                >
                  {card.title}
                </div>
                <div style={{ marginTop: 13, width: 146 * textIn, height: 5, background: card.accent, boxShadow: `0 0 24px ${card.accent}` }} />
                <div style={{ marginTop: 24, fontSize: 32, color: '#cbd5e1', letterSpacing: 2 }}>{card.sub}</div>
                <div style={{ marginTop: 30, display: 'flex', gap: 12 }}>
                  {card.chips.map((chip, chipIndex) => {
                    const chipIn = interpolate(p, [0.15 + chipIndex * 0.04, 0.3 + chipIndex * 0.04], [0, 1], clamp)
                    return (
                      <span
                        key={chip}
                        style={{
                          padding: '10px 20px',
                          borderRadius: 999,
                          border: `1px solid ${card.accent}55`,
                          background: `${card.accent}12`,
                          color: '#e2e8f0',
                          fontFamily: MONO,
                          fontSize: 17,
                          letterSpacing: 1,
                          opacity: chipIn,
                          transform: `translateY(${(1 - chipIn) * 14}px)`,
                          whiteSpace: 'nowrap',
                        }}
                      >
                        <span style={{ color: card.accent, marginRight: 8 }}>✓</span>
                        {chip}
                      </span>
                    )
                  })}
                </div>
              </div>
              <div
                style={{
                  position: 'absolute',
                  left: 168,
                  bottom: 118,
                  display: 'flex',
                  alignItems: 'center',
                  gap: 18,
                  opacity: textOpacity,
                }}
              >
                <span style={{ fontFamily: MONO, fontSize: 14, color: '#64748b', letterSpacing: 4 }}>SMARTDOC VISION</span>
                <span style={{ width: 1, height: 16, background: 'rgba(148,163,184,.25)' }} />
                <div style={{ display: 'flex', gap: 8 }}>
                  {CHAPTER_CARDS.map((entry, index) => (
                    <span
                      key={entry.at}
                      style={{
                        width: index === cardIndex ? 42 : 14,
                        height: 4,
                        borderRadius: 99,
                        background: index === cardIndex ? card.accent : 'rgba(148,163,184,.24)',
                        boxShadow: index === cardIndex ? `0 0 14px ${card.accent}` : 'none',
                      }}
                    />
                  ))}
                </div>
              </div>
              <div
                style={{
                  position: 'absolute',
                  right: 96,
                  bottom: 118,
                  fontFamily: MONO,
                  fontSize: 13,
                  color: 'rgba(148,163,184,.45)',
                  letterSpacing: 5,
                  opacity: textOpacity,
                }}
              >
                CORE ADVANTAGE
              </div>
              <AbsoluteFill style={{ background: '#fff', opacity: flash * 0.12, mixBlendMode: 'screen' }} />
            </div>
            <div
              style={{
                position: 'absolute',
                left: `calc(${x}% - 3px)`,
                top: 0,
                width: 6,
                height: '100%',
                background: `linear-gradient(180deg, transparent, ${card.accent}, transparent)`,
                opacity: 0.5 + flash * 0.4,
                boxShadow: `0 0 40px ${card.accent}`,
                transform: 'skewX(-10deg)',
              }}
            />
            <div
              style={{
                position: 'absolute',
                left: `calc(${x + 100}% - 3px)`,
                top: 0,
                width: 6,
                height: '100%',
                background: `linear-gradient(180deg, transparent, ${card.accent}, transparent)`,
                opacity: 0.5 + flash * 0.4,
                boxShadow: `0 0 40px ${card.accent}`,
                transform: 'skewX(-10deg)',
              }}
            />
          </React.Fragment>
        )
      })}
    </AbsoluteFill>
  )
}

const Equalizer: React.FC = () => {
  const frame = useCurrentFrame()
  const pulse = kick(frame)
  const bars = 64
  return (
    <div
      style={{
        position: 'absolute',
        left: 0,
        right: 0,
        bottom: 0,
        height: 150,
        display: 'flex',
        alignItems: 'flex-end',
        gap: 5,
        padding: '0 26px 18px',
        opacity: 0.5,
        maskImage: 'linear-gradient(180deg, transparent, black 55%)',
        WebkitMaskImage: 'linear-gradient(180deg, transparent, black 55%)',
      }}
    >
      {Array.from({ length: bars }).map((_, index) => {
        const wave =
          0.35 +
          0.3 * Math.abs(Math.sin(index * 0.42 + frame * 0.12)) +
          0.2 * Math.abs(Math.sin(index * 0.17 - frame * 0.2))
        const height = 16 + wave * 92 * (0.55 + pulse * 0.9)
        return (
          <div
            key={index}
            style={{
              flex: 1,
              height,
              borderRadius: 3,
              background:
                index % 3 === 0
                  ? 'linear-gradient(180deg, #a5f3fc, #0891b2)'
                  : 'linear-gradient(180deg, #93c5fd, #1d4ed8)',
              opacity: 0.35 + wave * 0.55,
            }}
          />
        )
      })}
    </div>
  )
}

const Flash: React.FC = () => {
  const frame = useCurrentFrame()
  const value = Math.min(0.5, snareFlash(frame) + impactEnergy(frame) * 0.42)
  return (
    <AbsoluteFill
      style={{
        background:
          'radial-gradient(circle at 50% 45%, rgba(255,255,255,.95), rgba(147,197,253,.35) 45%, transparent 72%)',
        opacity: value,
        mixBlendMode: 'screen',
        pointerEvents: 'none',
      }}
    />
  )
}

const Shockwave: React.FC = () => {
  const frame = useCurrentFrame()
  return (
    <AbsoluteFill style={{ pointerEvents: 'none' }}>
      {impactFrames.filter((hit) => hit === 0 || hit === 1080).map((hit) => {
        const local = frame - hit
        if (local < 0 || local > 45) return null
        const progress = local / 45
        const size = 260 + progress * 1500
        return (
          <div
            key={hit}
            style={{
              position: 'absolute',
              left: '50%',
              top: '46%',
              width: size,
              height: size,
              marginLeft: -size / 2,
              marginTop: -size / 2,
              borderRadius: '50%',
              border: `${3 + (1 - progress) * 5}px solid rgba(147,197,253,${(1 - progress) * 0.75})`,
              boxShadow: `0 0 ${60 * (1 - progress)}px rgba(34,211,238,${(1 - progress) * 0.55})`,
            }}
          />
        )
      })}
    </AbsoluteFill>
  )
}

const Particles: React.FC = () => {
  const frame = useCurrentFrame()
  const pulse = kick(frame)
  return (
    <AbsoluteFill style={{ pointerEvents: 'none' }}>
      {Array.from({ length: 54 }).map((_, index) => {
        const seedX = (index * 137.5) % 100
        const seedY = (index * 73.3) % 100
        const drift = (frame * (0.25 + (index % 5) * 0.08)) % 120
        const size = 2 + (index % 4)
        return (
          <div
            key={index}
            style={{
              position: 'absolute',
              left: `${seedX}%`,
              top: `${(seedY + drift * 0.3) % 110}%`,
              width: size,
              height: size,
              borderRadius: '50%',
              background: index % 3 === 0 ? CYAN : '#93c5fd',
              opacity: 0.15 + pulse * 0.35,
              boxShadow: `0 0 ${6 + pulse * 10}px rgba(147,197,253,.8)`,
            }}
          />
        )
      })}
    </AbsoluteFill>
  )
}

const GlitchTitle: React.FC<{ text: string; size?: number }> = ({ text, size = 150 }) => {
  const frame = useCurrentFrame()
  const pulse = kick(frame)
  const jitter = pulse > 0.35 ? (frame % 3) - 1 : 0
  return (
    <div style={{ position: 'relative', textAlign: 'center' }}>
      <div
        style={{
          position: 'absolute',
          inset: 0,
          color: '#f43f5e',
          transform: `translate(${jitter * 6}px, ${-jitter * 2}px)`,
          opacity: 0.55,
          fontSize: size,
          fontWeight: 800,
          letterSpacing: 12,
          filter: 'blur(1px)',
        }}
      >
        {text}
      </div>
      <div
        style={{
          position: 'absolute',
          inset: 0,
          color: '#22d3ee',
          transform: `translate(${-jitter * 6}px, ${jitter * 2}px)`,
          opacity: 0.55,
          fontSize: size,
          fontWeight: 800,
          letterSpacing: 12,
          filter: 'blur(1px)',
        }}
      >
        {text}
      </div>
      <div
        style={{
          position: 'relative',
          color: '#f8fafc',
          fontSize: size,
          fontWeight: 800,
          letterSpacing: 12,
          textShadow: '0 0 38px rgba(96,165,250,.55)',
        }}
      >
        {text}
      </div>
    </div>
  )
}

const Intro: React.FC = () => {
  const frame = useCurrentFrame()
  const pulse = kick(frame)
  const logoScale = spring({ frame, fps: 30, config: { damping: 12, stiffness: 110 } })
  return (
    <AbsoluteFill style={{ alignItems: 'center', justifyContent: 'center' }}>
      {[0, 1, 2].map((ring) => (
        <div
          key={ring}
          style={{
            position: 'absolute',
            width: 460 + ring * 190,
            height: 460 + ring * 190,
            borderRadius: '50%',
            border: `1px ${ring === 1 ? 'dashed' : 'solid'} rgba(96,165,250,${0.16 - ring * 0.04})`,
            transform: `rotate(${frame * (ring % 2 === 0 ? 0.4 : -0.3)}deg) scale(${1 + pulse * 0.02})`,
          }}
        />
      ))}
      <div style={{ transform: `scale(${logoScale + pulse * 0.02})` }}>
        <Logo size={120} />
      </div>
      <div style={{ marginTop: 36 }}>
        <GlitchTitle text="智审图档" size={146} />
      </div>
      <div
        style={{
          marginTop: 16,
          color: '#93c5fd',
          fontFamily: MONO,
          fontSize: 22,
          letterSpacing: 12,
          opacity: interpolate(frame, [18, 45], [0, 1], clamp),
        }}
      >
        SMARTDOC VISION
      </div>
      <div
        style={{
          marginTop: 34,
          display: 'flex',
          gap: 18,
          opacity: interpolate(frame, [56, 80], [0, 1], clamp),
        }}
      >
        {[
          { label: 'PASS', color: GREEN },
          { label: 'REVIEW', color: AMBER },
          { label: 'REJECT', color: RED },
        ].map((item, index) => (
          <span
            key={item.label}
            style={{
              padding: '10px 24px',
              borderRadius: 10,
              border: `1px solid ${item.color}66`,
              background: `${item.color}1a`,
              color: item.color,
              fontFamily: MONO,
              fontSize: 24,
              letterSpacing: 3,
              transform: `translateY(${interpolate(frame, [56 + index * 6, 74 + index * 6], [16, 0], clamp)}px)`,
            }}
          >
            {item.label}
          </span>
        ))}
      </div>
      <div
        style={{
          position: 'absolute',
          bottom: 84,
          color: 'rgba(148,163,184,.65)',
          fontFamily: MONO,
          fontSize: 16,
          letterSpacing: 7,
          opacity: interpolate(frame, [78, 104], [0, 1], clamp),
        }}
      >
        多模态 · 规则化 · 全链路闭环
      </div>
    </AbsoluteFill>
  )
}

const FLASH_WORDS = [
  { text: '多模态审核', sub: '文本 + 图片', accent: CYAN },
  { text: '规则化判定', sub: 'PASS / REVIEW / REJECT', accent: BLUE },
  { text: '结果动作', sub: 'HTTP · Redis · JS', accent: '#a78bfa' },
  { text: '模型接入', sub: '优先级 + 自动熔断', accent: GREEN },
  { text: '全量观测', sub: 'Token · 延迟 · 成本', accent: AMBER },
  { text: '零侵入接入', sub: 'OpenAPI 直接对接', accent: '#f472b6' },
  { text: '40 万上下文', sub: '长文档整本读入', accent: CYAN },
  { text: '全链路闭环', sub: '提交 → 判定 → 回调', accent: BLUE },
]

const FlashWords: React.FC = () => {
  const frame = useCurrentFrame()
  const index = Math.min(FLASH_WORDS.length - 1, Math.floor((frame - 120) / 15))
  const local = (frame - 120) % 15
  const item = FLASH_WORDS[index]
  const enter = interpolate(local, [0, 4], [0, 1], clamp)
  const hit = local < 4 ? 1 - local / 4 : 0
  return (
    <AbsoluteFill>
      <div
        style={{
          position: 'absolute',
          left: 0,
          top: 0,
          bottom: 0,
          width: '48%',
          clipPath: 'polygon(0 0, 100% 0, 78% 100%, 0 100%)',
          background: `linear-gradient(135deg, ${item.accent}26, transparent 72%)`,
        }}
      />
      <div
        style={{
          position: 'absolute',
          right: 70,
          top: '50%',
          transform: 'translateY(-50%)',
          fontFamily: MONO,
          fontSize: 420,
          fontWeight: 700,
          color: 'transparent',
          WebkitTextStroke: '2px rgba(148,163,184,.14)',
          letterSpacing: -10,
          lineHeight: 1,
        }}
      >
        {String(index + 1).padStart(2, '0')}
      </div>
      <div
        style={{
          position: 'absolute',
          left: 168,
          top: '50%',
          transform: `translateY(-50%) scale(${1.18 - enter * 0.18})`,
          opacity: 0.6 + enter * 0.4,
        }}
      >
        <div style={{ fontFamily: MONO, fontSize: 18, color: item.accent, letterSpacing: 8 }}>
          CORE ADVANTAGE
        </div>
        <div
          style={{
            marginTop: 20,
            fontSize: 136,
            fontWeight: 900,
            color: '#f8fafc',
            letterSpacing: 6,
            lineHeight: 1.05,
            textShadow: `0 0 60px ${item.accent}66`,
          }}
        >
          {item.text}
        </div>
        <div style={{ marginTop: 24, display: 'flex', alignItems: 'center', gap: 18 }}>
          <span style={{ width: 120 * enter, height: 4, background: item.accent, boxShadow: `0 0 20px ${item.accent}` }} />
          <span style={{ fontFamily: MONO, fontSize: 30, color: '#cbd5e1', letterSpacing: 2 }}>{item.sub}</span>
        </div>
      </div>
      <div style={{ position: 'absolute', left: 168, bottom: 132, display: 'flex', gap: 10 }}>
        {FLASH_WORDS.map((word, wordIndex) => (
          <span
            key={word.text}
            style={{
              width: wordIndex === index ? 46 : 14,
              height: 5,
              borderRadius: 99,
              background: wordIndex === index ? item.accent : 'rgba(148,163,184,.28)',
              boxShadow: wordIndex === index ? `0 0 16px ${item.accent}` : 'none',
            }}
          />
        ))}
      </div>
      <AbsoluteFill
        style={{ background: '#fff', opacity: hit * 0.28, mixBlendMode: 'screen', pointerEvents: 'none' }}
      />
    </AbsoluteFill>
  )
}

const InputCard: React.FC<{ kind: 'text' | 'image'; local: number; fly: number }> = ({ kind, local, fly }) => {
  const isText = kind === 'text'
  const delay = isText ? 4 : 10
  const enter = interpolate(local, [delay, delay + 18], [0, 1], clamp)
  const scan = interpolate(local, [14, 34], [0, 1], clamp)
  const shift = (isText ? 1 : -1) * fly * 560
  const opacity = enter * (1 - fly)
  return (
    <div
      style={{
        position: 'absolute',
        left: isText ? 190 : 1350,
        top: 330,
        width: 380,
        height: 340,
        borderRadius: 18,
        border: `1px solid ${isText ? 'rgba(34,211,238,.4)' : 'rgba(167,139,250,.4)'}`,
        background: 'linear-gradient(160deg, rgba(15,23,42,.96), rgba(8,15,30,.9))',
        boxShadow: `0 26px 70px rgba(2,6,23,.7), 0 0 40px ${isText ? 'rgba(34,211,238,.12)' : 'rgba(167,139,250,.12)'}`,
        transform: `translate(${shift}px, ${(1 - enter) * 34 + fly * 40}px) scale(${1 - fly * 0.5})`,
        opacity,
        overflow: 'hidden',
      }}
    >
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', padding: '20px 24px 0' }}>
        <span
          style={{
            padding: '5px 14px',
            borderRadius: 999,
            background: isText ? 'rgba(34,211,238,.14)' : 'rgba(167,139,250,.14)',
            color: isText ? '#a5f3fc' : '#ddd6fe',
            fontSize: 17,
            letterSpacing: 2,
          }}
        >
          {isText ? '文本' : '图片'}
        </span>
        <span style={{ fontFamily: MONO, fontSize: 14, color: '#64748b', letterSpacing: 1 }}>
          {isText ? 'TXT · 2 段' : 'JPG · 2 张'}
        </span>
      </div>
      {isText ? (
        <div style={{ padding: '22px 24px' }}>
          <div style={{ fontSize: 24, fontWeight: 700, color: '#e2e8f0' }}>采购合同 · 第 3.2 条</div>
          <div style={{ marginTop: 16, fontSize: 19, color: '#94a3b8', lineHeight: 1.8 }}>
            合同金额：￥ 1,280,000
            <br />
            签署主体：华东供应链
            <br />
            <span style={{ color: '#fbbf24' }}>付款周期：验收后 90 天</span>
          </div>
          <div style={{ marginTop: 18, display: 'flex', gap: 8 }}>
            <span style={{ padding: '4px 10px', borderRadius: 6, background: 'rgba(251,191,36,.12)', color: AMBER, fontSize: 14 }}>
              待核验条款
            </span>
            <span style={{ padding: '4px 10px', borderRadius: 6, background: 'rgba(148,163,184,.12)', color: '#94a3b8', fontSize: 14 }}>
              2,148 字
            </span>
          </div>
        </div>
      ) : (
        <div style={{ padding: '20px 24px', height: 260, position: 'relative' }}>
          <div
            style={{
              position: 'absolute',
              left: 60,
              top: 16,
              width: 250,
              height: 218,
              borderRadius: 6,
              background: 'linear-gradient(160deg, #f8fafc, #e2e8f0)',
              transform: 'rotate(-2deg)',
              boxShadow: '0 16px 36px rgba(2,6,23,.55)',
              padding: '22px 20px',
            }}
          >
            <div style={{ width: 120, height: 8, borderRadius: 4, background: '#94a3b8' }} />
            {[0, 1, 2, 3].map((line) => (
              <div
                key={line}
                style={{
                  marginTop: 12,
                  width: line === 3 ? '58%' : '100%',
                  height: 5,
                  borderRadius: 3,
                  background: '#cbd5e1',
                }}
              />
            ))}
            <div
              style={{
                position: 'absolute',
                right: 22,
                bottom: 20,
                width: 64,
                height: 64,
                borderRadius: '50%',
                border: '2px solid rgba(248,113,113,.75)',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                color: 'rgba(248,113,113,.8)',
                fontFamily: MONO,
                fontSize: 11,
                transform: 'rotate(-12deg)',
              }}
            >
              已核验
            </div>
          </div>
          <div
            style={{
              position: 'absolute',
              right: 16,
              bottom: 18,
              padding: '5px 12px',
              borderRadius: 6,
              background: 'rgba(2,6,23,.72)',
              color: '#c4b5fd',
              fontFamily: MONO,
              fontSize: 14,
            }}
          >
            清晰度 98
          </div>
          <div
            style={{
              position: 'absolute',
              left: 30,
              right: 30,
              top: 70 + scan * 170,
              height: 2,
              background: 'linear-gradient(90deg, transparent, rgba(34,211,238,.95), transparent)',
              boxShadow: '0 0 24px rgba(34,211,238,.85)',
              opacity: scan > 0 && scan < 1 ? 0.9 : 0,
            }}
          />
          <div
            style={{
              position: 'absolute',
              left: 76,
              top: 80,
              width: 222,
              height: 112,
              borderRadius: 4,
              border: '1px solid rgba(34,211,238,.45)',
              background: 'rgba(34,211,238,.04)',
              boxShadow: '0 0 24px rgba(34,211,238,.18)',
              opacity: interpolate(local, [18, 30], [0, 1], clamp) * (1 - fly),
            }}
          >
            <span
              style={{
                position: 'absolute',
                right: 4,
                top: -18,
                fontFamily: MONO,
                fontSize: 11,
                color: CYAN,
                letterSpacing: 1,
              }}
            >
              OCR
            </span>
          </div>
        </div>
      )}
    </div>
  )
}

const MultimodalDemo: React.FC = () => {
  const frame = useCurrentFrame()
  const local = frame - 240
  const fly = interpolate(local, [38, 64], [0, 1], clamp)
  const verdict = interpolate(local, [62, 74], [0, 1], clamp)
  const confidence = Math.round(interpolate(local, [62, 74], [0, 96], clamp))
  const coreScale = 0.86 + fly * 0.24 + kick(frame) * 0.02
  const tags = ['OCR 识别', '条款抽取', '风险识别', '图像检测']
  return (
    <AbsoluteFill>
      <SectionHeader
        index="01"
        kicker="MULTIMODAL"
        title="文本 + 图片，一次读懂"
        subtitle="多段文本 + 多张图片，联合判定，漏判更少"
        local={local}
      />
      <div
        style={{
          position: 'absolute',
          left: 640,
          top: 300,
          width: 640,
          height: 420,
          background: 'radial-gradient(circle, rgba(37,99,235,.32), transparent 66%)',
          filter: 'blur(24px)',
        }}
      />
      <InputCard kind="text" local={local} fly={fly} />
      <InputCard kind="image" local={local} fly={fly} />
      <div
        style={{
          position: 'absolute',
          left: 960,
          top: 478,
          width: 260,
          height: 260,
          transform: `translate(-50%, -50%) scale(${coreScale})`,
          opacity: Math.max(0, 1 - verdict * 1.8),
        }}
      >
        {[0, 1, 2].map((ring) => (
          <div
            key={ring}
            style={{
              position: 'absolute',
              inset: ring * 24,
              borderRadius: '50%',
              border: `1px ${ring === 1 ? 'dashed' : 'solid'} rgba(147,197,253,${0.42 - ring * 0.1})`,
              transform: `rotate(${frame * (ring % 2 === 0 ? 0.8 : -0.6)}deg)`,
            }}
          />
        ))}
        <div style={{ position: 'absolute', left: '50%', top: '50%', transform: 'translate(-50%, -50%)' }}>
          <Logo size={112} />
        </div>
        <div
          style={{
            position: 'absolute',
            left: '50%',
            bottom: -34,
            transform: 'translateX(-50%)',
            fontFamily: MONO,
            fontSize: 16,
            color: '#93c5fd',
            letterSpacing: 4,
            whiteSpace: 'nowrap',
          }}
        >
          AI 联合判定
        </div>
      </div>
      <div
        style={{
          position: 'absolute',
          left: 640,
          top: 344,
          width: 640,
          padding: '28px 34px',
          borderRadius: 20,
          border: '1px solid rgba(52,211,153,.5)',
          background: 'linear-gradient(135deg, rgba(6,78,59,.72), rgba(2,6,23,.92))',
          boxShadow: `0 0 ${70 + verdict * 50}px rgba(16,185,129,${0.12 + verdict * 0.2})`,
          opacity: verdict,
          transform: `translateY(${(1 - verdict) * 20}px) scale(${0.88 + verdict * 0.12})`,
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 20 }}>
            <div
              style={{
                width: 66,
                height: 66,
                borderRadius: '50%',
                border: '2px solid rgba(52,211,153,.7)',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                color: GREEN,
                fontSize: 36,
              }}
            >
              ✓
            </div>
            <div>
              <div style={{ fontFamily: MONO, fontSize: 20, color: '#6ee7b7', letterSpacing: 6 }}>RESULT</div>
              <div style={{ fontSize: 76, fontWeight: 900, color: '#ecfdf5', lineHeight: 1.05, letterSpacing: 6 }}>
                PASS
              </div>
            </div>
          </div>
          <div style={{ textAlign: 'right' }}>
            <div style={{ fontSize: 78, fontWeight: 800, color: GREEN, lineHeight: 1 }}>{confidence}</div>
            <div style={{ fontFamily: MONO, fontSize: 15, color: '#6ee7b7', letterSpacing: 2 }}>CONFIDENCE</div>
          </div>
        </div>
        <div style={{ marginTop: 20, fontSize: 22, color: '#d1fae5', letterSpacing: 1 }}>
          文本合规 · 图片清晰 · 主体一致
        </div>
        <div style={{ marginTop: 16, display: 'flex', gap: 10 }}>
          {['敏感词 0', '金额合规', '清晰度 98'].map((chip) => (
            <span
              key={chip}
              style={{
                padding: '6px 14px',
                borderRadius: 999,
                background: 'rgba(52,211,153,.12)',
                border: '1px solid rgba(52,211,153,.3)',
                color: '#a7f3d0',
                fontSize: 16,
              }}
            >
              {chip}
            </span>
          ))}
        </div>
      </div>
      {tags.map((tag, index) => {
        const tagIn = interpolate(local, [22 + index * 5, 38 + index * 5], [0, 1], clamp)
        const positions = [
          { left: 300, top: 748 },
          { left: 720, top: 790 },
          { left: 1120, top: 790 },
          { left: 1500, top: 748 },
        ]
        return (
          <div
            key={tag}
            style={{
              position: 'absolute',
              ...positions[index],
              padding: '8px 18px',
              borderRadius: 999,
              border: '1px solid rgba(147,197,253,.28)',
              background: 'rgba(2,6,23,.76)',
              color: '#bfdbfe',
              fontFamily: MONO,
              fontSize: 16,
              letterSpacing: 1,
              opacity: tagIn * 0.9,
              transform: `translateY(${(1 - tagIn) * 14}px)`,
            }}
          >
            {tag}
          </div>
        )
      })}
      <div
        style={{
          position: 'absolute',
          left: '50%',
          top: 866,
          transform: 'translateX(-50%)',
          display: 'flex',
          alignItems: 'center',
          gap: 14,
          padding: '12px 28px',
          borderRadius: 999,
          border: '1px solid rgba(52,211,153,.4)',
          background: 'rgba(6,78,59,.32)',
          boxShadow: '0 0 40px rgba(16,185,129,.16)',
          fontFamily: MONO,
          fontSize: 21,
          color: '#a7f3d0',
          letterSpacing: 3,
          opacity: interpolate(local, [78, 90], [0, 1], clamp),
          whiteSpace: 'nowrap',
        }}
      >
        <span style={{ color: GREEN, fontSize: 24 }}>✓</span>
        图文联防 · 漏判更少
      </div>
    </AbsoluteFill>
  )
}

const polar = (cx: number, cy: number, radius: number, degree: number) => {
  const radian = (degree * Math.PI) / 180
  return { x: cx + radius * Math.cos(radian), y: cy - radius * Math.sin(radian) }
}

const arcPath = (cx: number, cy: number, radius: number, from: number, to: number) => {
  const start = polar(cx, cy, radius, from)
  const end = polar(cx, cy, radius, to)
  return `M ${start.x} ${start.y} A ${radius} ${radius} 0 0 1 ${end.x} ${end.y}`
}

const RULES = [
  { name: '敏感词库', weight: '38 / 40', ratio: 0.95, status: '命中', color: RED },
  { name: '金额阈值', weight: '22 / 25', ratio: 0.88, status: '关注', color: AMBER },
  { name: '主体一致性', weight: '18 / 20', ratio: 0.9, status: '通过', color: GREEN },
  { name: '图像清晰度', weight: '10 / 15', ratio: 0.67, status: '通过', color: GREEN },
]

const RulesDemo: React.FC = () => {
  const frame = useCurrentFrame()
  const local = frame - 360
  const sweep = easeOut(interpolate(local, [12, 88], [0, 1], clamp))
  const score = 6 + sweep * 82
  const verdict = score < 50 ? 'PASS' : score < 75 ? 'REVIEW' : 'REJECT'
  const verdictColor = verdict === 'PASS' ? GREEN : verdict === 'REVIEW' ? AMBER : RED
  const needles = `rotate(${-score * 1.8} 280 300)`
  const panelLeft = interpolate(local, [0, 14], [0, 1], clamp)
  const panelRight = interpolate(local, [4, 20], [0, 1], clamp)
  const basis =
    verdict === 'PASS'
      ? '四项规则全部通过 · 结论 PASS'
      : verdict === 'REVIEW'
        ? '金额阈值接近上限 · 切换 REVIEW'
        : '命中敏感词库与金额阈值 · 触发 REJECT'
  return (
    <AbsoluteFill>
      <SectionHeader
        index="02"
        kicker="RISK ENGINE"
        title="规则判定，可解释可调优"
        subtitle="权重、阈值全部可配置，每一次结论都能回溯依据"
        local={local}
        accent={AMBER}
      />
      <div
        style={{
          position: 'absolute',
          left: 110,
          top: 272,
          width: 800,
          height: 618,
          borderRadius: 22,
          border: '1px solid rgba(147,197,253,.22)',
          background: 'linear-gradient(160deg, rgba(15,23,42,.9), rgba(2,6,23,.92))',
          boxShadow: '0 30px 90px rgba(2,6,23,.7)',
          opacity: panelLeft,
          transform: `translateX(${(1 - panelLeft) * -28}px) scale(${0.97 + panelLeft * 0.03})`,
        }}
      >
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', padding: '26px 34px 0' }}>
          <span style={{ fontSize: 26, fontWeight: 700, color: '#e2e8f0', letterSpacing: 2 }}>风险评分</span>
          <span
            style={{
              padding: '5px 12px',
              borderRadius: 999,
              border: '1px solid rgba(34,211,238,.32)',
              color: CYAN,
              fontFamily: MONO,
              fontSize: 14,
              letterSpacing: 2,
            }}
          >
            LIVE
          </span>
        </div>
        <svg viewBox="0 0 560 400" style={{ position: 'absolute', left: 120, top: 54, width: 560, height: 400 }}>
          <path d={arcPath(280, 300, 186, 180, 0)} stroke="rgba(51,65,85,.7)" strokeWidth={32} fill="none" strokeLinecap="round" />
          <path
            d={arcPath(280, 300, 186, 180, 90)}
            stroke={GREEN}
            strokeWidth={32}
            fill="none"
            strokeLinecap="round"
            opacity={verdict === 'PASS' ? 0.95 : 0.28}
          />
          <path
            d={arcPath(280, 300, 186, 90, 45)}
            stroke={AMBER}
            strokeWidth={32}
            fill="none"
            opacity={verdict === 'REVIEW' ? 0.95 : 0.28}
          />
          <path
            d={arcPath(280, 300, 186, 45, 0)}
            stroke={RED}
            strokeWidth={32}
            fill="none"
            strokeLinecap="round"
            opacity={verdict === 'REJECT' ? 0.95 : 0.28}
          />
          {[50, 75].map((mark) => {
            const outer = polar(280, 300, 208, 180 - mark * 1.8)
            const inner = polar(280, 300, 172, 180 - mark * 1.8)
            return <line key={mark} x1={inner.x} y1={inner.y} x2={outer.x} y2={outer.y} stroke="rgba(226,232,240,.55)" strokeWidth={2} />
          })}
          <text x={96} y={342} fill="rgba(52,211,153,.8)" fontFamily={MONO} fontSize={16} letterSpacing={2}>
            PASS
          </text>
          <text x={280} y={92} fill="rgba(251,191,36,.8)" fontFamily={MONO} fontSize={16} letterSpacing={2} textAnchor="middle">
            REVIEW
          </text>
          <text x={464} y={342} fill="rgba(248,113,113,.8)" fontFamily={MONO} fontSize={16} letterSpacing={2} textAnchor="end">
            REJECT
          </text>
          <g transform={needles}>
            <line x1={280} y1={300} x2={128} y2={300} stroke="#e2e8f0" strokeWidth={6} strokeLinecap="round" />
            <circle cx={280} cy={300} r={13} fill="#0f172a" stroke="#e2e8f0" strokeWidth={4} />
          </g>
        </svg>
        <div
          style={{
            position: 'absolute',
            left: 400,
            top: 376,
            transform: 'translateX(-50%)',
            textAlign: 'center',
          }}
        >
          <div style={{ fontSize: 84, fontWeight: 900, color: '#f8fafc', lineHeight: 1 }}>{Math.round(score)}</div>
          <div style={{ fontFamily: MONO, fontSize: 14, color: '#94a3b8', letterSpacing: 4 }}>RISK SCORE</div>
        </div>
        <div
          style={{
            position: 'absolute',
            left: 400,
            top: 498,
            transform: `translateX(-50%) scale(${1 + kick(frame) * (verdict === 'REJECT' ? 0.04 : 0.015)})`,
            padding: '10px 32px',
            borderRadius: 10,
            border: `1px solid ${verdictColor}66`,
            background: `${verdictColor}18`,
            color: verdictColor,
            fontFamily: MONO,
            fontSize: 30,
            fontWeight: 700,
            letterSpacing: 6,
            boxShadow: `0 0 ${30 + kick(frame) * 30}px ${verdictColor}${verdict === 'REJECT' ? '66' : '33'}`,
          }}
        >
          {verdict}
        </div>
        <div
          style={{
            position: 'absolute',
            left: 40,
            right: 40,
            bottom: 22,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            gap: 8,
            color: '#94a3b8',
            fontSize: 17,
          }}
        >
          <span style={{ color: AMBER }}>✓</span>
          阈值可调 · 每个分值可回溯
        </div>
      </div>
      <div
        style={{
          position: 'absolute',
          left: 946,
          top: 272,
          width: 864,
          height: 618,
          padding: '28px 38px',
          borderRadius: 22,
          border: '1px solid rgba(147,197,253,.22)',
          background: 'linear-gradient(160deg, rgba(15,23,42,.9), rgba(2,6,23,.92))',
          boxShadow: '0 30px 90px rgba(2,6,23,.7)',
          opacity: panelRight,
          transform: `translateX(${(1 - panelRight) * 28}px) scale(${0.97 + panelRight * 0.03})`,
        }}
      >
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <span style={{ fontSize: 26, fontWeight: 700, color: '#e2e8f0', letterSpacing: 2 }}>评分依据</span>
          <span style={{ fontFamily: MONO, fontSize: 26, color: AMBER }}>{Math.round(score)} / 100</span>
        </div>
        {RULES.map((rule, index) => {
          const rowIn = interpolate(local, [16 + index * 9, 32 + index * 9], [0, 1], clamp)
          const bar = interpolate(local, [18 + index * 9, 48 + index * 9], [0, rule.ratio], clamp)
          return (
            <div
              key={rule.name}
              style={{
                marginTop: 36,
                opacity: rowIn,
                transform: `translateX(${(1 - rowIn) * 18}px)`,
              }}
            >
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <span style={{ fontSize: 21, color: '#dbeafe', letterSpacing: 1 }}>{rule.name}</span>
                <span style={{ fontFamily: MONO, fontSize: 17, color: '#94a3b8' }}>
                  {rule.weight}
                  <span style={{ marginLeft: 12, color: rule.color }}>{rule.status}</span>
                </span>
              </div>
              <div style={{ marginTop: 12, height: 10, borderRadius: 999, background: 'rgba(51,65,85,.7)', overflow: 'hidden' }}>
                <div
                  style={{
                    width: `${bar * 100}%`,
                    height: '100%',
                    borderRadius: 999,
                    background: `linear-gradient(90deg, ${rule.color}88, ${rule.color})`,
                    boxShadow: `0 0 16px ${rule.color}66`,
                  }}
                />
              </div>
            </div>
          )
        })}
        <div style={{ marginTop: 40, display: 'flex', gap: 10 }}>
          {[
            { text: '< 50  PASS', color: GREEN },
            { text: '50 – 75  REVIEW', color: AMBER },
            { text: '≥ 75  REJECT', color: RED },
          ].map((item) => (
            <span
              key={item.text}
              style={{
                padding: '7px 14px',
                borderRadius: 8,
                border: `1px solid ${item.color}44`,
                background: `${item.color}12`,
                color: item.color,
                fontFamily: MONO,
                fontSize: 14,
                letterSpacing: 1,
              }}
            >
              {item.text}
            </span>
          ))}
        </div>
        <div style={{ marginTop: 24, display: 'flex', alignItems: 'center', gap: 9, color: verdictColor, fontSize: 16 }}>
          <span style={{ width: 7, height: 7, borderRadius: '50%', background: verdictColor, boxShadow: `0 0 12px ${verdictColor}` }} />
          {basis}
        </div>
      </div>
    </AbsoluteFill>
  )
}

const ACTIONS = [
  {
    title: 'HTTP 回调',
    sub: 'ctx.http.request',
    code: "headers: { 'X-Sign': ctx.crypto.md5(...) }",
    status: '200 OK · 96ms',
    accent: CYAN,
  },
  {
    title: 'Redis 队列',
    sub: 'XADD audit:result',
    code: '内置动作 · 自动重试',
    status: 'stream · 12 msgs',
    accent: '#a78bfa',
  },
  {
    title: 'JS 沙盒',
    sub: 'ctx.mysql.connect',
    code: 'db.query("UPDATE audit_task SET status = ?")',
    status: '1 row updated',
    accent: GREEN,
  },
]

const LOG_LINES = [
  { time: '16:20:01.204', channel: 'VERDICT', text: 'REJECT · score=86 · task=#1832', color: RED },
  { time: '16:20:01.310', channel: 'HTTP', text: 'POST /risk/callback → 200 · 96ms', color: CYAN },
  { time: '16:20:01.402', channel: 'REDIS', text: 'XADD audit:result → ok', color: '#c4b5fd' },
  { time: '16:20:02.518', channel: 'JS', text: 'update audit_task → 1 row', color: GREEN },
]

const ActionsDemo: React.FC = () => {
  const frame = useCurrentFrame()
  const local = frame - 480
  const nodeCenters = [290, 480, 670]
  const resultIn = interpolate(local, [0, 14], [0, 1], clamp)
  const logIn = interpolate(local, [14, 28], [0, 1], clamp)
  const flowIn = local > 50
  return (
    <AbsoluteFill>
      <SectionHeader
        index="03"
        kicker="RESULT ACTIONS"
        title="一次判定，并行触发所有动作"
        subtitle="HTTP / Redis / JS 并行执行，失败自动重试"
        local={local}
        accent={BLUE}
      />
      <svg width={1920} height={1080} style={{ position: 'absolute', inset: 0 }}>
        {nodeCenters.map((center, index) => (
          <line
            key={center}
            x1={500}
            y1={525}
            x2={1110}
            y2={center}
            stroke={flowIn ? `${ACTIONS[index].accent}99` : 'rgba(96,165,250,.2)'}
            strokeWidth={2}
            strokeDasharray="9 11"
            strokeDashoffset={-frame * 2.4}
          />
        ))}
      </svg>
      <div
        style={{
          position: 'absolute',
          left: 760,
          top: 525,
          transform: `translate(-50%, -50%) rotate(${frame * 0.5}deg)`,
          width: 190,
          height: 190,
          borderRadius: '50%',
          border: '1px dashed rgba(34,211,238,.28)',
        }}
      />
      <div
        style={{
          position: 'absolute',
          left: 760,
          top: 525,
          transform: 'translate(-50%, -50%)',
          fontFamily: MONO,
          fontSize: 15,
          color: 'rgba(147,197,253,.55)',
          letterSpacing: 5,
        }}
      >
        FAN-OUT
      </div>
      <div
        style={{
          position: 'absolute',
          left: 130,
          top: 396,
          width: 370,
          height: 258,
          padding: '28px 30px',
          borderRadius: 20,
          border: '1px solid rgba(248,113,113,.42)',
          background: 'linear-gradient(150deg, rgba(69,10,10,.6), rgba(2,6,23,.95))',
          boxShadow: `0 24px 70px rgba(2,6,23,.7), 0 0 ${30 + kick(frame) * 24}px rgba(248,113,113,.12)`,
          opacity: resultIn,
          transform: `translateX(${(1 - resultIn) * -26}px) scale(${0.96 + resultIn * 0.04})`,
        }}
      >
        <div style={{ fontFamily: MONO, fontSize: 15, color: '#94a3b8', letterSpacing: 3 }}>RESULT</div>
        <div style={{ marginTop: 8, fontSize: 58, fontWeight: 900, color: RED, letterSpacing: 3 }}>REJECT</div>
        <div style={{ marginTop: 6, fontFamily: MONO, fontSize: 16, color: '#cbd5e1' }}>task #1832 · score 86</div>
        <div style={{ marginTop: 20, display: 'flex', alignItems: 'center', gap: 10, color: '#fca5a5', fontSize: 16 }}>
          <span style={{ width: 8, height: 8, borderRadius: '50%', background: RED, boxShadow: `0 0 12px ${RED}` }} />
          结果分发
        </div>
      </div>
      <div
        style={{
          position: 'absolute',
          left: 130,
          top: 676,
          width: 370,
          display: 'flex',
          alignItems: 'center',
          gap: 9,
          padding: '10px 16px',
          borderRadius: 10,
          border: '1px solid rgba(52,211,153,.35)',
          background: 'rgba(6,78,59,.24)',
          color: '#6ee7b7',
          fontFamily: MONO,
          fontSize: 15,
          letterSpacing: 1,
          opacity: interpolate(local, [56, 68], [0, 1], clamp),
          transform: `translateY(${interpolate(local, [56, 68], [10, 0], clamp)}px)`,
        }}
      >
        <span style={{ color: GREEN }}>✓</span>
        三路并行 · 互不阻塞
      </div>
      {ACTIONS.map((action, index) => {
        const enter = interpolate(local, [12 + index * 10, 30 + index * 10], [0, 1], clamp)
        const done = local > 56 + index * 4
        const top = 210 + index * 190
        return (
          <React.Fragment key={action.title}>
            <div
              style={{
                position: 'absolute',
                left: 1110,
                top,
                width: 680,
                height: 160,
                padding: '22px 26px',
                borderRadius: 18,
                border: `1px solid ${done ? `${action.accent}66` : 'rgba(148,163,184,.22)'}`,
                background: 'linear-gradient(150deg, rgba(15,23,42,.94), rgba(2,6,23,.9))',
                boxShadow: done ? `0 0 42px ${action.accent}22` : '0 20px 60px rgba(2,6,23,.55)',
                opacity: enter,
                transform: `translateX(${(1 - enter) * 46}px)`,
              }}
            >
              <div style={{ display: 'flex', alignItems: 'center', gap: 18 }}>
                <div
                  style={{
                    width: 52,
                    height: 52,
                    borderRadius: 12,
                    border: `1px solid ${action.accent}55`,
                    background: `${action.accent}14`,
                    color: action.accent,
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    fontFamily: MONO,
                    fontSize: 20,
                  }}
                >
                  {index === 0 ? '↗' : index === 1 ? '≡' : '</>'}
                </div>
                <div style={{ flex: 1 }}>
                  <div style={{ fontSize: 26, fontWeight: 700, color: '#f1f5f9', letterSpacing: 1 }}>{action.title}</div>
                  <div style={{ marginTop: 3, fontFamily: MONO, fontSize: 15, color: '#94a3b8' }}>{action.sub}</div>
                </div>
                <span
                  style={{
                    padding: '7px 14px',
                    borderRadius: 999,
                    border: `1px solid ${done ? `${action.accent}55` : 'rgba(148,163,184,.25)'}`,
                    background: done ? `${action.accent}14` : 'rgba(148,163,184,.08)',
                    color: done ? action.accent : '#64748b',
                    fontFamily: MONO,
                    fontSize: 14,
                    whiteSpace: 'nowrap',
                  }}
                >
                  {done ? `✓ ${action.status}` : '等待执行'}
                </span>
              </div>
              <div style={{ marginTop: 16, fontFamily: MONO, fontSize: 14, color: '#64748b' }}>{action.code}</div>
            </div>
            {(() => {
              const start = 52
              const travel = interpolate(local, [start, start + 14], [0, 1], clamp)
              if (travel <= 0 || travel >= 1) return null
              const x = 500 + (1110 - 500) * travel
              const y = 525 + (nodeCenters[index] - 525) * travel
              return (
                <div
                  key={`pulse-${action.title}`}
                  style={{
                    position: 'absolute',
                    left: x - 7,
                    top: y - 7,
                    width: 14,
                    height: 14,
                    borderRadius: '50%',
                    background: action.accent,
                    boxShadow: `0 0 22px ${action.accent}, 0 0 50px ${action.accent}88`,
                  }}
                />
              )
            })()}
          </React.Fragment>
        )
      })}
      <div
        style={{
          position: 'absolute',
          left: 130,
          top: 724,
          width: 860,
          height: 188,
          padding: '18px 24px',
          borderRadius: 16,
          border: '1px solid rgba(148,163,184,.2)',
          background: 'rgba(2,6,23,.82)',
          fontFamily: MONO,
          opacity: logIn,
          transform: `translateY(${(1 - logIn) * 16}px)`,
        }}
      >
        <div style={{ fontSize: 13, color: '#475569', letterSpacing: 3, marginBottom: 12 }}>EXECUTION LOG</div>
        <div
          style={{
            position: 'absolute',
            left: 24,
            top: 42,
            display: 'flex',
            alignItems: 'center',
            gap: 10,
            fontSize: 15,
            lineHeight: '29px',
            color: '#475569',
            opacity: 1 - interpolate(local, [56, 66], [0, 1], clamp),
          }}
        >
          <span
            style={{
              width: 8,
              height: 8,
              borderRadius: '50%',
              background: CYAN,
              boxShadow: `0 0 12px ${CYAN}`,
              opacity: frame % 20 < 10 ? 1 : 0.2,
            }}
          />
          执行中 · 等待回执…
        </div>
        {LOG_LINES.map((line, index) => {
          const reveal = interpolate(local, [66 + index * 9, 76 + index * 9], [0, 1], clamp)
          return (
            <div
              key={line.time}
              style={{
                display: 'flex',
                gap: 18,
                fontSize: 15,
                lineHeight: '29px',
                opacity: reveal,
                transform: `translateX(${(1 - reveal) * 14}px)`,
              }}
            >
              <span style={{ color: '#475569' }}>{line.time}</span>
              <span style={{ color: line.color, width: 72 }}>{line.channel}</span>
              <span style={{ color: '#cbd5e1' }}>{line.text}</span>
            </div>
          )
        })}
      </div>
    </AbsoluteFill>
  )
}

type CodeSegment = { text: string; color: string }

const CODE_LINES: CodeSegment[][] = [
  [{ text: '// 结果动作：PASS / REJECT / REVIEW', color: '#64748b' }],
  [
    { text: 'const ', color: '#c084fc' },
    { text: 'task', color: '#7dd3fc' },
    { text: ' = ', color: '#cbd5e1' },
    { text: 'ctx.task', color: BLUE },
  ],
  [
    { text: 'const ', color: '#c084fc' },
    { text: 'verdict', color: '#7dd3fc' },
    { text: ' = ', color: '#cbd5e1' },
    { text: 'ctx.result', color: BLUE },
  ],
  [
    { text: 'const ', color: '#c084fc' },
    { text: 'sign', color: '#7dd3fc' },
    { text: ' = ', color: '#cbd5e1' },
    { text: 'ctx.crypto', color: BLUE },
    { text: '.md5', color: AMBER },
    { text: '(', color: '#cbd5e1' },
  ],
  [
    { text: '  ', color: '#cbd5e1' },
    { text: 'ctx.secret', color: BLUE },
    { text: "('APP_KEY')", color: '#86efac' },
    { text: ' + ', color: '#cbd5e1' },
    { text: 'task.id', color: '#e2e8f0' },
    { text: ',', color: '#cbd5e1' },
  ],
  [{ text: ')', color: '#cbd5e1' }],
  [
    { text: 'ctx.http', color: BLUE },
    { text: '.request', color: AMBER },
    { text: '({', color: '#cbd5e1' },
  ],
  [
    { text: '  url: ', color: '#cbd5e1' },
    { text: "'https://erp.example.com/callback'", color: '#86efac' },
    { text: ',', color: '#cbd5e1' },
  ],
  [
    { text: '  method: ', color: '#cbd5e1' },
    { text: "'POST'", color: '#86efac' },
    { text: ',', color: '#cbd5e1' },
  ],
  [
    { text: '  headers: { ', color: '#cbd5e1' },
    { text: "'X-Sign'", color: '#86efac' },
    { text: ': sign },', color: '#cbd5e1' },
  ],
  [
    { text: '  body: { taskId: ', color: '#cbd5e1' },
    { text: 'task.id', color: '#e2e8f0' },
    { text: ', verdict },', color: '#cbd5e1' },
  ],
  [{ text: '})', color: '#cbd5e1' }],
  [
    { text: 'const ', color: '#c084fc' },
    { text: 'db', color: '#7dd3fc' },
    { text: ' = ', color: '#cbd5e1' },
    { text: 'ctx.mysql', color: BLUE },
    { text: '.connect', color: AMBER },
    { text: '({', color: '#cbd5e1' },
  ],
  [
    { text: '  host: ', color: '#cbd5e1' },
    { text: "'10.0.0.12'", color: '#86efac' },
    { text: ', database: ', color: '#cbd5e1' },
    { text: "'biz'", color: '#86efac' },
    { text: ',', color: '#cbd5e1' },
  ],
  [
    { text: '  user: ', color: '#cbd5e1' },
    { text: 'ctx.secret', color: BLUE },
    { text: "('DB_USER')", color: '#86efac' },
    { text: ', password: ', color: '#cbd5e1' },
    { text: 'ctx.secret', color: BLUE },
    { text: "('DB_PASS')", color: '#86efac' },
    { text: ',', color: '#cbd5e1' },
  ],
  [{ text: '})', color: '#cbd5e1' }],
  [
    { text: 'db', color: '#e2e8f0' },
    { text: '.query', color: AMBER },
    { text: '(', color: '#cbd5e1' },
    { text: "'UPDATE audit_task SET status = ? WHERE id = ?'", color: '#86efac' },
    { text: ', [', color: '#cbd5e1' },
    { text: 'verdict', color: '#7dd3fc' },
    { text: ', ', color: '#cbd5e1' },
    { text: 'task.id', color: '#e2e8f0' },
    { text: '])', color: '#cbd5e1' },
  ],
  [
    { text: 'db', color: '#e2e8f0' },
    { text: '.close', color: AMBER },
    { text: '()', color: '#cbd5e1' },
  ],
  [
    { text: 'ctx.log', color: BLUE },
    { text: '(', color: '#cbd5e1' },
    { text: "'done'", color: '#86efac' },
    { text: ', ', color: '#cbd5e1' },
    { text: 'verdict', color: '#7dd3fc' },
    { text: ')', color: '#cbd5e1' },
  ],
]

const CAPABILITIES = [
  { name: 'HTTP 请求', code: 'ctx.http', accent: CYAN },
  { name: 'MySQL / PostgreSQL', code: 'ctx.mysql · ctx.pg', accent: GREEN },
  { name: 'MD5 / HMAC', code: 'md5 · hmacSha256', accent: AMBER },
  { name: 'AES / DES', code: 'CBC · ECB · PKCS7', accent: '#a78bfa' },
  { name: '项目密钥', code: "ctx.secret('APP_KEY')", accent: '#f472b6' },
]

const ScriptDemo: React.FC = () => {
  const frame = useCurrentFrame()
  const local = frame - 600
  const activeLine = Math.max(0, Math.min(CODE_LINES.length - 1, Math.floor((local - 12) / 4)))
  const runDone = local > 94
  const editorIn = interpolate(local, [0, 14], [0, 1], clamp)
  const sideIn = interpolate(local, [4, 18], [0, 1], clamp)
  const runPulse = interpolate(local, [88, 93, 102], [1, 1.08, 1], clamp)
  return (
    <AbsoluteFill>
      <SectionHeader
        index="04"
        kicker="CUSTOM SCRIPT"
        title="回调、查库、加签，一段 JS 就够"
        subtitle="内置 HTTP / 数据库 / 加密，沙盒执行，密钥不出服务端"
        local={local}
        accent="#a78bfa"
      />
      <div
        style={{
          position: 'absolute',
          left: 110,
          top: 258,
          width: 1140,
          height: 652,
          borderRadius: 20,
          border: '1px solid rgba(147,197,253,.24)',
          background: 'linear-gradient(160deg, rgba(15,23,42,.96), rgba(2,6,23,.97))',
          boxShadow: runDone
            ? '0 34px 100px rgba(2,6,23,.75), 0 0 70px rgba(52,211,153,.18)'
            : '0 34px 100px rgba(2,6,23,.75), 0 0 60px rgba(59,130,246,.08)',
          overflow: 'hidden',
          opacity: editorIn,
          transform: `translateY(${(1 - editorIn) * 20}px)`,
        }}
      >
        <div
          style={{
            height: 48,
            display: 'flex',
            alignItems: 'center',
            gap: 10,
            padding: '0 20px',
            background: 'linear-gradient(180deg, #16243c, #101a2c)',
            borderBottom: '1px solid rgba(148,163,184,.16)',
          }}
        >
          {['#f87171', '#fbbf24', '#34d399'].map((color) => (
            <span key={color} style={{ width: 11, height: 11, borderRadius: '50%', background: color }} />
          ))}
          <span style={{ marginLeft: 14, fontFamily: MONO, fontSize: 14, color: '#94a3b8' }}>result-action.js</span>
          <span
            style={{
              marginLeft: 10,
              padding: '3px 10px',
              borderRadius: 5,
              background: 'rgba(34,211,238,.1)',
              color: '#67e8f9',
              fontFamily: MONO,
              fontSize: 11,
              letterSpacing: 1,
            }}
          >
            SANDBOX
          </span>
        </div>
        <div
          style={{
            position: 'absolute',
            left: 8,
            right: 8,
            top: 56,
            bottom: 46,
            borderRadius: 14,
            border: '1px dashed rgba(34,211,238,.1)',
            boxShadow: runDone ? 'inset 0 0 80px rgba(52,211,153,.1)' : 'none',
            pointerEvents: 'none',
          }}
        />
        <div style={{ padding: '14px 26px', fontFamily: MONO, fontSize: 18, lineHeight: '28px' }}>
          {CODE_LINES.map((segments, lineIndex) => {
            const lineLength = segments.reduce((sum, segment) => sum + segment.text.length, 0)
            const start = 12 + lineIndex * 4
            const progress = interpolate(local, [start, start + 4], [0, 1], clamp)
            const limit = Math.ceil(progress * lineLength)
            let consumed = 0
            return (
              <div key={lineIndex} style={{ display: 'flex', height: 28 }}>
                <span style={{ width: 46, color: '#334155', textAlign: 'right', paddingRight: 18, fontSize: 14, lineHeight: '28px' }}>
                  {lineIndex + 1}
                </span>
                <span style={{ whiteSpace: 'pre' }}>
                  {segments.map((segment, segmentIndex) => {
                    const take = Math.max(0, Math.min(segment.text.length, limit - consumed))
                    consumed += segment.text.length
                    if (take <= 0) return null
                    return (
                      <span key={segmentIndex} style={{ color: segment.color }}>
                        {segment.text.slice(0, take)}
                      </span>
                    )
                  })}
                  {lineIndex === activeLine && local < 94 ? (
                    <span style={{ color: CYAN, opacity: frame % 20 < 10 ? 1 : 0.2 }}>▍</span>
                  ) : null}
                </span>
              </div>
            )
          })}
        </div>
        <div
          style={{
            position: 'absolute',
            left: 0,
            right: 0,
            bottom: 0,
            height: 38,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            padding: '0 22px',
            borderTop: '1px solid rgba(148,163,184,.14)',
            background: 'rgba(8,15,30,.8)',
            fontFamily: MONO,
            fontSize: 13,
            color: '#64748b',
          }}
        >
          <span>JavaScript · UTF-8</span>
          <span style={{ color: runDone ? GREEN : '#64748b' }}>
            {runDone ? '✓ 运行成功 · 200 OK · 128ms' : 'Ln 19, Col 16'}
          </span>
        </div>
        <div
          style={{
            position: 'absolute',
            top: 70,
            right: 26,
            padding: '7px 14px',
            borderRadius: 8,
            border: `1px solid ${runDone ? 'rgba(52,211,153,.4)' : 'rgba(34,211,238,.28)'}`,
            background: runDone ? 'rgba(52,211,153,.1)' : 'rgba(34,211,238,.08)',
            color: runDone ? GREEN : CYAN,
            fontFamily: MONO,
            fontSize: 13,
            opacity: interpolate(local, [88, 96], [0.45, 1], clamp),
            boxShadow: runDone ? '0 0 26px rgba(52,211,153,.25)' : 'none',
          }}
        >
          {runDone ? '✓ 运行成功' : '● RUNNING'}
        </div>
      </div>
      <div
        style={{
          position: 'absolute',
          left: 1294,
          top: 258,
          width: 516,
          height: 652,
          padding: '28px 32px',
          borderRadius: 20,
          border: '1px solid rgba(147,197,253,.22)',
          background: 'linear-gradient(160deg, rgba(15,23,42,.92), rgba(2,6,23,.94))',
          boxShadow: '0 30px 90px rgba(2,6,23,.7)',
          opacity: sideIn,
          transform: `translateX(${(1 - sideIn) * 24}px)`,
        }}
      >
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <span style={{ fontSize: 24, fontWeight: 700, color: '#e2e8f0', letterSpacing: 2 }}>沙盒能力</span>
          <span style={{ fontFamily: MONO, fontSize: 13, color: '#64748b' }}>v1.3</span>
        </div>
        {CAPABILITIES.map((capability, index) => {
          const reveal = interpolate(local, [24 + index * 6, 38 + index * 6], [0, 1], clamp)
          return (
            <div
              key={capability.name}
              style={{
                marginTop: 22,
                padding: '14px 16px',
                borderRadius: 12,
                border: '1px solid rgba(148,163,184,.16)',
                background: 'rgba(148,163,184,.05)',
                display: 'flex',
                alignItems: 'center',
                gap: 14,
                opacity: reveal,
                transform: `translateX(${(1 - reveal) * 18}px)`,
              }}
            >
              <span style={{ width: 8, height: 8, borderRadius: '50%', background: capability.accent, boxShadow: `0 0 12px ${capability.accent}` }} />
              <div style={{ flex: 1 }}>
                <div style={{ fontSize: 18, color: '#e2e8f0' }}>{capability.name}</div>
                <div style={{ marginTop: 2, fontFamily: MONO, fontSize: 12, color: '#64748b' }}>{capability.code}</div>
              </div>
              <span style={{ color: GREEN, fontSize: 16 }}>✓</span>
            </div>
          )
        })}
        <div style={{ marginTop: 26, display: 'flex', alignItems: 'center', gap: 14 }}>
          <span
            style={{
              padding: '11px 30px',
              borderRadius: 10,
              background: 'linear-gradient(90deg, #2563eb, #0891b2)',
              color: '#f8fafc',
              fontFamily: MONO,
              fontSize: 17,
              letterSpacing: 2,
              boxShadow: runDone ? '0 0 40px rgba(52,211,153,.45)' : `0 0 ${16 + kick(frame) * 30}px rgba(34,211,238,.45)`,
              transform: `scale(${runPulse})`,
            }}
          >
            {runDone ? '✓ 已执行' : '▶ 运行'}
          </span>
          <span style={{ fontFamily: MONO, fontSize: 14, color: runDone ? GREEN : '#64748b' }}>
            {runDone ? '200 OK · 128ms' : '等待执行'}
          </span>
        </div>
        <div
          style={{
            marginTop: 20,
            display: 'flex',
            alignItems: 'center',
            gap: 9,
            padding: '10px 12px',
            borderRadius: 9,
            border: '1px solid rgba(52,211,153,.26)',
            background: 'rgba(52,211,153,.07)',
            color: '#6ee7b7',
            fontFamily: MONO,
            fontSize: 13,
            letterSpacing: 1,
          }}
        >
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none">
            <path d="M12 3l7 3v5c0 4.4-2.9 8.4-7 9.8C7.9 19.4 5 15.4 5 11V6l7-3z" stroke="#34d399" strokeWidth="1.7" strokeLinejoin="round" />
            <path d="M9 12l2 2 4-4" stroke="#34d399" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round" />
          </svg>
          沙盒隔离 · 密钥不出服务端
        </div>
        <div style={{ marginTop: 9, fontSize: 13, color: '#64748b' }}>超时保护 · 内存限制 · 网络隔离</div>
      </div>
    </AbsoluteFill>
  )
}

const FailoverDemo: React.FC = () => {
  const frame = useCurrentFrame()
  const local = frame - 720
  const failure = interpolate(local, [34, 46], [0, 1], clamp)
  const timedOut = local >= 34
  const taking = local >= 46 && local < 66
  const success = local >= 66
  const breaker = local >= 46
  const statusText = local < 34 ? '主模型请求中…' : local < 66 ? '超时，正在切换备用 A' : '已由备用 A 完成'
  const flow = interpolate(local, [46, 66], [0, 1], clamp)
  const timeoutBurst = interpolate(local, [30, 56], [0, 1], clamp)
  const progress =
    local < 34
      ? (local / 34) * 38
      : local < 66
        ? 38 + ((local - 34) / 32) * 34
        : 72 + (Math.min(local, 120) - 66) / 54 * 24
  const providers = [
    { name: '主模型', priority: 1, accent: failure > 0.2 ? RED : CYAN, letter: 'A' },
    { name: '备用 A', priority: 10, accent: success ? GREEN : taking ? AMBER : BLUE, letter: 'B' },
    { name: '备用 B', priority: 100, accent: '#64748b', letter: 'C' },
  ]
  const statusFor = (index: number) => {
    if (index === 0) {
      if (timedOut) {
        return {
          title: 'TLS 超时 · 30s',
          sub: breaker ? '已熔断 · 冷却 5 分钟' : '超时重试中…',
          color: RED,
        }
      }
      return { title: '连接中…', sub: '主模型 · 优先级 1', color: CYAN }
    }
    if (index === 1) {
      if (success) return { title: '✓ 成功 · 1.2s', sub: '接管请求并完成审核', color: GREEN }
      if (taking) return { title: '接管请求中…', sub: '同类型模型 · 优先级 10', color: AMBER }
      return { title: '待命', sub: '同类型模型 · 优先级 10', color: '#64748b' }
    }
    return { title: '待命', sub: '同类型模型 · 优先级 100', color: '#64748b' }
  }
  return (
    <AbsoluteFill>
      <div
        style={{
          position: 'absolute',
          left: 20,
          top: 520,
          fontFamily: MONO,
          fontSize: 230,
          fontWeight: 800,
          color: 'transparent',
          WebkitTextStroke: '2px rgba(148,163,184,.06)',
          letterSpacing: -8,
          whiteSpace: 'nowrap',
        }}
      >
        FAILOVER
      </div>
      <SectionHeader
        index="05"
        kicker="MODEL ROUTING"
        title="优先级调度，故障自动回落"
        subtitle="主模型失败，同类型自动接管，审核不中断"
        local={local}
        accent={GREEN}
      />
      <div
        style={{
          position: 'absolute',
          left: 140,
          top: 258,
          width: 1640,
          height: 74,
          display: 'flex',
          alignItems: 'center',
          gap: 22,
          padding: '0 26px',
          borderRadius: 14,
          border: '1px solid rgba(96,165,250,.24)',
          background: 'rgba(15,23,42,.72)',
          opacity: interpolate(local, [0, 14], [0, 1], clamp),
          transform: `translateY(${interpolate(local, [0, 14], [-14, 0], clamp)}px)`,
        }}
      >
        <span style={{ fontFamily: MONO, fontSize: 17, color: CYAN, letterSpacing: 2 }}>审计请求 #1832</span>
        <span style={{ fontSize: 16, color: '#64748b' }}>同类型模型 · 自动选择</span>
        <div style={{ flex: 1, height: 6, borderRadius: 999, background: 'rgba(51,65,85,.8)', overflow: 'hidden' }}>
          <div
            style={{
              width: `${progress}%`,
              height: '100%',
              borderRadius: 999,
              background: success
                ? `linear-gradient(90deg, ${AMBER}, ${GREEN})`
                : 'linear-gradient(90deg, #2563eb, #22d3ee)',
              boxShadow: '0 0 16px rgba(34,211,238,.5)',
            }}
          />
        </div>
        {success ? (
          <span
            style={{
              padding: '8px 16px',
              borderRadius: 999,
              border: '1px solid rgba(52,211,153,.42)',
              background: 'rgba(52,211,153,.1)',
              color: '#6ee7b7',
              fontFamily: MONO,
              fontSize: 14,
              whiteSpace: 'nowrap',
              boxShadow: '0 0 24px rgba(52,211,153,.16)',
            }}
          >
            ✓ 审核不中断 · 1.2s
          </span>
        ) : (
          <span style={{ fontFamily: MONO, fontSize: 15, color: taking ? AMBER : '#93c5fd', whiteSpace: 'nowrap' }}>
            {statusText}
          </span>
        )}
      </div>
      {providers.map((provider, index) => {
        const state = statusFor(index)
        const enter = interpolate(local, [index * 5, index * 5 + 16], [0, 1], clamp)
        const dim = index === 2 ? 0.55 : 1
        const active = index === 0 || index === 1
        const pulseGlow = active ? kick(frame) * (index === 1 && success ? 36 : 10) : 0
        return (
          <div
            key={provider.name}
            style={{
              position: 'absolute',
              left: 120 + index * 590,
              top: 402,
              width: 500,
              height: 328,
              padding: '28px 30px',
              borderRadius: 20,
              border: `1px solid ${state.color}55`,
              background: 'linear-gradient(160deg, rgba(15,23,42,.96), rgba(2,6,23,.93))',
              boxShadow: `0 26px 80px rgba(2,6,23,.7), 0 0 ${pulseGlow}px ${state.color}44`,
              opacity: dim * enter,
              transform: `translateY(${(1 - enter) * 34 + (active ? (1 - Math.min(1, local / 16)) * 26 : 0)}px)`,
            }}
          >
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
              <div style={{ display: 'flex', gap: 16, alignItems: 'center' }}>
                <div
                  style={{
                    width: 54,
                    height: 54,
                    borderRadius: 14,
                    border: `1px solid ${state.color}55`,
                    background: `${state.color}12`,
                    color: state.color,
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    fontFamily: MONO,
                    fontSize: 22,
                    fontWeight: 700,
                  }}
                >
                  {provider.letter}
                </div>
                <div>
                  <div style={{ fontSize: 27, fontWeight: 700, color: '#f1f5f9', letterSpacing: 1 }}>{provider.name}</div>
                  <div style={{ marginTop: 4, fontFamily: MONO, fontSize: 13, color: '#64748b' }}>通用大模型</div>
                </div>
              </div>
              <span
                style={{
                  padding: '5px 12px',
                  borderRadius: 999,
                  border: '1px solid rgba(148,163,184,.25)',
                  color: '#94a3b8',
                  fontFamily: MONO,
                  fontSize: 13,
                }}
              >
                优先级 {provider.priority}
              </span>
            </div>
            <div
              style={{
                marginTop: 34,
                height: 94,
                borderRadius: 14,
                border: '1px solid rgba(148,163,184,.14)',
                background: 'rgba(2,6,23,.55)',
                display: 'flex',
                flexDirection: 'column',
                justifyContent: 'center',
                padding: '0 20px',
              }}
            >
              <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
                {index === 0 && local < 34 ? (
                  <span
                    style={{
                      width: 16,
                      height: 16,
                      borderRadius: '50%',
                      border: `2px solid ${CYAN}`,
                      borderTopColor: 'transparent',
                      transform: `rotate(${frame * 12}deg)`,
                    }}
                  />
                ) : null}
                {index === 1 && taking ? (
                  <span
                    style={{
                      width: 16,
                      height: 16,
                      borderRadius: '50%',
                      border: `2px solid ${AMBER}`,
                      borderTopColor: 'transparent',
                      transform: `rotate(${frame * 12}deg)`,
                    }}
                  />
                ) : null}
                <span style={{ fontSize: 24, fontWeight: 700, color: state.color }}>{state.title}</span>
              </div>
              <div style={{ marginTop: 8, fontFamily: MONO, fontSize: 13, color: '#64748b' }}>{state.sub}</div>
            </div>
            <div style={{ marginTop: 18, display: 'flex', gap: 10 }}>
              {index === 0 ? (
                <>
                  <span style={{ padding: '4px 10px', borderRadius: 6, background: 'rgba(248,113,113,.1)', color: '#fca5a5', fontSize: 13 }}>
                    超时重试 1 次
                  </span>
                  <span style={{ padding: '4px 10px', borderRadius: 6, background: 'rgba(148,163,184,.1)', color: '#94a3b8', fontSize: 13 }}>
                    熔断冷却
                  </span>
                </>
              ) : null}
              {index === 1 ? (
                <>
                  <span style={{ padding: '4px 10px', borderRadius: 6, background: 'rgba(52,211,153,.1)', color: '#6ee7b7', fontSize: 13 }}>
                    自动接管
                  </span>
                  <span style={{ padding: '4px 10px', borderRadius: 6, background: 'rgba(148,163,184,.1)', color: '#94a3b8', fontSize: 13 }}>
                    保留原始结论
                  </span>
                  {success ? (
                    <span
                      style={{
                        padding: '4px 10px',
                        borderRadius: 6,
                        background: 'rgba(52,211,153,.16)',
                        color: '#6ee7b7',
                        fontSize: 13,
                        boxShadow: '0 0 16px rgba(52,211,153,.18)',
                      }}
                    >
                      ✓ 审核不中断
                    </span>
                  ) : null}
                </>
              ) : null}
              {index === 2 ? (
                <span style={{ padding: '4px 10px', borderRadius: 6, background: 'rgba(148,163,184,.1)', color: '#94a3b8', fontSize: 13 }}>
                  未触发
                </span>
              ) : null}
            </div>
          </div>
        )
      })}
      {local >= 30 && local <= 56 ? (
        <div
          style={{
            position: 'absolute',
            left: 370 - (80 + timeoutBurst * 420) / 2,
            top: 566 - (80 + timeoutBurst * 420) / 2,
            width: 80 + timeoutBurst * 420,
            height: 80 + timeoutBurst * 420,
            borderRadius: '50%',
            border: `2px solid rgba(248,113,113,${(1 - timeoutBurst) * 0.6})`,
            boxShadow: `0 0 60px rgba(248,113,113,${(1 - timeoutBurst) * 0.35})`,
          }}
        />
      ) : null}
      {local >= 44 && local <= 70 ? (
        <>
          <div
            style={{
              position: 'absolute',
              left: 578 + flow * 128,
              top: 562,
              width: 46,
              height: 6,
              borderRadius: 3,
              background: `linear-gradient(90deg, transparent, ${success ? GREEN : AMBER})`,
              opacity: 0.7,
            }}
          />
          <div
            style={{
              position: 'absolute',
              left: 606 + flow * 124,
              top: 557,
              width: 16,
              height: 16,
              borderRadius: '50%',
              background: success ? GREEN : AMBER,
              boxShadow: `0 0 26px ${success ? GREEN : AMBER}, 0 0 60px ${success ? GREEN : AMBER}88`,
            }}
          />
        </>
      ) : null}
      {[0, 1].map((index) => {
        const activated = index === 0 ? breaker : false
        return (
          <div
            key={index}
            style={{
              position: 'absolute',
              left: 640 + index * 590,
              top: 546,
              width: 52,
              height: 40,
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              color: activated ? GREEN : 'rgba(148,163,184,.35)',
              fontSize: 30,
              opacity: activated ? 1 : 0.6,
              textShadow: activated ? `0 0 18px ${GREEN}` : 'none',
            }}
          >
            →
          </div>
        )
      })}
      <div
        style={{
          position: 'absolute',
          left: 690,
          top: 508,
          fontFamily: MONO,
          fontSize: 12,
          color: breaker ? GREEN : '#475569',
          letterSpacing: 1,
        }}
      >
        回落
      </div>
      <div
        style={{
          position: 'absolute',
          left: 0,
          right: 0,
          top: 784,
          display: 'flex',
          justifyContent: 'center',
          gap: 14,
          opacity: interpolate(local, [18, 32], [0, 1], clamp),
          transform: `translateY(${interpolate(local, [18, 32], [12, 0], clamp)}px)`,
        }}
      >
        {['绑定模型优先', '同类型按优先级回落', '熔断冷却后恢复'].map((chip, index) => (
          <span
            key={chip}
            style={{
              padding: '10px 20px',
              borderRadius: 999,
              border: '1px solid rgba(147,197,253,.25)',
              background: 'rgba(15,23,42,.7)',
              color: index === 1 ? '#6ee7b7' : '#bfdbfe',
              fontFamily: MONO,
              fontSize: 16,
              letterSpacing: 1,
            }}
          >
            {chip}
          </span>
        ))}
      </div>
    </AbsoluteFill>
  )
}

const Donut: React.FC<{ local: number }> = ({ local }) => {
  const circumference = 2 * Math.PI * 104
  const reveal = interpolate(local, [0, 38], [0, 1], clamp)
  const total = Math.round(interpolate(local, [0, 42], [0, 15600], clamp))
  const input = Math.round(total * 0.8)
  const output = total - input
  return (
    <div style={{ position: 'relative', width: 300, height: 300 }}>
      <svg width={300} height={300} viewBox="0 0 300 300">
        <circle cx={150} cy={150} r={104} stroke="rgba(148,163,184,.16)" strokeWidth={28} fill="none" />
        <circle
          cx={150}
          cy={150}
          r={104}
          stroke={CYAN}
          strokeWidth={28}
          fill="none"
          strokeLinecap="round"
          strokeDasharray={`${circumference * 0.8 * reveal} ${circumference}`}
          transform="rotate(-90 150 150)"
        />
        <circle
          cx={150}
          cy={150}
          r={104}
          stroke="#a78bfa"
          strokeWidth={28}
          fill="none"
          strokeLinecap="round"
          strokeDasharray={`${circumference * 0.2 * reveal} ${circumference}`}
          transform="rotate(198 150 150)"
        />
      </svg>
      <div
        style={{
          position: 'absolute',
          inset: 0,
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          justifyContent: 'center',
        }}
      >
        <div style={{ fontFamily: MONO, fontSize: 44, fontWeight: 700, color: '#f8fafc', letterSpacing: 1 }}>
          {total.toLocaleString()}
        </div>
        <div style={{ marginTop: 4, fontFamily: MONO, fontSize: 13, color: '#64748b', letterSpacing: 3 }}>TOKENS</div>
        <div style={{ marginTop: 12, fontFamily: MONO, fontSize: 12, color: '#94a3b8' }}>
          输入 {input.toLocaleString()} · 输出 {output.toLocaleString()}
        </div>
      </div>
    </div>
  )
}

const ObservabilityDemo: React.FC = () => {
  const frame = useCurrentFrame()
  const local = frame - 840
  const cardIn = interpolate(local, [0, 16], [0, 1], clamp)
  const panelIn = interpolate(local, [3, 18], [0, 1], clamp)
  const scan = interpolate(local, [18, 72], [0, 100], clamp)
  const callout = interpolate(local, [46, 60], [0, 1], clamp)
  const revealed = Math.round(interpolate(local, [0, 42], [0, 15600], clamp))
  const input = Math.round(revealed * 0.8)
  const output = revealed - input
  const latency = interpolate(local, [12, 38], [0, 1.2], clamp)
  const cost = interpolate(local, [12, 38], [0, 0.42], clamp)
  const successRate = interpolate(local, [12, 38], [0, 99.2], clamp)
  const tickerIn = interpolate(local, [38, 52], [0, 1], clamp)
  return (
    <AbsoluteFill>
      <SectionHeader
        index="06"
        kicker="OBSERVABILITY"
        title="Token、延迟、成本，全部可查"
        subtitle="每次模型调用都可计量、可追踪、可复盘"
        local={local}
        accent={CYAN}
      />
      <div
        style={{
          position: 'absolute',
          left: 110,
          top: 288,
          width: 980,
          borderRadius: 18,
          overflow: 'hidden',
          border: '1px solid rgba(147,197,253,.3)',
          boxShadow: '0 40px 100px rgba(2,6,23,.75)',
          opacity: cardIn,
          transform: `perspective(1800px) rotateY(${8 - cardIn * 8}deg) translateX(${(1 - cardIn) * -40}px) translateY(${(1 - cardIn) * 24}px)`,
        }}
      >
        <div
          style={{
            height: 38,
            display: 'flex',
            alignItems: 'center',
            gap: 8,
            padding: '0 14px',
            background: 'linear-gradient(180deg, #16243c, #101a2c)',
            borderBottom: '1px solid rgba(148,163,184,.18)',
          }}
        >
          {['#f87171', '#fbbf24', '#34d399'].map((color) => (
            <span key={color} style={{ width: 10, height: 10, borderRadius: '50%', background: color }} />
          ))}
          <span style={{ marginLeft: 12, color: '#64748b', fontFamily: MONO, fontSize: 12 }}>
            smartdoc.local / 模型接入
          </span>
        </div>
        <div style={{ position: 'relative', height: 551, overflow: 'hidden' }}>
          <Img
            src={staticFile('screens/providers.png')}
            style={{
              display: 'block',
              width: 980,
              height: 551,
              transform: `scale(${1.035 - cardIn * 0.035}) translateY(${(1 - cardIn) * 10}px)`,
            }}
          />
          <div
            style={{
              position: 'absolute',
              left: 0,
              right: 0,
              top: `${scan}%`,
              height: 3,
              background: 'linear-gradient(90deg, transparent, rgba(34,211,238,.85), transparent)',
              boxShadow: '0 0 26px rgba(34,211,238,.75)',
              opacity: 0.7,
            }}
          />
          <div
            style={{
              position: 'absolute',
              right: 22,
              top: 20,
              padding: '10px 16px',
              borderRadius: 10,
              border: '1px solid rgba(34,211,238,.4)',
              background: 'rgba(2,6,23,.82)',
              opacity: callout,
              transform: `translateY(${(1 - callout) * 12}px)`,
            }}
          >
            <div style={{ fontFamily: MONO, fontSize: 12, color: '#67e8f9', letterSpacing: 1 }}>INPUT</div>
            <div style={{ marginTop: 2, fontFamily: MONO, fontSize: 22, color: '#f8fafc' }}>{input.toLocaleString()} tokens</div>
          </div>
          <div
            style={{
              position: 'absolute',
              left: 22,
              bottom: 20,
              padding: '8px 14px',
              borderRadius: 999,
              border: '1px solid rgba(52,211,153,.35)',
              background: 'rgba(2,6,23,.82)',
              color: '#6ee7b7',
              fontFamily: MONO,
              fontSize: 13,
              opacity: callout,
            }}
          >
            ● 实时记录 · 缓存命中 18%
          </div>
        </div>
      </div>
      <div
        style={{
          position: 'absolute',
          left: 1150,
          top: 288,
          width: 660,
          height: 590,
          padding: '26px 30px',
          borderRadius: 20,
          border: '1px solid rgba(147,197,253,.22)',
          background: 'linear-gradient(160deg, rgba(15,23,42,.92), rgba(2,6,23,.94))',
          boxShadow: '0 30px 90px rgba(2,6,23,.7)',
          opacity: panelIn,
          transform: `translateX(${(1 - panelIn) * 26}px)`,
        }}
      >
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <span style={{ fontSize: 24, fontWeight: 700, color: '#e2e8f0', letterSpacing: 2 }}>Token 使用明细</span>
          <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
            <span
              style={{
                padding: '4px 10px',
                borderRadius: 999,
                border: '1px solid rgba(251,191,36,.28)',
                background: 'rgba(251,191,36,.06)',
                color: AMBER,
                fontFamily: MONO,
                fontSize: 11,
                letterSpacing: 1,
              }}
            >
              成本可核算
            </span>
            <span style={{ fontFamily: MONO, fontSize: 13, color: GREEN }}>● LIVE</span>
          </div>
        </div>
        <div style={{ display: 'flex', alignItems: 'center', marginTop: 14 }}>
          <Donut local={local} />
          <div style={{ marginLeft: 18, display: 'flex', flexDirection: 'column', gap: 16 }}>
            {[
              { label: '输入', value: input.toLocaleString(), color: CYAN },
              { label: '输出', value: output.toLocaleString(), color: '#a78bfa' },
              { label: '缓存', value: '18%', color: GREEN },
            ].map((item) => (
              <div key={item.label}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 8, fontFamily: MONO, fontSize: 13, color: '#94a3b8' }}>
                  <span style={{ width: 8, height: 8, borderRadius: '50%', background: item.color }} />
                  {item.label}
                </div>
                <div style={{ marginTop: 3, fontFamily: MONO, fontSize: 21, color: '#e2e8f0' }}>{item.value}</div>
              </div>
            ))}
          </div>
        </div>
        <div style={{ marginTop: 14, display: 'flex', gap: 12 }}>
          {[
            { label: '平均延迟', value: `${latency.toFixed(1)}s`, color: CYAN },
            { label: '今日成本', value: `¥${cost.toFixed(2)}`, color: AMBER },
            { label: '成功率', value: `${successRate.toFixed(1)}%`, color: GREEN },
          ].map((item) => (
            <div
              key={item.label}
              style={{
                flex: 1,
                padding: '14px 16px',
                borderRadius: 12,
                border: '1px solid rgba(148,163,184,.16)',
                background: 'rgba(148,163,184,.04)',
              }}
            >
              <div style={{ fontFamily: MONO, fontSize: 12, color: '#64748b', letterSpacing: 1 }}>{item.label}</div>
              <div style={{ marginTop: 6, fontFamily: MONO, fontSize: 26, color: item.color }}>{item.value}</div>
            </div>
          ))}
        </div>
        <div
          style={{
            marginTop: 14,
            display: 'flex',
            alignItems: 'center',
            gap: 9,
            fontFamily: MONO,
            fontSize: 12,
            color: '#64748b',
            opacity: tickerIn,
            transform: `translateY(${(1 - tickerIn) * 8}px)`,
          }}
        >
          <span
            style={{
              width: 7,
              height: 7,
              borderRadius: '50%',
              background: GREEN,
              boxShadow: `0 0 10px ${GREEN}`,
              opacity: frame % 20 < 10 ? 1 : 0.25,
            }}
          />
          最近调用 <span style={{ color: '#cbd5e1' }}>12:04:18</span> · 通用大模型 ·{' '}
          <span style={{ color: CYAN }}>1,280 tokens</span> · 1.1s
        </div>
        <div style={{ marginTop: 10, fontFamily: MONO, fontSize: 12, color: '#475569' }}>
          统计口径：最近 24 小时 · 按模型 / 审核配置聚合
        </div>
      </div>
    </AbsoluteFill>
  )
}

const RECAP = [
  { text: '40 万上下文', sub: '长文档整本读入，不丢细节', color: CYAN, stat: '400K', statLabel: 'TOKENS / REQUEST' },
  { text: '自动熔断', sub: '故障模型自动摘除，审计不中断', color: '#a78bfa', stat: '1.2s', statLabel: 'AUTO FAILOVER' },
  { text: '全链路回调', sub: 'HTTP · Redis · JS 三种动作', color: BLUE, stat: '3', statLabel: 'ACTION TYPES' },
  { text: '0 侵入接入', sub: 'OpenAPI 直接对接现有系统', color: GREEN, stat: '0', statLabel: 'CODE CHANGES' },
]

const RecapScene: React.FC = () => {
  const frame = useCurrentFrame()
  const index = Math.min(RECAP.length - 1, Math.floor((frame - 960) / 30))
  const local = (frame - 960) % 30
  const item = RECAP[index]
  const enter = interpolate(local, [0, 9], [0, 1], clamp)
  const sweep = interpolate(local, [10, 24], [-40, 140], clamp)
  return (
    <AbsoluteFill>
      <div
        style={{
          position: 'absolute',
          left: -160,
          top: -80,
          width: 780,
          height: 1240,
          transform: 'skewX(-12deg)',
          background: `linear-gradient(160deg, ${item.color}22, transparent 70%)`,
        }}
      />
      <div
        style={{
          position: 'absolute',
          right: 100,
          top: 60,
          fontFamily: MONO,
          fontSize: 360,
          fontWeight: 800,
          color: 'transparent',
          WebkitTextStroke: '2px rgba(148,163,184,.12)',
          lineHeight: 1,
        }}
      >
        0{index + 1}
      </div>
      <div style={{ position: 'absolute', left: 168, top: 250, fontFamily: MONO, fontSize: 17, color: item.color, letterSpacing: 8 }}>
        WHY SMARTDOC
      </div>
      <div
        style={{
          position: 'absolute',
          left: 1210,
          top: 456,
          width: 580,
          textAlign: 'center',
          opacity: enter,
          transform: `scale(${0.92 + enter * 0.08})`,
        }}
      >
        <div style={{ position: 'absolute', left: '50%', top: 20, width: 420, height: 420, marginLeft: -210, borderRadius: '50%', border: `1px dashed ${item.color}33`, transform: `rotate(${frame * 0.2}deg)` }} />
        <div
          style={{
            position: 'relative',
            fontFamily: MONO,
            fontSize: 214,
            fontWeight: 800,
            color: item.color,
            lineHeight: 1,
            letterSpacing: -6,
            textShadow: `0 0 90px ${item.color}55`,
          }}
        >
          {item.stat}
        </div>
        <div style={{ position: 'relative', marginTop: 10, fontFamily: MONO, fontSize: 16, color: '#94a3b8', letterSpacing: 4 }}>
          {item.statLabel}
        </div>
        <div style={{ position: 'relative', marginTop: 18, display: 'flex', justifyContent: 'center' }}>
          <span style={{ width: 140 * enter, height: 3, background: item.color, boxShadow: `0 0 20px ${item.color}` }} />
        </div>
      </div>
      <div
        style={{
          position: 'absolute',
          left: 160,
          top: 344,
          transform: `translateY(${(1 - enter) * 40}px)`,
          opacity: 0.6 + enter * 0.4,
        }}
      >
        <div
          style={{
            fontSize: 150,
            fontWeight: 900,
            color: '#f8fafc',
            letterSpacing: 6,
            lineHeight: 1.05,
            textShadow: `0 0 70px ${item.color}66`,
          }}
        >
          {item.text}
        </div>
        <div style={{ marginTop: 28, fontSize: 32, color: '#cbd5e1', letterSpacing: 2 }}>{item.sub}</div>
      </div>
      <div
        style={{
          position: 'absolute',
          left: 160,
          right: 160,
          top: 700,
          height: 2,
          background: 'rgba(148,163,184,.18)',
          overflow: 'hidden',
        }}
      >
        <div
          style={{
            position: 'absolute',
            left: `${sweep}%`,
            top: 0,
            width: '30%',
            height: '100%',
            background: `linear-gradient(90deg, transparent, ${item.color}, transparent)`,
            boxShadow: `0 0 20px ${item.color}`,
          }}
        />
      </div>
      <div style={{ position: 'absolute', left: 160, bottom: 132, display: 'flex', gap: 10 }}>
        {RECAP.map((entry, entryIndex) => (
          <span
            key={entry.text}
            style={{
              width: entryIndex === index ? 52 : 16,
              height: 5,
              borderRadius: 99,
              background: entryIndex === index ? item.color : 'rgba(148,163,184,.28)',
              boxShadow: entryIndex === index ? `0 0 16px ${item.color}` : 'none',
            }}
          />
        ))}
      </div>
    </AbsoluteFill>
  )
}

const Outro: React.FC = () => {
  const frame = useCurrentFrame()
  const local = frame - 1080
  const enter = spring({ frame: local, fps: 30, config: { damping: 15, stiffness: 90 } })
  const fade = interpolate(frame, [1182, 1200], [1, 0], clamp)
  const sweep = interpolate(local, [20, 86], [-30, 130], clamp)
  return (
    <AbsoluteFill style={{ alignItems: 'center', justifyContent: 'center', opacity: fade }}>
      {[0, 1].map((ring) => (
        <div
          key={ring}
          style={{
            position: 'absolute',
            width: 620 + ring * 220,
            height: 620 + ring * 220,
            borderRadius: '50%',
            border: `1px ${ring === 0 ? 'solid' : 'dashed'} rgba(96,165,250,${0.18 - ring * 0.06})`,
            transform: `rotate(${local * (ring === 0 ? 0.4 : -0.3)}deg)`,
          }}
        />
      ))}
      <div style={{ transform: `scale(${enter})` }}>
        <Logo size={112} />
      </div>
      <div
        style={{
          marginTop: 30,
          fontSize: 92,
          fontWeight: 900,
          color: '#f8fafc',
          letterSpacing: 10,
          opacity: interpolate(local, [14, 42], [0, 1], clamp),
          textShadow: '0 0 70px rgba(59,130,246,.65)',
        }}
      >
        智审图档
      </div>
      <div
        style={{
          marginTop: 18,
          fontSize: 40,
          color: '#dbeafe',
          letterSpacing: 4,
          opacity: interpolate(local, [22, 48], [0, 1], clamp),
        }}
      >
        让每一份档案，都有据可审
      </div>
      <div
        style={{
          marginTop: 34,
          display: 'flex',
          gap: 12,
          opacity: interpolate(local, [38, 60], [0, 1], clamp),
        }}
      >
        {['多模态', '规则化', '可编排', '全链路'].map((chip) => (
          <span
            key={chip}
            style={{
              padding: '9px 20px',
              borderRadius: 999,
              border: '1px solid rgba(34,211,238,.4)',
              background: 'rgba(2,6,23,.7)',
              color: '#a5f3fc',
              fontSize: 19,
              letterSpacing: 2,
            }}
          >
            {chip}
          </span>
        ))}
      </div>
      <div
        style={{
          marginTop: 54,
          color: '#93c5fd',
          fontFamily: MONO,
          fontSize: 17,
          letterSpacing: 6,
          opacity: interpolate(local, [48, 68], [0, 1], clamp),
        }}
      >
        SMARTDOC VISION · v1.3.0
      </div>
      <div
        style={{
          position: 'absolute',
          left: `${sweep}%`,
          top: 0,
          width: 320,
          height: '100%',
          transform: 'skewX(-14deg)',
          background: 'linear-gradient(90deg, transparent, rgba(191,219,254,.16), transparent)',
        }}
      />
    </AbsoluteFill>
  )
}

const CHAPTERS = [
  { start: 0, end: 120, label: 'OPENING' },
  { start: 120, end: 240, label: 'ADVANTAGES' },
  { start: 240, end: 360, label: 'MULTIMODAL' },
  { start: 360, end: 480, label: 'RULES' },
  { start: 480, end: 600, label: 'ACTIONS' },
  { start: 600, end: 720, label: 'SCRIPT' },
  { start: 720, end: 840, label: 'FAILOVER' },
  { start: 840, end: 960, label: 'OBSERVABILITY' },
  { start: 960, end: 1080, label: 'RECAP' },
  { start: 1080, end: 1200, label: 'CLOSING' },
]

const Launch: React.FC = () => {
  const frame = useCurrentFrame()
  const pulse = kick(frame)
  const impact = impactEnergy(frame)
  const shakeX = Math.sin(frame * 1.7) * 5 * pulse + Math.sin(frame * 0.9) * 9 * impact
  const shakeY = Math.cos(frame * 1.3) * 4 * pulse + Math.cos(frame * 1.1) * 7 * impact
  const chapter = CHAPTERS.find((entry) => frame >= entry.start && frame < entry.end)?.label ?? 'CLOSING'
  const timecode = `00:${String(Math.floor(frame / 30)).padStart(2, '0')}`
  const chapterIndex = Math.min(CHAPTERS.length - 1, Math.floor(frame / 120))
  const sceneLocal = frame - chapterIndex * 120
  const sceneEnter = easeOut(saturate(sceneLocal / 12))
  const scenePan = chapterIndex % 2 === 0 ? 1 : -1
  const showFeatureGhost = chapterIndex >= 2 && chapterIndex <= 7

  const showIntro = frame < 120
  const showFlash = frame >= 120 && frame < 240
  const showMultimodal = frame >= 240 && frame < 360
  const showRules = frame >= 360 && frame < 480
  const showActions = frame >= 480 && frame < 600
  const showScript = frame >= 600 && frame < 720
  const showFailover = frame >= 720 && frame < 840
  const showObservability = frame >= 840 && frame < 960
  const showRecap = frame >= 960 && frame < 1080
  const showOutro = frame >= 1080

  return (
    <AbsoluteFill style={{ backgroundColor: '#040812', fontFamily: FONT }}>
      <Audio src={staticFile('audio/launch-beat.mp3')} />
      <AbsoluteFill
        style={{
          backgroundImage:
            'radial-gradient(circle at 50% 30%, rgba(37,99,235,.32), transparent 56%), linear-gradient(rgba(148,163,184,.055) 1px, transparent 1px), linear-gradient(90deg, rgba(148,163,184,.055) 1px, transparent 1px)',
          backgroundSize: '100% 100%, 64px 64px, 64px 64px',
          transform: `translate(${shakeX}px, ${shakeY}px) scale(${1 + pulse * 0.006})`,
        }}
      >
        <Particles />
        {showFeatureGhost ? (
          <>
            <div
              style={{
                position: 'absolute',
                right: -40,
                top: 30,
                width: 540,
                height: 540,
                borderRadius: '50%',
                border: '1px dashed rgba(148,163,184,.08)',
                transform: `rotate(${frame * 0.12}deg)`,
              }}
            />
            <div
              style={{
                position: 'absolute',
                right: 70,
                top: 118,
                fontFamily: MONO,
                fontSize: 330,
                fontWeight: 800,
                color: 'transparent',
                WebkitTextStroke: '2px rgba(148,163,184,.07)',
                letterSpacing: -14,
                lineHeight: 1,
                transform: `translateY(${Math.sin(frame * 0.05) * 7}px)`,
              }}
            >
              {String(chapterIndex - 1).padStart(2, '0')}
            </div>
          </>
        ) : null}
        <AbsoluteFill
          style={{
            transform: `scale(${1.035 - sceneEnter * 0.035}) translateX(${(1 - sceneEnter) * scenePan * 26}px)`,
          }}
        >
          {showIntro ? <Intro /> : null}
          {showFlash ? <FlashWords /> : null}
          {showMultimodal ? <MultimodalDemo /> : null}
          {showRules ? <RulesDemo /> : null}
          {showActions ? <ActionsDemo /> : null}
          {showScript ? <ScriptDemo /> : null}
          {showFailover ? <FailoverDemo /> : null}
          {showObservability ? <ObservabilityDemo /> : null}
          {showRecap ? <RecapScene /> : null}
          {showOutro ? <Outro /> : null}
        </AbsoluteFill>
        <div
          style={{
            position: 'absolute',
            left: -420 + ((frame % 120) / 120) * 2680,
            top: 0,
            width: 380,
            height: '100%',
            transform: 'skewX(-14deg)',
            background: 'linear-gradient(90deg, transparent, rgba(34,211,238,.055), transparent)',
            pointerEvents: 'none',
          }}
        />
      </AbsoluteFill>
      <Equalizer />
      <Shockwave />
      <TransitionOverlay />
      <Flash />
      <ChapterCut />
      <AbsoluteFill
        style={{
          background:
            'repeating-linear-gradient(180deg, rgba(255,255,255,.028) 0px, rgba(255,255,255,.028) 1px, transparent 1px, transparent 4px)',
          pointerEvents: 'none',
          opacity: 0.5,
        }}
      />
      <AbsoluteFill
        style={{
          background: 'radial-gradient(ellipse at center, transparent 45%, rgba(2,6,23,.75) 100%)',
          pointerEvents: 'none',
        }}
      />
      <div
        style={{
          position: 'absolute',
          top: 0,
          left: 0,
          height: 2,
          width: `${(frame / 1200) * 100}%`,
          background: 'linear-gradient(90deg, #2563eb, #22d3ee)',
          boxShadow: '0 0 14px rgba(34,211,238,.8)',
          opacity: 0.7,
        }}
      />
      <div
        style={{
          position: 'absolute',
          right: 78,
          top: 50,
          display: 'flex',
          alignItems: 'center',
          gap: 16,
          fontFamily: MONO,
          fontSize: 14,
          letterSpacing: 3,
          color: 'rgba(148,163,184,.55)',
        }}
      >
        <span>{chapter}</span>
        <span style={{ color: 'rgba(34,211,238,.85)' }}>{timecode}</span>
      </div>
    </AbsoluteFill>
  )
}

export const LaunchComposition: React.FC = () => <Launch />
