# Emby Episodes 分段诊断

仅服务端设置 `MEDIASTATION_DIAGNOSTICS_EPISODES=1` 时启用，默认关闭。
作用域是 `/Shows/:id/Episodes` handler，不涵盖其前面的鉴权中间件、
公网链路、图片请求及客户端渲染。日志名为 `emby episode timing`，
不记录用户 ID、令牌、查询字符串、媒体路径或响应内容。

`stage_ms` 单位为毫秒：

- `projection_check_repair`：分组键失效检查及必要修复。
- `season_identity_query` / `season_media_query`：季身份、目标季媒体查询。
- `episode_filter_sort` / `episode_version_collapse`：筛选排序、版本折叠。
- `library_snapshot` / `series_titles` / `media_version_siblings`：响应所需资料读取。
- `user_favorites_history`：收藏及播放进度查询、映射组装。
- `item_payload_assembly` / `attach_media_tokens`：条目及媒体源令牌组装。
- `json_encode_and_write`（错误时 `error_json_write`）：编码及响应写出，不是纯 JSON CPU 时间。

`service_total` 包含服务内各子阶段，不能与子阶段相加。
未走到的阶段不会出现；`total` 还包括参数解析等未细分工作。
SQL 阶段包括连接获取、驱动执行和结果解码，不等于数据库执行计划时间。

`pool_window_wait_*_delta` 来自进程共享连接池，在请求窗口内取差值。
并行请求的窗口可能重叠，不能归因到单请求或跨请求求和。
池指标缺失表示没有取得底层 SQL 连接池，不表示等待为零。

验证先在隔离、禁止出网的 PostgreSQL 媒资快照上执行，比较逐季请求
和客户端已有的并行取季模式；不会为了诊断让生产客户端增加并发。
本地 SQLite 测试只证明默认关闭、响应一致和计时隔离，不能作为生产性能结论。
生产启用需要单独发布和配置授权；诊断结束关闭开关。
