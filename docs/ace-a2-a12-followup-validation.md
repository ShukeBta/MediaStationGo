# A2–A12 后续验证与本地合并

2026-09-28，用户授权修复验证问题、将 `codex/port-acecandy` 本地合入 `main`，然后 Review。本记录补充 [上一阶段结果](ace-a2-a12-validation.md)；不包含推送或部署。

## 修复内容

业务文件转移逻辑保持原样。Windows 测试先在独立子进程中调用 `os.Symlink`，检查真实环境能力，再验证业务函数。权限拒绝或已进入系统调用后的探测超时明确记为环境跳过；进程无法启动、其他错误，以及探测成功后的业务函数错误仍然使测试失败。

探测结果在当前测试进程中复用，最长等待 40 秒。此主机通常约 31 秒返回 `Access is denied`；过早结束探针会干扰紧随其后的外部程序测试，因此允许系统正常返回，同时保留硬性超时。扫描、监听、NFO、STRM 与转移测试共用该能力检查。没有更改 Windows 权限或安全设置。

## 实际结果

计数包含 Go 子测试；跳过项不算作通过。

| 检查 | 结果 | 本地证据 |
|---|---|---|
| Windows Go 全套：`go test -json ./... -timeout 6m` | 2286 通过、0 失败、11 跳过；8 项依赖软链接能力，3 项未配置 PostgreSQL DSN | `.codex-local/merge-review-20260928/windows-summary.json`、`go-test.jsonl` |
| Linux Go 全套：`go test -p 2 -json ./... -count=1 -timeout=6m` | 2292 通过、0 失败、5 跳过；3 项未配置 PostgreSQL DSN，2 项缺少 Linux FFmpeg | `.codex-local/linux-validation-20260928/full-summary.json`、`full-go-tests.jsonl` |
| Linux 真实软链接专项 | 8 项全部通过，0 跳过；包括文件转移、目录循环、离线目录、NFO 与 STRM 越界 | `.codex-local/linux-validation-20260928/test-results.json` |
| Windows 软链接/FFmpeg 顺序回归 | 系统能力预检按权限拒绝跳过；随后真实 FFmpeg 裁剪/WebP 测试通过，最终全套中的两个 FFmpeg 测试也通过 | `.codex-local/merge-review-20260928/symlink-ffmpeg-sequence.log`、`go-test.jsonl` |
| Go 静态检查及补丁格式 | `go vet ./...`、`git diff --check` 通过 | `.codex-local/merge-review-20260928/go-vet.log` |

Linux 环境使用任务专用的 `Codex-MediaStation-Validation-20260928` WSL2 发行版，Alpine 3.22.1、Go 1.25.0、`CGO_ENABLED=0`，临时文件位于 Linux ext 文件系统。官方系统与工具链下载均校验 SHA-256；既有 Ubuntu、Docker 和默认发行版未修改。该环境只用于测试，未部署应用。

原 Ubuntu 因缺少虚拟磁盘无法启动，故未使用其数据。临时 Linux 环境补装 FFmpeg 时包索引下载超时，已停止该尝试，相关两项仍按未执行记录；Windows 上相同实际 FFmpeg 用例已通过。

本轮只修改测试代码。前轮真实 PostgreSQL 16.15 集成、56 项前端测试、前端构建与 Windows/Linux amd64/Linux arm64 构建结果仍适用，详见上一阶段记录；本轮未将无 DSN 的跳过结果冒充 PostgreSQL 通过。

## 合并与 Review 范围

合并前 `main` 为 `66401914a209ddf5faeed58b7ee3c4a1050c28a5`，是工作分支的祖先；采用 `git merge --ff-only`，保留全部提交历史。此前 Claude 已导入但尚未进入 main 的改动随该分支一并进入本地主分支。

后续只读 Review 聚焦 `973121bc` 之后的 A2–A12、此次测试修复及合并一致性；不把此前 Claude 导入的全部历史改动声称为本次重新逐项审查。真实外部 API、Emby 客户端及生产部署仍需独立验收。
