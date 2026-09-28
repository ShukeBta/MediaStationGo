# A2–A12 本地验证记录

2026-09-28，分支 `codex/port-acecandy`。本次承接 Claude 最新的“Mgo任务1 / Mgo任务”，范围为 A2–A12；A1 红果短剧除外。功能沿用现有 Media 和双数据库架构，保留下载器、PT、订阅、115、CloudDrive2、播放与转码模块。使用方法见 [功能说明](ace-a2-a12-guide.md)，逐项人工检查步骤见 [验收清单](ace-a2-a12-acceptance.md)。

本记录区分自动化检查、浏览器实际操作与尚未执行的环境验证。分支没有推送、合并到 main 或部署；`main` 仍为 `66401914a209ddf5faeed58b7ee3c4a1050c28a5`。

**当前结论：功能实现、三个平台构建、前端和真实 PostgreSQL 检查已完成；末轮 Go 全量测试仍有一项原有软链接用例受 Windows 文件操作限制，不能记为全绿。**

## 环境与代码版本

- Windows amd64，Go 1.25.0，Node 24.15.0，npm 11.12.1。
- 数据库：隔离 SQLite 数据库与本地原生 PostgreSQL 16.15 实例。PostgreSQL 测试创建并销毁临时数据库；未使用业务数据库。
- 最终后端与构建检查对应 `f2d433998c6c864ebe8f7c1d90a93e938e039cd3`；前端最终检查对应 `37bf2ad1`，之后未改前端代码。后续交付提交仅包含本记录、文档链接和 Dockerfile 示例注释。
- 脱敏结果日志与构建产物位于本地 `.codex-local/ace-review-20260928/`，不纳入 Git。原始 Claude 导入记录位于 `.codex-local/claude-import-20260928/`，保留原文，不将原任务状态改写为此次执行结果。

## 自动化检查

在仓库根目录执行；前端构建和 lint 在 `web` 目录执行。

| 检查 | 命令 | 实际结果 / 证据 |
|---|---|---|
| Go 全量测试 | `go test ./... -timeout 6m` | 末轮未全通过：原有 `TestTransferFileSymlinkKeepsSource` 一项被 Windows 拒绝创建软链接，其余未报告失败；`final-go-test.log` |
| Go 静态检查 | `go vet ./...` | 通过；`final-go-vet.log` |
| 真实 PostgreSQL 集成 | `go test ./internal/service -run '^TestPostgres(AceFeatureIntegration\|BackupRestoreRoundTrip)$' -count=1 -timeout 3m -v` | 连接真实实例通过，无跳过，22.056 秒；`final-postgres-test.log` |
| 纯 WASM 图片回归 | `go test -tags nodynamic ./internal/service -run 'TestImage(Variant\|Proxy)' -count=1 -timeout 2m` | 通过，6.003 秒；`final-image-wasm-test.log` |
| 前端单元测试 | PowerShell：`$webTestFiles = @(rg --files web/src -g '*test.mjs'); node --test $webTestFiles` | 56/56 通过，无跳过；`final-web-test.log` |
| TypeScript 与前端构建 | `npm run build` | 通过；`final-web-build.log` |
| ESLint | `npm run lint` | 0 错误、4 警告；`final-web-lint.log` |
| Windows 后端 | `go build -o .codex-local/ace-review-20260928/mediastation-final.exe ./cmd/server` | 通过 |
| Linux amd64 后端 | 设置 `GOOS=linux`、`GOARCH=amd64`、`CGO_ENABLED=0` 后执行 `go build -o .codex-local/ace-review-20260928/mediastation-linux-amd64 ./cmd/server` | 交叉编译通过 |
| Linux arm64 后端 | 设置 `GOOS=linux`、`GOARCH=arm64`、`CGO_ENABLED=0` 后执行 `go build -o .codex-local/ace-review-20260928/mediastation-linux-arm64 ./cmd/server` | 交叉编译通过 |
| 格式与补丁 | 对本轮 183 个 Go 文件规范化 CRLF 后比较 gofmt 输出；`git diff --check` | 无格式差异、无补丁空白错误 |

Windows 与两个 Linux 目标均使用 `CGO_ENABLED=0` 构建。构建产物 SHA-256 保存在 `build-sha256.json`。本地 `verify-final-backend.ps1` 可重跑各组检查；PostgreSQL 检查前需启动隔离的测试实例。

末轮唯一失败的软链接用例直接调用 `os.Symlink`，对应 `transfer.go` 与测试在本轮没有变化。系统临时目录重试仍返回 `Access is denied`，另一次在 Windows `CreateSymbolicLink` 调用内超时；改用项目内 D 盘临时目录也被拒绝。独立最小程序仅导入 `os`、`path/filepath`，创建源文件成功，仍在 `os.Symlink` 挂住；10 秒后终止，未获得返回值。该探测保存在 `.codex-local/symlink-probe-5be6eb6bda644932b673e31a980d2a9c/`，表明问题脱离应用依赖也可复现。

保留 `go-test-symlink-access-denied.log`、`symlink-retry.log` 和 `symlink-local-temp-test.log`，没有扩大跳过条件或调整系统安全设置以消除失败。需要在允许创建软链接的 Windows 环境重跑该用例及全量测试；本记录不将末轮 Go 全量检查标为通过。

PostgreSQL 用例需要设置 `MEDIASTATION_TEST_POSTGRES_DSN`，并将匹配服务器主版本的 `pg_dump`、`pg_restore`、`psql` 所在目录加入 PATH。不配置 DSN 时，相关用例会跳过，不能据此声称双数据库验证通过。本轮实际使用 PostgreSQL 16.15，测试输出及连接配置保留在本地验证目录，本文不包含凭据。

真实 PostgreSQL 用例覆盖首次及重复完整迁移、默认资料源配置、豆瓣补全保留已有值、人物身份和演职员关系、TMDb 完整快照与租约、NFO 写入、播放事件去重和 UTC 聚合、探测文档及清理触发器、任务持久化和重启恢复、多 Part 分组键，以及压缩/未压缩备份恢复。

全量 Go 检查包含原有云盘、下载器、扫描、播放、字幕等模块的自动化用例；这不代表已连接真实外部服务逐项验收。除上述系统软链接限制外，STRM 删除、越界路径、NFO 回滚、播放可见性等服务/接口测试没有报告失败。

## 浏览器实际操作

使用隔离的 `http://127.0.0.1:18086` SQLite 测试应用、临时管理员和合成媒体样本，执行了以下操作：

| 页面 / 操作 | 观察到的结果 |
|---|---|
| 登录与首次启动 | 管理员可登录；修复配置模型重复迁移导致的缺列问题后，重新启动未出现相同初始化错误 |
| 任务中心 | 启动状态、维护任务目录、历史记录、按天查询/分页控件及已有业务任务区域正常呈现 |
| 播放统计 | 筛选、统计表与明细页面可打开 |
| 人物列表 | 显示合成人物及卡片，页面布局正常 |
| 媒体详情 | 展示测试 STRM 实际指向的文件路径 |
| NFO 编辑 | 已有 NFO 默认选中同步；保存后直接读取文件，标题更新为“功能验收示例（已编辑）”，原演员角色与自定义 XML 节点仍保留 |
| STRM 删除预览 | 展示目标路径、父目录选项和确认约束；未在浏览器中执行删除，删除行为由隔离文件测试覆盖 |

这些步骤验证了页面入口和上述具体行为，没有将只有界面呈现的功能记为完整业务端到端通过。

## 本轮发现并修复的问题

- PostgreSQL 重复迁移时，探测文档清理触发器阻止列修改；现按迁移生命周期暂停/恢复触发器。
- `APIConfig` 与 `ApiConfig` 重复注册同表，导致新代理字段缺失；现统一为完整模型并保留兼容别名。
- NFO 编辑清理被修改字段的旧别名，保留无关图片和 XML，修正单集日期，并同步手动编辑的演员关系。
- Web/Emby 进度上报先检查媒体可见性，再按实际选中版本的时长统一进度和播放统计规则。
- Emby 先合并云端/本地库入口再应用显隐，避免隐藏后影子入口重新出现；多 Part 分组键满足 PostgreSQL 字段长度。
- 前端分组测试区分单项路径键与既有批量归并规则，并增加不同 ID、跨库、多集目录反例；字幕阶段文案测试与已有多模型 ASR 行为对齐。
- WebP 依赖的生成代码导致 ARM64 编译内存耗尽；改用 `gen2brain/webp v0.5.5` 的 WASM 编码后，三个目标构建均通过。静态图片由 x/image 解码，保留 RGB/透明度并单独处理 EXIF；在分配像素前校验 RIFF 边界、画布与实际位流尺寸，异常结构不会转入 FFmpeg 回退。

## 尚未执行与已知边界

- 未调用真实 TMDb、豆瓣、Resin、AI 付费服务完成整条流程；相关数据处理、重试和权限逻辑使用自动化夹具覆盖。
- 未使用真实 Emby 客户端、真实媒体文件进行多音轨/字幕、转码或完整播放验收；当前 PATH 无 FFmpeg，FFmpeg 回退使用替身测试。
- Windows 上执行测试，Linux 仅交叉编译，未在 Linux 主机实测文件系统权限、符号链接及播放。
- 当前 Windows 环境拒绝创建软链接，末轮 Go 全量测试因此仍有一项失败；需在具备软链接能力的环境补验。
- 未在本机执行 Docker 镜像构建/启动，也未触发或观察远程 CI。
- Vite 仍提示 HLS chunk 大于 500 KB；ESLint 的 4 条警告涉及 Hook 依赖/清理、未使用抑制注释和 Fast Refresh 导出约束。
- NFO 编辑基于现有电影、单集与整剧界面，没有新增独立 canonical 季对象编辑器。整剧批量保存是逐项提交，不是覆盖整批的单一事务。

上述未执行项保留在 [验收清单](ace-a2-a12-acceptance.md)，适合在独立的真实媒体验收环境补做，不能由本地自动化结果代替。

## 临时服务清理

本轮隔离的 SQLite 验收应用与 PostgreSQL 集群均已停止，检查结果见本地 `runtime-cleanup.json`。测试数据、脚本、构建产物和日志保留，未触碰实际部署。
