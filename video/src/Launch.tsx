import React from 'react'
import {
  AbsoluteFill,
  Audio,
  Img,
  interpolate,
  spring,
  staticFile,
  useCurrentFrame,
  useVideoConfig,
} from 'remotion'

const FONT = '"Microsoft YaHei", "PingFang SC", "Noto Sans SC", "Segoe UI", sans-serif'
const MONO = '"Cascadia Code", "JetBrains Mono", Consolas, monospace'
const BEAT = 15
const BAR = 60
const BRAND = '#3b82f6'
const CYAN = '#22d3ee'

const clamp = {
  extrapolateLeft: 'clamp' as const,
  extrapolateRight: 'clamp' as const,
}

const kick = (frame: number) => Math.exp(-((frame % BEAT) / 3.2))
const barLocal = (frame: number) => frame % BAR

function snareFlash(frame: number) {
  const local = barLocal(frame)
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
      {impactFrames.map((hit) => {
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

const BrowserCard: React.FC<{
  image: string
  label: string
  local: number
  index: number
}> = ({ image, label, local, index }) => {
  const frame = useCurrentFrame()
  const pulse = kick(frame)
  const zoomDirection = index % 2 === 0 ? 1 : -1
  const scale = 1.02 + zoomDirection * (local / 60) * 0.045 + pulse * 0.014
  const shiftX = zoomDirection * (local / 60) * 26
  const rotate = Math.sin(local / 18) * 1.8
  return (
    <div
      style={{
        position: 'absolute',
        left: 270,
        top: 120,
        width: 1380,
        transform: `perspective(1600px) rotateY(${rotate}deg) scale(${scale}) translateX(${shiftX}px)`,
        filter: 'drop-shadow(0 40px 90px rgba(2,6,23,.75))',
      }}
    >
      <div
        style={{
          borderRadius: 18,
          overflow: 'hidden',
          border: '1px solid rgba(147,197,253,.35)',
          boxShadow: `0 0 ${26 + pulse * 30}px rgba(59,130,246,${0.25 + pulse * 0.25})`,
        }}
      >
        <div
          style={{
            height: 42,
            display: 'flex',
            alignItems: 'center',
            gap: 8,
            padding: '0 16px',
            background: 'linear-gradient(180deg, #16243c, #101a2c)',
            borderBottom: '1px solid rgba(148,163,184,.18)',
          }}
        >
          {['#f87171', '#fbbf24', '#34d399'].map((color) => (
            <span key={color} style={{ width: 10, height: 10, borderRadius: '50%', background: color }} />
          ))}
          <span style={{ marginLeft: 14, color: '#64748b', fontFamily: MONO, fontSize: 12 }}>
            smartdoc.local / {label}
          </span>
        </div>
        <Img src={image} style={{ display: 'block', width: 1380, height: 776 }} />
      </div>
    </div>
  )
}

const words = [
  { text: '多模态审核', sub: '文本 + 图片' },
  { text: '规则化判定', sub: 'PASS / REJECT / REVIEW' },
  { text: '结果动作', sub: 'HTTP · Redis · JS' },
  { text: '全链路闭环', sub: '提交 → 判定 → 回调' },
]

const WordScene: React.FC = () => {
  const frame = useCurrentFrame()
  const { fps } = useVideoConfig()
  const index = Math.min(3, Math.floor((frame - 120) / 30))
  const local = (frame - 120) % 30
  const word = words[index]
  const enter = spring({ frame: local, fps, config: { damping: 12, stiffness: 220, mass: 0.6 } })
  const pulse = kick(frame)
  return (
    <AbsoluteFill style={{ alignItems: 'center', justifyContent: 'center' }}>
      <div style={{ transform: `scale(${0.9 + enter * 0.1 + pulse * 0.02})`, textAlign: 'center' }}>
        <div
          style={{
            color: '#f8fafc',
            fontSize: 118,
            fontWeight: 800,
            letterSpacing: 8,
            textShadow: '0 0 50px rgba(59,130,246,.65)',
          }}
        >
          {word.text}
        </div>
        <div
          style={{
            marginTop: 18,
            color: '#93c5fd',
            fontFamily: MONO,
            fontSize: 28,
            letterSpacing: 6,
            opacity: enter,
          }}
        >
          {word.sub}
        </div>
      </div>
      <div style={{ position: 'absolute', bottom: 120, display: 'flex', gap: 12 }}>
        {words.map((item, itemIndex) => (
          <span
            key={item.text}
            style={{
              width: itemIndex === index ? 42 : 12,
              height: 5,
              borderRadius: 99,
              background: itemIndex === index ? CYAN : 'rgba(148,163,184,.3)',
              boxShadow: itemIndex === index ? `0 0 14px ${CYAN}` : 'none',
            }}
          />
        ))}
      </div>
    </AbsoluteFill>
  )
}

const Hero: React.FC = () => {
  const frame = useCurrentFrame()
  const local = frame - 720
  const pulse = kick(frame)
  const scale = 1.02 + (local / 120) * 0.06 + pulse * 0.012
  return (
    <AbsoluteFill>
      <div
        style={{
          position: 'absolute',
          left: 210,
          top: 130,
          width: 1500,
          transform: `scale(${scale})`,
          borderRadius: 20,
          overflow: 'hidden',
          border: '1px solid rgba(147,197,253,.35)',
          boxShadow: `0 0 ${30 + pulse * 40}px rgba(59,130,246,${0.28 + pulse * 0.3})`,
        }}
      >
        <Img src={staticFile('screens/script-editor.png')} style={{ display: 'block', width: 1500, height: 844 }} />
      </div>
      <div
        style={{
          position: 'absolute',
          left: 150,
          bottom: 92,
          display: 'flex',
          gap: 14,
        }}
      >
        {['AI 读写编辑器', 'MD5 / AES / DES', '沙盒运行', '项目密钥'].map((chip, index) => {
          const item = interpolate(frame, [740 + index * 10, 766 + index * 10], [0, 1], clamp)
          return (
            <span
              key={chip}
              style={{
                padding: '10px 20px',
                borderRadius: 999,
                border: '1px solid rgba(34,211,238,.45)',
                background: 'rgba(2,6,23,.78)',
                color: '#a5f3fc',
                fontSize: 20,
                letterSpacing: 1,
                opacity: item,
                transform: `translateY(${(1 - item) * 14}px)`,
              }}
            >
              {chip}
            </span>
          )
        })}
      </div>
    </AbsoluteFill>
  )
}

const DropWord: React.FC<{ text: string; local: number }> = ({ text, local }) => {
  const frame = useCurrentFrame()
  const pulse = kick(frame)
  const enter = interpolate(local, [0, 5], [1.35, 1], clamp)
  return (
    <div
      style={{
        position: 'absolute',
        left: 70,
        bottom: 64,
        padding: '10px 34px 12px 26px',
        borderRadius: 10,
        background: 'linear-gradient(90deg, rgba(2,6,23,.88), rgba(2,6,23,.45) 62%, transparent)',
        color: '#f8fafc',
        fontSize: 92,
        fontWeight: 800,
        letterSpacing: 6,
        opacity: interpolate(local, [0, 3, 26, 30], [0, 1, 1, 0], clamp),
        transform: `scale(${enter + pulse * 0.02})`,
        transformOrigin: 'left bottom',
        textShadow: '0 0 46px rgba(34,211,238,.55)',
        zIndex: 4,
      }}
    >
      {text}
    </div>
  )
}

const grooveCuts = [
  { image: 'dashboard.png', label: '工作台' },
  { image: 'audits.png', label: '审核配置' },
  { image: 'providers.png', label: '模型接入' },
  { image: 'script-editor.png', label: '结果动作' },
]

const dropWords = [
  '文本 + 图片',
  '规则判定',
  '三种结论',
  '脚本闭环',
  'Token 明细',
  '自动熔断',
  '40 万上下文',
  '全链路',
]

const Launch: React.FC = () => {
  const frame = useCurrentFrame()
  const pulse = kick(frame)
  const impact = impactEnergy(frame)
  const shakeX = Math.sin(frame * 1.7) * 5 * pulse + Math.sin(frame * 0.9) * 9 * impact
  const shakeY = Math.cos(frame * 1.3) * 4 * pulse + Math.cos(frame * 1.1) * 7 * impact
  const globalFlash = Math.min(0.45, snareFlash(frame) + impact * 0.4)

  const showIntro = frame < 120
  const showWords = frame >= 120 && frame < 240
  const showGroove = frame >= 240 && frame < 480
  const showDrop1 = frame >= 480 && frame < 720
  const showHero = frame >= 720 && frame < 840
  const showDrop2 = frame >= 840 && frame < 1080
  const showOutro = frame >= 1080

  const grooveIndex = Math.min(3, Math.floor((frame - 240) / 60))
  const drop1Index = Math.min(7, Math.floor((frame - 480) / 30))
  const drop2Index = Math.min(7, Math.floor((frame - 840) / 30))

  const outroEnter = spring({ frame: frame - 1080, fps: 30, config: { damping: 15, stiffness: 90 } })

  return (
    <AbsoluteFill style={{ backgroundColor: '#040812', fontFamily: FONT }}>
      <Audio src={staticFile('audio/launch-beat.mp3')} />

      <AbsoluteFill
        style={{
          backgroundImage:
            'radial-gradient(circle at 50% 30%, rgba(37,99,235,.35), transparent 55%), linear-gradient(rgba(148,163,184,.06) 1px, transparent 1px), linear-gradient(90deg, rgba(148,163,184,.06) 1px, transparent 1px)',
          backgroundSize: '100% 100%, 64px 64px, 64px 64px',
          transform: `translate(${shakeX}px, ${shakeY}px) scale(${1 + pulse * 0.006})`,
        }}
      >
        <Particles />

        {showIntro ? (
          <AbsoluteFill style={{ alignItems: 'center', justifyContent: 'center' }}>
            <div
              style={{
                transform: `scale(${spring({ frame, fps: 30, config: { damping: 12, stiffness: 110 } }) + pulse * 0.02})`,
              }}
            >
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
                opacity: interpolate(frame, [60, 84], [0, 1], clamp),
              }}
            >
              {[
                { label: 'PASS', color: '#34d399' },
                { label: 'REVIEW', color: '#fbbf24' },
                { label: 'REJECT', color: '#f87171' },
              ].map((item) => (
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
                  }}
                >
                  {item.label}
                </span>
              ))}
            </div>
          </AbsoluteFill>
        ) : null}

        {showWords ? <WordScene /> : null}

        {showGroove ? (
          <>
            <BrowserCard
              image={staticFile(`screens/${grooveCuts[grooveIndex].image}`)}
              label={grooveCuts[grooveIndex].label}
              local={(frame - 240) % 60}
              index={grooveIndex}
            />
            <DropWord text={grooveCuts[grooveIndex].label} local={(frame - 240) % 60} />
          </>
        ) : null}

        {showDrop1 ? (
          <>
            <BrowserCard
              image={staticFile(`screens/${grooveCuts[drop1Index % 4].image}`)}
              label={grooveCuts[drop1Index % 4].label}
              local={(frame - 480) % 30}
              index={drop1Index}
            />
            <DropWord text={dropWords[drop1Index]} local={(frame - 480) % 30} />
          </>
        ) : null}

        {showHero ? <Hero /> : null}

        {showDrop2 ? (
          <>
            <BrowserCard
              image={staticFile(`screens/${grooveCuts[drop2Index % 4].image}`)}
              label={grooveCuts[drop2Index % 4].label}
              local={(frame - 840) % 30}
              index={drop2Index + 1}
            />
            <DropWord text={dropWords[drop2Index]} local={(frame - 840) % 30} />
          </>
        ) : null}

        {showOutro ? (
          <AbsoluteFill
            style={{
              alignItems: 'center',
              justifyContent: 'center',
              opacity: interpolate(frame, [1170, 1200], [1, 0], clamp),
            }}
          >
            <div style={{ transform: `scale(${outroEnter})` }}>
              <Logo size={110} />
            </div>
            <div
              style={{
                marginTop: 34,
                color: '#f8fafc',
                fontSize: 54,
                fontWeight: 700,
                letterSpacing: 4,
                opacity: interpolate(frame, [1100, 1128], [0, 1], clamp),
              }}
            >
              让每一份档案，都有据可审
            </div>
            <div
              style={{
                marginTop: 26,
                color: '#93c5fd',
                fontFamily: MONO,
                fontSize: 18,
                letterSpacing: 6,
                opacity: interpolate(frame, [1120, 1148], [0, 1], clamp),
              }}
            >
              SMARTDOC VISION · v1.3.0
            </div>
          </AbsoluteFill>
        ) : null}
      </AbsoluteFill>

      <Shockwave />
      <Flash />
      <Equalizer />

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
      <AbsoluteFill
        style={{
          background: 'radial-gradient(circle at 50% 45%, rgba(255,255,255,.9), transparent 65%)',
          opacity: globalFlash * 0.5,
          mixBlendMode: 'screen',
          pointerEvents: 'none',
        }}
      />
    </AbsoluteFill>
  )
}

export const LaunchComposition: React.FC = () => <Launch />
