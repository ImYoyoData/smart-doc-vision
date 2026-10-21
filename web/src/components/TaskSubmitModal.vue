<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import {
  NButton,
  NInput,
  NModal,
  NSelect,
  NSpace,
  useMessage,
} from 'naive-ui'
import { FlashOutline, ImagesOutline, TextOutline, WarningOutline } from '@vicons/ionicons5'
import { auditApi, taskApi } from '@/api'
import type { Audit, Task } from '@/api/types'

const props = defineProps<{
  show: boolean
  projectId: number | null
  defaultAuditId?: number | null
}>()

const emit = defineEmits<{
  'update:show': [value: boolean]
  created: [task: Task, duplicated: boolean]
}>()

const message = useMessage()
const audits = ref<Audit[]>([])
const loadingAudits = ref(false)
const submitting = ref(false)
const errorText = ref('')
const form = reactive({
  auditId: null as number | null,
  bizKey: '',
  text: '',
  images: '',
  callbackUrl: '',
  ext: '',
})

const auditOptions = computed(() =>
  audits.value.map((a) => ({
    label: `${a.name} · ${a.provider_name || '未绑定模型'}`,
    value: a.id,
  })),
)

const selectedAudit = computed(() => audits.value.find((a) => a.id === form.auditId) || null)

async function loadAudits() {
  if (!props.projectId) {
    audits.value = []
    return
  }
  loadingAudits.value = true
  try {
    const data = await auditApi.list({ project_id: props.projectId, status: 'ACTIVE', page: 1, page_size: 100 })
    audits.value = data.items
    if (props.defaultAuditId && data.items.some((a) => a.id === props.defaultAuditId)) {
      form.auditId = props.defaultAuditId
    } else if (!form.auditId && data.items.length > 0) {
      form.auditId = data.items[0].id
    }
  } catch (error) {
    message.error(error instanceof Error ? error.message : '加载审核配置失败')
  } finally {
    loadingAudits.value = false
  }
}

watch(
  () => props.show,
  (visible) => {
    if (visible) {
      form.bizKey = ''
      form.text = ''
      form.images = ''
      form.callbackUrl = ''
      form.ext = ''
      form.auditId = props.defaultAuditId || null
      errorText.value = ''
      loadAudits()
    }
  },
)

watch(
  () => props.projectId,
  () => {
    if (props.show) {
      form.auditId = null
      loadAudits()
    }
  },
)

function close() {
  if (submitting.value) return
  emit('update:show', false)
}

async function submit() {
  errorText.value = ''
  if (!props.projectId) {
    errorText.value = '请先在左上角选择项目'
    return
  }
  if (!form.auditId) {
    errorText.value = '请选择审核配置'
    return
  }
  const images = form.images
    .split(/\r?\n/)
    .map((s) => s.trim())
    .filter(Boolean)
  if (!form.text.trim() && images.length === 0) {
    errorText.value = '请填写文档文本或至少一张图片 URL'
    return
  }
  let ext: Record<string, unknown> | undefined
  if (form.ext.trim()) {
    try {
      const parsed = JSON.parse(form.ext)
      if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
        ext = parsed as Record<string, unknown>
      } else {
        throw new Error('扩展信息需为 JSON 对象')
      }
    } catch (error) {
      errorText.value = error instanceof Error ? `扩展信息 JSON 无效：${error.message}` : '扩展信息 JSON 无效'
      return
    }
  }

  submitting.value = true
  try {
    const media = []
    if (form.text.trim()) media.push({ type: 'text', content: form.text.trim(), name: 'document.txt' })
    for (const url of images) media.push({ type: 'image', url })
    const result = await taskApi.submit({
      project_id: props.projectId,
      audit_id: form.auditId,
      biz_key: form.bizKey.trim(),
      media,
      ext,
      callback_url: form.callbackUrl.trim() || undefined,
    })
    if (result.duplicated) {
      message.warning('命中幂等去重，已返回原任务结果')
    } else {
      message.success('审核任务已提交，正在异步处理')
    }
    emit('created', result.task, result.duplicated)
    emit('update:show', false)
  } catch (error) {
    errorText.value = error instanceof Error ? error.message : '提交失败'
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <n-modal :show="show" :mask-closable="!submitting" @update:show="close">
    <div class="submit-modal">
      <div class="modal-head">
        <div>
          <div class="modal-kicker">NEW AUDIT</div>
          <h2>发起一次图档审核</h2>
          <p>任务将进入异步审核引擎，并按审核配置的规则与结果路由处理。</p>
        </div>
        <span class="modal-icon"><FlashOutline /></span>
      </div>

      <div class="modal-body">
        <div class="field">
          <label>审核配置 <span class="required">*</span></label>
          <n-select
            v-model:value="form.auditId"
            :options="auditOptions"
            :loading="loadingAudits"
            placeholder="选择要执行的审核流程"
            filterable
          />
          <div v-if="selectedAudit" class="audit-hint">
            <span class="hint-dot"></span>
            {{ selectedAudit.provider_name }} ·
            {{ selectedAudit.provider_kind === 'JEV' ? 'JEV System One 结构化裁决' : selectedAudit.provider_kind === 'ANTHROPIC' ? 'Anthropic Messages 结构化输出' : 'OpenAI 兼容结构化输出' }}
          </div>
          <div v-else-if="!loadingAudits && audits.length === 0" class="form-hint warn">
            <WarningOutline /> 当前项目还没有启用的审核配置，请先创建。
          </div>
        </div>

        <div class="field">
          <label>业务键</label>
          <n-input v-model:value="form.bizKey" placeholder="例如 archive:2026:0001，用于幂等去重" />
          <div class="form-hint">相同业务键 + 相同内容会在去重窗口内复用原任务。</div>
        </div>

        <div class="field">
          <label><TextOutline class="label-icon" /> 文档文本</label>
          <n-input
            v-model:value="form.text"
            type="textarea"
            :autosize="{ minRows: 4, maxRows: 8 }"
            placeholder="粘贴待审核的档案文本、OCR 结果或说明内容…"
          />
        </div>

        <div class="field">
          <label><ImagesOutline class="label-icon" /> 图片 URL</label>
          <n-input
            v-model:value="form.images"
            type="textarea"
            :autosize="{ minRows: 2, maxRows: 5 }"
            placeholder="每行一个图档图片地址（PNG / JPEG）&#10;https://example.com/archive/page-01.jpg"
          />
          <div class="form-hint">
            支持多个文本段落 + 多张图片；当前不支持视频/音频。JEV 原生多模态，通用模型需在接入配置中开启图片理解。
          </div>
        </div>

        <div class="two-col">
          <div class="field">
            <label>回调地址</label>
            <n-input v-model:value="form.callbackUrl" placeholder="https://your-service/callback（可选）" />
          </div>
          <div class="field">
            <label>扩展信息 JSON</label>
            <n-input v-model:value="form.ext" placeholder='{"scene":"archive"}' />
          </div>
        </div>

        <div v-if="errorText" class="form-error">{{ errorText }}</div>

        <div class="modal-actions">
          <n-space>
            <n-button :disabled="submitting" @click="close">取消</n-button>
            <n-button type="primary" :loading="submitting" @click="submit">
              <template #icon><FlashOutline /></template>
              提交审核
            </n-button>
          </n-space>
        </div>
      </div>
    </div>
  </n-modal>
</template>

<style scoped>
.submit-modal {
  width: min(620px, calc(100vw - 32px));
  overflow: hidden;
  border-radius: 18px;
  background: #fff;
  box-shadow: 0 30px 80px -20px rgba(15, 23, 42, 0.35);
}

.modal-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 20px;
  padding: 24px 26px 18px;
  border-bottom: 1px solid #f0f3f8;
  background: linear-gradient(130deg, #f8fbff, #fff);
}

.modal-kicker {
  color: var(--brand);
  font-family: var(--mono);
  font-size: 9px;
  font-weight: 650;
  letter-spacing: 0.14em;
}

.modal-head h2 {
  margin: 5px 0 4px;
  color: var(--ink);
  font-size: 20px;
  font-weight: 650;
  letter-spacing: -0.025em;
}

.modal-head p {
  margin: 0;
  color: var(--muted);
  font-size: 12px;
}

.modal-icon {
  display: flex;
  width: 42px;
  height: 42px;
  align-items: center;
  justify-content: center;
  border-radius: 12px;
  background: var(--brand);
  color: #fff;
  flex-shrink: 0;
  box-shadow: 0 8px 18px -6px rgba(43, 108, 246, 0.5);
}

.modal-icon :deep(svg) {
  width: 20px;
  height: 20px;
}

.modal-body {
  max-height: min(70vh, 650px);
  overflow-y: auto;
  padding: 22px 26px 24px;
}

.field {
  margin-bottom: 18px;
}

.field > label {
  display: flex;
  align-items: center;
  gap: 5px;
  margin-bottom: 7px;
  color: var(--text);
  font-size: 12.5px;
  font-weight: 550;
}

.label-icon {
  width: 13px;
  height: 13px;
  color: var(--muted-light);
}

.required {
  color: var(--red);
}

.audit-hint {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-top: 7px;
  color: var(--muted);
  font-size: 11px;
}

.hint-dot {
  width: 5px;
  height: 5px;
  border-radius: 50%;
  background: #22c55e;
}

.form-hint.warn {
  display: flex;
  align-items: center;
  gap: 5px;
  color: #b45309;
}

.form-hint.warn :deep(svg) {
  width: 12px;
  height: 12px;
}

.two-col {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 14px;
}

.form-error {
  margin: 4px 0 14px;
  padding: 9px 12px;
  border: 1px solid #fee2e2;
  border-radius: 9px;
  background: #fef2f2;
  color: #b91c1c;
  font-size: 12px;
}

.modal-actions {
  display: flex;
  justify-content: flex-end;
  padding-top: 6px;
}

@media (max-width: 560px) {
  .two-col {
    grid-template-columns: 1fr;
  }

  .modal-head,
  .modal-body {
    padding-left: 18px;
    padding-right: 18px;
  }
}
</style>
