<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { NButton, NCheckbox, NCheckboxGroup, NModal, useMessage } from 'naive-ui'
import {
  BookOutline,
  ChevronForwardOutline,
  DownloadOutline,
  GlobeOutline,
  HelpCircleOutline,
  KeyOutline,
  LayersOutline,
  ShieldCheckmarkOutline,
  SparklesOutline,
  SwapHorizontalOutline,
} from '@vicons/ionicons5'
import PageHeader from '@/components/PageHeader.vue'
import CodeBlock from '@/components/CodeBlock.vue'
import CopyButton from '@/components/CopyButton.vue'
import { copyText } from '@/utils/clipboard'
import {
  INTEGRATION_METHODS,
  buildMethodPrompt,
  buildSelectedPrompt,
  buildSkillPrompt,
  type IntegrationMethod,
  type IntegrationPromptContext,
} from '@/utils/integrationPrompt'

const sections = [
  { id: 'overview', label: '接入总览', icon: BookOutline },
  { id: 'auth', label: '凭证与权限', icon: KeyOutline },
  { id: 'openapi', label: '开放 API', icon: GlobeOutline },
  { id: 'redis', label: 'Redis Stream', icon: LayersOutline },
  { id: 'websocket', label: 'WebSocket', icon: SwapHorizontalOutline },
  { id: 'callback', label: '结果回调', icon: ShieldCheckmarkOutline },
  { id: 'mysql', label: 'MySQL 说明', icon: HelpCircleOutline },
]
const message = useMessage()
const activeSection = ref('overview')

const skillDocsURL = `${window.location.origin}${import.meta.env.BASE_URL}help`

const promptShow = ref(false)
const promptMethods = ref<IntegrationMethod[]>(['OPENAPI'])
const aiMethodOptions = INTEGRATION_METHODS

function buildGenericContext(): IntegrationPromptContext {
  return {
    origin: window.location.origin,
    projectCode: 'PROJECT_CODE',
    auditCode: 'AUDIT_CODE',
    auditName: '业务审核',
    docsURL: skillDocsURL,
    credentialName: '在「接入凭证」页面创建并授权当前审核配置',
    appKey: 'APP_KEY',
    openapiPath: '/open/v1/projects/PROJECT_CODE/audits/AUDIT_CODE/submit',
    openapiEnabled: true,
    sampleBody: `{
  "biz_key": "archive:2026:0001",
  "request_id": "audit-20261031-000001",
  "media": [
    {"type": "text", "name": "page-1.txt", "content": "第一页档案文本"},
    {"type": "image", "name": "page-1.jpg", "url": "https://your-domain.example.com/page-1.jpg"}
  ],
  "ext": {"scene": "archive_review"}
}`,
    redisEnabled: true,
    redisStream: 'sdv:PROJECT_CODE:audit:AUDIT_CODE',
    redisGroup: 'sdv-cg-<audit_id>',
    websocketEnabled: true,
    websocketPath: '/open/v1/ws/projects/PROJECT_CODE/audits/AUDIT_CODE',
    websocketURL: 'wss://your-smartdoc-host/open/v1/ws/projects/PROJECT_CODE/audits/AUDIT_CODE',
    callback: {
      enabled: false,
      results: ['REJECT'],
      includeContent: true,
      secretSet: false,
    },
  }
}

function openPromptPicker() {
  promptShow.value = true
}

function aiMethodLabel(method: IntegrationMethod): string {
  return INTEGRATION_METHODS.find((item) => item.key === method)?.label || method
}

async function copyPromptFor(method?: IntegrationMethod) {
  if (!method && !promptMethods.value.length) {
    message.warning('请至少选择一种接入方式')
    return
  }
  const context = buildGenericContext()
  const text = method
    ? buildMethodPrompt(method, context)
    : promptMethods.value.length === 1
      ? buildMethodPrompt(promptMethods.value[0], context)
      : buildSelectedPrompt(promptMethods.value, context)
  const ok = await copyText(text)
  if (ok) {
    message.success(method ? `${aiMethodLabel(method)}专用提示词已复制` : '所选方式的 AI 提示词已复制')
    promptShow.value = false
  } else {
    message.error('复制失败，请下载统一 Skill 文档')
  }
}

function selectAllPromptMethods() {
  promptMethods.value = INTEGRATION_METHODS.map((item) => item.key)
}

function clearPromptMethods() {
  promptMethods.value = []
}

function downloadSkill() {
  const blob = new Blob([buildSkillPrompt(buildGenericContext())], { type: 'text/markdown;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = 'smartdoc-integration-skill.md'
  a.click()
  URL.revokeObjectURL(url)
}

async function copySkillDocsURL() {
  const ok = await copyText(skillDocsURL)
  if (ok) message.success('文档地址已复制')
}

function scrollTo(id: string) {
  activeSection.value = id
  document.getElementById(`help-${id}`)?.scrollIntoView({ behavior: 'smooth', block: 'start' })
}

onMounted(() => {
  const hash = window.location.hash.replace('#', '')
  if (hash) scrollTo(hash)
})

const authCanonical = `HTTPMethod + "\\n" +
Path + "\\n" +
Timestamp + "\\n" +
Nonce + "\\n" +
SHA256HEX(RequestBody)

# Sign = HMAC-SHA256(AppSecret, CanonicalString) 的小写十六进制
# 必填 Header：
# X-Api-Key: <项目 AppKey>
# X-Timestamp: <毫秒时间戳，5 分钟有效>
# X-Nonce: <8~128 位随机串，10 分钟防重放>
# X-Signature: <Sign>`

const pythonSign = `import hashlib, hmac, json, time, uuid

body = json.dumps({
    "biz_key": "archive:2026:0001",
    "request_id": "audit-20261031-000001",
    "media": [
        {"type": "text", "content": "第一页档案文本"},
        {"type": "text", "content": "第二页档案文本"},
        {"type": "image", "url": "https://your-domain.com/page-1.jpg"},
        {"type": "image", "url": "https://your-domain.com/page-2.jpg"},
    ],
    "ext": {"scene": "archive_review"},
}, ensure_ascii=False).encode("utf-8")

path = "/open/v1/projects/smartdoc-demo/audits/archive_review/submit"
ts = str(int(time.time() * 1000))
nonce = str(uuid.uuid4())
body_hash = hashlib.sha256(body).hexdigest()
canonical = f"POST\\n{path}\\n{ts}\\n{nonce}\\n{body_hash}"
signature = hmac.new(APP_SECRET.encode(), canonical.encode(), hashlib.sha256).hexdigest()`

const submitBody = `{
  "biz_key": "archive:2026:0001",
  "request_id": "audit-20261031-000001",
  "media": [
    { "type": "text",  "name": "page-1.txt", "content": "第一页档案文本或 OCR 结果" },
    { "type": "text",  "name": "page-2.txt", "content": "第二页档案文本或 OCR 结果" },
    { "type": "image", "name": "page-1.jpg", "url": "https://your-domain.com/page-1.jpg" },
    { "type": "image", "name": "page-2.jpg", "url": "https://your-domain.com/page-2.jpg" }
  ],
  "ext": { "scene": "archive_review", "department": "archive" },
  "callback_url": "https://your-service.example.com/callback",
  "mode": "async",
  "wait_ms": 3000
}`

const submitResponse = `{
  "code": 0,
  "message": "OK",
  "data": {
    "task_id": 12,
    "task_no": "SDV20261021190000ABCDEF",
    "request_id": "audit-20261031-000001",
    "status": "QUEUED",
    "result": "UNKNOWN",
    "deduplicated": false,
    "sync": false,
    "task": { "...": "任务摘要，同步模式下包含最终 result_detail" }
  },
  "trace_id": "..."
}`

const queryResponse = `{
  "task_id": 12,
  "task_no": "SDV20261021190000ABCDEF",
  "request_id": "audit-20261031-000001",
  "status": "SUCCESS",
  "result": "REJECT",
  "result_detail": {
    "summary": "命中文档完整性风险",
    "reasons": ["关键页缺失", "印章无法辨认"],
    "labels": ["内容安全", "文档不完整"],
    "risk_level": "high",
    "score": 0.93,
    "category": "contract",
    "category_name": "合同协议",
    "category_confidence": 0.92,
    "rule": { "matched_rule": "高风险直接驳回" }
  },
  "input": {
    "biz_key": "archive:2026:0001",
    "request_id": "audit-20261031-000001",
    "media": [{ "type": "text", "content": "档案正文或 OCR 文本" }]
  }
}`

const redisMessage = `redis-cli -h 127.0.0.1 -p 6379 -n <DB> XADD sdv:smartdoc-demo:audit:archive_review * request_id 'audit-20261031-000001' callback_url 'https://your-service.example.com/callback' data '{"request_id":"audit-20261031-000001","biz_key":"archive:2026:0001","media":[{"type":"text","content":"待审核文本"}],"ext":{"scene":"archive_review"}}'

# DB 由审核触发源选择，必须与「数据连接」中的 Redis 连接指向同一实例
# Stream 由平台消费，消费组默认为 sdv-cg-<audit_id>
# request_id 建议在消息顶层和 data 中同时填写；平台通过 audit.accepted / deduplicated / completed / submit_failed HTTP 回调通知，不写业务 Redis 结果流
# 消息必须是 data 字段（也兼容 payload），值为 TaskInput JSON
# 处理失败的消息会写入 <stream>:dlq`

const wsSign = `import hashlib, hmac, time, uuid

path = "/open/v1/ws/projects/smartdoc-demo/audits/archive_review"
ts = str(int(time.time() * 1000))
nonce = str(uuid.uuid4())
canonical = f"GET\\n{path}\\n{ts}\\n{nonce}\\n" + hashlib.sha256(b"").hexdigest()
sign = hmac.new(APP_SECRET.encode(), canonical.encode(), hashlib.sha256).hexdigest()

url = f"ws://127.0.0.1:8080{path}?app_key={APP_KEY}&timestamp={ts}&nonce={nonce}&sign={sign}"`

const wsFrames = `// 客户端 → 平台
{
  "type": "submit",
  "data": {
    "biz_key": "archive:2026:0001",
    "request_id": "audit-20261031-000001",
    "media": [{"type": "text", "content": "待审核的档案文本"}]
  }
}

// 平台 → 客户端（受理）
{
  "type": "ack",
  "data": {
    "task_id": 12,
    "task_no": "SDV...",
    "request_id": "audit-20261031-000001",
    "status": "QUEUED",
    "deduplicated": false
  }
}

// 平台 → 客户端（异步结果，status 变为终态后推送）
{
  "type": "result",
  "data": { "task_no": "SDV...", "status": "SUCCESS", "result": "REJECT", "result_detail": {} }
}`

const callbackPayload = `{
  "event": "audit.completed",
  "task_id": 12,
  "task_no": "SDV...",
  "request_id": "audit-20261031-000001",
  "source_ref": "1792578000000-0",
  "project_code": "smartdoc-demo",
  "audit_code": "archive_review",
  "biz_key": "archive:2026:0001",
  "status": "SUCCESS",
  "result": "REJECT",
  "category": "contract",
  "category_name": "合同协议",
  "category_confidence": 0.92,
  "summary": "命中文档完整性风险",
  "reasons": ["关键页缺失"],
  "labels": ["内容安全"],
  "risk_level": "high",
  "score": 0.93,
  "media": [
    {"type": "text", "content": "档案正文或 OCR 文本"},
    {"type": "image", "url": "https://.../page-01.jpg"}
  ],
  "ext": {"scene": "archive_review"},
  "finished_at": "2026-10-21T19:00:02+08:00"
}

# Header
# X-SmartDoc-Signature: sha256=HMAC-SHA256(callback_secret, raw_body)
# X-SmartDoc-Event: audit.completed
# X-SmartDoc-Result: REJECT
# X-Idempotency-Key: SDV...`

const callbackVerify = `import hashlib, hmac

expected = "sha256=" + hmac.new(secret.encode(), raw_body, hashlib.sha256).hexdigest()
actual = request.headers.get("X-SmartDoc-Signature", "")
if not hmac.compare_digest(expected, actual):
    raise HTTPException(status_code=401, detail="invalid signature")`
</script>

<template>
  <div class="page help-page">
    <PageHeader
      eyebrow="INTEGRATION GUIDE"
      title="对接文档"
      subtitle="项目级鉴权、三种触发方式、任务参数、WebSocket 协议、回调签名与 MySQL 边界说明。"
    >
      <template #actions>
        <n-button secondary @click="openPromptPicker">
          <template #icon><SparklesOutline /></template>
          按方式复制 AI 提示词
        </n-button>
        <n-button type="primary" @click="downloadSkill">
          <template #icon><DownloadOutline /></template>
          下载统一 Skill.md
        </n-button>
        <n-button secondary @click="$router.push({ name: 'audits' })">
          进入审核配置 <ChevronForwardOutline class="arrow" />
        </n-button>
      </template>
    </PageHeader>

    <div class="help-layout">
      <aside class="help-nav panel">
        <div class="help-nav-title">目录</div>
        <button
          v-for="section in sections"
          :key="section.id"
          type="button"
          :class="{ active: activeSection === section.id }"
          @click="scrollTo(section.id)"
        >
          <component :is="section.icon" />
          {{ section.label }}
        </button>
        <div class="help-nav-tip">
          每个审核配置可在「审核配置 → 接入指南」查看带当前项目和审核编码的动态示例。
        </div>
      </aside>

      <div class="help-content">
        <section id="help-overview" class="doc-section">
          <div class="doc-kicker">01 / OVERVIEW</div>
          <h2>接入总览</h2>
          <p>每个审核配置独立拥有三种触发方式开关，管理员可在后台按业务需要启用。所有触发方式最终都会生成同一种 <code>TaskInput</code>，进入统一审核引擎。</p>
          <div class="trigger-cards">
            <div class="trigger-card"><span>API</span><strong>开放 API</strong><p>业务方主动提交，HMAC 签名、防重放，可同步等待结果。</p></div>
            <div class="trigger-card"><span>REDIS</span><strong>Redis Stream</strong><p>从项目「数据连接」中选择 Redis，监听指定 DB 与 Stream；一个队列只允许一个审核消费。</p></div>
            <div class="trigger-card"><span>WS</span><strong>WebSocket</strong><p>双向长连接，提交后返回 ACK，并主动推送终态结果。</p></div>
          </div>
          <div class="doc-warning" style="margin-top: 12px;">
            <strong>AI 对接助手：</strong>「按方式复制 AI 提示词」会先让你选择开放 API / Redis / WebSocket（可多选），只生成对应实现要求；「下载统一 Skill.md」包含三种方式，并会要求编码助手先询问你采用哪一种或哪几种。
          </div>
          <div class="doc-warning">
            <strong>MySQL 不是触发方式。</strong>平台自身用 MySQL 存储任务、审核记录与审计日志；如果需要从业务 MySQL 获取数据，请由业务方查询后通过开放 API、Redis Stream 或 WebSocket 推送。平台不支持 HTTP/MySQL 轮询。
          </div>
        </section>

        <section id="help-auth" class="doc-section">
          <div class="doc-kicker">02 / CREDENTIALS</div>
          <h2>凭证与权限</h2>
          <p>开放 API 与 WebSocket 使用项目内「接入凭证」页面创建的 <strong>AppKey / AppSecret</strong>。AppKey 标识项目，AppSecret 只保存在服务端和业务系统的安全配置中，不随请求传输。</p>
          <ul class="doc-list">
            <li>项目下可以创建多条接入凭证，每条凭证可勾选允许访问的审核配置。</li>
            <li>调用时同时校验：凭证有效、项目匹配、审核配置存在且启用、审核配置在该凭证的授权列表中。</li>
            <li>审核配置删除、凭证停用或凭证未授权时，访问会被拒绝。</li>
            <li>AppSecret 使用 AES-256-GCM 加密落库；后台仅展示脱敏值。首次生成或轮换时显示一次明文。</li>
            <li>轮换凭证后旧 AppSecret 立即失效，需同步更新所有业务系统。</li>
            <li>「接入凭证」页面支持创建多条凭证、选择授权审核配置、启用/停用、轮换和删除。</li>
          </ul>
          <h3>签名规范</h3>
          <code-block :code="authCanonical" title="Canonical String" />
          <p class="doc-note">签名使用 HMAC-SHA256，输出小写十六进制。时间戳窗口 5 分钟，Nonce 在 Redis 中记录 10 分钟，重复使用返回 <code>REPLAY_ATTACK</code>。</p>
          <code-block :code="pythonSign" title="Python 签名示例" language="python" />
        </section>

        <section id="help-openapi" class="doc-section">
          <div class="doc-kicker">03 / OPEN API</div>
          <h2>开放 API</h2>
          <p>业务方主动提交任务。路径中的项目编码与审核编码必须属于当前 AppKey 的项目，且审核配置已启用开放 API。</p>
          <div class="doc-inline-action">
            <span>让编码助手只实现 HMAC 提交客户端、任务查询和签名错误处理。</span>
            <n-button size="tiny" secondary @click="copyPromptFor('OPENAPI')">
              <template #icon><SparklesOutline /></template>
              复制开放 API 专用提示词
            </n-button>
          </div>
          <div class="endpoint">
            <span class="method">POST</span>
            <code>/open/v1/projects/{project_code}/audits/{audit_code}/submit</code>
          </div>
          <h3>请求参数</h3>
          <table class="doc-table">
            <thead><tr><th>参数</th><th>必填</th><th>说明</th></tr></thead>
            <tbody>
              <tr><td><code>request_id</code></td><td>强烈建议</td><td>调用方生成的唯一提交 ID；平台按它幂等。相同 request_id + 相同内容返回原任务，内容不同返回 409 REQUEST_ID_CONFLICT。回调、结果帧和查询均返回该值。</td></tr>
              <tr><td><code>biz_key</code></td><td>建议</td><td>业务分组键；未填写 request_id 时，相同业务键 + 相同媒体内容在去重窗口内只生成一个任务。</td></tr>
              <tr><td><code>media</code></td><td>是</td><td>媒体数组，至少一项。type 支持 text / image。text 使用 content，image 使用 url。</td></tr>
              <tr><td><code>ext</code></td><td>否</td><td>业务扩展对象，规则可通过 <code>ext.*</code> 访问，回调会原样带回。</td></tr>
              <tr><td><code>callback_url</code></td><td>否</td><td>任务级回调地址；优先于审核配置的回调地址。</td></tr>
              <tr><td><code>mode</code></td><td>否</td><td><code>async</code>（默认）或 <code>sync</code>。sync 最多等待 wait_ms。</td></tr>
              <tr><td><code>wait_ms</code></td><td>否</td><td>sync 模式等待毫秒数，默认 3000，最大 10000。</td></tr>
            </tbody>
          </table>
          <h3>请求体示例</h3>
          <code-block :code="submitBody" title="JSON Body" />
          <h3>响应</h3>
          <code-block :code="submitResponse" title="Submit Response" />
          <h3>查询任务结果</h3>
          <p>使用同样的签名规范，方法为 GET，Body 为空字符串（SHA256 为空内容的哈希）。</p>
          <div class="endpoint">
            <span class="method get">GET</span>
            <code>/open/v1/projects/{project_code}/tasks/{task_no}</code>
          </div>
          <p>也可以按 request_id 查询：</p>
          <div class="endpoint">
            <span class="method get">GET</span>
            <code>/open/v1/projects/{project_code}/task-by-request-id/{request_id}</code>
          </div>
          <code-block :code="queryResponse" title="Task Result" />
          <p class="doc-note">结果中包含 <code>result_detail.reasons</code>、<code>labels</code>、<code>score</code>、<code>rule</code>，并可选择包含原始媒体。不合格内容可通过这些字段回传业务方。</p>
        </section>

        <section id="help-redis" class="doc-section">
          <div class="doc-kicker">04 / REDIS</div>
          <h2>Redis Stream 触发</h2>
          <p>审核配置启用 Redis 触发后，平台使用当前项目「数据连接」中选定的 Redis 连接和 DB 消费指定 Stream。业务方负责 XADD，平台负责消费、去重、审核和回调。</p>
          <div class="doc-inline-action">
            <span>让编码助手只实现消息生产端，并明确结果需通过回调获取。</span>
            <n-button size="tiny" secondary @click="copyPromptFor('REDIS')">
              <template #icon><SparklesOutline /></template>
              复制 Redis 专用提示词
            </n-button>
          </div>
          <table class="doc-table">
            <thead><tr><th>配置</th><th>默认</th><th>说明</th></tr></thead>
            <tbody>
              <tr><td>connection</td><td>—</td><td>当前项目「数据连接」中已启用且测试成功的 Redis 连接。</td></tr>
              <tr><td>db</td><td>0</td><td>连接实例内的 DB 编号，0~255。</td></tr>
              <tr><td>stream</td><td><code>sdv:{project_code}:audit:{audit_code}</code></td><td>业务写入的 Stream 键。</td></tr>
              <tr><td>group</td><td><code>sdv-cg-{audit_id}</code></td><td>平台消费组，支持多实例负载均衡。</td></tr>
              <tr><td>start_position</td><td><code>$</code></td><td><code>$</code> 只消费新消息，<code>0</code> 从头消费。</td></tr>
              <tr><td>batch_size</td><td>16</td><td>单次读取消息数量，1~32。</td></tr>
              <tr><td>max_inflight</td><td>1</td><td>同一审核同时处理的消息数，1~32。</td></tr>
            </tbody>
          </table>
          <h3>消息格式</h3>
          <p>Stream 消息必须是单个 <code>data</code> 字段（兼容 <code>payload</code>），值为 <code>TaskInput</code> JSON。非法消息会被 ACK 并写入 <code>{stream}:dlq</code>。</p>
          <code-block :code="redisMessage" title="Redis Commands" />
          <div class="doc-warning">
            业务生产端与平台消费必须使用同一个项目 Redis 连接和同一个 DB。连接停用或网络故障会自动暂停触发源；恢复后按连接的「自动恢复触发源」配置恢复，或手动重新开启触发源。
          </div>
        </section>

        <section id="help-websocket" class="doc-section">
          <div class="doc-kicker">05 / WEBSOCKET</div>
          <h2>WebSocket 触发</h2>
          <p>WebSocket 连接同样使用项目级凭证。由于浏览器原生 WebSocket 无法自定义 Header，鉴权参数放在 query string 中；签名使用空请求体的 SHA256。</p>
          <div class="doc-inline-action">
            <span>让编码助手实现连接签名、ack/result 关联、心跳和断线重连。</span>
            <n-button size="tiny" secondary @click="copyPromptFor('WEBSOCKET')">
              <template #icon><SparklesOutline /></template>
              复制 WebSocket 专用提示词
            </n-button>
          </div>
          <h3>签名与连接</h3>
          <code-block :code="wsSign" title="连接签名" language="python" />
          <div class="endpoint">
            <span class="method get">WS</span>
            <code>/open/v1/ws/projects/{project_code}/audits/{audit_code}?app_key=&amp;timestamp=&amp;nonce=&amp;sign=</code>
          </div>
          <h3>帧协议</h3>
          <code-block :code="wsFrames" title="Frame Examples" />
          <p class="doc-note">连接后平台先发送 <code>ready</code> 帧。心跳使用 <code>ping</code> 帧或 WebSocket Ping/Pong；90 秒无响应会断开。终态结果（SUCCESS / FAILED / REVIEW_PENDING）会通过 <code>result</code> 帧主动推送。</p>
        </section>

        <section id="help-callback" class="doc-section">
          <div class="doc-kicker">07 / CALLBACK</div>
          <h2>审核结果回调</h2>
          <p>审核配置启用回调后，最终结果（通过、驳回，或人工复核后的结论）会以 JSON POST 到业务方地址。人工复核前处于待议状态不会回调，除非任务被处理为终态。</p>
          <table class="doc-table">
            <thead><tr><th>配置</th><th>说明</th></tr></thead>
            <tbody>
              <tr><td>callback_url</td><td>业务回调地址；任务级 callback_url 可覆盖。</td></tr>
              <tr><td>callback_results</td><td>需要回调的结论集合：PASS / REJECT / REVIEW。默认待议任务经人工复核后再回调；配置「外部系统复核」时勾选 REVIEW，待议任务会直接完成并回调通知业务方自行复核。</td></tr>
              <tr><td>callback_secret</td><td>HMAC 签名密钥，加密保存。</td></tr>
              <tr><td>callback_include_content</td><td>是否回传原文/文本内容；关闭时只回传 URL 和元信息。</td></tr>
            </tbody>
          </table>
          <h3>回调体</h3>
          <code-block :code="callbackPayload" title="Callback Payload" />
          <h3>验签</h3>
          <code-block :code="callbackVerify" title="Verify Signature" language="python" />
          <p class="doc-note">回调失败会按 1s / 2s / 4s 重试 3 次。事件包括 audit.accepted / deduplicated / completed / submit_failed；Header 会带 X-SmartDoc-Request-Id 和幂等键，业务方应按 request_id 优先、task_no 兜底去重。</p>
        </section>

        <section id="help-mysql" class="doc-section">
          <div class="doc-kicker">08 / MYSQL</div>
          <h2>MySQL 边界与推荐做法</h2>
          <p>SmartDoc Vision 的 MySQL 存储的是平台元数据，不直接连接业务 MySQL。三种触发方式中没有“直接查询业务 MySQL”的模式，原因如下：</p>
          <ul class="doc-list">
            <li>平台不应持有业务库的长期账号，避免权限扩散与审计盲区；</li>
            <li>不同业务表结构、增量字段、审核字段差异大，无法用统一 SQL 安全表达；</li>
            <li>业务方可控的查询逻辑可以做鉴权、字段脱敏、限流和只读保护。</li>
          </ul>
          <p>推荐方案：业务方查询自己的 MySQL 后，通过开放 API、Redis Stream 或 WebSocket 主动推送 TaskInput。平台不再提供 HTTP/MySQL 定时轮询。</p>
          <div class="doc-warning">平台不直连业务 MySQL。如果确实需要数据库读取，请由业务服务完成查询、脱敏和增量控制，再把数据推送到平台。</div>
        </section>

        <section class="doc-section">
          <div class="doc-kicker">HELP</div>
          <h2>还需要帮助？</h2>
          <p>每个审核配置的「接入指南」会根据当前项目编码、审核编码和 AppKey 生成可直接复制执行的示例。你可以在审核配置页点击「接入」打开。</p>
          <n-button type="primary" @click="$router.push({ name: 'audits' })">前往审核配置</n-button>
        </section>
      </div>
    </div>

    <n-modal v-model:show="promptShow">
      <div class="prompt-modal">
        <div class="prompt-modal-head">
          <div>
            <span class="prompt-kicker">AI INTEGRATION HELPER</span>
            <h2>选择要生成的接入方式</h2>
            <p>每种方式生成不同的提示词；可以只选一种，也可以多选生成组合 Skill。统一 Skill 下载仍包含全部方式，并会要求 AI 先询问。</p>
          </div>
          <span class="prompt-icon"><SparklesOutline /></span>
        </div>
        <div class="prompt-modal-body">
          <div class="prompt-option-head">
            <span>接入方式（可多选）</span>
            <div>
              <n-button text size="tiny" type="primary" @click="selectAllPromptMethods">全选</n-button>
              <n-button text size="tiny" @click="clearPromptMethods">清空</n-button>
            </div>
          </div>
          <n-checkbox-group v-model:value="promptMethods">
            <div class="prompt-option-grid">
              <div v-for="method in aiMethodOptions" :key="method.key" class="prompt-option">
                <n-checkbox :value="method.key" />
                <div>
                  <strong>{{ method.label }}</strong>
                  <span>{{ method.short }}</span>
                </div>
              </div>
            </div>
          </n-checkbox-group>
          <div class="prompt-hint">
            <strong>为什么分开？</strong>
            <span>开放 API 是 HMAC 提交端；Redis 是消息生产端；WebSocket 是长连接客户端。混在一份泛化提示词里容易让 AI 生成错误方向的代码。</span>
          </div>
        </div>
        <div class="prompt-modal-footer">
          <n-button @click="promptShow = false">取消</n-button>
          <n-button type="primary" :disabled="!promptMethods.length" @click="copyPromptFor()">
            <template #icon><SparklesOutline /></template>
            复制所选方式提示词
          </n-button>
        </div>
      </div>
    </n-modal>
  </div>
</template>

<style scoped>
.help-layout {
  display: grid;
  grid-template-columns: 210px minmax(0, 1fr);
  gap: 20px;
  align-items: start;
}

.help-nav {
  position: sticky;
  top: 80px;
  padding: 15px 10px 13px;
}

.help-nav-title {
  padding: 0 10px 8px;
  color: var(--muted-light);
  font-size: 9px;
  font-weight: 650;
  letter-spacing: 0.1em;
  text-transform: uppercase;
}

.help-nav button {
  display: flex;
  width: 100%;
  align-items: center;
  gap: 8px;
  padding: 7px 9px;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: var(--muted);
  font-size: 11px;
  text-align: left;
  cursor: pointer;
}

.help-nav button:hover,
.help-nav button.active {
  background: var(--brand-soft);
  color: var(--brand-strong);
}

.help-nav button :deep(svg) {
  width: 13px;
  height: 13px;
}

.help-nav-tip {
  margin: 13px 7px 0;
  padding-top: 11px;
  border-top: 1px solid #f0f3f8;
  color: var(--muted-light);
  font-size: 9px;
  line-height: 1.7;
}

.help-content {
  min-width: 0;
}

.doc-section {
  margin-bottom: 16px;
  padding: 22px 24px;
  border: 1px solid var(--border);
  border-radius: 16px;
  background: #fff;
  box-shadow: var(--shadow-sm);
  scroll-margin-top: 80px;
}

.doc-kicker {
  color: var(--brand);
  font-family: var(--mono);
  font-size: 8.5px;
  font-weight: 650;
  letter-spacing: 0.12em;
}

.doc-section h2 {
  margin: 5px 0 8px;
  color: var(--ink);
  font-size: 20px;
  font-weight: 650;
  letter-spacing: -0.02em;
}

.doc-section h3 {
  margin: 20px 0 8px;
  color: var(--text);
  font-size: 13px;
  font-weight: 620;
}

.doc-section p {
  margin: 0 0 10px;
  color: var(--muted);
  font-size: 12px;
  line-height: 1.8;
}

.doc-inline-action {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  margin: 2px 0 14px;
  padding: 9px 12px;
  border: 1px solid #e9e4fb;
  border-radius: 9px;
  background: #fbfaff;
}

.doc-inline-action span {
  color: #75658f;
  font-size: 10.5px;
  line-height: 1.6;
}

.doc-section code {
  padding: 1px 4px;
  border-radius: 4px;
  background: #f1f5f9;
  color: #475569;
  font-family: var(--mono);
  font-size: 10px;
}

.doc-note {
  padding: 9px 11px;
  border-left: 2px solid #bcd0f8;
  background: #f9fbff;
  color: #64748b !important;
  font-size: 10.5px !important;
}

.trigger-cards {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 8px;
  margin: 15px 0;
}

.trigger-card {
  padding: 12px;
  border: 1px solid #edf1f7;
  border-radius: 10px;
  background: #fbfcfe;
}

.trigger-card > span {
  color: var(--brand);
  font-family: var(--mono);
  font-size: 8px;
  font-weight: 650;
  letter-spacing: 0.08em;
}

.trigger-card strong {
  display: block;
  margin-top: 3px;
  color: var(--ink);
  font-size: 11px;
  font-weight: 600;
}

.trigger-card p {
  margin: 4px 0 0;
  color: var(--muted-light);
  font-size: 9.5px;
  line-height: 1.6;
}

.doc-warning {
  margin: 12px 0;
  padding: 11px 13px;
  border: 1px solid #f4e1bb;
  border-radius: 9px;
  background: #fffbf2;
  color: #8a5a12;
  font-size: 10.5px;
  line-height: 1.7;
}

.doc-warning strong {
  font-weight: 650;
}

.doc-list {
  margin: 8px 0 12px;
  padding-left: 17px;
  color: var(--muted);
  font-size: 11.5px;
  line-height: 1.8;
}

.doc-list li {
  margin-bottom: 3px;
}

.doc-table {
  width: 100%;
  margin: 8px 0 14px;
  border-collapse: collapse;
  font-size: 10.5px;
}

.doc-table th {
  padding: 7px 9px;
  border-bottom: 1px solid #eef1f6;
  color: var(--muted-light);
  font-size: 9px;
  font-weight: 600;
  text-align: left;
  text-transform: uppercase;
}

.doc-table td {
  padding: 8px 9px;
  border-bottom: 1px solid #f4f6fa;
  color: var(--muted);
  line-height: 1.6;
  vertical-align: top;
}

.doc-table tr:last-child td {
  border-bottom: 0;
}

.endpoint {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 9px 0 13px;
  padding: 9px 11px;
  border: 1px solid #e5ebf4;
  border-radius: 8px;
  background: #f8fafc;
}

.method {
  padding: 2px 6px;
  border-radius: 4px;
  background: var(--brand-soft);
  color: var(--brand-strong);
  font-family: var(--mono);
  font-size: 8.5px;
  font-weight: 650;
}

.method.get {
  background: #eafaf2;
  color: #15803d;
}

.endpoint code {
  color: #475569;
  font-size: 10px;
  word-break: break-all;
}

.arrow {
  width: 11px;
  height: 11px;
  vertical-align: -1px;
}

.prompt-modal {
  width: min(620px, calc(100vw - 32px));
  overflow: hidden;
  border-radius: 18px;
  background: #fff;
  box-shadow: 0 30px 80px -20px rgba(15, 23, 42, 0.35);
}

.prompt-modal-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 18px;
  padding: 23px 26px 18px;
  border-bottom: 1px solid #f0f3f8;
  background: linear-gradient(130deg, #faf8ff, #fff);
}

.prompt-kicker {
  color: var(--purple);
  font-family: var(--mono);
  font-size: 8.5px;
  font-weight: 650;
  letter-spacing: 0.12em;
}

.prompt-modal-head h2 {
  margin: 4px 0 4px;
  color: var(--ink);
  font-size: 19px;
  font-weight: 660;
  letter-spacing: -0.02em;
}

.prompt-modal-head p {
  max-width: 440px;
  margin: 0;
  color: var(--muted);
  font-size: 11px;
  line-height: 1.65;
}

.prompt-icon {
  display: flex;
  width: 40px;
  height: 40px;
  align-items: center;
  justify-content: center;
  border-radius: 11px;
  background: var(--purple-soft);
  color: var(--purple);
  flex-shrink: 0;
}

.prompt-icon :deep(svg) {
  width: 18px;
  height: 18px;
}

.prompt-modal-body {
  padding: 18px 26px;
}

.prompt-option-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 8px;
  color: var(--text);
  font-size: 11.5px;
  font-weight: 550;
}

.prompt-option-head > div {
  display: flex;
  gap: 8px;
}

.prompt-option-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 8px;
}

.prompt-option {
  display: flex;
  align-items: flex-start;
  gap: 7px;
  padding: 11px;
  border: 1px solid #e9eef6;
  border-radius: 9px;
  background: #fbfcfe;
  cursor: pointer;
}

.prompt-option:has(.n-checkbox--checked) {
  border-color: #c7b8f3;
  background: #fbfaff;
}

.prompt-option strong {
  display: block;
  color: var(--text);
  font-size: 11px;
  font-weight: 600;
}

.prompt-option span {
  display: block;
  margin-top: 2px;
  color: var(--muted-light);
  font-size: 9px;
}

.prompt-hint {
  display: flex;
  gap: 8px;
  margin-top: 14px;
  padding: 10px 11px;
  border-radius: 8px;
  background: #f8fafc;
  color: var(--muted);
  font-size: 9.5px;
  line-height: 1.65;
}

.prompt-hint strong {
  color: var(--text);
  flex-shrink: 0;
}

.prompt-modal-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding: 14px 26px;
  border-top: 1px solid #f0f3f8;
}

@media (max-width: 1100px) {
  .help-layout {
    grid-template-columns: 1fr;
  }

  .help-nav {
    position: static;
    display: flex;
    overflow-x: auto;
    align-items: center;
    gap: 3px;
    padding: 8px;
  }

  .help-nav-title,
  .help-nav-tip {
    display: none;
  }

  .help-nav button {
    width: auto;
    flex-shrink: 0;
  }

  .trigger-cards {
    grid-template-columns: 1fr 1fr;
  }
}

@media (max-width: 650px) {
  .doc-section {
    padding: 18px 16px;
  }

  .trigger-cards {
    grid-template-columns: 1fr;
  }

  .doc-inline-action {
    align-items: flex-start;
    flex-direction: column;
  }

  .prompt-option-grid {
    grid-template-columns: 1fr;
  }

  .prompt-modal-head,
  .prompt-modal-body,
  .prompt-modal-footer {
    padding-left: 18px;
    padding-right: 18px;
  }

  .prompt-modal-footer {
    flex-wrap: wrap;
  }
}
</style>
