<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { EditorView, basicSetup } from 'codemirror'
import { EditorState } from '@codemirror/state'

const props = withDefaults(
  defineProps<{
    modelValue: string
    maxLength?: number
    height?: string
  }>(),
  {
    maxLength: 80000,
    height: '300px',
  },
)

const emit = defineEmits<{
  (event: 'update:modelValue', value: string): void
}>()

const host = ref<HTMLElement | null>(null)
const count = ref(0)
let view: EditorView | null = null
let syncing = false

const full = computed(() => Boolean(props.maxLength) && count.value >= props.maxLength)
const countText = computed(() => `${count.value} / ${props.maxLength} 字`)

function countTextLength(value: string) {
  return value.length
}

const editorTheme = EditorView.theme({
  '&': {
    backgroundColor: 'var(--card, #fff)',
    color: 'var(--text, #334155)',
    fontSize: '11.5px',
  },
  '&.cm-focused': {
    outline: 'none',
  },
  '.cm-content': {
    padding: '10px 0',
    caretColor: 'var(--brand, #2b6cf6)',
    fontFamily: 'var(--mono, "JetBrains Mono", Consolas, monospace)',
    lineHeight: '1.7',
  },
  '.cm-line': {
    padding: '0 12px',
  },
  '.cm-gutters': {
    border: 'none',
    borderRight: '1px solid var(--border, #e3e8f1)',
    backgroundColor: 'var(--surface-soft, #f8fafd)',
    color: 'var(--muted-light, #9aa6b8)',
  },
  '.cm-activeLine': {
    backgroundColor: 'var(--brand-softer, rgba(43, 108, 246, 0.045))',
  },
  '.cm-activeLineGutter': {
    backgroundColor: 'var(--brand-softer, rgba(43, 108, 246, 0.045))',
    color: 'var(--brand, #2b6cf6)',
  },
  '&.cm-focused .cm-selectionBackground, .cm-selectionBackground, .cm-content ::selection': {
    backgroundColor: 'rgba(91, 142, 247, 0.24)',
  },
  '.cm-scroller': {
    overflow: 'auto',
  },
  '.cm-editor': {
    height: '100%',
  },
})

onMounted(() => {
  if (!host.value) return
  count.value = countTextLength(props.modelValue)
  view = new EditorView({
    parent: host.value,
    state: EditorState.create({
      doc: props.modelValue,
      extensions: [
        basicSetup,
        editorTheme,
        EditorView.lineWrapping,
        EditorState.transactionFilter.of((transaction) => {
          if (!transaction.docChanged || !props.maxLength) return transaction
          if (countTextLength(transaction.newDoc.toString()) > props.maxLength) return []
          return transaction
        }),
        EditorView.updateListener.of((update) => {
          if (!update.docChanged) return
          const text = update.state.doc.toString()
          count.value = countTextLength(text)
          if (!syncing) emit('update:modelValue', text)
        }),
      ],
    }),
  })
})

watch(
  () => props.modelValue,
  (value) => {
    if (!view || view.state.doc.toString() === value) return
    syncing = true
    view.dispatch({
      changes: { from: 0, to: view.state.doc.length, insert: value },
    })
    count.value = countTextLength(value)
    syncing = false
  },
)

onBeforeUnmount(() => {
  view?.destroy()
  view = null
})
</script>

<template>
  <div class="virtual-text-editor" :style="{ minHeight: height }">
    <div ref="host" class="virtual-text-host" :style="{ height }"></div>
    <div class="virtual-text-meta" :class="{ full }">{{ countText }}</div>
  </div>
</template>

<style scoped>
.virtual-text-editor {
  overflow: hidden;
  border: 1px solid var(--border, #e3e8f1);
  border-radius: 10px;
  background: var(--card, #fff);
}

.virtual-text-editor:focus-within {
  border-color: var(--brand, #2b6cf6);
  box-shadow: 0 0 0 3px rgba(43, 108, 246, 0.1);
}

.virtual-text-host {
  overflow: hidden;
}

.virtual-text-meta {
  padding: 5px 12px 6px;
  border-top: 1px solid var(--border, #e3e8f1);
  background: var(--surface-soft, #f8fafd);
  color: var(--muted-light, #9aa6b8);
  font-family: var(--mono, monospace);
  font-size: 9px;
  text-align: right;
}

.virtual-text-meta.full {
  color: var(--red);
}

.virtual-text-editor :deep(.cm-editor) {
  height: 100%;
}

.virtual-text-editor :deep(.cm-scroller) {
  overflow: auto;
}
</style>
