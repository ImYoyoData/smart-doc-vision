import React from 'react'
import {
  AbsoluteFill,
  Img,
  interpolate,
  Sequence,
  spring,
  staticFile,
  useCurrentFrame,
  useVideoConfig,
} from 'remotion'

const FONT = '"Microsoft YaHei", "PingFang SC", "Noto Sans SC", "Segoe UI", sans-serif'
const MONO = '"Cascadia Code", "JetBrains Mono", Consolas, monospace'
const BRAND = '#3b82f6'
const CYAN = '#22d3ee'
const INK = '#e2e8f0'
const MUTED = '#94a3b8'

const clamp = {
  extrapolateLeft: 'clamp' as const,
  extrapolateRight: 'clamp' as const,
}

const Fade: React.FC<{ duration: number; children: React.ReactNode }> = ({ duration, children }) => {
  const frame = useCurrentFrame()
  const opacity = interpolate(frame, [0, 12, duration - 12, duration], [0, 1, 1, 0], clamp)
  return <AbsoluteFill style={{ opacity }}>{children}</AbsoluteFill>
}

const Backdrop: React.FC = () => {
  const frame = useCurrentFrame()
  const drift = Math.sin(frame / 90) * 60
  return (
    <AbsoluteFill style={{ backgroundColor: '#070d1b' }}>
      <AbsoluteFill
        style={{
          backgroundImage:
            'linear-gradient(rgba(148,163,184,.07) 1px, transparent 1px), linear-gradient(90deg, rgba(148,163,184,.07) 1px, transparent 1px)',
          backgroundSize: '72px 72px',
          backgroundPosition: `${(frame * 0.25) % 72}px ${(frame * 0.15) % 72}px`,
          opacity: 0.75,
        }}
      />
      <div
        style={{
          position: 'absolute',
          left: -260 + drift,
          top: -320,
          width: 980,
          height: 980,
          borderRadius: '50%',
          background: 'radial-gradient(circle, rgba(37,99,235,.38), rgba(37,99,235,0) 65%)',
          filter: 'blur(8px)',
        }}
      />
      <div
        style={{
          position: 'absolute',
          right: -260 - drift * 0.6,
          bottom: -340,
          width: 900,
          height: 900,
          borderRadius: '50%',
          background: 'radial-gradient(circle, rgba(34,211,238,.22), rgba(34,211,238,0) 65%)',
          filter: 'blur(8px)',
        }}
      />
      <AbsoluteFill
        style={{
          background:
            'linear-gradient(180deg, rgba(7,13,27,0) 55%, rgba(7,13,27,.82) 100%)',
        }}
      />
    </AbsoluteFill>
  )
}

const LogoMark: React.FC<{ size?: number }> = ({ size = 72 }) => (
  <div
    style={{
      width: size,
      height: size,
      borderRadius: size * 0.28,
      background: 'linear-gradient(145deg, #60a5fa, #2563eb 52%, #0ea5e9)',
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      border: '1px solid rgba(191,219,254,.4)',
      boxShadow: '0 22px 60px rgba(37,99,235,.5)',
    }}
  >
    <svg width={size * 0.54} height={size * 0.54} viewBox="0 0 24 24" fill="none">
      <path d="M7 3.8h7l4 4v12.4H7z" stroke="#fff" strokeWidth="1.5" strokeLinejoin="round" />
      <path d="M14 3.8v4h4" stroke="#fff" strokeWidth="1.5" strokeLinejoin="round" />
      <path
        d="M9.4 13.4l1.9 1.9 3.5-3.7"
        stroke="#dbeafe"
        strokeWidth="1.6"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  </div>
)

const Chip: React.FC<{ children: React.ReactNode; tone?: 'brand' | 'cyan' | 'plain' }> = ({
  children,
  tone = 'plain',
}) => {
  const palette =
    tone === 'brand'
      ? { border: 'rgba(96,165,250,.45)', background: 'rgba(37,99,235,.16)', color: '#bfdbfe' }
      : tone === 'cyan'
        ? { border: 'rgba(34,211,238,.4)', background: 'rgba(8,145,178,.16)', color: '#a5f3fc' }
        : { border: 'rgba(148,163,184,.28)', background: 'rgba(148,163,184,.08)', color: MUTED }
  return (
    <span
      style={{
        padding: '8px 16px',
        borderRadius: 999,
        border: `1px solid ${palette.border}`,
        background: palette.background,
        color: palette.color,
        fontSize: 20,
        letterSpacing: 0.5,
        whiteSpace: 'nowrap',
      }}
    >
      {children}
    </span>
  )
}

const Intro: React.FC<{ duration: number }> = ({ duration }) => {
  const frame = useCurrentFrame()
  const { fps } = useVideoConfig()
  const logo = spring({ frame, fps, config: { damping: 14, mass: 0.7, stiffness: 90 } })
  const title = spring({ frame: frame - 10, fps, config: { damping: 18, stiffness: 80 } })
  const line = interpolate(frame, [34, 72], [0, 1], clamp)
  const pills = interpolate(frame, [52, 82], [0, 1], clamp)
  const tagline = interpolate(frame, [40, 76], [0, 1], clamp)

  const results = [
    { label: 'PASS', color: '#34d399', bg: 'rgba(16,185,129,.14)' },
    { label: 'REVIEW', color: '#fbbf24', bg: 'rgba(245,158,11,.14)' },
    { label: 'REJECT', color: '#f87171', bg: 'rgba(239,68,68,.14)' },
  ]

  return (
    <Fade duration={duration}>
      <AbsoluteFill style={{ alignItems: 'center', justifyContent: 'center' }}>
        <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center' }}>
          <div style={{ transform: `scale(${logo})`, opacity: logo }}>
            <LogoMark size={104} />
          </div>
          <div
            style={{
              marginTop: 34,
              color: '#93c5fd',
              fontFamily: MONO,
              fontSize: 20,
              letterSpacing: 10,
              opacity: interpolate(frame, [14, 44], [0, 1], clamp),
            }}
          >
            SMARTDOC VISION
          </div>
          <div
            style={{
              marginTop: 14,
              color: '#f8fafc',
              fontSize: 118,
              fontWeight: 700,
              letterSpacing: 14,
              opacity: title,
              transform: `translateY(${(1 - title) * 26}px)`,
            }}
          >
            智审图档
          </div>
          <div
            style={{
              marginTop: 8,
              color: INK,
              fontSize: 38,
              letterSpacing: 6,
              opacity: tagline,
            }}
          >
            多模态档案审核平台
          </div>
          <div
            style={{
              width: 620 * line,
              height: 2,
              marginTop: 30,
              background: `linear-gradient(90deg, transparent, ${BRAND}, ${CYAN}, transparent)`,
            }}
          />
          <div
            style={{
              marginTop: 26,
              color: MUTED,
              fontSize: 24,
              letterSpacing: 1.5,
              opacity: tagline,
            }}
          >
            从提交、判定到业务回调，一条流程跑完
          </div>
          <div
            style={{
              marginTop: 46,
              display: 'flex',
              gap: 16,
              opacity: pills,
              transform: `translateY(${(1 - pills) * 18}px)`,
            }}
          >
            {results.map((item) => (
              <span
                key={item.label}
                style={{
                  padding: '10px 22px',
                  borderRadius: 10,
                  border: `1px solid ${item.color}55`,
                  background: item.bg,
                  color: item.color,
                  fontFamily: MONO,
                  fontSize: 22,
                  letterSpacing: 2,
                }}
              >
                {item.label}
              </span>
            ))}
          </div>
        </div>
      </AbsoluteFill>
    </Fade>
  )
}

const icons: Record<string, React.ReactNode> = {
  shield: (
    <>
      <path d="M12 3l7 3v6c0 4.4-3 7.6-7 9-4-1.4-7-4.6-7-9V6l7-3z" />
      <path d="M9 12l2 2 4-4" />
    </>
  ),
  layers: (
    <>
      <path d="M12 3l8 4.5-8 4.5-8-4.5L12 3z" />
      <path d="M4 12l8 4.5 8-4.5" />
      <path d="M4 16.5L12 21l8-4.5" />
    </>
  ),
  bolt: <path d="M13 2L4 14h6l-1 8 9-12h-6l1-8z" />,
  chart: (
    <>
      <path d="M4 20V10M10 20V4M16 20v-7M22 20H2" />
    </>
  ),
}

const FeatureCard: React.FC<{
  icon: string
  title: string
  desc: string
  index: number
}> = ({ icon, title, desc, index }) => {
  const frame = useCurrentFrame()
  const { fps } = useVideoConfig()
  const enter = spring({
    frame: frame - 18 - index * 7,
    fps,
    config: { damping: 18, stiffness: 90 },
  })
  return (
    <div
      style={{
        width: 384,
        height: 300,
        padding: '28px 26px',
        borderRadius: 22,
        border: '1px solid rgba(148,163,184,.2)',
        background:
          'linear-gradient(160deg, rgba(30,41,59,.75), rgba(15,23,42,.82))',
        boxShadow: '0 24px 60px rgba(2,6,23,.35)',
        opacity: enter,
        transform: `translateY(${(1 - enter) * 44}px)`,
      }}
    >
      <div
        style={{
          width: 56,
          height: 56,
          borderRadius: 16,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          background: index % 2 === 0 ? 'rgba(37,99,235,.2)' : 'rgba(34,211,238,.16)',
          border: `1px solid ${index % 2 === 0 ? 'rgba(96,165,250,.35)' : 'rgba(34,211,238,.3)'}`,
          color: index % 2 === 0 ? '#93c5fd' : '#a5f3fc',
        }}
      >
        <svg
          width="28"
          height="28"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="1.6"
          strokeLinecap="round"
          strokeLinejoin="round"
        >
          {icons[icon]}
        </svg>
      </div>
      <div style={{ marginTop: 24, color: '#f1f5f9', fontSize: 27, fontWeight: 600 }}>
        {title}
      </div>
      <div style={{ marginTop: 12, color: MUTED, fontSize: 19, lineHeight: 1.65 }}>{desc}</div>
    </div>
  )
}

const Features: React.FC<{ duration: number }> = ({ duration }) => {
  const frame = useCurrentFrame()
  const head = interpolate(frame, [8, 34], [0, 1], clamp)
  return (
    <Fade duration={duration}>
      <AbsoluteFill style={{ padding: '118px 140px' }}>
        <div
          style={{
            color: '#60a5fa',
            fontFamily: MONO,
            fontSize: 18,
            letterSpacing: 7,
            opacity: head,
          }}
        >
          CAPABILITIES
        </div>
        <div
          style={{
            marginTop: 12,
            color: '#f8fafc',
            fontSize: 58,
            fontWeight: 700,
            letterSpacing: 3,
            opacity: head,
            transform: `translateY(${(1 - head) * 20}px)`,
          }}
        >
          把审核做成可复用的流程
        </div>
        <div style={{ marginTop: 12, color: MUTED, fontSize: 23, opacity: head }}>
          输入、判定、规则、动作、用量，全部在同一个后台编排
        </div>
        <div style={{ display: 'flex', gap: 24, marginTop: 62 }}>
          <FeatureCard
            icon="shield"
            title="多模态审核"
            desc="多文本 + 多图片同送模型，识别缺页、遮挡、敏感内容与图文不一致"
            index={0}
          />
          <FeatureCard
            icon="layers"
            title="规则化判定"
            desc="风险分、规则组与分类映射叠加，灰区自动转人工复核"
            index={1}
          />
          <FeatureCard
            icon="bolt"
            title="结果动作闭环"
            desc="按 PASS / REJECT / REVIEW 编排 HTTP、WebSocket、Redis 与 JS"
            index={2}
          />
          <FeatureCard
            icon="chart"
            title="用量可观测"
            desc="模型请求明细、缓存命中与 Token 统计，AI 上下文环形进度"
            index={3}
          />
        </div>
      </AbsoluteFill>
    </Fade>
  )
}

const BrowserFrame: React.FC<{
  image: string
  address: string
  scale: number
}> = ({ image, address, scale }) => (
  <div
    style={{
      width: 1320,
      borderRadius: 18,
      overflow: 'hidden',
      border: '1px solid rgba(148,163,184,.28)',
      background: '#0f172a',
      boxShadow: '0 50px 120px rgba(2,6,23,.7)',
      transform: `scale(${scale})`,
    }}
  >
    <div
      style={{
        height: 44,
        display: 'flex',
        alignItems: 'center',
        gap: 9,
        padding: '0 18px',
        background: 'linear-gradient(180deg, #16243c, #111c30)',
        borderBottom: '1px solid rgba(148,163,184,.16)',
      }}
    >
      {['#f87171', '#fbbf24', '#34d399'].map((color) => (
        <span
          key={color}
          style={{ width: 11, height: 11, borderRadius: '50%', background: color, opacity: 0.85 }}
        />
      ))}
      <div
        style={{
          marginLeft: 14,
          flex: 1,
          height: 24,
          borderRadius: 7,
          background: 'rgba(148,163,184,.12)',
          border: '1px solid rgba(148,163,184,.14)',
          color: '#7c8ba1',
          fontFamily: MONO,
          fontSize: 12,
          display: 'flex',
          alignItems: 'center',
          padding: '0 12px',
        }}
      >
        {address}
      </div>
      <div style={{ color: '#64748b', fontFamily: MONO, fontSize: 11 }}>SMARTDOC</div>
    </div>
    <Img src={image} style={{ display: 'block', width: 1320, height: 742.5 }} />
  </div>
)

const Showcase: React.FC<{
  duration: number
  image: string
  address: string
  eyebrow: string
  title: string
  desc: string
  points: string[]
  chips?: string[]
}> = ({ duration, image, address, eyebrow, title, desc, points, chips }) => {
  const frame = useCurrentFrame()
  const { fps } = useVideoConfig()
  const enter = spring({ frame, fps, config: { damping: 20, stiffness: 70 } })
  const zoom = interpolate(frame, [0, duration], [1.055, 1.0], clamp)
  const caption = interpolate(frame, [10, 38], [0, 1], clamp)

  return (
    <Fade duration={duration}>
      <AbsoluteFill>
        <div style={{ position: 'absolute', left: 142, top: 262, width: 320 }}>
          <div style={{ color: '#60a5fa', fontFamily: MONO, fontSize: 16, letterSpacing: 6, opacity: caption }}>
            {eyebrow}
          </div>
          <div
            style={{
              marginTop: 14,
              color: '#f8fafc',
              fontSize: 48,
              lineHeight: 1.22,
              fontWeight: 700,
              letterSpacing: 1,
              opacity: caption,
              transform: `translateY(${(1 - caption) * 20}px)`,
            }}
          >
            {title}
          </div>
          <div style={{ marginTop: 16, color: MUTED, fontSize: 19, lineHeight: 1.7, opacity: caption }}>
            {desc}
          </div>
          <div style={{ marginTop: 28, display: 'flex', flexDirection: 'column', gap: 14 }}>
            {points.map((point, index) => {
              const item = interpolate(frame, [30 + index * 8, 56 + index * 8], [0, 1], clamp)
              return (
                <div
                  key={point}
                  style={{
                    display: 'flex',
                    alignItems: 'flex-start',
                    gap: 10,
                    color: INK,
                    fontSize: 18,
                    lineHeight: 1.6,
                    opacity: item,
                    transform: `translateX(${(1 - item) * -16}px)`,
                  }}
                >
                  <span
                    style={{
                      width: 7,
                      height: 7,
                      marginTop: 10,
                      borderRadius: '50%',
                      background: CYAN,
                      boxShadow: `0 0 12px ${CYAN}`,
                      flexShrink: 0,
                    }}
                  />
                  {point}
                </div>
              )
            })}
          </div>
          {chips && chips.length > 0 ? (
            <div style={{ marginTop: 30, display: 'flex', flexWrap: 'wrap', gap: 10 }}>
              {chips.map((chip, index) => {
                const item = interpolate(frame, [58 + index * 7, 78 + index * 7], [0, 1], clamp)
                return (
                  <span key={chip} style={{ opacity: item }}>
                    <Chip tone={index === 0 ? 'brand' : index === 1 ? 'cyan' : 'plain'}>{chip}</Chip>
                  </span>
                )
              })}
            </div>
          ) : null}
        </div>
        <div
          style={{
            position: 'absolute',
            left: 512,
            top: 168,
            opacity: enter,
            transform: `translateY(${(1 - enter) * 34}px)`,
          }}
        >
          <BrowserFrame image={image} address={address} scale={zoom} />
        </div>
      </AbsoluteFill>
    </Fade>
  )
}

const Outro: React.FC<{ duration: number }> = ({ duration }) => {
  const frame = useCurrentFrame()
  const { fps } = useVideoConfig()
  const enter = spring({ frame, fps, config: { damping: 16, stiffness: 80 } })
  const fade = interpolate(frame, [8, 40], [0, 1], clamp)
  return (
    <Fade duration={duration}>
      <AbsoluteFill style={{ alignItems: 'center', justifyContent: 'center' }}>
        <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center' }}>
          <div style={{ transform: `scale(${enter})` }}>
            <LogoMark size={92} />
          </div>
          <div
            style={{
              marginTop: 30,
              color: '#f8fafc',
              fontSize: 42,
              fontWeight: 700,
              letterSpacing: 2,
              opacity: fade,
            }}
          >
            让每一份档案，都有据可审
          </div>
          <div
            style={{
              marginTop: 22,
              display: 'flex',
              gap: 12,
              opacity: interpolate(frame, [24, 52], [0, 1], clamp),
            }}
          >
            <Chip tone="brand">OpenAI 兼容</Chip>
            <Chip tone="cyan">Anthropic</Chip>
            <Chip>JEV 决策模型</Chip>
            <Chip>文本 + 图片</Chip>
          </div>
          <div
            style={{
              marginTop: 52,
              color: MUTED,
              fontFamily: MONO,
              fontSize: 16,
              letterSpacing: 4,
              opacity: interpolate(frame, [40, 66], [0, 1], clamp),
            }}
          >
            SMARTDOC VISION · v1.3.0
          </div>
          <div
            style={{
              marginTop: 10,
              color: '#64748b',
              fontFamily: MONO,
              fontSize: 15,
              letterSpacing: 1,
              opacity: interpolate(frame, [48, 72], [0, 1], clamp),
            }}
          >
            github.com/ImYoyoData/smart-doc-vision
          </div>
        </div>
      </AbsoluteFill>
    </Fade>
  )
}

const Watermark: React.FC = () => {
  const frame = useCurrentFrame()
  return (
    <div
      style={{
        position: 'absolute',
        top: 44,
        right: 56,
        display: 'flex',
        alignItems: 'center',
        gap: 10,
        opacity: interpolate(frame, [0, 30], [0, 0.55], clamp),
      }}
    >
      <div style={{ width: 7, height: 7, borderRadius: '50%', background: '#34d399', boxShadow: '0 0 10px #34d399' }} />
      <span style={{ color: '#64748b', fontFamily: MONO, fontSize: 13, letterSpacing: 3 }}>
        AUDIT ENGINE RUNNING
      </span>
    </div>
  )
}

const ProgressBar: React.FC = () => {
  const frame = useCurrentFrame()
  const { durationInFrames } = useVideoConfig()
  return (
    <div
      style={{
        position: 'absolute',
        left: 0,
        bottom: 0,
        height: 4,
        width: `${(frame / durationInFrames) * 100}%`,
        background: `linear-gradient(90deg, ${BRAND}, ${CYAN})`,
        boxShadow: `0 0 18px ${CYAN}`,
      }}
    />
  )
}

export const Promo: React.FC = () => {
  return (
    <AbsoluteFill style={{ fontFamily: FONT, backgroundColor: '#070d1b' }}>
      <Backdrop />
      <Watermark />

      <Sequence from={0} durationInFrames={120}>
        <Intro duration={120} />
      </Sequence>

      <Sequence from={120} durationInFrames={220}>
        <Features duration={220} />
      </Sequence>

      <Sequence from={340} durationInFrames={180}>
        <Showcase
          duration={180}
          image={staticFile('screens/dashboard.png')}
          address="smartdoc.local/dashboard"
          eyebrow="WORKBENCH"
          title="审核工作台"
          desc="今日任务、通过率、待复核与模型状态一屏掌握。"
          points={['今日处理与待人工复核实时汇总', '按项目与审核配置快速筛选', '最近任务直接进入复核或详情']}
        />
      </Sequence>

      <Sequence from={520} durationInFrames={180}>
        <Showcase
          duration={180}
          image={staticFile('screens/audits.png')}
          address="smartdoc.local/audits"
          eyebrow="AUDIT FLOWS"
          title="可编排的审核流程"
          desc="一条配置同时支持文本与图片，绑定模型、规则与接入方式。"
          points={['文本 + 图片多模态输入', '规则组决定 PASS / REJECT / REVIEW', '开放 API、Redis Stream、WebSocket 接入']}
        />
      </Sequence>

      <Sequence from={700} durationInFrames={240}>
        <Showcase
          duration={240}
          image={staticFile('screens/script-editor.png')}
          address="smartdoc.local/audits/js_callback_demo"
          eyebrow="RESULT ACTIONS"
          title="结果动作，用脚本闭环"
          desc="按最终结论执行 JS 脚本：查业务库、加签加密、回调第三方。"
          points={['全局 ctx，无需定义 main 函数', 'MD5 / HMAC / AES / DES 内置', '沙盒模拟运行，密钥按项目注入']}
          chips={['AI 读写编辑器', 'Web 检索', '沙盒运行', '项目密钥']}
        />
      </Sequence>

      <Sequence from={940} durationInFrames={160}>
        <Showcase
          duration={160}
          image={staticFile('screens/providers.png')}
          address="smartdoc.local/providers"
          eyebrow="MODEL GATEWAY"
          title="模型接入与用量"
          desc="请求明细、Token 统计与失败熔断，模型状态全程可见。"
          points={['输入 / 缓存命中 / 输出 Token 明细', 'JEV 只统计输入，输出免费', '失败自动切换，熔断后自动恢复']}
        />
      </Sequence>

      <Sequence from={1100} durationInFrames={100}>
        <Outro duration={100} />
      </Sequence>

      <ProgressBar />
    </AbsoluteFill>
  )
}
