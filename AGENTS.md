# SmartDoc Vision 项目规则

## 分支与提交

- 日常开发统一在 `dev` 分支进行，禁止直接向 `main` 提交功能改动。
- 每完成一项可验证的改动，都提交并推送到 `origin/dev`。
- 提交前至少执行与改动相关的检查：Go 改动运行 `gofmt`、`go vet ./...`、`go test ./...`；前端改动运行 `npx vue-tsc --noEmit` 和 `npm run build`。
- 不提交 `.env`、`dbhub.toml`、模型 API Key、AppSecret、JWT_SECRET、MASTER_KEY 等敏感信息。

## 版本与发布

- 默认不修改 `VERSION`、不修改 `CHANGELOG.md` 的版本段落、不创建版本标签、不发布 Release。
- 只有用户明确说“发布新版本 / 发版 / Release”时，才执行发布流程：
  1. 更新 `VERSION` 与 `backend/internal/version/version.go`；
  2. 在 `CHANGELOG.md` 增加对应版本段落；
  3. 在 `dev` 分支完成验证并提交推送；
  4. 将 `dev` 合并到 `main`（保持线性历史或明确的发布提交）；
  5. 创建 `vX.Y.Z` 标签并推送，触发 `.github/workflows/release.yml`；
  6. 确认 Windows amd64、Linux amd64、Linux arm64 资产与 `SHA256SUMS.txt` 发布成功。
- 发布前必须确保 `CHANGELOG.md` 中存在与版本号完全匹配的 `## X.Y.Z` 段落，发布流水线依赖该段落生成 Release Notes。

## 代码与产品约定

- 审核输入仅支持多文本 + 多图片，不支持视频/音频。
- 通用大模型包含 OpenAI 兼容协议与 Anthropic 协议，二者属于同一业务分类，只是协议不同；JEV 是独立的结构化决策模型。
- 纯分类器（`CLASSIFY_ONLY`）成功审核固定 PASS，只返回分类类型；未知类型必须可编辑。
- 修改审核配置、规则、触发方式或数据库配置时，同步更新 README 或相关文档。
- 不覆盖用户已有的 `.env`；数据库、Redis 等运行时配置统一通过 `.env`（或系统环境变量）提供。
