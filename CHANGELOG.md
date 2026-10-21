# Changelog

## 1.0.0 - 2026-10-22

### 核心审核

- 多项目隔离与项目成员数据范围。
- OpenAI 兼容、Anthropic Claude Messages API、JEV / TypeSafe System One 三类模型接入。
- 合规审核与分类审核两种模式；分类审核支持分类映射、置信度阈值和 JEV 原生 choice 分类。
- 多文本 + 多图片审核，图片 SSRF 防护、格式校验、尺寸归一化；不支持视频/音频。
- 规则组、停止命中、风险阈值、模型失败兜底、人工复核与结果回调。
- Redis Stream、WebSocket、HTTP 定时轮询、OpenAPI HMAC 提交；回调支持签名、幂等和失败重试。
- 审核任务队列恢复、幂等去重、CSV 记录导出和分类分布报表。

### 管理后台

- Vue 3 + TypeScript + Naive UI + ECharts。
- 桌面 / 平板 / 手机响应式布局；移动端侧栏抽屉、表格横向滚动、固定列适配。
- 审核配置分类编辑、按触发方式生成 AI 提示词、统一 Skill 下载、`$BASE_URL` 真实地址替换。
- 接入凭证独立管理，按审核配置授权 AppSecret 范围。
- 用户与角色、菜单权限、项目范围、个人中心和修改密码。
- 登录滑块拼图验证码，验证码绑定 IP、120 秒有效、一次性使用；同一 IP 连续 3 次失败封禁 10 分钟。
- 模型接入弹窗、表单和移动端布局修复。

### 配置与部署

- 支持 `config.yml` 和 `.env`，优先级：系统环境变量 > `config.yml` > `.env` > 内置默认值。
- 首次启动且无配置文件时自动生成带默认配置和随机密钥的 `.env`。
- MySQL 字段/DSN、Redis 地址/密码/DB、HTTP 端口等可通过配置文件设置。
- 后台入口固定为 `/admin`；Vite 生产构建使用 `/admin/` 资源前缀。
- 单文件构建：前端嵌入 Go 二进制，支持 Windows amd64、Linux amd64、Linux arm64。
- 本地 `build.ps1` 支持交互选择目标，检测 UPX 并询问压缩。
- GitHub Actions Release 流程：自动构建前端、测试、生成三平台单文件、生成 SHA256 校验和并发布 GitHub Release。
- GitHub Release 流程从本 CHANGELOG 提取版本更新内容并写入 Release。
