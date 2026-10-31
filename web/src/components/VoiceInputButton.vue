<script setup lang="ts">
// 语音输入按钮：使用浏览器内置 Web Speech API（SpeechRecognition）。
// 不支持该 API 的环境整颗按钮不渲染；支持时点击弹出录音小窗，
// 实时显示识别文本，最长 2 分钟（倒计时），可提前完成填入或取消。
import { computed, onBeforeUnmount, ref } from 'vue'
import { NButton } from 'naive-ui'
import { CloseOutline, MicOutline, PauseOutline } from '@vicons/ionicons5'

const props = withDefaults(defineProps<{ disabled?: boolean; lang?: string }>(), {
  disabled: false,
  lang: 'zh-CN',
})
const emit = defineEmits<{ (e: 'transcript', text: string): void }>()

interface SRAlternative { transcript: string }
interface SRResult { isFinal: boolean; 0: SRAlternative; length: number }
interface SREvent { resultIndex: number; results: { length: number; [index: number]: SRResult } }
interface SRErrorEvent { error: string }
interface SpeechRecognitionLike {
  lang: string
  continuous: boolean
  interimResults: boolean
  start(): void
  stop(): void
  abort(): void
  onresult: ((event: SREvent) => void) | null
  onerror: ((event: SRErrorEvent) => void) | null
  onend: (() => void) | null
}
type SRCtor = new () => SpeechRecognitionLike

function getRecognitionCtor(): SRCtor | null {
  const w = window as unknown as { SpeechRecognition?: SRCtor; webkitSpeechRecognition?: SRCtor }
  return w.SpeechRecognition || w.webkitSpeechRecognition || null
}

const supported = computed(() => !!getRecognitionCtor())

const show = ref(false)
const listening = ref(false)
const finalText = ref('')
const interimText = ref('')
const errorText = ref('')
const remainSec = ref(120)
const finishing = ref(false)
let recognition: SpeechRecognitionLike | null = null
let timer: ReturnType<typeof setInterval> | null = null
let manualStop = false

const transcriptView = computed(() => normalizeTranscript(finalText.value + interimText.value) || '（等待语音输入…）')
const remainLabel = computed(() => {
  const m = Math.floor(remainSec.value / 60)
  const s = remainSec.value % 60
  return `${m}:${String(s).padStart(2, '0')}`
})

// 浏览器对中文识别的每段 transcript 会按英文习惯在词间加空格，
// 多段拼接后变成「每 个 字 都 有 空 格」。这里去掉 CJK 字符之间的
// 空白（含中文标点），保留英文单词与数字之间的正常空格。
const CJK_CLASS = '\\u4e00-\\u9fa5\\u3400-\\u4dbf\\u3000-\\u303f\\uff00-\\uffef'
const CJK_BETWEEN = new RegExp(`([${CJK_CLASS}])\\s+(?=[${CJK_CLASS}])`, 'g')

function normalizeTranscript(text: string): string {
  let out = text.replace(CJK_BETWEEN, '$1')
  // 多轮替换处理连续空格与紧邻标点的边界
  while (CJK_BETWEEN.test(out)) {
    out = out.replace(CJK_BETWEEN, '$1')
  }
  return out
}

function teardown() {
  if (timer) {
    clearInterval(timer)
    timer = null
  }
  if (recognition) {
    recognition.onresult = null
    recognition.onerror = null
    recognition.onend = null
    try {
      recognition.abort()
    } catch {
      /* already stopped */
    }
    recognition = null
  }
  listening.value = false
}

function open() {
  const Ctor = getRecognitionCtor()
  if (!Ctor || props.disabled) return
  finalText.value = ''
  interimText.value = ''
  errorText.value = ''
  remainSec.value = 120
  finishing.value = false
  manualStop = false
  show.value = true

  recognition = new Ctor()
  recognition.lang = props.lang
  recognition.continuous = true
  recognition.interimResults = true
  recognition.onresult = (event) => {
    let interim = ''
    for (let i = event.resultIndex; i < event.results.length; i += 1) {
      const item = event.results[i]
      if (item.isFinal) {
        finalText.value += normalizeTranscript(item[0].transcript)
      } else {
        interim += item[0].transcript
      }
    }
    interimText.value = normalizeTranscript(interim)
  }
  recognition.onerror = (event) => {
    if (event.error === 'no-speech' || event.error === 'aborted') return
    if (event.error === 'not-allowed' || event.error === 'service-not-allowed') {
      errorText.value = '麦克风权限被拒绝，请在浏览器地址栏允许麦克风后重试'
      manualStop = true
      teardown()
      return
    }
    if (event.error === 'network') {
      errorText.value = '语音识别服务不可用（浏览器识别服务需要网络），可改用键盘输入'
      manualStop = true
      teardown()
      return
    }
    errorText.value = `语音识别出错：${event.error}`
  }
  recognition.onend = () => {
    // continuous 模式在部分浏览器会自行结束，未主动停止时自动续录。
    if (!manualStop && recognition && listening.value) {
      try {
        recognition.start()
      } catch {
        /* restarting too fast */
      }
    }
  }
  try {
    recognition.start()
    listening.value = true
  } catch {
    errorText.value = '无法启动语音识别，请重试'
  }
  timer = setInterval(() => {
    remainSec.value -= 1
    if (remainSec.value <= 0) finish()
  }, 1000)
}

function finish() {
  if (finishing.value) return
  finishing.value = true
  manualStop = true
  const text = normalizeTranscript(finalText.value + interimText.value).trim()
  teardown()
  if (text) emit('transcript', text)
  show.value = false
}

function cancel() {
  manualStop = true
  teardown()
  show.value = false
}

onBeforeUnmount(teardown)
</script>

<template>
  <n-button
    v-if="supported"
    quaternary
    circle
    size="small"
    title="语音输入"
    aria-label="语音输入"
    :disabled="props.disabled"
    @click="open"
  >
    <template #icon><MicOutline /></template>
  </n-button>

  <teleport to="body">
    <div v-if="show" class="voice-mask" @click.self="cancel">
      <div class="voice-modal" role="dialog" aria-label="语音输入">
        <div class="voice-head">
          <span class="voice-icon" :class="{ pulse: listening }"><MicOutline /></span>
          <div class="voice-title">
            <h3>{{ listening ? '正在聆听，请说出你的需求' : '语音输入已停止' }}</h3>
            <span class="voice-count">最长录制 2 分钟 · 剩余 {{ remainLabel }}</span>
          </div>
        </div>
        <div class="voice-body">
          <p class="voice-text">{{ transcriptView }}</p>
          <p v-if="errorText" class="voice-error">{{ errorText }}</p>
        </div>
        <div class="voice-actions">
          <n-button size="small" @click="cancel">
            <template #icon><CloseOutline /></template>
            取消
          </n-button>
          <n-button size="small" type="primary" :loading="finishing" @click="finish">
            <template #icon><PauseOutline /></template>
            完成并填入
          </n-button>
        </div>
      </div>
    </div>
  </teleport>
</template>

<style scoped>
.voice-mask {
  position: fixed;
  inset: 0;
  z-index: 3000;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(15, 23, 42, 0.45);
}

.voice-modal {
  width: min(460px, calc(100vw - 40px));
  padding: 20px;
  border-radius: 14px;
  background: var(--card, #fff);
  box-shadow: 0 24px 60px rgba(15, 23, 42, 0.28);
}

.voice-head {
  display: flex;
  gap: 12px;
  align-items: center;
}

.voice-icon {
  display: inline-flex;
  width: 42px;
  height: 42px;
  flex-shrink: 0;
  align-items: center;
  justify-content: center;
  border-radius: 12px;
  background: rgba(43, 108, 246, 0.12);
  color: var(--brand, #2b6cf6);
}

.voice-icon :deep(svg) {
  width: 20px;
  height: 20px;
}

.voice-icon.pulse {
  animation: voice-pulse 1.4s ease-in-out infinite;
}

@keyframes voice-pulse {
  0%, 100% { box-shadow: 0 0 0 0 rgba(43, 108, 246, 0.35); }
  50% { box-shadow: 0 0 0 9px rgba(43, 108, 246, 0); }
}

.voice-title h3 {
  margin: 0 0 3px;
  font-size: 14.5px;
}

.voice-count {
  color: var(--muted-light, #94a3b8);
  font-family: var(--mono, monospace);
  font-size: 11px;
}

.voice-body {
  min-height: 96px;
  max-height: 200px;
  margin-top: 14px;
  padding: 12px 14px;
  overflow: auto;
  border-radius: 10px;
  background: var(--surface-soft);
}

.voice-text {
  margin: 0;
  color: var(--text, #1e293b);
  font-size: 13.5px;
  line-height: 1.7;
  word-break: break-all;
}

.voice-error {
  margin: 8px 0 0;
  color: var(--red);
  font-size: 12px;
}

.voice-actions {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  margin-top: 16px;
}
</style>
