# 贡献规范

感谢你愿意帮助 MediaStationGo 变得更稳定。这个项目主要面向 NAS、Docker 部署、媒体库整理、订阅下载和多端播放场景；提交 Issue 或 Pull Request 时，请尽量提供可复现、可验证的信息。

## Issue 提交规范

提交 Issue 前，请先确认：

- 已搜索现有 Issues，避免重复提交同一个问题。
- 使用的是最新镜像、最新主分支，或已说明当前版本号 / 镜像摘要。
- 如果是部署或运行问题，已附上部署方式和关键配置。

### Bug Report 必填信息

- 问题现象：实际发生了什么，是否稳定复现。
- 期望行为：你认为正确结果应该是什么。
- 复现步骤：从哪个页面、点击什么、填写什么、触发什么任务。
- 部署方式：Docker 第一档 / 第二档 / 第三档、裸机运行、反代方式等。
- 环境信息：NAS 型号或系统、Docker / Compose 版本、浏览器、MediaStationGo 镜像版本。
- 相关配置：路径映射、下载器保存路径、媒体库路径、站点类型等。请隐藏 Cookie、API Key、密码和 Token。
- 日志和任务信息：优先提供应用日志、任务队列详情、浏览器控制台错误、网络请求错误。

### 日志建议

排查订阅、站点搜索、下载器、整理入库、网盘扫描时，建议临时把日志级别调整为 `info` 或 `debug`，复现后再恢复。

Docker 部署常用命令：

```bash
docker compose ps
docker compose logs --tail=300 mediastation-go
docker compose exec mediastation-go sh -lc 'ls -la /data/logs || true'
```

PostgreSQL 部署查询示例：

```bash
docker compose exec postgres psql -U mediastation -d mediastation -c "select key,value,updated_at from settings order by updated_at desc limit 30;"
```

请勿公开粘贴以下敏感信息：

- 站点 Cookie、Passkey、API Key、YemaPT Auth Key、M-Team API Key。
- qBittorrent / Transmission / Aria2 密码。
- Telegram Bot Token。
- JWT、数据库密码、私有下载链接。

## Pull Request 提交规范

所有非紧急变更都应通过 Pull Request 合入 `main`，不要直接向主分支推送。贡献者可以从 fork 或本仓库的独立分支发起 PR；维护者只有在紧急安全修复、发布流水线修复等特殊场景下，才可以短暂绕过 PR 流程，并需要在提交说明或后续 Issue 中补充原因。

PR 应该尽量小而清晰。一次 PR 聚焦一个问题或一组强相关改动，避免把无关重构、格式化和功能混在一起。

### 分支要求

- 分支从最新 `main` 创建，提交前先同步远端主分支。
- 分支名建议使用 `fix/...`、`feat/...`、`docs/...` 或 `test/...`。
- 不要在 `main` 上直接开发和提交 PR 内容。
- 不要把个人部署配置、NAS 本地路径、私有镜像标签或测试数据提交进 PR。
- 如果基于魔改版、私有部署版或临时补丁开发，请先确认改动能在本仓库最新 `main` 上复现和应用，再提交 PR。

### PR 描述应包含

- 背景：修复哪个 Issue / 哪个用户场景 / 哪个回归。
- 改动摘要：后端、前端、配置、文档分别改了什么。
- 验证结果：运行过哪些命令，是否有无法运行的测试。
- 风险说明：数据迁移、Docker 配置、路径映射、下载器行为、站点 API 限流等是否受影响。
- 截图或录屏：涉及 UI、任务队列、错误提示、设置页时请附上。

### 推荐验证命令

根据改动范围选择运行：

```bash
go test ./...
npm --prefix web run build
git diff --check
```

如果只改动某个模块，可以先跑定向测试，例如：

```bash
go test ./internal/service -run "TestOrganize|TestSubscription|TestMTeam" -count=1
go test ./cmd/server -count=1
```

### 代码要求

- Go 代码使用 `gofmt`。
- 前端 TypeScript 需要通过 `npm --prefix web run build`。
- 用户可见错误需要可操作：说明失败原因、下一步怎么查或怎么修。
- 后台任务失败不要静默吞掉，应进入任务队列、日志或 API 响应。
- Docker/NAS 路径相关改动必须考虑宿主机路径与容器路径映射。
- 站点 API、订阅和下载器改动需要注意去重、限流、敏感信息脱敏。

## Commit Message 建议

优先使用简短清晰的动词开头：

```text
fix organizer hardlink diagnostics
feat(yemapt): add auth-only site adapter
docs: add issue and pull request guidelines
test: cover subscription restore matching
```

## 维护者合并检查

合并前建议确认：

- PR 范围清楚，没有夹带无关改动。
- 自动化检查通过，或失败原因已说明。
- 修改涉及的用户路径已有日志、错误提示或回归测试。
- 文档、示例配置和 README 是否需要同步更新。
