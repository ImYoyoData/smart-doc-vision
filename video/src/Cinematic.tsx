import React from 'react'
import {
  AbsoluteFill,
  Audio,
  Img,
  Sequence,
  interpolate,
  spring,
  staticFile,
  useCurrentFrame,
  useVideoConfig,
} from 'remotion'

const FONT = '"Microsoft YaHei", "PingFang SC", "Noto Sans SC", "Segoe UI", sans-serif'
const MONO = '"Cascadia Code", "JetBrains Mono", Consolas, monospace'
const BEAT = 15
const C = {
  bg: '#030814',
  ink: '#f8fbff',
  text: '#c8d8ee',
  muted: '#7f93ad',
  cyan: '#22d3ee',
  blue: '#4f8bff',
  green: '#34d399',
  amber: '#fbbf24',
  red: '#fb7185',
  purple: '#a78bfa',
}
const clamp = {
  extrapolateLeft: 'clamp' as const,
  extrapolateRight: 'clamp' as const,
}
const beatPulse = (frame: number, decay = 3.4) => Math.exp(-((frame % BEAT) / decay))
const easeOut = (value: number) => 1 - Math.pow(1 - value, 3)
const lerp = (from: number, to: number, progress: number) => from + (to - from) * progress
const appear = (frame: number, start: number, duration = 18) =>
  interpolate(frame, [start, start + duration], [0, 1], clamp)
const travel = (frame: number, start: number, end: number, from: number, to: number) =>
  interpolate(frame, [start, end], [from, to], clamp)

const Glass: React.FC<{ children?: React.ReactNode; style?: React.CSSProperties }> = ({ children, style }) => (
  <div
    style={{
      position: 'relative',
      border: '1px solid rgba(148,187,255,.28)',
      borderRadius: 24,
      background: 'linear-gradient(145deg, rgba(16,31,58,.84), rgba(7,17,35,.75) 54%, rgba(12,28,50,.9))',
      boxShadow: '0 35px 90px -30px rgba(0,0,0,.85), inset 0 1px 0 rgba(255,255,255,.12), 0 0 45px rgba(34,211,238,.08)',
      backdropFilter: 'blur(18px)',
      ...style,
    }}
  >
    {children}
  </div>
)

const LogoMark: React.FC<{ size?: number; glow?: number }> = ({ size = 90, glow = 1 }) => (
  <div
    style={{
      width: size,
      height: size,
      borderRadius: size * 0.26,
      background: 'linear-gradient(145deg, #8fdcff, #2b6cf6 45%, #1644b8)',
      border: '1px solid rgba(216,239,255,.68)',
      boxShadow: `0 0 ${70 * glow}px rgba(43,108,246,.8), 0 0 24px rgba(34,211,238,.65), inset 0 1px 0 rgba(255,255,255,.4)`,
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      position: 'relative',
      overflow: 'hidden',
    }}
  >
    <div style={{ position: 'absolute', inset: 0, background: 'linear-gradient(120deg, rgba(255,255,255,.25), transparent 35%, rgba(255,255,255,.08))' }} />
    <svg width={size * 0.52} height={size * 0.52} viewBox="0 0 24 24" fill="none">
      <path d="M7 3.8h7l4 4v12.4H7z" stroke="#fff" strokeWidth="1.5" strokeLinejoin="round" />
      <path d="M14 3.8v4h4" stroke="#fff" strokeWidth="1.5" strokeLinejoin="round" />
      <path d="M9.4 13.4l1.9 1.9 3.5-3.7" stroke="#e0f2fe" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  </div>
)

const Backdrop: React.FC<{ frame: number }> = ({ frame }) => {
  const glow = beatPulse(frame, 4)
  const particles = React.useMemo(
    () =>
      Array.from({ length: 48 }).map((_, index) => ({
        x: (index * 157 + 83) % 1920,
        y: (index * 241 + 47) % 1080,
        size: 1 + (index % 3),
        speed: 0.2 + (index % 7) * 0.07,
        phase: index * 0.63,
      })),
    [],
  )
  return (
    <AbsoluteFill style={{ background: C.bg, overflow: 'hidden' }}>
      <AbsoluteFill
        style={{
          background: `radial-gradient(900px 560px at 50% 26%, rgba(43,108,246,.26), transparent 70%),
          radial-gradient(700px 500px at 10% 90%, rgba(34,211,238,.14), transparent 74%),
          radial-gradient(760px 520px at 92% 10%, rgba(124,58,237,.15), transparent 72%),
          linear-gradient(135deg, #030814 0%, #071225 50%, #030914 100%)`,
        }}
      />
      <div
        style={{
          position: 'absolute',
          left: '-18%',
          right: '-18%',
          top: '48%',
          height: '80%',
          transform: 'perspective(720px) rotateX(62deg) translateY(40px)',
          transformOrigin: 'center top',
          backgroundImage:
            'linear-gradient(rgba(79,139,255,.22) 1px, transparent 1px), linear-gradient(90deg, rgba(79,139,255,.22) 1px, transparent 1px)',
          backgroundSize: '76px 76px',
          maskImage: 'linear-gradient(to bottom, rgba(0,0,0,.8), transparent 80%)',
          opacity: 0.5,
        }}
      />
      {particles.map((particle, index) => (
        <div
          key={index}
          style={{
            position: 'absolute',
            left: particle.x,
            top: (particle.y + frame * particle.speed) % 1080,
            width: particle.size,
            height: particle.size,
            borderRadius: '50%',
            background: index % 5 === 0 ? C.cyan : '#b6d6ff',
            boxShadow: `0 0 8px ${index % 5 === 0 ? C.cyan : '#8cc7ff'}`,
            opacity: 0.22 + 0.2 * Math.sin(frame * 0.03 + particle.phase),
          }}
        />
      ))}
      <div
        style={{
          position: 'absolute',
          left: '50%',
          top: '50%',
          width: 900,
          height: 900,
          transform: 'translate(-50%, -50%)',
          borderRadius: '50%',
          background: `radial-gradient(circle, rgba(34,211,238,${0.08 + glow * 0.06}), transparent 63%)`,
        }}
      />
      <AbsoluteFill style={{ background: 'radial-gradient(ellipse at center, transparent 45%, rgba(1,4,10,.7) 100%)' }} />
    </AbsoluteFill>
  )
}

const DocumentCard: React.FC<{
  x: number
  y: number
  z?: number
  rotateX?: number
  rotateY?: number
  rotateZ?: number
  scale?: number
  opacity?: number
  accent?: string
  stamp?: string
  title?: string
  glow?: number
}> = ({
  x,
  y,
  z = 0,
  rotateX = 0,
  rotateY = 0,
  rotateZ = 0,
  scale = 1,
  opacity = 1,
  accent = C.cyan,
  stamp,
  title = '建设工程档案',
  glow = 0,
}) => (
  <div
    style={{
      position: 'absolute',
      left: '50%',
      top: '50%',
      width: 278,
      height: 356,
      marginLeft: -139,
      marginTop: -178,
      transform: `translate3d(${x}px, ${y}px, ${z}px) rotateX(${rotateX}deg) rotateY(${rotateY}deg) rotateZ(${rotateZ}deg) scale(${scale})`,
      opacity,
      transformStyle: 'preserve-3d',
      filter: `drop-shadow(0 34px 35px rgba(0,0,0,.46)) drop-shadow(0 0 ${glow}px ${accent}55)`,
    }}
  >
    <div
      style={{
        position: 'relative',
        width: '100%',
        height: '100%',
        borderRadius: 20,
        border: '1px solid rgba(171,208,255,.38)',
        background: 'linear-gradient(150deg, rgba(23,44,76,.95), rgba(8,21,42,.96))',
        boxShadow: 'inset 0 1px 0 rgba(255,255,255,.14)',
        overflow: 'hidden',
        padding: 20,
        boxSizing: 'border-box',
      }}
    >
      <div style={{ position: 'absolute', inset: 0, background: 'linear-gradient(115deg, rgba(255,255,255,.13), transparent 30%, transparent 66%, rgba(79,139,255,.1))' }} />
      <div style={{ position: 'relative', display: 'flex', alignItems: 'center', gap: 9 }}>
        <div style={{ width: 10, height: 10, borderRadius: '50%', background: accent, boxShadow: `0 0 16px ${accent}` }} />
        <div style={{ width: 104, height: 8, borderRadius: 99, background: 'rgba(226,240,255,.68)' }} />
        <div style={{ marginLeft: 'auto', width: 24, height: 24, borderRadius: 7, border: '1px solid rgba(212,232,255,.3)' }} />
      </div>
      <div style={{ position: 'relative', marginTop: 22, fontFamily: FONT, color: C.ink, fontSize: 18, fontWeight: 700, letterSpacing: 1 }}>{title}</div>
      <div style={{ position: 'relative', marginTop: 7, color: accent, fontFamily: MONO, fontSize: 10, letterSpacing: 2 }}>ARCHIVE / {stamp ? 'VERIFIED' : 'IN REVIEW'}</div>
      <div style={{ position: 'relative', display: 'flex', gap: 11, marginTop: 20 }}>
        <div style={{ flex: 1, height: 134, borderRadius: 9, border: '1px solid rgba(168,206,255,.2)', background: 'linear-gradient(145deg, rgba(79,139,255,.14), rgba(3,8,20,.4))', padding: 10, boxSizing: 'border-box' }}>
          <div style={{ width: '72%', height: 5, borderRadius: 4, background: 'rgba(190,220,255,.5)' }} />
          <div style={{ width: '94%', height: 4, borderRadius: 4, background: 'rgba(150,183,224,.28)', marginTop: 13 }} />
          <div style={{ width: '84%', height: 4, borderRadius: 4, background: 'rgba(150,183,224,.24)', marginTop: 8 }} />
          <div style={{ width: '91%', height: 4, borderRadius: 4, background: 'rgba(150,183,224,.22)', marginTop: 8 }} />
          <div style={{ width: '62%', height: 4, borderRadius: 4, background: 'rgba(150,183,224,.18)', marginTop: 8 }} />
        </div>
        <div style={{ width: 86, height: 134, borderRadius: 9, border: '1px solid rgba(168,206,255,.2)', background: 'linear-gradient(160deg, rgba(34,211,238,.16), rgba(6,14,28,.6))', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
          <div style={{ width: 52, height: 52, borderRadius: '50%', border: `1px solid ${accent}80`, background: `radial-gradient(circle at 34% 28%, ${accent}55, rgba(4,10,21,.3))` }} />
        </div>
      </div>
      <div style={{ position: 'relative', display: 'flex', gap: 7, marginTop: 18 }}>
        {['多文本', '多图片', 'OCR'].map((label) => (
          <span key={label} style={{ padding: '4px 7px', borderRadius: 5, background: 'rgba(79,139,255,.16)', border: '1px solid rgba(141,188,255,.2)', color: '#bbd6f7', fontFamily: FONT, fontSize: 9 }}>{label}</span>
        ))}
      </div>
      {stamp && (
        <div
          style={{
            position: 'absolute',
            right: 18,
            bottom: 22,
            transform: `rotate(-12deg) scale(${1 + glow * 0.08})`,
            padding: '5px 12px',
            border: `2px solid ${accent}`,
            borderRadius: 5,
            color: accent,
            fontFamily: FONT,
            fontSize: 13,
            fontWeight: 800,
            letterSpacing: 3,
            boxShadow: `0 0 20px ${accent}55`,
          }}
        >
          {stamp}
        </div>
      )}
    </div>
  </div>
)

const OrbitCard: React.FC<{
  x: number
  y: number
  z: number
  rotateY: number
  label: string
  sub: string
  color: string
}> = ({ x, y, z, rotateY, label, sub, color }) => (
  <div
    style={{
      position: 'absolute',
      left: '50%',
      top: '50%',
      width: 205,
      marginLeft: -102,
      marginTop: -38,
      transform: `translate3d(${x}px, ${y}px, ${z}px) rotateY(${rotateY}deg)`,
      transformStyle: 'preserve-3d',
    }}
  >
    <Glass style={{ padding: '15px 17px', borderRadius: 15, borderColor: `${color}66`, boxShadow: `0 22px 50px -20px #000, 0 0 28px ${color}22` }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
        <span style={{ width: 7, height: 7, borderRadius: '50%', background: color, boxShadow: `0 0 14px ${color}` }} />
        <span style={{ color, fontFamily: MONO, fontSize: 8, letterSpacing: 1.3 }}>{sub}</span>
      </div>
      <div style={{ marginTop: 8, color: C.ink, fontFamily: FONT, fontSize: 15, fontWeight: 700 }}>{label}</div>
      <div style={{ marginTop: 9, width: '82%', height: 3, borderRadius: 3, background: `${color}70` }} />
      <div style={{ marginTop: 6, width: '58%', height: 3, borderRadius: 3, background: 'rgba(180,211,249,.22)' }} />
    </Glass>
  </div>
)

const Kicker: React.FC<{ children: React.ReactNode; color?: string; center?: boolean }> = ({ children, color = C.cyan, center }) => (
  <div style={{ display: 'flex', alignItems: 'center', justifyContent: center ? 'center' : 'flex-start', gap: 12, fontFamily: MONO, fontSize: 14, letterSpacing: 4, color }}>
    <span style={{ width: 40, height: 2, background: color, boxShadow: `0 0 16px ${color}` }} />
    {children}
  </div>
)

const IntroScene: React.FC = () => {
  const frame = useCurrentFrame()
  const { fps } = useVideoConfig()
  const titleIn = spring({ frame, fps, config: { damping: 16, stiffness: 110 } })
  const subIn = appear(frame, 18, 22)
  const pulseValue = beatPulse(frame)
  return (
    <AbsoluteFill style={{ perspective: 1500, transformStyle: 'preserve-3d' }}>
      <DocumentCard x={-490} y={-40} z={-120} rotateY={24} rotateZ={-9} scale={0.78} opacity={0.45 * appear(frame, 0, 20)} />
      <DocumentCard x={490} y={40} z={-100} rotateY={-24} rotateZ={10} scale={0.78} opacity={0.45 * appear(frame, 0, 20)} />
      <DocumentCard x={-300} y={200} z={-270} rotateY={16} rotateZ={-4} scale={0.58} opacity={0.2 * appear(frame, 0, 20)} />
      <DocumentCard x={330} y={230} z={-300} rotateY={-17} rotateZ={3} scale={0.54} opacity={0.18 * appear(frame, 0, 20)} />
      <div
        style={{
          position: 'absolute',
          left: 960,
          top: 220,
          width: 720,
          textAlign: 'center',
          transform: `translate(-50%, 0) translateY(${(1 - titleIn) * 36}px) scale(${0.9 + titleIn * 0.1 + pulseValue * 0.012})`,
          opacity: titleIn,
        }}
      >
        <Kicker color={C.cyan} center>SMARTDOC VISION / 2026</Kicker>
        <div style={{ marginTop: 20, fontFamily: FONT, fontSize: 77, lineHeight: 1.12, fontWeight: 800, letterSpacing: 2, color: C.ink, textShadow: '0 0 60px rgba(79,139,255,.38)' }}>
          让每一份图档
          <br />
          <span style={{ background: 'linear-gradient(90deg, #8fdcff, #4f8bff 50%, #a78bfa)', WebkitBackgroundClip: 'text', color: 'transparent' }}>都经过可信的智能审核</span>
        </div>
        <div style={{ marginTop: 20, opacity: subIn, transform: `translateY(${(1 - subIn) * 15}px)`, color: C.text, fontFamily: FONT, fontSize: 19, letterSpacing: 1 }}>多模型 · 多模态 · 可追溯 · 可复核</div>
      </div>
      <div style={{ position: 'absolute', left: 960, top: 700, transform: 'translateX(-50%)', opacity: appear(frame, 32, 20) }}>
        <LogoMark size={74} glow={1 + pulseValue * 0.25} />
      </div>
      <div style={{ position: 'absolute', left: 110, bottom: 72, color: C.muted, fontFamily: MONO, fontSize: 10, letterSpacing: 2 }}>DOCUMENT INTELLIGENCE / AUDIT INFRASTRUCTURE</div>
    </AbsoluteFill>
  )
}

const EngineScene: React.FC = () => {
  const frame = useCurrentFrame()
  const { fps } = useVideoConfig()
  const enter = spring({ frame, fps, config: { damping: 18, stiffness: 90 } })
  const ringPulse = 1 + beatPulse(frame) * 0.035
  const corePulse = 1 + beatPulse(frame, 4) * 0.06
  const orbit = frame * 0.009
  const verdicts = [
    { label: 'PASS', color: C.green, x: 300, y: -240, delay: 74 },
    { label: 'REVIEW', color: C.amber, x: -330, y: -210, delay: 88 },
    { label: 'REJECT', color: C.red, x: 250, y: 190, delay: 102 },
  ]
  return (
    <AbsoluteFill style={{ perspective: 1400, transformStyle: 'preserve-3d' }}>
      <div style={{ position: 'absolute', left: 110, top: 72, opacity: enter }}>
        <Kicker color={C.blue}>01 / MULTI-MODEL ENGINE</Kicker>
        <div style={{ marginTop: 16, color: C.ink, fontFamily: FONT, fontSize: 49, fontWeight: 800, letterSpacing: 2 }}>多模型协同裁决</div>
        <div style={{ marginTop: 8, color: C.muted, fontFamily: FONT, fontSize: 17 }}>通用大模型理解语义，JEV 输出结构化概率，规则与人工兜底</div>
      </div>
      <div style={{ position: 'absolute', left: 960, top: 510, transform: `translate(-50%, -50%) scale(${enter})` }}>
        <div style={{ position: 'relative', width: 470, height: 470, transform: 'translateZ(60px)' }}>
          {[0, 1, 2].map((index) => (
            <div
              key={index}
              style={{
                position: 'absolute',
                inset: 30 + index * 45,
                borderRadius: '50%',
                border: `1px solid ${[C.cyan, C.blue, C.purple][index]}55`,
                transform: `rotateX(67deg) rotateZ(${orbit * (index % 2 === 0 ? 1 : -1) * 80 + index * 40}deg) scale(${ringPulse})`,
                boxShadow: `0 0 24px ${[C.cyan, C.blue, C.purple][index]}22`,
              }}
            />
          ))}
          <div
            style={{
              position: 'absolute',
              left: '50%',
              top: '50%',
              width: 260,
              height: 260,
              transform: `translate(-50%, -50%) translateZ(120px) scale(${corePulse})`,
              borderRadius: '50%',
              background: 'radial-gradient(circle at 36% 28%, rgba(195,240,255,.95), rgba(79,139,255,.45) 28%, rgba(8,20,44,.9) 72%)',
              border: '1px solid rgba(179,227,255,.65)',
              boxShadow: '0 0 95px rgba(34,211,238,.5), inset 0 0 45px rgba(34,211,238,.26)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              flexDirection: 'column',
            }}
          >
            <div style={{ color: C.ink, fontFamily: FONT, fontSize: 23, fontWeight: 800, letterSpacing: 2 }}>审核引擎</div>
            <div style={{ marginTop: 6, color: '#c4ecff', fontFamily: MONO, fontSize: 10, letterSpacing: 2 }}>RISK · RULE · REVIEW</div>
          </div>
          <OrbitCard x={Math.cos(orbit) * 410} y={Math.sin(orbit) * 125 - 90} z={Math.sin(orbit) * 120} rotateY={-18 + Math.sin(orbit) * 8} label="通用大模型" sub="OPENAI / ANTHROPIC" color={C.cyan} />
          <OrbitCard x={Math.cos(orbit + 2.1) * 400} y={Math.sin(orbit + 2.1) * 120 + 100} z={Math.cos(orbit + 2.1) * 120} rotateY={15 - Math.cos(orbit) * 8} label="JEV 决策模型" sub="STRUCTURED CHOICE" color={C.purple} />
          <OrbitCard x={Math.cos(orbit + 4.2) * 380} y={Math.sin(orbit + 4.2) * 110 - 120} z={Math.sin(orbit + 4.2) * 130} rotateY={-10} label="规则引擎" sub="PRIORITY / STOP ON HIT" color={C.green} />
        </div>
      </div>
      <DocumentCard x={-620} y={170} z={-120} rotateY={24} scale={0.72} opacity={0.7 * appear(frame, 12, 18)} title="合同扫描件" accent={C.cyan} />
      <DocumentCard x={620} y={-160} z={-100} rotateY={-24} scale={0.68} opacity={0.66 * appear(frame, 18, 18)} title="工程图纸" accent={C.blue} />
      {verdicts.map((verdict) => {
        const progress = spring({ frame: Math.max(0, frame - verdict.delay), fps, config: { damping: 14, stiffness: 120 } })
        return (
          <div
            key={verdict.label}
            style={{
              position: 'absolute',
              left: 960 + verdict.x,
              top: 505 + verdict.y,
              transform: `translate(-50%, -50%) scale(${progress}) rotateZ(${verdict.x > 0 ? 5 : -5}deg)`,
              opacity: progress,
              padding: '10px 18px',
              border: `1px solid ${verdict.color}99`,
              borderRadius: 10,
              background: `linear-gradient(135deg, ${verdict.color}26, rgba(4,12,25,.85))`,
              boxShadow: `0 0 34px ${verdict.color}44`,
              color: verdict.color,
              fontFamily: MONO,
              fontSize: 17,
              fontWeight: 800,
              letterSpacing: 3,
            }}
          >
            {verdict.label}
          </div>
        )
      })}
    </AbsoluteFill>
  )
}

const TriggerScene: React.FC = () => {
  const frame = useCurrentFrame()
  const { fps } = useVideoConfig()
  const enter = spring({ frame, fps, config: { damping: 18, stiffness: 90 } })
  const panels = [
    { label: 'OPEN API', sub: 'HMAC · 防重放 · 限流', color: C.cyan, code: 'POST /open/v1/...' },
    { label: 'REDIS STREAM', sub: '消费组 · ACK · DLQ', color: C.amber, code: 'XADD stream * ...' },
    { label: 'WEBSOCKET', sub: '长连接 · 实时结果', color: C.purple, code: 'WS /open/v1/ws/...' },
  ]
  return (
    <AbsoluteFill style={{ perspective: 1500 }}>
      <div style={{ position: 'absolute', left: 110, top: 72, opacity: enter }}>
        <Kicker color={C.amber}>02 / ONE REQUEST ID</Kicker>
        <div style={{ marginTop: 16, color: C.ink, fontFamily: FONT, fontSize: 49, fontWeight: 800, letterSpacing: 2 }}>三种接入，一条链路</div>
        <div style={{ marginTop: 8, color: C.muted, fontFamily: FONT, fontSize: 17 }}>开放 API、Redis Stream、WebSocket；同一个 request_id 贯穿提交、幂等与回调</div>
      </div>
      <div style={{ position: 'absolute', left: 960, top: 180, transform: 'translateX(-50%)', display: 'flex', gap: 20 }}>
        {panels.map((panel, index) => {
          const itemEnter = spring({ frame: Math.max(0, frame - 10 - index * 8), fps, config: { damping: 18, stiffness: 100 } })
          return (
            <div key={panel.label} style={{ width: 430, height: 190, transform: `translateY(${(1 - itemEnter) * 35}px) translateZ(${index === 1 ? 50 : 0}px) rotateY(${index === 0 ? 17 : index === 2 ? -17 : 0}deg)`, opacity: itemEnter }}>
              <Glass style={{ height: '100%', padding: '24px 24px', boxSizing: 'border-box', borderColor: `${panel.color}66` }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 9 }}>
                  <span style={{ width: 9, height: 9, borderRadius: '50%', background: panel.color, boxShadow: `0 0 16px ${panel.color}` }} />
                  <span style={{ color: panel.color, fontFamily: MONO, fontSize: 12, letterSpacing: 2 }}>{panel.label}</span>
                  <span style={{ marginLeft: 'auto', color: C.muted, fontFamily: MONO, fontSize: 9 }}>0{index + 1}</span>
                </div>
                <div style={{ marginTop: 22, color: C.ink, fontFamily: FONT, fontSize: 23, fontWeight: 750 }}>{panel.sub}</div>
                <div style={{ marginTop: 20, padding: '9px 11px', borderRadius: 8, background: 'rgba(2,8,18,.46)', border: '1px solid rgba(143,188,255,.16)', color: '#9fd9ff', fontFamily: MONO, fontSize: 11 }}>{panel.code}</div>
              </Glass>
            </div>
          )
        })}
      </div>
      <div style={{ position: 'absolute', left: 960, top: 470, width: 980, height: 2, transform: 'translateX(-50%)', background: 'linear-gradient(90deg, transparent, rgba(34,211,238,.5), rgba(79,139,255,.8), rgba(167,139,250,.5), transparent)', boxShadow: '0 0 22px rgba(34,211,238,.45)' }} />
      {[0, 1, 2].map((index) => {
        const progress = (((frame - 38 - index * 10) % 70) + 70) % 70 / 70
        const startX = 960 + (index - 1) * 555
        const x = lerp(startX, 960, progress)
        return (
          <div
            key={index}
            style={{
              position: 'absolute', left: x, top: 450 + Math.sin(progress * Math.PI) * -90,
              transform: `translate(-50%, -50%) scale(${0.8 + Math.sin(progress * Math.PI) * 0.25})`,
              opacity: Math.sin(progress * Math.PI) * 0.85,
              padding: '6px 10px', borderRadius: 999,
              border: '1px solid rgba(160,225,255,.55)', background: 'rgba(8,24,46,.86)',
              color: '#bceaff', fontFamily: MONO, fontSize: 10, whiteSpace: 'nowrap',
              boxShadow: '0 0 20px rgba(34,211,238,.35)',
            }}
          >
            request_id · 01HX...
          </div>
        )
      })}
      <Glass style={{ position: 'absolute', left: 960, top: 610, width: 660, transform: `translateX(-50%) translateY(${(1 - enter) * 24}px)`, padding: '19px 24px', borderColor: 'rgba(79,139,255,.48)' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
          <span style={{ width: 10, height: 10, borderRadius: '50%', background: C.cyan, boxShadow: `0 0 18px ${C.cyan}` }} />
          <span style={{ color: C.ink, fontFamily: MONO, fontSize: 13, letterSpacing: 1 }}>SDV202610310001</span>
          <span style={{ marginLeft: 'auto', padding: '4px 8px', borderRadius: 5, background: 'rgba(251,191,36,.14)', border: '1px solid rgba(251,191,36,.38)', color: C.amber, fontFamily: FONT, fontSize: 10 }}>同一 request_id 重复提交只返回原任务</span>
        </div>
        <div style={{ marginTop: 13, display: 'grid', gridTemplateColumns: '1fr 1fr 1fr', gap: 10 }}>
          {['接收任务', '去重 / 幂等', '审核完成回调'].map((item, index) => (
            <div key={item} style={{ padding: '9px 10px', borderRadius: 8, background: 'rgba(34,211,238,.07)', border: '1px solid rgba(34,211,238,.18)' }}>
              <div style={{ color: C.muted, fontFamily: MONO, fontSize: 8 }}>STEP 0{index + 1}</div>
              <div style={{ marginTop: 4, color: C.text, fontFamily: FONT, fontSize: 11 }}>{item}</div>
            </div>
          ))}
        </div>
      </Glass>
    </AbsoluteFill>
  )
}

const ReviewScene: React.FC = () => {
  const frame = useCurrentFrame()
  const { fps } = useVideoConfig()
  const enter = spring({ frame, fps, config: { damping: 18, stiffness: 92 } })
  const approve = spring({ frame: Math.max(0, frame - 112), fps, config: { damping: 15, stiffness: 110 } })
  const returned = frame >= 112
  return (
    <AbsoluteFill style={{ perspective: 1500 }}>
      <div style={{ position: 'absolute', left: 110, top: 72, opacity: enter }}>
        <Kicker color={C.green}>03 / HUMAN IN THE LOOP</Kicker>
        <div style={{ marginTop: 16, color: C.ink, fontFamily: FONT, fontSize: 49, fontWeight: 800, letterSpacing: 2 }}>规则与人工，最终闭环</div>
        <div style={{ marginTop: 8, color: C.muted, fontFamily: FONT, fontSize: 17 }}>模型给出建议，人工确认结论，结果动作自动回调业务系统</div>
      </div>
      <div style={{ position: 'absolute', left: 960, top: 560, transform: 'translate(-50%, -50%)' }}>
        <div style={{ position: 'relative', width: 720, height: 450 }}>
          {[2, 1].map((item) => (
            <Glass key={item} style={{ position: 'absolute', left: 50 + item * 22, top: 24 + item * 20, width: 640, height: 360, opacity: 0.2 + (2 - item) * 0.12, transform: `translateZ(${-item * 50}px) rotateZ(${item * 1.2}deg)` }} />
          ))}
          <Glass
            style={{
              position: 'absolute', left: 0, top: 0, width: 680, height: 390, padding: '24px 26px', boxSizing: 'border-box',
              transform: `scale(${enter}) translateZ(80px)`,
              borderColor: returned ? 'rgba(52,211,153,.62)' : 'rgba(251,191,36,.52)',
              boxShadow: returned ? '0 0 60px rgba(52,211,153,.12), 0 40px 80px -30px #000' : '0 0 60px rgba(251,191,36,.1), 0 40px 80px -30px #000',
            }}
          >
            <div style={{ display: 'flex', alignItems: 'center', gap: 11 }}>
              <span style={{ width: 10, height: 10, borderRadius: '50%', background: returned ? C.green : C.amber, boxShadow: `0 0 18px ${returned ? C.green : C.amber}` }} />
              <span style={{ color: returned ? C.green : C.amber, fontFamily: MONO, fontSize: 11, letterSpacing: 1.5 }}>{returned ? 'APPROVED / CALLBACK SENT' : 'PENDING HUMAN REVIEW'}</span>
              <span style={{ marginLeft: 'auto', color: C.muted, fontFamily: MONO, fontSize: 10 }}>SDV202610310001</span>
            </div>
            <div style={{ marginTop: 19, display: 'flex', gap: 18 }}>
              <div style={{ flex: 1 }}>
                <div style={{ color: C.ink, fontFamily: FONT, fontSize: 23, fontWeight: 750 }}>建设工程档案 · 施工合同</div>
                <div style={{ marginTop: 6, color: C.muted, fontFamily: FONT, fontSize: 11 }}>档案图档通用合规审核 · 合同扫描件 · 2 张图片 / 4 段文本</div>
                <div style={{ marginTop: 22, display: 'flex', gap: 14 }}>
                  {[{ label: '风险分', value: '55', color: C.amber }, { label: '规则命中', value: '1', color: C.amber }, { label: '等待时长', value: '6h', color: C.cyan }].map((metric) => (
                    <div key={metric.label} style={{ minWidth: 110, padding: '10px 12px', borderRadius: 9, background: 'rgba(7,18,38,.62)', border: '1px solid rgba(148,187,255,.16)' }}>
                      <div style={{ color: C.muted, fontFamily: FONT, fontSize: 9 }}>{metric.label}</div>
                      <div style={{ marginTop: 3, color: metric.color, fontFamily: MONO, fontSize: 20, fontWeight: 800 }}>{metric.value}</div>
                    </div>
                  ))}
                </div>
              </div>
              <div style={{ width: 190, padding: 14, borderRadius: 10, background: 'rgba(251,191,36,.08)', border: '1px solid rgba(251,191,36,.28)', color: '#ffe0a3', fontFamily: FONT, fontSize: 10, lineHeight: 1.7 }}>
                <div style={{ color: C.amber, fontWeight: 700, marginBottom: 6 }}>模型建议</div>
                文档存在编码异常，关键要素无法辨认，建议人工复核原始文件。
              </div>
            </div>
            <div style={{ marginTop: 23, display: 'flex', gap: 10 }}>
              <div style={{ padding: '10px 15px', borderRadius: 8, border: '1px solid rgba(251,191,36,.4)', background: 'rgba(251,191,36,.13)', color: C.amber, fontFamily: FONT, fontSize: 12 }}>不通过</div>
              <div style={{ padding: '10px 15px', borderRadius: 8, border: `1px solid ${returned ? 'rgba(52,211,153,.6)' : 'rgba(148,187,255,.3)'}`, background: returned ? 'rgba(52,211,153,.16)' : 'rgba(79,139,255,.12)', color: returned ? C.green : '#b9d7ff', fontFamily: FONT, fontSize: 12, transform: `scale(${returned ? 1 + approve * 0.03 : 1})`, boxShadow: returned ? `0 0 ${24 * approve}px rgba(52,211,153,.3)` : 'none' }}>
                {returned ? '已通过并回调' : '通过并回调'}
              </div>
              <span style={{ marginLeft: 'auto', alignSelf: 'center', color: C.muted, fontFamily: MONO, fontSize: 9 }}>规则命中 → 人工复核 → 结果动作</span>
            </div>
          </Glass>
        </div>
      </div>
      <div style={{ position: 'absolute', left: 110, bottom: 92, display: 'flex', gap: 12 }}>
        {['HTTP CALLBACK', 'REDIS ACTION', 'WEBHOOK'].map((item, index) => (
          <div key={item} style={{ opacity: appear(frame, 84 + index * 7, 16), transform: `translateY(${(1 - appear(frame, 84 + index * 7, 16)) * 12}px)`, padding: '7px 11px', borderRadius: 6, border: '1px solid rgba(52,211,153,.28)', background: 'rgba(52,211,153,.07)', color: '#9cebc4', fontFamily: MONO, fontSize: 10 }}>{item}</div>
        ))}
      </div>
    </AbsoluteFill>
  )
}

const ScreenshotPanel: React.FC<{ src: string; x: number; y: number; z?: number; rotateY: number; rotateX?: number; width: number; opacity?: number; label: string }> = ({ src, x, y, z = 0, rotateY, rotateX = 0, width, opacity = 1, label }) => (
  <div style={{ position: 'absolute', left: '50%', top: '50%', width, transform: `translate(-50%, -50%) translate3d(${x}px, ${y}px, ${z}px) rotateX(${rotateX}deg) rotateY(${rotateY}deg)`, opacity, transformStyle: 'preserve-3d' }}>
    <div style={{ overflow: 'hidden', border: '1px solid rgba(173,213,255,.34)', borderRadius: 18, background: '#071326', boxShadow: '0 35px 75px -25px rgba(0,0,0,.82), 0 0 40px rgba(34,211,238,.08)' }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 6, height: 28, padding: '0 12px', background: 'rgba(10,26,48,.96)', borderBottom: '1px solid rgba(144,188,255,.14)' }}>
        <span style={{ width: 6, height: 6, borderRadius: '50%', background: C.red }} />
        <span style={{ width: 6, height: 6, borderRadius: '50%', background: C.amber }} />
        <span style={{ width: 6, height: 6, borderRadius: '50%', background: C.green }} />
        <span style={{ marginLeft: 8, color: C.muted, fontFamily: MONO, fontSize: 8, letterSpacing: 1 }}>{label}</span>
      </div>
      <Img src={staticFile(src)} style={{ display: 'block', width: '100%' }} />
      <div style={{ position: 'absolute', inset: 0, background: 'linear-gradient(120deg, rgba(255,255,255,.1), transparent 26%, transparent 72%, rgba(79,139,255,.08))', pointerEvents: 'none' }} />
    </div>
  </div>
)

const PlatformScene: React.FC = () => {
  const frame = useCurrentFrame()
  const { fps } = useVideoConfig()
  const enter = spring({ frame, fps, config: { damping: 17, stiffness: 90 } })
  const drift = Math.sin(frame * 0.018) * 16
  const features = ['审核配置助手', 'AI 日志查询', 'JS 结果动作', '审核条约', '站点品牌', '数据库升级门禁', '外置文件存储', 'RBAC / 审计']
  return (
    <AbsoluteFill style={{ perspective: 1600 }}>
      <div style={{ position: 'absolute', left: 110, top: 72, opacity: enter }}>
        <Kicker color={C.purple}>04 / ONE CONTROL PLANE</Kicker>
        <div style={{ marginTop: 16, color: C.ink, fontFamily: FONT, fontSize: 49, fontWeight: 800, letterSpacing: 2 }}>不止审核，更是审核中台</div>
        <div style={{ marginTop: 8, color: C.muted, fontFamily: FONT, fontSize: 17 }}>配置、接入、模型、AI 助手、品牌、存储与安全治理，在一个后台闭环</div>
      </div>
      <div style={{ position: 'absolute', inset: 0, transform: `translateY(${drift}px)` }}>
        <ScreenshotPanel src="screens/audits.png" label="AUDIT CONFIGURATION" x={-640} y={40} z={-80} rotateY={24} rotateX={2} width={620} opacity={0.76 * enter} />
        <ScreenshotPanel src="screens/dashboard.png" label="DASHBOARD" x={0} y={-15} z={80} rotateY={0} width={820} opacity={enter} />
        <ScreenshotPanel src="screens/script-editor.png" label="JS RESULT ACTION" x={640} y={55} z={-80} rotateY={-24} rotateX={2} width={620} opacity={0.76 * enter} />
        <ScreenshotPanel src="screens/providers.png" label="MODEL PROVIDERS" x={-370} y={310} z={-180} rotateY={12} rotateX={8} width={420} opacity={0.34 * enter} />
        <ScreenshotPanel src="screens/dashboard.png" label="LIVE OPERATIONS" x={390} y={320} z={-200} rotateY={-12} rotateX={8} width={420} opacity={0.24 * enter} />
      </div>
      <div style={{ position: 'absolute', left: 110, bottom: 76, display: 'flex', flexWrap: 'wrap', gap: 9, width: 1160 }}>
        {features.map((feature, index) => {
          const itemIn = appear(frame, 26 + index * 4, 16)
          return <div key={feature} style={{ padding: '7px 11px', borderRadius: 6, border: '1px solid rgba(167,139,250,.26)', background: 'rgba(124,58,237,.1)', color: '#d8c9ff', fontFamily: FONT, fontSize: 11, opacity: itemIn, transform: `translateY(${(1 - itemIn) * 10}px)` }}>{feature}</div>
        })}
      </div>
      <div style={{ position: 'absolute', right: 110, bottom: 76, width: 360, padding: '16px 18px', borderRadius: 14, border: '1px solid rgba(34,211,238,.3)', background: 'rgba(5,18,32,.8)', boxShadow: '0 25px 60px -24px #000', opacity: appear(frame, 38, 18) }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 9 }}>
          <span style={{ width: 7, height: 7, borderRadius: '50%', background: C.cyan, boxShadow: `0 0 14px ${C.cyan}` }} />
          <span style={{ color: C.cyan, fontFamily: MONO, fontSize: 9, letterSpacing: 1.5 }}>AI COPILOT / TOOL CALLS</span>
        </div>
        <div style={{ marginTop: 10, color: C.ink, fontFamily: FONT, fontSize: 14, fontWeight: 700 }}>日志查询 · 上下文自动压缩</div>
        <div style={{ marginTop: 8, height: 4, borderRadius: 4, background: 'rgba(79,139,255,.18)', overflow: 'hidden' }}>
          <div style={{ width: `${38 + Math.sin(frame * 0.04) * 18}%`, height: '100%', borderRadius: 4, background: 'linear-gradient(90deg, #22d3ee, #4f8bff)' }} />
        </div>
        <div style={{ marginTop: 8, color: C.muted, fontFamily: MONO, fontSize: 8 }}>TOOL CALL · 统计分析 → 明细查询 → 应用筛选</div>
      </div>
    </AbsoluteFill>
  )
}

const OutroScene: React.FC = () => {
  const frame = useCurrentFrame()
  const { fps } = useVideoConfig()
  const enter = spring({ frame, fps, config: { damping: 16, stiffness: 90 } })
  const glow = 1 + beatPulse(frame) * 0.22
  return (
    <AbsoluteFill style={{ alignItems: 'center', justifyContent: 'center', perspective: 1200 }}>
      <div style={{ position: 'absolute', width: 700, height: 700, borderRadius: '50%', border: '1px solid rgba(79,139,255,.22)', transform: `scale(${1.1 + Math.sin(frame * 0.03) * 0.04}) rotateZ(${frame * 0.22}deg)` }} />
      <div style={{ position: 'absolute', width: 520, height: 520, borderRadius: '50%', border: '1px solid rgba(34,211,238,.18)', transform: `scale(${1.2 + Math.cos(frame * 0.03) * 0.04}) rotateZ(${-frame * 0.3}deg)` }} />
      <div style={{ display: 'flex', alignItems: 'center', flexDirection: 'column', transform: `scale(${0.86 + enter * 0.14})`, opacity: enter }}>
        <LogoMark size={96} glow={glow} />
        <div style={{ marginTop: 24, color: C.ink, fontFamily: FONT, fontSize: 64, fontWeight: 800, letterSpacing: 7, textShadow: '0 0 60px rgba(79,139,255,.45)' }}>智审图档</div>
        <div style={{ marginTop: 8, color: C.cyan, fontFamily: MONO, fontSize: 12, letterSpacing: 5 }}>SMARTDOC VISION</div>
        <div style={{ marginTop: 22, color: C.text, fontFamily: FONT, fontSize: 16, letterSpacing: 1.5 }}>让审核可追溯、可复用、可进化</div>
        <div style={{ marginTop: 25, display: 'flex', gap: 13 }}>
          {['OPENAPI', 'REDIS STREAM', 'WEBSOCKET', 'MIT'].map((item) => (
            <span key={item} style={{ padding: '7px 11px', borderRadius: 5, border: '1px solid rgba(148,187,255,.22)', background: 'rgba(79,139,255,.08)', color: '#a9c9f4', fontFamily: MONO, fontSize: 9, letterSpacing: 1 }}>{item}</span>
          ))}
        </div>
      </div>
      <div style={{ position: 'absolute', bottom: 45, color: '#5f7188', fontFamily: MONO, fontSize: 9, letterSpacing: 2 }}>SELF-HOSTED · MULTI-MODAL · AUDIT INFRASTRUCTURE</div>
    </AbsoluteFill>
  )
}

const CutFlash: React.FC<{ at: number }> = ({ at }) => {
  const frame = useCurrentFrame()
  const local = frame - at
  if (local < 0 || local > 20) return null
  const energy = Math.exp(-local / 4)
  return <AbsoluteFill style={{ background: '#dff4ff', opacity: energy * 0.22, mixBlendMode: 'screen', pointerEvents: 'none' }} />
}

const Cinematic: React.FC = () => {
  const frame = useCurrentFrame()
  return (
    <AbsoluteFill style={{ background: C.bg, fontFamily: FONT }}>
      <Audio src={staticFile('audio/launch-beat.mp3')} volume={0.92} />
      <Backdrop frame={frame} />
      <Sequence from={0} durationInFrames={120}><IntroScene /></Sequence>
      <Sequence from={120} durationInFrames={240}><EngineScene /></Sequence>
      <Sequence from={360} durationInFrames={240}><TriggerScene /></Sequence>
      <Sequence from={600} durationInFrames={240}><ReviewScene /></Sequence>
      <Sequence from={840} durationInFrames={240}><PlatformScene /></Sequence>
      <Sequence from={1080} durationInFrames={120}><OutroScene /></Sequence>
      {[120, 360, 600, 840, 1080].map((cut) => <CutFlash key={cut} at={cut} />)}
      <div style={{ position: 'absolute', inset: 0, pointerEvents: 'none', background: 'linear-gradient(180deg, rgba(1,5,12,.32), transparent 16%, transparent 84%, rgba(1,5,12,.55))' }} />
    </AbsoluteFill>
  )
}

export const CinematicComposition: React.FC = () => <Cinematic />
