// Synthesises a 120 BPM, 40-second electronic cue for the launch video.
// Everything is generated from oscillators and noise, so the track is original
// and license-free. Run: node scripts/make-beat.mjs
import { writeFileSync } from 'node:fs'
import path from 'node:path'

const SR = 44100
const BPM = 120
const BEAT = 60 / BPM
const BAR = BEAT * 4
const DURATION = 40
const TOTAL = Math.round(SR * DURATION)

let seed = 20261025
function rand() {
  seed = (seed * 1664525 + 1013904223) >>> 0
  return seed / 0xffffffff
}

const left = new Float32Array(TOTAL)
const right = new Float32Array(TOTAL)
const duck = new Float32Array(TOTAL).fill(1)

function addSample(index, value, pan = 0) {
  if (index < 0 || index >= TOTAL) return
  const l = Math.sqrt((1 - pan) / 2)
  const r = Math.sqrt((1 + pan) / 2)
  left[index] += value * l
  right[index] += value * r
}

function applyDuck(start, duration = 0.26, depth = 0.65) {
  const from = Math.round(start * SR)
  const to = Math.min(TOTAL, Math.round((start + duration) * SR))
  for (let i = from; i < to; i += 1) {
    const t = (i - from) / SR
    const value = 1 - depth * Math.exp(-t * 15)
    if (value < duck[i]) duck[i] = value
  }
}

function kick(t0, gain = 1) {
  const start = Math.round(t0 * SR)
  const length = Math.round(0.42 * SR)
  let phase = 0
  for (let i = 0; i < length; i += 1) {
    const t = i / SR
    const freq = 42 + 150 * Math.exp(-t * 22)
    phase += (2 * Math.PI * freq) / SR
    const env = Math.exp(-t * 7.5)
    const body = Math.sin(phase) * env * 0.95
    const click = t < 0.012 ? (rand() * 2 - 1) * Math.exp(-t * 320) * 0.45 : 0
    addSample(start + i, (body + click) * gain)
  }
  applyDuck(t0)
}

function snare(t0, gain = 1) {
  const start = Math.round(t0 * SR)
  const length = Math.round(0.24 * SR)
  let prev = 0
  for (let i = 0; i < length; i += 1) {
    const t = i / SR
    const noise = rand() * 2 - 1
    const bright = noise - prev * 0.65
    prev = noise
    const env = Math.exp(-t * 24)
    const tone = Math.sin(2 * Math.PI * 190 * t) * Math.exp(-t * 34) * 0.35
    addSample(start + i, (bright * 0.5 * env + tone) * gain * 0.75, 0.05)
  }
}

function clap(t0, gain = 1) {
  const start = Math.round(t0 * SR)
  const length = Math.round(0.22 * SR)
  for (let i = 0; i < length; i += 1) {
    const t = i / SR
    let env = 0
    for (const offset of [0, 0.011, 0.023]) {
      if (t >= offset) env += Math.exp(-(t - offset) * 55)
    }
    const noise = rand() * 2 - 1
    addSample(start + i, noise * env * 0.16 * gain, -0.1)
  }
}

function hat(t0, gain = 1, open = false) {
  const start = Math.round(t0 * SR)
  const length = Math.round((open ? 0.18 : 0.045) * SR)
  let prev = 0
  for (let i = 0; i < length; i += 1) {
    const t = i / SR
    const noise = rand() * 2 - 1
    const bright = noise - prev
    prev = noise
    const env = Math.exp(-t * (open ? 20 : 95))
    addSample(start + i, bright * env * 0.1 * gain, 0.2)
  }
}

function saw(phase) {
  return 2 * (phase - Math.floor(phase + 0.5))
}

function bass(t0, duration, freq, gain = 1) {
  const start = Math.round(t0 * SR)
  const length = Math.round(duration * SR)
  let phase = 0
  let lp = 0
  for (let i = 0; i < length; i += 1) {
    const t = i / SR
    phase += freq / SR
    if (phase > 1) phase -= 1
    const cutoff = Math.min(1, (2 * Math.PI * 280) / SR)
    lp += cutoff * (saw(phase) - lp)
    const attack = Math.min(1, t / 0.006)
    const env = attack * Math.exp(-t * 4.5)
    addSample(start + i, lp * env * 0.32 * gain)
  }
}

function pad(t0, duration, freqs, gain = 1) {
  const start = Math.round(t0 * SR)
  const length = Math.round(duration * SR)
  const phases = freqs.map(() => [rand(), rand(), rand()])
  for (let i = 0; i < length; i += 1) {
    const t = i / SR
    const attack = Math.min(1, t / 0.35)
    const release = Math.min(1, (duration - t) / 0.4)
    let value = 0
    for (let n = 0; n < freqs.length; n += 1) {
      const detune = 1 + (phases[n][0] - 0.5) * 0.004
      phases[n][1] += (freqs[n] * detune) / SR
      phases[n][2] += (freqs[n] * 1.004) / SR
      if (phases[n][1] > 1) phases[n][1] -= 1
      if (phases[n][2] > 1) phases[n][2] -= 1
      value += saw(phases[n][1]) + saw(phases[n][2])
    }
    const env = attack * release
    addSample(start + i, value * env * 0.016 * gain, Math.sin(t * 0.7) * 0.35)
  }
}

function arp(t0, freq, gain = 1) {
  const start = Math.round(t0 * SR)
  const length = Math.round(0.16 * SR)
  let phase = 0
  for (let i = 0; i < length; i += 1) {
    const t = i / SR
    phase += freq / SR
    if (phase > 1) phase -= 1
    const env = Math.exp(-t * 26)
    const value = (Math.abs(saw(phase)) * 2 - 1) * env * 0.09 * gain
    addSample(start + i, value, -0.25)
    addSample(start + i + Math.round(0.375 * SR), value * 0.35, 0.35)
  }
}

function riser(t0, duration, gain = 1) {
  const start = Math.round(t0 * SR)
  const length = Math.round(duration * SR)
  let prev = 0
  let phase = 0
  for (let i = 0; i < length; i += 1) {
    const t = i / SR
    const progress = t / duration
    const noise = rand() * 2 - 1
    const bright = noise - prev
    prev = noise
    phase += (200 + 1100 * progress) / SR
    const sweep = Math.sin(2 * Math.PI * phase) * 0.06
    const env = Math.pow(progress, 1.8)
    addSample(start + i, (bright * 0.16 + sweep) * env * gain, progress * 0.5)
  }
}

function impact(t0, gain = 1) {
  const start = Math.round(t0 * SR)
  const length = Math.round(1.4 * SR)
  let phase = 0
  let prev = 0
  for (let i = 0; i < length; i += 1) {
    const t = i / SR
    const freq = 30 + 60 * Math.exp(-t * 3)
    phase += (2 * Math.PI * freq) / SR
    const body = Math.sin(phase) * Math.exp(-t * 2.4) * 0.9
    const noise = rand() * 2 - 1
    const bright = (noise - prev) * Math.exp(-t * 8) * 0.3
    prev = noise
    addSample(start + i, (body + bright) * gain)
  }
  applyDuck(t0, 0.5, 0.8)
}

function crash(t0, gain = 1) {
  const start = Math.round(t0 * SR)
  const length = Math.round(1.8 * SR)
  let prev = 0
  for (let i = 0; i < length; i += 1) {
    const t = i / SR
    const noise = rand() * 2 - 1
    const bright = noise - prev * 0.8
    prev = noise
    addSample(start + i, bright * Math.exp(-t * 2.6) * 0.12 * gain, 0.15)
  }
}

const chords = [
  { root: 55.0, notes: [220.0, 261.63, 329.63, 440.0] }, // Am
  { root: 43.65, notes: [174.61, 220.0, 261.63, 349.23] }, // F
  { root: 65.41, notes: [196.0, 261.63, 329.63, 392.0] }, // C
  { root: 49.0, notes: [196.0, 246.94, 293.66, 392.0] }, // G
]

const dropBars = new Set([8, 9, 10, 11, 14, 15, 16, 17])
const grooveBars = new Set([4, 5, 6, 7])
const breakdownBars = new Set([12, 13])

for (let bar = 0; bar < 20; bar += 1) {
  const barStart = bar * BAR
  const chord = chords[bar % 4]

  if (bar <= 1 || dropBars.has(bar) || grooveBars.has(bar) || bar >= 18) {
    pad(barStart, BAR, chord.notes, breakdownBars.has(bar) ? 1.8 : 1)
  }

  if (bar === 0) {
    impact(0, 1)
    crash(0, 0.9)
  }
  if (bar === 8 || bar === 16) {
    impact(barStart, 1.1)
    crash(barStart, 1)
  }
  if (bar === 14) {
    impact(barStart, 1.15)
    crash(barStart, 1)
  }
  if (bar === 18) {
    impact(barStart, 1.2)
    crash(barStart, 1.1)
  }

  if (bar === 3 || bar === 7 || bar === 13 || bar === 17) {
    riser(barStart + BAR - 2, 2, bar >= 13 ? 1.1 : 0.9)
  }

  const fullDrums = grooveBars.has(bar) || dropBars.has(bar) || bar >= 18
  if (fullDrums || bar === 1 || bar === 2) {
    for (let beat = 0; beat < 4; beat += 1) {
      if (bar === 1 && beat === 0) continue
      kick(barStart + beat * BEAT, bar >= 8 ? 1 : 0.9)
    }
  }

  if (bar >= 2 && (fullDrums || bar === 2 || bar === 3)) {
    snare(barStart + BEAT, 0.85)
    clap(barStart + BEAT * 3, 0.9)
  }

  const sixteenth = dropBars.has(bar)
  const hatStep = sixteenth ? BEAT / 4 : BEAT / 2
  if (bar >= 1) {
    for (let t = 0; t < BAR - 0.001; t += hatStep) {
      const isOffbeat = Math.abs((t / BEAT) % 1) < 0.01
      hat(barStart + t, isOffbeat ? 1 : 0.55, sixteenth && isOffbeat)
    }
  }

  if (bar >= 2 && bar <= 17 && !(bar === 12)) {
    const pattern = [0, 0.75, 1, 1.5, 2, 2.75, 3, 3.5]
    for (const step of pattern) {
      const gain = breakdownBars.has(bar) ? 0.55 : dropBars.has(bar) ? 1 : 0.8
      bass(barStart + step * BEAT, BEAT * 0.42, chord.root, gain)
    }
  }

  if (bar >= 6 && bar <= 17) {
    const arpNotes = [chord.notes[0], chord.notes[1], chord.notes[2], chord.notes[3]]
    const gain = breakdownBars.has(bar) ? 1.1 : 0.85
    for (let step = 0; step < 16; step += 1) {
      const note = arpNotes[step % arpNotes.length] * (step >= 8 ? 2 : 1)
      arp(barStart + (step * BEAT) / 4, note, gain)
    }
  }
}

let peak = 0
for (let i = 0; i < TOTAL; i += 1) {
  left[i] *= duck[i]
  right[i] *= duck[i]
  peak = Math.max(peak, Math.abs(left[i]), Math.abs(right[i]))
}
const scale = peak > 0 ? 0.92 / peak : 1
const fadeStart = Math.round((DURATION - 1.2) * SR)

const pcm = Buffer.alloc(44 + TOTAL * 4)
pcm.write('RIFF', 0)
pcm.writeUInt32LE(36 + TOTAL * 4, 4)
pcm.write('WAVE', 8)
pcm.write('fmt ', 12)
pcm.writeUInt32LE(16, 16)
pcm.writeUInt16LE(1, 20)
pcm.writeUInt16LE(2, 22)
pcm.writeUInt32LE(SR, 24)
pcm.writeUInt32LE(SR * 4, 28)
pcm.writeUInt16LE(4, 32)
pcm.writeUInt16LE(16, 34)
pcm.write('data', 36)
pcm.writeUInt32LE(TOTAL * 4, 40)
for (let i = 0; i < TOTAL; i += 1) {
  const fade = i >= fadeStart ? Math.max(0, (TOTAL - i) / (TOTAL - fadeStart)) : 1
  const l = Math.tanh(left[i] * scale * 1.1) * fade
  const r = Math.tanh(right[i] * scale * 1.1) * fade
  pcm.writeInt16LE(Math.round(Math.max(-1, Math.min(1, l)) * 32767), 44 + i * 4)
  pcm.writeInt16LE(Math.round(Math.max(-1, Math.min(1, r)) * 32767), 46 + i * 4)
}

const out = path.resolve('public/audio/launch-beat.wav')
writeFileSync(out, pcm)
console.log('wrote', out, pcm.length, 'bytes')
