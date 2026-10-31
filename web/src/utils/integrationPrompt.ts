import type { AuditCategory } from '@/api/types'

export type IntegrationMethod = 'OPENAPI' | 'REDIS' | 'WEBSOCKET'

export interface IntegrationMethodMeta {
  key: IntegrationMethod
  label: string
  short: string
  docsAnchor: string
}

export const INTEGRATION_METHODS: IntegrationMethodMeta[] = [
  { key: 'OPENAPI', label: '开放 API', short: 'HMAC 主动提交', docsAnchor: 'openapi' },
  { key: 'REDIS', label: 'Redis Stream', short: '队列写入消息', docsAnchor: 'redis' },
  { key: 'WEBSOCKET', label: 'WebSocket', short: '长连接提交与推送', docsAnchor: 'websocket' },
]

export interface IntegrationPromptContext {
  origin: string
  projectCode: string
  auditCode: string
  auditName: string
  docsURL: string
  credentialName?: string
  appKey?: string
  appSecretEnv?: string
  openapiPath?: string
  openapiEnabled?: boolean
  sampleBody?: string
  sampleCurl?: string
  redisEnabled?: boolean
  redisStream?: string
  redisGroup?: string
  websocketEnabled?: boolean
  websocketPath?: string
  websocketURL?: string
  callback?: {
    enabled: boolean
    url?: string
    results?: string[]
    includeContent?: boolean
    secretSet?: boolean
  }
  decisionMode?: string
  categories?: AuditCategory[] | null
}

function methodLabel(method: IntegrationMethod): string {
  return INTEGRATION_METHODS.find((item) => item.key === method)?.label || method
}

function scopeLine(methods: IntegrationMethod[]): string {
  if (!methods.length) {
    return '开始前必须先询问业务方：要接入哪一种或哪几种触发方式（开放 API / Redis Stream / WebSocket）？是否还需要结果回调？在得到明确选择前不要编写代码。'
  }
  if (methods.length === 1) {
    return `本次只实现【${methodLabel(methods[0])}】这一种接入方式，不要生成其他触发方式的提交代码。`
  }
  return `本次实现以下 ${methods.length} 种接入方式：${methods.map(methodLabel).join('、')}。请分别提供独立适配器，并抽取统一的 TaskInput 构造、错误处理和日志脱敏层；支持业务方按配置选择其中一种或多种运行。`
}

function classificationSection(ctx: IntegrationPromptContext): string {
  if (ctx.decisionMode !== 'CLASSIFY' && ctx.decisionMode !== 'CLASSIFY_ONLY') return ''
  const categories = ctx.categories || []
  const mapping = categories.length
    ? categories
        .map((category) => `- ${category.code}：${category.name} → ${category.result}${category.description ? `（${category.description}）` : ''}`)
        .join('\n')
    : '- 当前配置未读取到分类列表，请提醒业务方在审核配置中补齐。'
  const pure = ctx.decisionMode === 'CLASSIFY_ONLY'
  return `
## 分类审核结果（必须处理）

这是一条${pure ? '纯分类器' : '分类审核'}配置，平台结果除 result 外还会返回：

- category：分类编码
- category_name：分类名称
- category_confidence：0~1 的分类置信度

分类映射：
${mapping}

业务端必须原样保存分类编码和置信度，不要自行根据名称重新分类，也不要把 category 当作 PASS/REJECT 使用；终审结论以 result 为准。${pure ? '\n\n纯分类器的成功审核结论固定为 PASS，category 就是业务需要的具体类型值。' : ''}
`
}

function commonHeader(ctx: IntegrationPromptContext, methods: IntegrationMethod[], title: string): string {
  const credentialName = ctx.credentialName || '尚未创建（请提醒业务方在「接入凭证」中创建并授权当前审核配置）'
  const appKey = ctx.appKey || 'APP_KEY（在接入凭证页创建后填写）'
  const docsURL = ctx.docsURL || `${ctx.origin}/help`
  const callback = ctx.callback
  const callbackText = callback?.enabled
    ? `已启用，地址 ${callback.url || '由业务方提供'}；回调结论 ${(callback.results || []).join(' / ') || '未配置'}；${callback.includeContent ? '包含原始内容' : '不包含原始内容'}；${callback.secretSet ? '已配置 HMAC 签名 Secret' : '未配置签名 Secret'}。`
    : '当前未启用；如果本次接入需要异步获知最终结果，请提醒业务方在接入指南中启用回调。'

  return `# ${title}

你是一名负责对接“智审图档 SmartDoc Vision”审核平台的资深后端工程师。请交付可运行代码、配置示例、错误处理和测试用例，而不是只给伪代码。

## 本次范围

${scopeLine(methods)}

## 对接目标

- 平台地址：${ctx.origin}
- 项目编码：${ctx.projectCode}
- 审核编码：${ctx.auditCode}
- 审核名称：${ctx.auditName || '未命名审核'}
- 接入凭证：${credentialName}
- AppKey：${appKey}
- AppSecret：从环境变量 ${ctx.appSecretEnv || 'SMARTDOC_APP_SECRET'} 读取，禁止写入代码、日志或示例响应
- 对接文档：${docsURL}

## 输入契约

平台只支持多段文本 + 多张图片，不支持视频、音频。统一输入结构：

~~~json
{
  "biz_key": "业务分组键，可选",
  "request_id": "调用方生成的项目内唯一提交 ID，强烈建议填写",
  "media": [
    {"type": "text", "name": "page-1.txt", "content": "第一页文本或 OCR 结果"},
    {"type": "text", "name": "page-2.txt", "content": "第二页文本或 OCR 结果"},
    {"type": "image", "name": "page-1.jpg", "url": "https://your-domain.example.com/page-1.jpg"},
    {"type": "image", "name": "page-2.jpg", "url": "https://your-domain.example.com/page-2.jpg"}
  ],
  "ext": {"scene": "archive_review"}
}
~~~

要求：图片 URL 必须是平台可访问的 http/https 地址；文本和图片都可以多条。request_id 是调用方生成的项目内唯一提交 ID：相同 request_id + 相同内容返回原任务并标记 duplicated，相同 request_id + 不同内容返回 REQUEST_ID_CONFLICT。填写 request_id 时平台按它做幂等；未填写时仍按 biz_key + media 的内容去重。
${classificationSection(ctx)}

## 结果回调

${callbackText}

回调 Header 包含 X-SmartDoc-Signature、X-SmartDoc-Event、X-SmartDoc-Task、X-SmartDoc-Request-Id、X-SmartDoc-Result、X-Idempotency-Key。事件包括 audit.accepted（Redis 受理）、audit.deduplicated（重复提交）、audit.completed（最终结果）和 audit.submit_failed（Redis 提交失败）。业务方必须用原始请求体验签，并按 request_id（优先）或 task_no 幂等落库。失败会按 1s / 2s / 4s 重试 3 次。

## 安全与验收

- 不要输出或记录 AppSecret、Token、完整敏感文档内容。
- 实现超时、有限重试、日志脱敏和单元测试。
- 如果缺少 AppSecret 或凭证未授权，请明确要求业务方通过安全环境变量提供，并到「接入凭证」页面授权当前审核配置。
- 完成后列出启动方式、配置项、测试命令和至少一个失败场景的验证结果。
`
}

function openapiSection(ctx: IntegrationPromptContext): string {
  const path = ctx.openapiPath || `/open/v1/projects/${ctx.projectCode}/audits/${ctx.auditCode}/submit`
  const body = ctx.sampleBody || `{
  "biz_key": "archive:2026:0001",
  "request_id": "audit-20261031-000001",
  "media": [
    {"type": "text", "name": "page-1.txt", "content": "第一页档案文本"},
    {"type": "image", "name": "page-1.jpg", "url": "https://your-domain.example.com/page-1.jpg"}
  ],
  "ext": {"scene": "archive_review"}
}`
  return `## 开放 API 实现要求

1. 提交入口：POST ${path}
2. 必填 Header：
   - X-Api-Key：AppKey
   - X-Timestamp：毫秒时间戳
   - X-Nonce：至少 8 位的随机串，10 分钟内不可重复
   - X-Signature：HMAC-SHA256(AppSecret, CanonicalString) 的小写十六进制
3. CanonicalString = HTTPMethod + "\\n" + Path + "\\n" + Timestamp + "\\n" + Nonce + "\\n" + SHA256HEX(原始 RequestBody 字节)。路径不含 query，Body 必须使用实际发送的同一份字节计算签名。
4. 请求体：
~~~json
${body}
~~~
5. 请求体必须包含业务方生成的 request_id；响应会原样返回 request_id 和 task_no。
6. 支持异步提交；如平台返回 queued，使用 GET /open/v1/projects/${ctx.projectCode}/tasks/{task_no} 或 GET /open/v1/projects/${ctx.projectCode}/task-by-request-id/{request_id} 查询终态。若业务需要同步等待，实现超时和轮询退避，不要无限阻塞。
7. 处理 duplicated 返回，复用已有 task_no；相同 request_id 但内容不同会返回 409 REQUEST_ID_CONFLICT，不要静默重试。
7. 处理 400、401 INVALID_SIGNATURE、403 AUDIT_OUT_OF_SCOPE / CREDENTIAL_DISABLED、409 REPLAY_ATTACK、429、5xx；签名错误不要盲目重试，时间戳过期需重新签名。
${ctx.sampleCurl ? `\n8. 平台生成的 cURL 示例（签名会过期，仅用于对照格式）：\n~~~bash\n${ctx.sampleCurl}\n~~~` : ''}

交付一个最小可运行的提交客户端 + 一个签名函数单测 + 一个针对 403/签名错误的异常处理测试。`
}

function redisSection(ctx: IntegrationPromptContext): string {
  const stream = ctx.redisStream || `sdv:${ctx.projectCode}:audit:${ctx.auditCode}`
  const group = ctx.redisGroup || '平台消费组（由平台维护）'
  return `## Redis Stream 实现要求

注意：业务方是消息生产方，平台是消费方。不要实现 HTTP 提交，也不要创建消费者的消费组。

1. 目标 Stream：${stream}
2. 平台消费组：${group}（平台负责创建和维护，业务方不要抢占消费组）
3. 使用一条 XADD 写入，消息必须包含 data 字段（也兼容 payload），值为 TaskInput JSON 字符串；强烈建议同时在消息顶层提供 request_id 和 callback_url，保证 data 解析失败时也能收到失败通知：
~~~bash
XADD ${stream} * request_id 'audit-20261031-000001' callback_url 'https://your-service.example.com/callback' data '{"request_id":"audit-20261031-000001","biz_key":"archive:2026:0001","media":[{"type":"text","name":"page-1.txt","content":"第一页档案文本"},{"type":"image","name":"page-1.jpg","url":"https://your-domain.example.com/page-1.jpg"}],"ext":{"scene":"archive_review"}}'
~~~
4. 消息体必须是 UTF-8 JSON，不支持的字段（video/audio）会导致平台拒收；非法消息会 ACK 后写入 ${stream}:dlq，并在可解析 callback_url 时发送 audit.submit_failed。
5. Redis 不返回 task_no。平台会在受理、去重、失败时通过消息级 callback_url 发送 audit.accepted / audit.deduplicated / audit.submit_failed；最终结果仍按审核配置级回调或消息级 callback_url 发送 audit.completed。事件里同时包含 request_id、task_no、source_ref（Redis message_id）。业务方也可以 GET /open/v1/projects/${ctx.projectCode}/task-by-request-id/{request_id} 查询。平台不写业务 Redis 结果流。
6. 相同 request_id + 相同内容会返回原任务并发送 deduplicated；相同 request_id + 不同内容会发送 submit_failed（REQUEST_ID_CONFLICT）。
7. 处理连接失败、超时、Redis 重连和重复写入；生产环境建议用连接池，并把消息体构造与 XADD 解耦，方便单测。
8. 交付：配置读取、TaskInput 序列化、XADD 发送函数、失败重试/日志脱敏、一个真实消息格式测试。`
}

function websocketSection(ctx: IntegrationPromptContext): string {
  const path = ctx.websocketPath || `/open/v1/ws/projects/${ctx.projectCode}/audits/${ctx.auditCode}`
  const url = ctx.websocketURL || `wss://your-smartdoc-host${path}`
  return `## WebSocket 实现要求

1. 连接地址：${url}
2. Query 参数：app_key、timestamp（毫秒）、nonce（10 分钟内不可重复）、sign。
3. 签名串：GET + "\\n" + Path + "\\n" + Timestamp + "\\n" + Nonce + "\\n" + SHA256HEX(空 Body)，其中 Path 不含 query；sign = HMAC-SHA256(AppSecret, canonical)，小写十六进制。
4. 连接建立后平台会发送 ready 帧。发送提交帧：
~~~json
{
  "type": "submit",
  "data": {
    "biz_key": "archive:2026:0001",
    "request_id": "audit-20261031-000001",
    "media": [
      {"type": "text", "name": "page-1.txt", "content": "第一页档案文本"},
      {"type": "image", "name": "page-1.jpg", "url": "https://your-domain.example.com/page-1.jpg"}
    ]
  }
}
~~~
5. 处理 ack 帧（task_no、request_id、status、duplicated）和 result 帧（终态 request_id、result、result_detail）。result 帧可能晚于 ack 到达，必须按 request_id 优先、task_no 兜底关联，不能只靠发送顺序。
6. 实现心跳 ping/pong、断线重连、重连后重新签名；重连恢复时使用 biz_key 幂等，避免重复审核。
7. 交付：连接鉴权函数、帧编解码、ack/result 关联表、超时和重连测试。`
}

function methodSection(method: IntegrationMethod, ctx: IntegrationPromptContext): string {
  switch (method) {
    case 'OPENAPI':
      return openapiSection(ctx)
    case 'REDIS':
      return redisSection(ctx)
    case 'WEBSOCKET':
      return websocketSection(ctx)
    default:
      return ''
  }
}

export function buildMethodPrompt(method: IntegrationMethod, ctx: IntegrationPromptContext): string {
  return `${commonHeader(ctx, [method], `SmartDoc Vision ${methodLabel(method)} 对接实现提示词`)}
${methodSection(method, ctx)}
`
}

export function buildSelectedPrompt(methods: IntegrationMethod[], ctx: IntegrationPromptContext): string {
  if (methods.length === 1) return buildMethodPrompt(methods[0], ctx)
  return buildSkillPrompt(ctx, methods)
}

export function buildSkillPrompt(ctx: IntegrationPromptContext, methods: IntegrationMethod[] = []): string {
  const selected = methods.length ? methods : INTEGRATION_METHODS.map((item) => item.key)
  const title = methods.length ? 'SmartDoc Vision 组合接入实现 Skill' : 'SmartDoc Vision 审核接入统一 Skill'
  const intro = methods.length
    ? commonHeader(ctx, methods, title)
    : `${commonHeader(ctx, [], title)}
> 这是一个统一 Skill，包含三种触发方式。请先向用户确认选择一种或多种方式，再只阅读和执行对应章节；也支持同时实现多种方式。`
  const sections = selected.map((method) => methodSection(method, ctx)).join('\n')
  const selectionHints = !methods.length
    ? `
## 选择向导（必须先执行）

1. 先问用户：“你要接入哪种方式？开放 API / Redis Stream / WebSocket，可多选。是否需要结果回调？”
2. 如果用户选择开放 API：实现 HMAC 签名提交客户端。
3. 如果用户选择 Redis：实现消息生产端；不要创建平台消费组；必须生成并在消息顶层和 data 中携带项目内唯一的 request_id，建议同时提供 callback_url 用于受理/去重/失败通知。
4. 如果用户选择 WebSocket：实现带签名 URL 的长连接客户端和 ack/result 关联。
5. 如果用户多选：为每种方式提供独立适配器，并共用 TaskInput 和结果处理层。
`
    : ''
  return `${intro}${selectionHints}
${sections}
`
}
