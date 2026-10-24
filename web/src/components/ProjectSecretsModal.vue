<script setup lang="ts">
import { ref, watch } from 'vue'
import { NButton, NIcon, NInput, NModal, useDialog, useMessage } from 'naive-ui'
import { AddOutline, RefreshOutline, TrashOutline } from '@vicons/ionicons5'
import { projectApi } from '@/api'
import type { ProjectSecret } from '@/api/types'

const props = defineProps<{
  show: boolean
  projectId: number
  projectName: string
}>()
const emit = defineEmits<{ (e: 'update:show', value: boolean): void }>()

const message = useMessage()
const dialog = useDialog()
const loading = ref(false)
const saving = ref(false)
const error = ref('')
const items = ref<ProjectSecret[]>([])
const name = ref('')
const value = ref('')

async function load() {
  if (!props.projectId) return
  loading.value = true
  error.value = ''
  try {
    const data = await projectApi.secrets(props.projectId)
    items.value = data.items
  } catch (err) {
    error.value = err instanceof Error ? err.message : '加载项目密钥失败'
  } finally {
    loading.value = false
  }
}

watch(() => props.show, (visible) => {
  if (visible) {
    name.value = ''
    value.value = ''
    void load()
  }
})

const NAME_ALPHABET = 'ABCDEFGHJKLMNPQRSTUVWXYZ23456789'

function generateSecretName() {
  const bytes = new Uint8Array(10)
  crypto.getRandomValues(bytes)
  let suffix = ''
  for (const byte of bytes) suffix += NAME_ALPHABET[byte % NAME_ALPHABET.length]
  name.value = `SECRET_${suffix}`
  error.value = ''
}

function generateSecretValue() {
  const bytes = new Uint8Array(32)
  crypto.getRandomValues(bytes)
  let binary = ''
  for (const byte of bytes) binary += String.fromCharCode(byte)
  value.value = btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
  error.value = ''
}

async function save() {
  const secretName = name.value.trim()
  if (!/^[A-Za-z][A-Za-z0-9_]{1,63}$/.test(secretName)) {
    error.value = '密钥名称需以字母开头，仅含字母、数字、下划线，长度 2~64'
    return
  }
  if (!value.value.trim()) {
    error.value = '请填写密钥值'
    return
  }
  saving.value = true
  error.value = ''
  try {
    await projectApi.setSecret(props.projectId, secretName, value.value.trim())
    message.success(`密钥 ${secretName} 已保存`)
    name.value = ''
    value.value = ''
    await load()
  } catch (err) {
    error.value = err instanceof Error ? err.message : '保存密钥失败'
  } finally {
    saving.value = false
  }
}

function remove(secret: ProjectSecret) {
  dialog.warning({
    title: '删除项目密钥',
    content: `确认删除密钥「${secret.name}」？引用它的脚本会在下次执行时报错。`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await projectApi.deleteSecret(props.projectId, secret.name)
        message.success('密钥已删除')
        await load()
      } catch (err) {
        error.value = err instanceof Error ? err.message : '删除密钥失败'
      }
    },
  })
}
</script>

<template>
  <n-modal :show="show" :mask-closable="!saving" @update:show="(visible: boolean) => emit('update:show', visible)">
    <div class="secrets-modal">
      <div class="modal-head">
        <div>
          <div class="modal-kicker">PROJECT SECRETS</div>
          <h2>项目密钥 · {{ projectName }}</h2>
          <p>脚本通过 <code>ctx.secret('NAME')</code> 读取；值加密存储，接口只返回名称。</p>
        </div>
      </div>
      <div class="modal-body">
        <div class="secret-list">
          <div v-if="loading" class="secret-empty">正在加载…</div>
          <div v-else-if="!items.length" class="secret-empty">还没有项目密钥。</div>
          <div v-for="secret in items" :key="secret.id" class="secret-row">
            <code>{{ secret.name }}</code>
            <span>{{ new Date(secret.updated_at).toLocaleString() }}</span>
            <n-button quaternary size="tiny" type="error" @click="remove(secret)">
              <template #icon><TrashOutline /></template>
            </n-button>
          </div>
        </div>

        <div class="secret-form">
          <div class="field">
            <div class="field-head">
              <label>密钥名称</label>
              <n-button text type="primary" size="tiny" @click="generateSecretName">
                <template #icon><n-icon :component="RefreshOutline" /></template>
                随机生成
              </n-button>
            </div>
            <n-input v-model:value="name" placeholder="例如 BILLING_DB_PASSWORD" maxlength="64" />
            <div class="form-hint">点击「随机生成」填充 SECRET_ 开头的随机名称。</div>
          </div>
          <div class="field">
            <div class="field-head">
              <label>密钥值</label>
              <n-button text type="primary" size="tiny" @click="generateSecretValue">
                <template #icon><n-icon :component="RefreshOutline" /></template>
                随机生成
              </n-button>
            </div>
            <n-input v-model:value="value" type="password" show-password-on="click" placeholder="输入或更新密钥值" />
            <div class="form-hint">点击「随机生成」填充 32 字节安全随机值（Base64URL，43 个字符）。</div>
          </div>
          <n-button type="primary" :loading="saving" @click="save">
            <template #icon><AddOutline /></template>
            保存/更新密钥
          </n-button>
        </div>
        <div v-if="error" class="modal-error">{{ error }}</div>
      </div>
      <div class="modal-footer">
        <n-button @click="emit('update:show', false)">关闭</n-button>
      </div>
    </div>
  </n-modal>
</template>

<style scoped>
.secrets-modal {
  width: min(620px, calc(100vw - 32px));
  max-height: calc(100vh - 32px);
  overflow: hidden;
  border-radius: 16px;
  background: #fff;
  box-shadow: 0 30px 80px -20px rgba(15, 23, 42, 0.35);
}

.modal-head {
  padding: 20px 22px 16px;
  border-bottom: 1px solid #f0f3f8;
  background: linear-gradient(130deg, #f8fbff, #fff);
}

.modal-kicker {
  color: var(--brand);
  font-family: var(--mono);
  font-size: 8.5px;
  font-weight: 650;
  letter-spacing: 0.12em;
}

.modal-head h2 {
  margin: 4px 0 3px;
  color: var(--ink);
  font-size: 19px;
  font-weight: 650;
  line-height: 1.35;
  letter-spacing: -0.02em;
}

.modal-head p {
  margin: 0;
  color: var(--muted);
  font-size: 10.5px;
  line-height: 1.6;
}

.modal-head code {
  padding: 1px 4px;
  border-radius: 4px;
  background: #eef4ff;
  color: var(--brand);
  font-family: var(--mono);
  font-size: 9.5px;
}

.modal-body {
  max-height: min(70vh, 600px);
  overflow-y: auto;
  padding: 18px 22px;
}

.field {
  margin-bottom: 14px;
}

.field label {
  display: block;
  margin-bottom: 6px;
  color: var(--text);
  font-size: 11.5px;
  font-weight: 550;
}

.field-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 6px;
}

.field-head label {
  margin-bottom: 0;
}

.form-hint {
  margin-top: 5px;
  color: var(--muted-light);
  font-size: 9.5px;
  line-height: 1.5;
}

.modal-error {
  margin-top: 12px;
  padding: 9px 11px;
  border: 1px solid #fecaca;
  border-radius: 9px;
  background: #fff7f7;
  color: #b91c1c;
  font-size: 10px;
  line-height: 1.5;
}

.modal-footer {
  display: flex;
  justify-content: flex-end;
  padding: 12px 22px;
  border-top: 1px solid #f0f3f8;
  background: #fff;
}

.secret-list {
  max-height: 240px;
  overflow-y: auto;
  margin-bottom: 16px;
  border: 1px solid #e8eef7;
  border-radius: 9px;
}

.secret-empty {
  padding: 18px;
  color: var(--muted-light);
  font-size: 10px;
  text-align: center;
}

.secret-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 10px;
  border-bottom: 1px solid #f1f4f9;
}

.secret-row:last-child {
  border-bottom: 0;
}

.secret-row code {
  flex: 1;
  color: var(--ink);
  font-family: var(--mono);
  font-size: 10px;
}

.secret-row span {
  color: var(--muted-light);
  font-size: 9px;
}

.secret-form {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

@media (max-width: 640px) {
  .secrets-modal {
    width: calc(100vw - 16px);
    max-height: calc(100vh - 16px);
  }

  .modal-head {
    padding: 16px 16px 13px;
  }

  .modal-head h2 {
    font-size: 16px;
  }

  .modal-body {
    max-height: calc(100vh - 210px);
    padding: 14px 16px;
  }

  .modal-footer {
    padding: 10px 16px;
  }
}
</style>
