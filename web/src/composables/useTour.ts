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

let speech: SpeechSynthesisUtterance | null = null

function cancelSpeech() {
  if (typeof window === 'undefined' || !('speechSynthesis' in window)) return
  speech = null
  window.speechSynthesis.cancel()
}

function speak(text: string) {
  if (!state.voiceEnabled || !text) return
  if (typeof window === 'undefined' || !('speechSynthesis' in window)) return
  window.speechSynthesis.cancel()
  const utterance = new SpeechSynthesisUtterance(text)
  utterance.lang = 'zh-CN'
  utterance.rate = 1
  utterance.pitch = 1
  const voices = window.speechSynthesis.getVoices()
  const chinese = voices.find((voice) => /^zh(-|_)?/i.test(voice.lang))
  if (chinese) utterance.voice = chinese
  speech = utterance
  window.speechSynthesis.speak(utterance)
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
  cancelSpeech()
  await nextTick()
  // Give scroll-into-view and any page transition a frame before measuring.
  window.setTimeout(() => {
    measure(currentStep.value)
    speak(currentStep.value?.body || '')
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
  cancelSpeech()
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
  if (!enabled) cancelSpeech()
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
    shouldAutoStartBasic,
    markAutoStartHandled,
    cancelSpeech,
  }
}
