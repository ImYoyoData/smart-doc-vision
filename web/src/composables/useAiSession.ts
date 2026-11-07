import { ref, type Ref } from 'vue'

function randomSessionId(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID()
  }
  // Fallback for insecure origins where randomUUID is unavailable.
  const bytes = new Uint8Array(16)
  if (typeof crypto !== 'undefined' && typeof crypto.getRandomValues === 'function') {
    crypto.getRandomValues(bytes)
  } else {
    for (let index = 0; index < bytes.length; index += 1) {
      bytes[index] = Math.floor(Math.random() * 256)
    }
  }
  bytes[6] = (bytes[6] & 0x0f) | 0x40
  bytes[8] = (bytes[8] & 0x3f) | 0x80
  const hex = Array.from(bytes, (value) => value.toString(16).padStart(2, '0')).join('')
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`
}

/**
 * One chat = one X-Session-Id. Every model request inside the same conversation
 * reuses the current ID; startNewSession() mints a fresh one when the user opens
 * or resets the conversation.
 */
export function useAiSession(): { sessionId: Ref<string>; startNewSession: () => string } {
  const sessionId = ref(randomSessionId())
  const startNewSession = () => {
    sessionId.value = randomSessionId()
    return sessionId.value
  }
  return { sessionId, startNewSession }
}
