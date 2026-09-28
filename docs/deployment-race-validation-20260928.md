# 本地部署与 race 验证

2026-09-28，在完成 A2–A12 与全仓 Review 修复后，对候选版本进行了本地部署和并发检查。
生产源码对应 `ea5986bc`；随后仅调整贡献者文档、同步远端 README，以及在 `8a32859f`
修复异步云播放预热测试夹具。最终 race 执行前后的全部 Go 源文件哈希一致。

## 实际结果

| 检查 | 结果 |
|---|---|
| Windows Go 全量 | 2382 通过，0 失败；6 个 PostgreSQL 环境场景另行验证 |
| PostgreSQL 16.15 真实集成 | 全部通过，包含跨实例刷新令牌消费和轮换/登出的真实行锁竞争 |
| Go 静态检查 | `go vet ./...` 通过 |
| 前端测试 / 构建 | 60 项通过；TypeScript/Vite 构建通过；lint 0 错误、4 条既有警告 |
| Linux 全量 race | 2380 通过，0 失败，0 DATA RACE；8 个环境依赖场景跳过 |
| Windows 隔离部署 HTTP | 91 项通过，0 失败 |
| 浏览器页面 | 管理员登录、任务中心、统计周筛选、请求日志、人物页通过；无未捕获脚本错误 |

## Race 环境与修复

使用任务专用的 `Codex-MediaStation-Validation-20260928` WSL2 发行版，Alpine 3.22.1、
Go 1.25.0、GCC 14.2.0。没有配置或修复缺失磁盘的默认 Ubuntu，也没有使用 Docker。
编译器依赖从 Alpine 官方仓库下载，经发行版原有签名校验后安装。

```sh
CGO_ENABLED=1 go test -race -p 2 -json ./... -count=1 -timeout=12m
```

首轮检测到 `stream_test.go` 中的测试夹具竞争：后台预热写入解析器调用计数，测试同时
读取该值。旧断言还错误地期望不发生预热。现在使用通道传递调用参数和协调生命周期，
阻塞模拟解析器以证明 302 不等待预热，并检查云类型、引用、User-Agent 与内部跳转认证。
生产预热逻辑没有变化。该用例定向 race 重复 30 次通过，相关播放用例重复 3 次通过，
Windows 定向测试重复 30 次通过，随后重新运行完整 race 并通过。

全量 race 的 8 个跳过项包括 6 个需要 PostgreSQL DSN 的测试/子测试，以及 2 个需要
Linux FFmpeg 的图片测试。这些场景已分别在 Windows 的真实 PostgreSQL 16.15 或
真实 FFmpeg 下通过普通测试；它们没有被计入 race 通过数。

## Windows 部署与真实 HLS

在独立工作目录中运行 Windows 二进制，使用新建 SQLite、数据、缓存、媒体和配置目录，
监听测试端口 18088，访问地址为 `http://127.0.0.1:18088`。许可证服务地址指向本机
未开放端口；管理员使用随机测试密码。没有扫描真实媒体目录或调用真实 Telegram。

91 项 HTTP 检查覆盖健康与静态页面、账户登录、管理权限、配置敏感值脱敏、显式 false
权限持久化、Emby 令牌范围、媒体库隔离、播放权限撤销、会话隔离、刷新重放拒绝、
设备被踢出后的请求拒绝，以及 Webhook 缺失/伪造来源密钥拒绝。设备状态通过隔离数据库
夹具设置，不代表测试了真实 Telegram 踢设备命令。

使用既有 Bilibili FFmpeg 3.0.1 生成 35 秒 testsrc + sine 合成视频，由应用实际转 HLS。
两名普通用户访问同一播放列表；第一位停止后，转码任务仍运行且开始时间未变化，第二位
仍能取得分片，下载分片经 FFmpeg 解码成功。播放列表和分片均返回 `private, no-store`，
匿名请求被拒绝。

浏览器检查使用独立 headless 会话，完成后已关闭。数据为空时的页面展示和筛选通过，
不等同于真实外部 API、Safari/电视客户端或数小时连续播放验证。

隔离实例曾记录一次任务持久化 `database is locked (517)` 警告，出现在夹具写入阶段，
未导致本次 HTTP 检查失败。本记录不把这条警告或未执行的客户端场景计作通过。

## 证据与交付边界

本地忽略目录 `.codex-local/deploy-verify-20260928/` 保留：

- `smoke-results.json`、`http-smoke.log`、`http-smoke-config.log`、`browser-summary.json`。
- `browser-tasks.png`、页面快照、`ffmpeg-segment-decode.log`。
- 首轮失败的 `race-tests.jsonl` 与修复后 `race-final/` 下的完整日志、环境和源码哈希。
- 隔离启动脚本和二进制；密码仅在本地凭据文件中，不写入 Git 或验证结果。

远端 README 按 blob 哈希逐字节核对并保留。贡献者与功能来源独立记录在
[CONTRIBUTORS.md](../CONTRIBUTORS.md)，原始开发历史保留在来源分支。

这些验证针对本地候选版本；GitHub CI 和最终 PR 合并状态以对应 PR 为准。
