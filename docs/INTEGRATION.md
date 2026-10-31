# 智审图档 · 项目对接与接口规格

本文档面向业务系统开发者，说明如何向 SmartDoc Vision 提交审核、接收结果以及处理人工复核回调。

后台内置了同一份文档的可视化版本：登录后点击右上角「?」或侧边栏「对接文档」，也可以在「审核配置 → 接入指南」查看带当前项目编码的实时示例。

---

## 1. 基本概念

| 概念 | 说明 |
|---|---|
| 项目 Project | 配置、凭证、任务、统计的隔离边界。项目编码 `project_code` 全局唯一。 |
| 审核配置 Audit | 一套审核流程，以项目内唯一的审核编码 `audit_code` 标识。 |
| 触发方式 Trigger | 审核配置的输入开关：开放 API、Redis Stream、WebSocket。 |
| 任务 Task | 一次审核实例，`task_no` 全局唯一。 |
| 结果 Result | `PASS` 通过 / `REJECT` 不通过 / `REVIEW` 待议 / `UNKNOWN` 未判定。 |
| 待议复核 | 模型无法确定或规则命中 REVIEW 时进入人工复核，复核后触发最终路由。 |

**调用一个审核配置时必须同时指定项目编码和审核编码**，并使用项目内「接入凭证」页面创建的 AppKey/AppSecret 鉴权。每条凭证只能访问被显式授权的审核配置。

---

## 2. 接入步骤

1. 在后台创建项目。
2. 在「审核配置」创建审核，填写审核要求、绑定模型、选择规则模板。
3. 在「接入凭证」页面创建一条或多条项目凭证，并勾选该凭证允许访问的审核配置；创建时复制一次 AppSecret。
4. 进入该审核配置的「接入指南」，选择一条已授权凭证，按示例对接。
5. 在指南中开启需要的一种或多种触发方式，并按示例对接。
6. 提交任务后，业务方可以：
   - 通过开放 API 查询任务结果；
   - 配置审核结果回调；
   - 通过 WebSocket 接收异步结果帧；
   - 在任务中心与数据报表中查看状态与统计。

> AppSecret 只在「生成/轮换」时展示一次；界面默认脱敏。轮换后旧密钥立即失效。

### 2.1 接入凭证管理

「接入凭证」是项目级独立页面，可创建多条凭证：

| 能力 | 说明 |
|---|---|
| 凭证名称 | 项目内唯一，用于区分业务系统或环境 |
| AppKey | 系统生成，用于标识凭证 |
| AppSecret | 系统生成，AES-256-GCM 加密存储，仅创建/轮换时显示一次 |
| 授权审核配置 | 多选；只有勾选的审核配置可以被该凭证访问 |
| 状态 | 启用/停用；停用后所有签名请求返回 401 |
| 最近使用 | 记录最近一次通过鉴权的时间 |

管理接口（后台会话鉴权）：

```http
GET    /api/admin/v1/projects/{pid}/credentials
POST   /api/admin/v1/projects/{pid}/credentials
PUT    /api/admin/v1/projects/{pid}/credentials/{cid}
POST   /api/admin/v1/projects/{pid}/credentials/{cid}/rotate
DELETE /api/admin/v1/projects/{pid}/credentials/{cid}
```

审核配置被删除后，即使凭证的 `audit_ids` 中仍有该 ID，提交与查询也会因审核不存在而失败。审核配置恢复后，如果授权 ID 仍匹配，访问自动恢复。

---

## 3. 通用任务输入 TaskInput

所有触发方式最终都转换成同一个对象：

```json
{
  "biz_key": "archive:2026:0001",
  "request_id": "audit-20261031-000001",
  "media": [
    { "type": "text",  "content": "档案正文或 OCR 文本", "name": "document.txt" },
    { "type": "image", "url": "https://your-domain.com/page-01.jpg" }
  ],
  "ext": {
    "scene": "archive_review",
    "department": "archive"
  },
  "callback_url": "https://your-service.example.com/callback"
}
```

| 字段 | 必填 | 说明 |
|---|---|---|
| request_id | 强烈建议 | 调用方生成的项目内唯一提交 ID。填写后平台按它幂等：相同 request_id + 相同内容返回原任务；内容不同返回 409 REQUEST_ID_CONFLICT。回调和按 request_id 查询都会原样返回。 |
| biz_key | 建议 | 业务分组键。未填写 request_id 时，相同业务键 + 相同媒体 + 相同来源在去重窗口内只创建一次任务。 |
| media | 是 | 至少一段文本或一张图片。支持**多个 `text` 和多个 `image`**；不支持 video/audio。 |
| media[].content | text 必填 | 文档文本 / OCR 结果。 |
| media[].url | image 必填 | 图片地址，必须可被平台访问；JEV 会下载并做尺寸归一化。 |
| ext | 否 | 业务扩展信息，规则可用 `ext.*` 读取，并会进入回调体。 |
| callback_url | 否 | 任务级回调地址，优先于审核配置中的回调地址。 |

### 文本编码

- HTTP/JSON 请求必须使用 UTF-8；如果历史系统输出 GBK/GB18030 或 Big5 文本，请尽量先转换为 UTF-8。
- 平台在入库和送模型前也会做编码兜底：去除 UTF-8 BOM/NULL，尝试修复 GB18030、Big5 和常见 Latin-1 乱码；无法修复的非法字节会被安全替换。
- MySQL 统一使用 utf8mb4；CSV 导出使用 UTF-8 BOM，Excel/WPS 打开不会乱码。
- 回调、任务详情和报表均按 UTF-8 JSON 返回。

### 媒体限制

- 文本建议不超过 18,000 字，超出部分会被截断后送审。
- 图片默认只接受 JPEG/PNG；JEV 下载上限 20MB，单请求最多 8 张。通用大模型需启用图片理解后同样支持多图。
- 默认启用 SSRF 防护，不允许图片 URL 指向内网/回环地址。
- 当前版本不提供本地文件上传，只接受文本或公开图片 URL；**不支持视频、音频**。

---

## 4. 开放 API（3 种触发之一）

### 4.1 鉴权

使用**接入凭证页面创建的 AppKey / AppSecret + HMAC-SHA256**。一条凭证只能访问其授权列表中的审核配置。

必填 Header：

| Header | 说明 |
|---|---|
| `X-Api-Key` | 接入凭证页面创建的 AppKey |
| `X-Timestamp` | 当前毫秒时间戳，与服务端偏差 ≤ 5 分钟 |
| `X-Nonce` | 8~128 位随机串，10 分钟内不可重复 |
| `X-Signature` | HMAC-SHA256(AppSecret, CanonicalString) 的小写十六进制 |

CanonicalString：

```text
HTTPMethod + "\n" +
Path + "\n" +
Timestamp + "\n" +
Nonce + "\n" +
SHA256HEX(RequestBody)
```

- `Path` 是完整请求路径，不含 query string。
- GET 请求的 Body 为空，使用空字符串的 SHA256。
- 签名和请求体严格绑定：任何 Body 字节变化都会导致验签失败。

Python 示例：

```python
import hashlib, hmac, json, time, uuid, requests

APP_KEY = "..."
APP_SECRET = "..."
path = "/open/v1/projects/smartdoc-demo/audits/archive_review/submit"
body = json.dumps({
    "biz_key": "archive:2026:0001",
    "media": [{"type": "text", "content": "待审核的档案文本"}],
}, ensure_ascii=False).encode("utf-8")

ts = str(int(time.time() * 1000))
nonce = str(uuid.uuid4())
canonical = f"POST\n{path}\n{ts}\n{nonce}\n{hashlib.sha256(body).hexdigest()}"
signature = hmac.new(APP_SECRET.encode(), canonical.encode(), hashlib.sha256).hexdigest()

resp = requests.post(
    "http://127.0.0.1:8080" + path,
    data=body,
    headers={
        "Content-Type": "application/json",
        "X-Api-Key": APP_KEY,
        "X-Timestamp": ts,
        "X-Nonce": nonce,
        "X-Signature": signature,
    },
)
```

### 4.2 提交任务

```http
POST /open/v1/projects/{project_code}/audits/{audit_code}/submit
```

**前置条件**：该审核配置必须处于启用状态，且开放 API 触发开关为启用。

请求体：

```json
{
  "biz_key": "archive:2026:0001",
  "media": [
    { "type": "text", "content": "待审核的档案文本" },
    { "type": "image", "url": "https://example.com/page-01.jpg" }
  ],
  "ext": { "scene": "archive_review" },
  "callback_url": "https://your-service.example.com/callback",
  "mode": "async",
  "wait_ms": 3000
}
```

| 参数 | 说明 |
|---|---|
| mode | `async`（默认）立即返回受理；`sync` 在 wait_ms 内等待终态。 |
| wait_ms | sync 模式最长等待，默认 3000，最大 10000 毫秒。 |

成功响应（HTTP 200）：

```json
{
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
    "task": {}
  },
  "trace_id": "..."
}
```

幂等命中时 `deduplicated=true`，返回原任务当前状态，不重复创建任务。

### 4.3 查询任务

```http
GET /open/v1/projects/{project_code}/tasks/{task_no}
GET /open/v1/projects/{project_code}/task-by-request-id/{request_id}
```

签名规则与提交相同：GET、Path、空 Body 的 SHA256。

响应包含：

```json
{
  "task_id": 12,
  "task_no": "SDV...",
  "request_id": "audit-20261031-000001",
  "source_ref": "1792578000000-0",
  "status": "SUCCESS",
  "result": "REJECT",
  "result_detail": {
    "summary": "命中文档完整性风险",
    "reasons": ["关键页缺失", "印章无法辨认"],
    "labels": ["文档完整性", "内容安全"],
    "risk_level": "high",
    "score": 0.93,
    "rule": { "matched_rule": "高风险直接驳回" }
  },
  "input": {
    "biz_key": "archive:2026:0001",
    "media": []
  },
  "model_result": {},
  "rule_trace": {}
}
```

任务状态：

| status | 含义 |
|---|---|
| QUEUED | 排队中 |
| PROCESSING | 审核中 |
| REVIEW_PENDING | 自动审核完成，等待人工复核（此时 result 通常为 REVIEW） |
| SUCCESS | 流程完成，result 为 PASS 或 REJECT |
| FAILED | 执行失败 |
| STOPPED | 已停止 |

> 开放 API 只能读取凭证所属项目内、且审核配置在授权范围内的任务；其他情况返回 404/403。

---

## 5. Redis Stream 触发

### 5.1 平台行为

审核配置启用 Redis 触发后，平台在 5 秒内启动消费者：

- 使用当前项目「数据连接」中选定的业务 Redis 连接和 DB 编号（0~255），不使用平台缓存 Redis；
- Stream、消费组、消费起点、批量大小、最大并发由审核配置中的触发源设置决定；
- 消费组默认 `sdv-cg-{audit_id}`；消费起点可选 `$`（只消费新消息）或 `0`（从头消费）；
- 一个 `(连接, DB, Stream)` 只允许一个审核消费，不做同队列多审核广播；
- 支持多个平台 Worker 实例共享同一消费组；
- 连接停用或网络故障时触发源自动暂停；网络恢复后按连接的「自动恢复触发源」决定是否恢复；
- 非法消息会 ACK 并写入 `{stream}:dlq`。

### 5.2 默认键

```text
stream: sdv:{project_code}:audit:{audit_code}
group:  sdv-cg-{audit_id}
dlq:    {stream}:dlq
```

DB 由审核触发源单独选择，不在连接配置里固定。

### 5.3 消息格式

Stream 消息必须有一个 `data` 字段（兼容 `payload`），其值为 TaskInput JSON 字符串。`-n` 后面是审核触发源中选择的 DB 编号：

```bash
redis-cli -n {db} XADD sdv:smartdoc-demo:audit:archive_review '*' data '{
  "biz_key": "archive:2026:0001",
  "media": [{"type":"text","content":"待审核的档案文本"}],
  "ext": {"scene":"archive_review"}
}'
```

如果消息没有 `biz_key`，平台使用 `redis:{message_id}` 作为业务键。

### 5.4 启用步骤

1. 项目空间 → 数据连接 → 新增 Redis 连接，填写地址、DB（0~255）/账号密码并测试连接；触发源默认使用该 DB，也可以单独覆盖；
2. 审核配置 → 接入指南 → Redis；
3. 选择当前项目已启用且测试成功的连接，填写 DB、Stream、消费组、消费起点、批量大小和最大并发；
4. 打开开关；平台会校验连接、DB、Stream 和消费组，并拒绝已被其他审核占用的队列；
5. 使用 XADD 向对应 DB/Stream 写入消息；
6. 在任务中心筛选来源 `REDIS` 查看结果。

---

## 6. WebSocket 触发

### 6.1 连接地址

```text
ws://{host}/open/v1/ws/projects/{project_code}/audits/{audit_code}
  ?app_key=...
  &timestamp=...
  &nonce=...
  &sign=...
```

签名 CanonicalString：

```text
GET + "\n" +
Path + "\n" +
Timestamp + "\n" +
Nonce + "\n" +
SHA256HEX("")
```

Path 不包含 query string。

### 6.2 帧协议

连接成功后平台发送：

```json
{ "type": "ready", "data": { "project": "...", "audit": "..." } }
```

客户端提交：

```json
{
  "type": "submit",
  "data": {
    "biz_key": "archive:2026:0001",
    "request_id": "audit-20261031-000001",
    "media": [{ "type": "text", "content": "待审核文本" }],
    "ext": { "scene": "archive_review" }
  }
}
```

平台受理：

```json
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
```

任务终态后平台主动推送：

```json
{
  "type": "result",
  "data": {
    "task_no": "SDV...",
    "request_id": "audit-20261031-000001",
    "status": "SUCCESS",
    "result": "REJECT",
    "result_detail": {},
    "input": {}
  }
}
```

- 支持 `{"type":"ping"}` → `{"type":"pong"}`。
- 也可以响应 WebSocket 原生 Ping/Pong。
- 读超时 90 秒；建议客户端每 30 秒一次心跳。
- 未启用 WebSocket 触发或审核未启用时，HTTP 握手阶段直接拒绝。

---

## 7. 审核结果回调

### 7.1 配置

在接入指南 → 结果回调中配置：

| 配置 | 说明 |
|---|---|
| callback_enabled | 是否启用 |
| callback_url | 回调地址，支持公开 `http(s)://` Webhook 或 `ws(s)://` WebSocket（内网需平台开启 MEDIA_ALLOW_PRIVATE） |
| callback_results | 需要回调的最终结论：PASS / REJECT（待议任务经人工复核后再回调） |
| callback_secret | HMAC 签名密钥，加密存储 |
| callback_include_content | 是否把原始文本/内容回传；关闭时只传 URL/元信息 |
| callback_headers | 可选自定义 Header（需通过 API 配置） |

> 自动审核产生 `REVIEW_PENDING` 时**不会**回调；人工复核完成后，按最终 PASS/REJECT 触发回调。这样业务方只收到最终结论。

### 7.2 回调体

```json
{
  "event": "audit.completed",
  "task_id": 12,
  "task_no": "SDV...",
  "project_id": 1,
  "project_code": "smartdoc-demo",
  "project_name": "智审图档 · 演示空间",
  "audit_id": 1,
  "audit_code": "archive_review",
  "audit_name": "档案图档智能审核",
  "source_type": "OPENAPI",
  "biz_key": "archive:2026:0001",
  "status": "SUCCESS",
  "result": "REJECT",
  "category": "contract",
  "category_name": "合同协议",
  "category_confidence": 0.92,
  "summary": "命中文档完整性风险",
  "reasons": ["关键页缺失"],
  "labels": ["内容安全", "文档完整性"],
  "risk_level": "high",
  "score": 0.93,
  "rule": {},
  "provider": "OPENAI_COMPATIBLE",
  "model": "deepseek/deepseek-v4.1-flash",
  "media": [
    { "type": "text", "content": "档案正文或 OCR 文本" },
    { "type": "image", "url": "https://..." }
  ],
  "ext": { "scene": "archive_review" },
  "result_detail": {},
  "finished_at": "..."
}
```

分类审核会额外返回 `category`、`category_name`、`category_confidence`；合规审核这三个字段为空。业务方应按 `result` 处理终审结论，分类字段只用于业务分流和统计。

### 7.3 请求 Header

```http
Content-Type: application/json
X-Idempotency-Key: SDV...
X-SmartDoc-Event: audit.completed
X-SmartDoc-Task: SDV...
X-SmartDoc-Request-Id: audit-20261031-000001
X-SmartDoc-Result: REJECT
X-SmartDoc-Timestamp: 1792578000000
X-SmartDoc-Signature: sha256=<HMAC-SHA256(callback_secret, raw_body)>
```

验签示例（Python）：

```python
expected = "sha256=" + hmac.new(secret.encode(), raw_body, hashlib.sha256).hexdigest()
if not hmac.compare_digest(expected, request.headers["X-SmartDoc-Signature"]):
    return 401
```

- 业务方返回 2xx 视为成功。
- 失败按 1s / 2s / 4s 重试 3 次。
- 最终失败会写入任务详情的 `result_detail.callback`，并记录审计。
- 事件包括 `audit.accepted`（Redis/异步受理）、`audit.deduplicated`（request_id 重复）、`audit.completed`（最终结果）、`audit.submit_failed`（Redis 解析/提交失败）；业务方按 `request_id` 优先、`task_no` 兜底关联。

### 7.4 WebSocket 回调

`callback_url` 使用 `ws://` 或 `wss://` 时，平台按 WebSocket 推送结果：

1. 建立连接时携带与 HTTP 回调相同的 `X-Idempotency-Key`、`X-SmartDoc-Event`、`X-SmartDoc-Task`、`X-SmartDoc-Result`、`X-SmartDoc-Timestamp`、`X-SmartDoc-Signature` 握手请求头；
2. 连接建立后发送一条 UTF-8 JSON 文本帧，内容与 8.2 的回调体一致；
3. 帧发送成功即视为回调成功，失败同样重试 3 次；连接在发送后关闭，业务方如需长期订阅请使用 WebSocket 触发或在业务侧持久化。

### 7.5 结果动作编排

除统一业务回调外，审核配置还可以按 PASS / REJECT / REVIEW 分别编排结果动作，动作在结果落库前执行，执行记录写入 `result_detail.routes`：

| type | 必填字段 | 可选字段 | 说明 |
|---|---|---|---|
| http | url | method、params、headers、body、timeout_ms | params/headers 为字符串字典；body 留空发送标准结果 JSON |
| websocket | url（ws/wss） | headers、payload、timeout_ms | headers 用于握手鉴权；payload 留空发送标准结果 JSON |
| redis | connection_id、db、target | command（publish/set/xadd）、payload | 使用项目 Redis 连接写入指定 DB；payload 留空发送标准结果 JSON |
| js | script | remark、timeout_ms、memory_mb | 直接写顶层代码，平台注入全局 `ctx`（无需 `main`，旧脚本仍兼容）；可用 `ctx.mysql` / `ctx.pg` / `ctx.http` / `ctx.crypto` / `ctx.secret`；不提供文件、socket、eval、定时器；编辑器「运行」可沙盒模拟，见下方说明 |

沙盒模拟运行：脚本编辑器右上角「运行」按钮使用当前脚本 + 模拟上下文执行一次，默认结论取该动作所在的 PASS / REJECT / REVIEW，齿轮按钮可配置任务号、业务主键、来源、输入文本与 `ctx.detail`（摘要 / 风险等级 / 风险分 / 原因 / 标签 / 分类 / 扩展 JSON）。模拟时 `ctx.secret` 返回占位字符串、数据库连接不建立（`query` 返回空、`exec` 返回 0 行）、`ctx.http` 直接返回 `200 {"simulated":true}`，日志会逐条标注 `[SIM]`；面板展示临时日志、返回值与临时变量，`ctx.simulation` 为 `true`，且不写入脚本执行记录。

加解密与密钥：`ctx.crypto` 提供 MD5 / SHA-1 / SHA-256、HMAC-MD5 / HMAC-SHA1 / HMAC-SHA256、Base64 / Hex、AES / DES（CBC / ECB，PKCS7 或 NONE，base64 或 hex），可用于第三方接口的签名拼装与报文加解密；`keyEncoding` / `ivEncoding` 支持 hex 或 base64 密钥。凭据统一通过 `ctx.secret('NAME')` 读取，在后台「项目空间 → 项目密钥」按项目维护；脚本编辑器的「API 文档」提供图文流程和「去配置项目密钥」跳转按钮。

每个结论可配置 `on_error`：`continue` 继续执行、`stop` 停止后续动作、`to_review` 转人工复核。模板变量：`${task_no}`、`${task_id}`、`${request_id}`、`${biz_key}`、`${source_type}`、`${source_ref}`、`${input_json}`、`${ext_json}`、`${project_code}`、`${project_id}`、`${audit_code}`、`${audit_id}`、`${result}`、`${summary}`、`${reasons}`、`${labels}`、`${risk_level}`、`${score}`、`${category}`、`${category_name}`、`${category_confidence}`、`${review_note}`。

取值须知：

- 模板按纯文本替换，替换结果一律是字符串，数字变量（`${task_id}` 等）写进 JSON 也会变成字符串；
- `${result}` 为 `PASS` / `REJECT` / `REVIEW`，人工复核后以人工结论为准；
- `${score}` 为 0~1 的风险分，**越高越危险**，不是通过率；模型失败兜底时按结论取 `0.2` / `0.5` / `0.9`；
- `${category}` 系列只对分类类审核有值，普通文本审核通常为空；
- `${reasons}` / `${labels}` 是数组，替换后为空格分隔的裸文本，会破坏 JSON —— 需要结构化字段时把请求体 / 消息内容留空，改用标准结果 JSON。

> HTTP 与 WebSocket 动作同样受 SSRF 防护约束；后台提供结构化编辑器与「高级 JSON」两种编辑方式，保存时会校验动作类型、必填字段和失败策略。

---

## 8. MySQL 与业务数据库

标准三种触发方式中**没有直连业务 MySQL**，也**没有 HTTP/MySQL 轮询**：

- 平台用 MySQL 存储项目、审核配置、任务、审计与统计；
- 平台不持有业务库长期账号，避免权限扩散；
- 不同业务表结构和增量规则差异大，直接配置 SQL 难以保证安全。

推荐做法：业务方自行查询自己的 MySQL、完成字段脱敏和增量控制，然后把每一行转换成 TaskInput，通过开放 API、Redis Stream 或 WebSocket 主动推送给平台。

如果确实需要平台直连业务 MySQL，需要新增独立的数据源扩展，并配置：加密 DSN、仅 SELECT 白名单、游标字段、字段映射、脱敏规则和连接池上限。当前版本未开放该项。

---

## 9. 权限范围

- AppKey / AppSecret 属于**项目**，但每条凭证可在「接入凭证」页面单独勾选审核配置范围。
- 开放 API 与 WebSocket 还要求目标审核配置的对应触发开关已启用，且审核配置存在。
- 审核配置被删除、凭证停用或未授权时，请求会被拒绝。
- 未启用开放 API 的审核会返回 `TRIGGER_DISABLED`。
- 分页、查询、回调配置等管理操作走后台登录会话和角色权限，AppKey 不能调用管理 API。
- 项目管理员可在「接入凭证」页面轮换或删除凭证；轮换后旧签名请求立即失效。
- 回调密钥和 Redis 连接密码均加密存储，接口不回传明文。

---

## 10. 错误码

| HTTP | code | 含义 |
|---|---|---|
| 400 | INVALID_PARAM | 参数不合法 |
| 400 | INVALID_SIGNATURE | 签名错误 |
| 400 | TIMESTAMP_EXPIRED | 时间偏差超过 5 分钟 |
| 400 | REPLAY_ATTACK | Nonce 重复 |
| 401 | UNAUTHORIZED | 缺少/无效 AppKey |
| 403 | PROJECT_OUT_OF_SCOPE | 凭证不属于路径项目 |
| 403 | AUDIT_OUT_OF_SCOPE | 该凭证未被授权访问此审核配置 |
| 401 | CREDENTIAL_DISABLED | 凭证已停用 |
| 403 | AUDIT_NOT_ACTIVE | 审核配置未启用 |
| 403 | TRIGGER_DISABLED | 对应触发方式未启用 |
| 404 | NOT_FOUND | 审核配置或任务不存在 |
| 409 | DUPLICATED_TASK | 幂等命中（开放 API 返回 deduplicated=true） |
| 422 | DEPENDENCY_MISSING | 模型供应商/依赖缺失 |
| 429 | RATE_LIMITED | 请求过于频繁 |
| 500 | INTERNAL_ERROR | 服务内部错误 |
| 503 | DOWNSTREAM_UNAVAILABLE | 依赖服务不可用 |

---

## 附录 A. 使用 AI 辅助实现提交端

AI 提示词不是一份泛化总览，而是按触发方式拆分：

| 触发方式 | 专用提示词要求的实现方向 |
|---|---|
| 开放 API | HMAC CanonicalString、Header、签名、任务查询、错误码和异步等待 |
| Redis Stream | 业务侧只做 XADD 消息生产；不创建平台消费组；结果通过回调或 biz_key 查询 |
| WebSocket | 带签名连接 URL、submit/ack/result 帧关联、心跳、断线和重连 |

每个审核配置的「接入指南 → 总览」提供：

- **按触发方式复制提示词**：先勾选一种或多种方式，再复制对应专用提示词；多选时会生成组合提示词，并保留每种方式的独立适配要求。
- **下载统一 Skill.md**：包含三种触发方式，并在开头明确要求编码助手先询问业务方采用哪一种或哪几种方式；如果业务方选择多种，则分别实现适配器。
- **复制文档地址**：复制当前后台的完整文档 URL。
- 指南内同时也提供可直接执行的签名 cURL、Redis 命令和 WebSocket 地址示例。

「对接文档」页顶部也提供相同的按方式选择弹窗，并在开放 API、Redis、WebSocket 三个章节分别提供专用提示词按钮。

AppSecret 不会写入提示词，请在 AI 生成的代码中通过环境变量 `SMARTDOC_APP_SECRET` 注入。

## 11. 常见问题

**Q：我配置了新审核，怎么触发？**
A：打开该审核的「接入指南」。默认开放 API 是启用的；Redis 和 WebSocket 需要在该页面手动开启。Redis 还需要先在「数据连接」创建并测试项目 Redis 连接。

**Q：审核编码在哪里复制？**
A：审核卡片的「更多 → 复制审核编码」，或者接入指南顶部的「复制审核编码」。调用开放 API 时路径需要同时带项目编码和审核编码。

**Q：为什么开放 API 返回 403 TRIGGER_DISABLED？**
A：审核配置的开放 API 开关被关闭，或审核本身未启用。

**Q：为什么 Redis 没有消费？**
A：依次检查：项目中 Redis 连接是否启用且测试成功；触发源是否启用；DB、Stream、消费组是否填写正确；连接健康状态是否为 OK。连接停用或网络故障会自动暂停触发源，网络恢复后按「自动恢复触发源」配置决定是否恢复；管理员手动停用连接后需要手动恢复触发源。配置修改后最多 5 秒生效。

**Q：回调里为什么没有内容？**
A：检查 `callback_include_content` 是否开启，以及回调是否在最终结论后触发。待议任务必须先经过人工复核。

**Q：回调签名怎么验证？**
A：对完整的原始请求体做 HMAC-SHA256，密钥为回调配置的 callback_secret，比较 `X-SmartDoc-Signature` 的 `sha256=` 前缀值。
