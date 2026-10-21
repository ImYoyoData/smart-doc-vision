# JEV / TypeSafe System One 接入说明

SmartDoc Vision 将 JEV 作为与通用大模型并列的“结构化决策模型”接入。JEV 不生成自由文本，而是接收 `state` 和一组类型化问题，返回带概率和置信度的结构化答案。

## 1. 接入信息

| 项 | 值 |
|---|---|
| 厂商 | TypeSafe AI |
| 模型 | 早期访问模型中为 `jev-latest`，实际响应会返回具体版本（例如 `jev-1.13.0`） |
| 接口 | `POST https://api.typesafe.ai/v1/systemone` |
| 认证 | `Authorization: Bearer <TYPESAFE_API_KEY>` |
| 图片开关 | 含图片时必须携带 `TypeSafe-Danger-Zone-Images: true` |
| 后台配置 | 「模型接入」→ 新增 / 编辑 → 类型选择 JEV 决策模型 |

在后台填写：

- **BaseURL**：`https://api.typesafe.ai/v1`（平台会自动追加 `/systemone`）
- **模型 ID**：`jev-latest` 或指定版本
- **API Key**：TypeSafe Console 生成的密钥
- **图片理解**：JEV 类型默认开启

> 连接测试会向 System One 发送一个 Noul 连通性问题并校验返回结构，不回显 API Key。

## 2. 请求结构

```json
{
  "model": "jev-latest",
  "state": {
    "task": {
      "biz_key": "archive:2026:0001",
      "ext": { "scene": "engineering_archive" }
    },
    "document_text": "待审核文本……",
    "audit_requirements": "档案内容安全、完整性与关键要素审核要求",
    "document_image": "img-<uuid>",
    "additional_images": ["img-<uuid-2>"]
  },
  "assets": {
    "img-<uuid>": {
      "type": "image",
      "format": "jpg",
      "data_base64": "<标准 base64，无 data: 前缀>"
    }
  },
  "questions": {
    "audit_decision": {
      "type": "choice",
      "instructions": "根据 state 判断最终审核结论，不确定时选择 REVIEW。",
      "criteria": {
        "PASS": "图档清晰完整，未发现违规、敏感信息、伪造篡改或明显缺页。",
        "REVIEW": "存在模糊、遮挡、缺页、信息不足或无法确认的内容。",
        "REJECT": "明确包含违法违规、敏感不当内容，或存在明显伪造、篡改、严重缺失。"
      }
    },
    "sensitive_content": {
      "type": "noul",
      "instructions": "文档中是否包含需要拦截的违法、色情、暴力、政治敏感、广告导流或敏感个人信息？"
    },
    "document_incomplete": {
      "type": "noul",
      "instructions": "文档是否模糊、缺页、遮挡、关键要素无法辨认，因而无法可靠审核？"
    }
  }
}
```

### 响应结构（示意）

```json
{
  "model": "jev-1.13.0",
  "answers": {
    "audit_decision": {
      "type": "choice",
      "choice": "REVIEW",
      "confidence": 0.86,
      "probabilities": {
        "PASS": 0.10,
        "REVIEW": 0.86,
        "REJECT": 0.04
      }
    },
    "sensitive_content": {
      "type": "noul",
      "noul": 0.03,
      "confidence": 0.98
    },
    "document_incomplete": {
      "type": "noul",
      "noul": 0.71,
      "confidence": 0.93
    }
  },
  "usage": {
    "input_tokens": 0,
    "output_tokens": 0
  }
}
```

具体计费与额度以 TypeSafe 官方控制台为准。

### 分类审核模式（推荐 JEV）

当审核配置的 `decision_mode` 为 `CLASSIFY` 时，平台不再询问 `audit_decision`，而是发送 `document_category` 这个动态 `choice` 问题，选项来自审核配置的分类列表。JEV 的原生 choice + confidence 非常适合做分类决策。

请求中的关键部分：

```json
{
  "state": {
    "decision_mode": "CLASSIFY",
    "categories": [
      {"code": "contract", "name": "合同协议", "description": "合同、协议、补充条款", "result": "PASS"},
      {"code": "invoice", "name": "发票票据", "description": "发票、收据、付款凭证", "result": "PASS"},
      {"code": "id_document", "name": "身份证件", "description": "身份证、护照、驾驶证", "result": "REJECT"},
      {"code": "other", "name": "其他 / 无法识别", "description": "信息不足", "result": "REVIEW"}
    ]
  },
  "questions": {
    "document_category": {
      "type": "choice",
      "instructions": "从给定分类中选择唯一最匹配的编码，不要创建新类别。",
      "criteria": {
        "contract": "合同、协议、补充条款（终审 PASS）",
        "invoice": "发票、收据、付款凭证（终审 PASS）",
        "id_document": "身份证、护照、驾驶证（终审 REJECT）",
        "other": "信息不足或无法分类（终审 REVIEW）"
      }
    },
    "sensitive_content": { "type": "noul", "instructions": "是否包含需要拦截的敏感内容？" },
    "document_incomplete": { "type": "noul", "instructions": "文档是否不完整或无法可靠分类？" }
  }
}
```

处理规则：

1. 平台用返回的 `choice` 匹配审核配置中的分类编码，并取该分类的 `result` 作为模型结论。
2. 返回分类不在配置中时，强制转人工复核。
3. 分类置信度低于模板阈值（标准 0.60 / 严格 0.75 / 宽松 0.45）且映射结论为 PASS 时，规则引擎自动转人工。
4. `sensitive_content ≥ 0.75` 会覆盖分类映射并强制 REJECT；`document_incomplete` 较高时，PASS 也会转人工。
5. 结果详情、回调、CSV 和数据报表保留 `category`、`category_name`、`category_confidence`，便于业务分流和统计。

演示配置 `archive_classify` 的分类映射为：合同/发票/工程图纸 → PASS，身份证件 → REJECT，其他 → REVIEW。通用大模型也能按 JSON 返回分类，但平台在新建分类审核时会优先推荐 JEV。

## 3. 图片处理

JEV 只接收 **PNG / JPEG** 图片资产，并通过 `assets` + `state` 引用。SmartDoc Vision 的 `internal/media` 适配器会：

1. 校验 URL 协议与目标 IP，默认阻断内网 / 回环 / 链路本地 / CGNAT 地址（SSRF 防护）；
2. 下载图片，单图上限 20MB；
3. 校验格式必须为 PNG / JPEG；
4. 将短边放大到至少 280px、长边压缩到最多 1568px（保持比例）；
5. 重新编码为原格式（JPEG 质量 88 / PNG），确保大小不超过 16MB；
6. 生成唯一 UUID 引用，写入 `state` 与 `assets`。

限制（来自 TypeSafe 文档）：

- 每张图 ≤ 16MiB；
- 宽高 280~1568px；
- 总像素 78,400~2,458,624；
- 请求体 ≤ 32MiB；
- 单请求最多 8 张图（平台默认配置，可在供应商设置中调整）。

如果关闭 SSRF 防护（`MEDIA_ALLOW_PRIVATE=true`），可审核内网图片，但**不建议生产开启**。

## 4. 风险归一化

JEV 返回概率而非风险分，平台按下式生成 `score`：

```text
risk = sensitive * 0.55
     + incomplete * 0.20
     + P(REJECT) * 0.25
     + (1 - decision_confidence) * 0.10
risk = clamp(risk, 0, 1)
```

结论映射优先级：

1. `decision == REJECT` 或 `sensitive >= 0.75` 或 `P(REJECT) >= 0.70` → **REJECT**
2. `decision == PASS` 且 `sensitive < 0.35` 且 `incomplete < 0.40` → **PASS**
3. `decision == REVIEW` 或 `incomplete >= 0.50` → **REVIEW**
4. 其他情况 → **REVIEW**

生成后的 `risk_level / score / labels / summary` 进入与通用大模型相同的规则引擎，由审核配置的规则模板做最终二次判定。因此你可以在后台通过规则阈值进一步收紧或放宽 JEV 的原始结论。

## 5. 如何测试

### 后台

1. 进入「模型接入」；
2. 新增或编辑 JEV 供应商；
3. 填写 BaseURL、模型 ID、API Key，保存；
4. 点击「测试连接」；
5. 测试成功后会显示延迟和 System One 返回正常的提示。

> 如果测试失败，先检查 BaseURL 是否被误填成通用大模型的地址。JEV 必须指向 TypeSafe 的 `/v1` 接口。

### API

```bash
# 登录后获取 TOKEN
curl -X POST http://127.0.0.1:8080/api/admin/v1/providers/2/test \
  -H "Authorization: Bearer $TOKEN"
```

成功响应中的 `data`：

```json
{
  "ok": true,
  "latency_ms": 1084,
  "message": "System One 连接成功，Noul 返回正常",
  "model": "jev-1.13.0"
}
```

### 端到端任务

在审核配置中选择 JEV 供应商，提交任务。合规审核的任务详情中可看到：

- `model_result.provider = "JEV"`
- `model_result.model = "jev-1.13.0"`（以实际响应为准）
- `raw.decision` / `raw.sensitive_content` / `raw.document_incomplete`
- 归一化后的 `risk_level` 与 `score`

分类审核的任务详情还会显示：

- `model_result.category` / `category_name` / `category_confidence`
- `raw.category_choice` 及分类概率
- `result_detail.category` 与最终映射结论

演示环境已创建 `archive_classify` 并通过一条合同文本测试：JEV 返回 `contract / 合同协议 / 0.96`，规则映射为 `PASS`。另已创建 `archive_jev_review`，使用 2 张图片 + 1 段文本完成多模态测试。

## 6. 代码位置

| 能力 | 文件 |
|---|---|
| JEV 适配器 | `backend/internal/ai/jev.go` |
| 图片下载 / 缩放 / SSRF | `backend/internal/media/images.go` |
| Provider 配置 / 测试 | `backend/internal/api/providers.go` |
| 模型调用调度 | `backend/internal/engine/engine.go` |
| 规则判定 | `backend/internal/rules/engine.go` |
| 后台接入页面 | `web/src/views/ProvidersView.vue` |

## 7. 参考

- TypeSafe 文档：<https://docs.typesafe.ai/>
- System One 端点：`https://api.typesafe.ai/v1/systemone`
- OpenAPI：<https://api.typesafe.ai/openapi.json>
- 图片输入说明：<https://docs.typesafe.ai/images.md>
