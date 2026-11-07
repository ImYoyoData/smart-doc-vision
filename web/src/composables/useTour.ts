import { computed, nextTick, reactive, ref } from 'vue'
import { getTour, type TourDefinition, type TourStep } from '@/constants/tours'

const SEEN_PREFIX = 'sdv:tour:seen:'
const VOICE_PREF_KEY = 'sdv:tour:voice'
const AUTO_BASIC_KEY = 'sdv:tour:auto-basic'

function storageGet(key: string): string | null {
  try {
    return localStorage.getItem(key)
  } catch {
    return null
  }
}

function storageSet(key: string, value: string) {
  try {
    localStorage.setItem(key, value)
  } catch {
    // Private mode may reject writes; the tour still works without persistence.
  }
}

export interface TourRect {
  top: number
  left: number
  width: number
  height: number
}

const state = reactive({
  active: false,
  tourKey: '',
  index: 0,
  rect: null as TourRect | null,
  voiceEnabled: storageGet(VOICE_PREF_KEY) !== '0',
  /** Set when the user explicitly asks to replay, so "seen" is refreshed only after finishing. */
  replay: false,
})

const tour = ref<TourDefinition | null>(null)
const currentStep = computed<TourStep | null>(() => tour.value?.steps[state.index] ?? null)
const totalSteps = computed(() => tour.value?.steps.length ?? 0)

/** Pre-generated narration state, surfaced in the tour card. */
const audio = reactive({ playing: false, available: true })

let audioElement: HTMLAudioElement | null = null
let speech: SpeechSynthesisUtterance | null = null
let fallbackUsed = false

function cancelSpeech() {
  if (typeof window === 'undefined' || !('speechSynthesis' in window)) return
  speech = null
  window.speechSynthesis.cancel()
}

function stopAudio() {
  audio.playing = false
  if (!audioElement) return
  audioElement.pause()
  audioElement = null
}

/** Stops whatever is currently narrating (recorded audio or TTS). */
function stopNarration() {
  stopAudio()
  cancelSpeech()
}

function audioURL(tourKey: string, index: number) {
  const base = import.meta.env.BASE_URL || '/'
  return `${base}audio/tour/${tourKey}-${index}.mp3`
}

// Fallback used when a clip is missing (for example a newly added step before
// the audio is regenerated). Most browsers still ship a zh-CN voice.
function speakFallback(text: string) {
  if (!text) return
  if (typeof window === 'undefined' || !('speechSynthesis' in window)) return
  window.speechSynthesis.cancel()
  const utterance = new SpeechSynthesisUtterance(text)
  utterance.lang = 'zh-CN'
  utterance.rate = 1
  utterance.pitch = 1
  const chinese = window.speechSynthesis.getVoices().find((voice) => /^zh(-|_)?/i.test(voice.lang))
  if (chinese) utterance.voice = chinese
  speech = utterance
  fallbackUsed = true
  window.speechSynthesis.speak(utterance)
}

// narrate plays the recorded narration for the current step. The clips are
// generated once with the Gentle_Senior voice and shipped in public/audio/tour,
// which sounds identical everywhere and costs no TTS quota at runtime.
function narrate(tourKey: string, index: number, text: string) {
  stopNarration()
  if (!state.voiceEnabled) return
  if (typeof window === 'undefined' || typeof Audio === 'undefined') return
  const element = new Audio(audioURL(tourKey, index))
  element.preload = 'auto'
  audioElement = element
  fallbackUsed = false
  element.addEventListener('playing', () => {
    if (audioElement === element) audio.playing = true
  })
  element.addEventListener('ended', () => {
    if (audioElement === element) audio.playing = false
  })
  element.addEventListener('error', () => {
    if (audioElement !== element) return
    audio.playing = false
    audioElement = null
    if (!fallbackUsed) {
      audio.available = false
      speakFallback(text)
    }
  })
  void element.play().catch(() => {
    // Autoplay may be blocked before the first user gesture; the text is still
    // on screen, so this is not an error worth surfacing.
    if (audioElement === element) audio.playing = false
  })
}

function measure(step: TourStep | null) {
  if (!step?.target || typeof document === 'undefined') {
    state.rect = null
    return
  }
  const element = document.querySelector(step.target)
  if (!element) {
    state.rect = null
    return
  }
  element.scrollIntoView({ block: 'center', behavior: 'smooth' })
  const box = element.getBoundingClientRect()
  const padding = 8
  state.rect = {
    top: Math.max(8, box.top - padding),
    left: Math.max(8, box.left - padding),
    width: Math.min(window.innerWidth - 16, box.width + padding * 2),
    height: Math.min(window.innerHeight - 16, box.height + padding * 2),
  }
}

async function applyStep() {
  stopNarration()
  await nextTick()
  // Give scroll-into-view and any page transition a frame before measuring.
  window.setTimeout(() => {
    measure(currentStep.value)
    narrate(state.tourKey, state.index, currentStep.value?.body || '')
  }, 260)
}

function start(key: string, replay = false) {
  const definition = getTour(key)
  if (!definition || !definition.steps.length) return
  tour.value = definition
  state.tourKey = key
  state.index = 0
  state.replay = replay
  state.active = true
  void applyStep()
}

function next() {
  if (!state.active) return
  if (state.index >= totalSteps.value - 1) {
    finish()
    return
  }
  state.index += 1
  void applyStep()
}

function prev() {
  if (!state.active || state.index === 0) return
  state.index -= 1
  void applyStep()
}

function dismiss() {
  if (!state.active) return
  stopNarration()
  if (state.tourKey) markSeen(state.tourKey)
  state.active = false
  state.rect = null
  tour.value = null
}

function finish() {
  dismiss()
}

function markSeen(key: string) {
  storageSet(SEEN_PREFIX + key, '1')
}

function hasSeen(key: string): boolean {
  return storageGet(SEEN_PREFIX + key) === '1'
}

const OFFER_PREFIX = 'sdv:tour:offered:'

/** True the first time a page is offered a tour and the tour was never seen. */
function shouldOffer(key: string): boolean {
  if (hasSeen(key)) return false
  return storageGet(OFFER_PREFIX + key) !== '1'
}

function markOffered(key: string) {
  storageSet(OFFER_PREFIX + key, '1')
}

function setVoice(enabled: boolean) {
  state.voiceEnabled = enabled
  storageSet(VOICE_PREF_KEY, enabled ? '1' : '0')
  if (enabled) {
    if (state.active) narrate(state.tourKey, state.index, currentStep.value?.body || '')
    return
  }
  stopNarration()
}

/** Replays the narration of the current step (used by the card's replay button). */
function replayNarration() {
  if (!state.active) return
  narrate(state.tourKey, state.index, currentStep.value?.body || '')
}

/** True once per browser until the basic tour has been completed or skipped. */
function shouldAutoStartBasic(): boolean {
  if (storageGet(AUTO_BASIC_KEY) === '1') return false
  return !hasSeen('basic')
}

function markAutoStartHandled() {
  storageSet(AUTO_BASIC_KEY, '1')
}

function onResize() {
  if (state.active) measure(currentStep.value)
}

if (typeof window !== 'undefined') {
  window.addEventListener('resize', onResize)
  window.addEventListener('scroll', onResize, true)
  // Voices load asynchronously in Chromium; touching the list warms them up.
  if ('speechSynthesis' in window) window.speechSynthesis.getVoices()
}

export function useTour() {
  return {
    state,
    audio,
    tour,
    currentStep,
    totalSteps,
    start,
    next,
    prev,
    dismiss,
    finish,
    markSeen,
    hasSeen,
    shouldOffer,
    markOffered,
    setVoice,
    replayNarration,
    shouldAutoStartBasic,
    markAutoStartHandled,
    stopNarration,
    cancelSpeech,
  }
}
