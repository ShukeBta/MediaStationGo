# Episodes 分段测量：2026-09-09

## 结论

隔离 PostgreSQL 样本中，版本折叠是最大耗时阶段。不是 JSON 编码主导。
并行请求放大重复库查询及连接池排队，不能以增加并发作为根治方案。

代码链路：episodeItems -> collapseMediaVersionRows -> mediaVersionKey ->
MergedLibraryIDsForLibrary。折叠前未加载请求内库快照，逐集执行
Library.FindByID 和 Library.List，两者还预加载 Roots。
快照直到后面的 payloadsForMediaRows 才加载，无法帮助之前的版本折叠。
优先修复方向是复用现有请求内快照并提前加载，保持分组、合并库和排序规则不变；
无需增加跨请求缓存、TTL、第二数据源或连接池上限。本次仅测量，未实施该修复。

## 环境与证据边界

- 生产读取时 revision：8651502b3b9a054345187590295fa10aa04a9e68。
- 生产镜像：ghcr.io/timefunnel/mediastation-go@sha256:8dd48de4053e1c71dde21adb8ab034ff0776a270696b2d731904960a9c5abb77。
- 诊断候选：796d62d6ddd6078104ebde5cc44818c16930cee9，仅增加默认关闭的计时。
- 候选镜像：ghcr.io/timefunnel/mediastation-go@sha256:98ee9656037127fc397c464194a3f897d43a2577fd646422524b80778333fafd。
- Actions：<https://github.com/timefunnel/MediaStationGo/actions/runs/34300317709>。
- 校验 linux/amd64、完整 revision、version=sha-完整SHA；启动使用 --no-build --pull never。
- 隔离 PostgreSQL 快照含 12,388 条未删除媒资，只复制 libraries/library_roots/series/media。
- 未复制真实用户、播放历史、设置、云盘配置；用户数据阶段不代表真实账号负载。
- 临时内部网络禁止出网；115 请求为 0，未切换生产、未改生产数据。
- 《办公室》9 季，各逐季请求一次，再进行一次 9 请求并行，合计 18 次 Episodes。
- 逐季与并行的响应内容完全一致，所有请求 HTTP 200。
- 临时容器、网络、数据目录已清理。

## 结果

逐季请求 handler 总耗时 81.5–255.4 ms；并行时 239.8–1316.0 ms。
并行探针外部完成耗时 2065.1 ms，包含 docker exec 调度开销，不是 iOS 实际页面耗时。

下列比例为该阶段耗时之和 / 9 个请求总耗时之和，不能当作并行墙钟时间分解。

| 阶段 | 逐季占比 | 并行占比 |
| --- | ---: | ---: |
| 版本折叠 | 30.0% | 49.0% |
| 条目组装 | 18.0% | 18.1% |
| 分组键检查及修复 | 24.0% | 13.6% |
| 媒体版本关联查询 | 14.2% | 8.0% |
| 目标季媒体查询 | 5.8% | 5.5% |
| 季身份查询 | 4.4% | 3.3% |
| 收藏及进度 | 1.7% | 1.0% |
| 编码及写出 | 0.9% | 0.8% |

最慢请求 1316.0 ms，其中版本折叠 814.9 ms、条目组装 215.6 ms、
检查及修复 107.4 ms、媒体版本关联 95.1 ms、编码及写出 12.5 ms。

池上限为 4。逐季请求窗口等待计数均为 0；并行期间多个窗口观察到上千次池等待。
这些是进程共享窗口统计，彼此重叠，不应相加，也不能归属某一个请求。
它证明有连接池争用，但不证明扩大池上限就是最佳修复。

条目组装内部和检查查询的数据库执行计划仍需进一步拆分，不能把阶段名称
直接当成纯 CPU 或纯 SQL 时间；也不能由此宣称公网、图片或 iOS 的全部延迟已解释。

## 本地验证

go test ./...、go vet ./internal/service ./internal/handler、git diff --check 通过。
开关前后响应一致，9 个并行 trace 状态隔离测试通过。
go test -race 因本机 CGO 未启用而未执行，未声称竞态检测通过。
