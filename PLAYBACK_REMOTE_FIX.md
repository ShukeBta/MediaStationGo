# 远程播放卡顿排查与修复

## 1. HTTP 播放与文件响应

- Web 直放路由是 `internal/handler/routes_authenticated_core.go:registerAuthedPlaybackAndProxyRoutes` 注册的 `/api/stream/:id` 和 `HEAD` 路由，进入 `internal/handler/media.go:streamHandler`，再由 `internal/service/stream_file.go:StreamService.serveMediaWithCloudMode` 打开本地目标文件。文件通过 `http.ServeContent` 返回，不是整文件 `io.ReadAll` 或手写复制；标准库处理 `Range`、`If-Range`、`HEAD`、`Content-Range` 和 `Content-Length`。本地 `.strm` 目标由 `internal/service/strm_local_target.go:localMediaPlaybackPath` 解析，读取目标视频，不会把 `.strm` 文本当视频发送。
- Emby/Jellyfin 直放路由在 `internal/handler/emby_routes.go:registerEmbyVideoStreamRoutes` 和 `registerEmbyAuthenticatedPlaybackRoutes` 注册，最终进入 `internal/handler/emby_playback.go:embyVideoStreamHandler`，共用 `StreamService` 文件服务。Emby 下载路由进入 `internal/handler/emby_items_handlers.go:embyItemDownloadHandler`；`ServeDownloadForUser` 当前固定返回 `ErrDownloadDisabled`，下载功能被禁用，与播放流路由分开。
- 云盘反代路径在 `internal/handler/cloud_playback.go:proxyCloudResolvedLink`，转发客户端 `Range`，透传上游 `Content-Range`、`Content-Length`、`Accept-Ranges` 等头，然后用 `io.Copy` 流式复制。`internal/service/strm_proxy.go:STRMService.ProxySTRM` 也使用 `io.Copy` 并转发 Range 相关头。两处没有整文件入内存；标准 `io.Copy` 在两端没有优化接口时使用 32 KiB 缓冲。远程 HTTP STRM 若以 302 发给客户端，则 Range 由最终文件源处理。
- 静态资源在 `cmd/server/router.go:serveSPA` 中通过 Gin `Static`/`c.File` 提供；其底层 `http.FileServer`/`http.ServeFile` 支持 Range。它们与媒体流路由分开。
- 结论：本地文件 Range/seek 路径本身已有正确实现；拖动或重连对本地文件请求的是对应字节区间，不会被服务端强制从头整文件重传。新增测试断言本地 STRM Range 返回 206、正确长度及 Range 头。

## 2. 中间件、日志与鉴权

- `cmd/server/router.go:buildRouter` 的全局中间件只有 Recovery、`internal/middleware/request_logger.go:RequestLogger` 和 CORS；`internal/handler/handler.go:Register` 添加安全响应头。没有 gzip/压缩中间件，也没有会收集整段响应的中间件包装。
- `RequestLogger` 在 `c.Next()` 后记录请求状态和耗时，不读取或缓冲响应体。Emby 路由在 `internal/handler/emby_routes.go:registerEmbyRoutes` 使用 `PlayerRequestLogger`；记录通过 `PlayerRequestLogService.Record` 放进有界队列，数据库写入由后台 worker 执行，不在字节转发循环中同步写 DB。
- Web 鉴权在每个 HTTP 请求上验证 JWT，`activeUserRequired` 重新读取用户状态；非管理员的 `requirePermission` 会读取有效权限。Emby 路由也逐请求校验账号。HLS 每个分片请求都会重新经过这些检查，这是逐请求鉴权，没有逐块鉴权。
- 发现 `/api/stream/:id` 原先先由 handler 读取媒体行用于可见性校验，随后 `ServeFileForUser` 又按 ID 读同一行。现改为 `StreamService.ServeMediaForUser` 复用已校验的媒体记录，去掉此路径上重复的一次媒体查询。其余身份/权限校验保留，Emby 兼容路径仍有各自的媒体源解析与选择查询。

## 3. `.strm` 目标与播放 URL

- 本地 `.strm` 目标经 `internal/service/strm_local_target.go:mediaSTRMTarget` 和 `localSTRMTargetPath` 识别：目标是可访问本地视频文件时直接打开文件，使用 Range 响应。
- 云盘 STRM 的 `StreamService.serveMediaWithCloudMode` 会解析成云盘播放路由。`internal/handler/cloud_playback.go:serveCloudResolvedLink` 对可直连来源返回 302，让客户端从 CDN 取字节；必须带鉴权头的来源才由 `proxyCloudResolvedLink` 反代字节。HTTP 直链 STRM 也采用重定向。不存在统一把所有 `.strm` 文件读入内存代理的行为。
- `/api/playback/:id/info` 在 `internal/handler/playback_extra.go:playbackInfoHandler` 返回同源相对 URL。`externalPlaybackURL` 用于外部播放器 URL，会按配置使用 `app.server_url` 等公开域名。另一个会写入域名的路径是 `internal/service/strm_url.go:STRMService.strmPlaybackURL`：STRM 生成器按请求的 `BaseURL` 或 `PublicServerURL` 写入 `/api/stream/:id` 绝对地址，这是供外部播放器访问本服务的预期行为。普通 Web 播放 URL 本身不固化 `https://mgo.meogo.top`。
- 如果同一 MediaStationGo 再把生成的 `.strm` 扫描进媒体库并通过它播放，且 STRM/重定向播放设置已启用，当前服务会把绝对 `/api/stream/:id` 目标当 HTTP 直链返回 302，客户端再向公开域名发一次请求；这会多一次 API 往返（外网客户端会再次经过 frp），但视频字节只由最终媒体行服务一次，不会被服务端整段代理两遍。若外部播放器在家中局域网使用这些生成文件，`BaseURL` 解析到公网时也会走公网入口；应用不能判断播放器是否在家，需由所用 STRM 地址/局域网 DNS 决定访问入口。云盘内部播放在需要时也会多一次本服务 API 重定向，然后直达 CDN。

## 4. 转码、HLS 与探测

- `internal/config/defaults.go` 将 `transcoder.enabled` 默认设为 `true`；`internal/service/stream_hls.go:StreamService.ServeHLSPlaylist` 只有收到 HLS 播放清单请求时才调用 `TranscoderService.EnsureJob`。`internal/service/transcoder.go:TranscoderService.EnsureJob` 会启动 FFmpeg，并等待清单就绪；没有启动时预转码所有媒体的路径。
- React 播放器在 `web/src/pages/playerPageModel.ts:pickPlayerMode` 中对浏览器不支持的容器/编码自动选择 HLS。确凿问题在于本地扫描的 `.strm` 在 `internal/service/scanner_local_ingest.go:buildLocalScanMedia` 中固定记录为 `container=strm`；`internal/service/scanner.go:mediaExtensionSupportsProbe` 又明确跳过 `.strm` 探测。因此即使 `.strm` 指向浏览器能直放的 MP4，播放器也把它判成不支持并启动 HLS/FFmpeg，增加首帧延迟和 NAS CPU 负担。
- 修复在媒体详情响应中通过 `internal/handler/media_response.go:mediaForPlaybackResponse` 增加非持久化 `playback_container`，由 `internal/service/strm_local_target.go:STRMTargetContainer` 从 STRM 目标推导容器；路径和 STRM URL 仍按原规则对非管理员隐藏。`web/src/pages/playerPageModel.ts:needsTranscodeForBrowser` 优先使用该字段，目标 MP4/WebM/M4V 可走直接 Range 播放；目标仍不受浏览器支持时继续使用 HLS。
- 云盘播放后缺失媒体轨道信息时，`internal/service/stream_file.go:redirectResolvedCloudPlayback` 可能排队异步探测；队列在 `internal/service/stream_cloud_probe_queue.go:enqueuePlaybackCloudProbe` 中按媒体去重、有界并发，播放响应不等待探测。它不是本地 `.strm` 的默认播放探测路径。

## 5. 响应头

- 本地媒体由 `http.ServeContent` 根据文件句柄、大小和请求头生成 `Content-Length`、`Content-Range`、206/416 状态；服务显式设置 `Accept-Ranges: bytes`。现新增 `Cache-Control: private, no-store`，避免带用户授权的媒体被共享缓存保存。
- 云盘反代会透传上游长度、范围和验证器头，并将 Range 状态及响应体转发给客户端；若上游没有 `Accept-Ranges`，当前 handler 会补 `bytes`，但它本身不缓存或切分上游文件。真实 seek 仍依赖源站接受 Range；若源站对 Range 返回 200，代理也会转发 200 和完整响应体，代码无法在不额外读取/缓存整段源文件的情况下制造部分响应。纯 302 的响应由最终 CDN 负责媒体头。HLS 清单和切片由 `internal/service/stream_hls.go:ServeHLSPlaylist`/`ServeHLSSegment` 使用 `http.ServeContent`，并设为 `private, no-store`。

## 已修改内容

- 修复本地 `.strm` 目标容器丢失导致的自动 HLS/FFmpeg 转码；播放器根据实际目标格式选择 direct/HLS。
- `/api/stream` 复用可见性校验时加载的媒体记录，减少重复 DB 查询。
- 本地流补充私有禁止缓存头；已有 Range 响应通过新增测试锁定。
- 云代理新增测试锁定 Range 转发及 206 部分响应行为。云代理原有 32 KiB 默认复制缓冲已满足流式缓冲要求，无需额外复制层。

## 测试结果

- `go test ./internal/service -run '^TestServeFileLocalSTRMTargets$' -count=1`：通过。
- `go test ./internal/handler -run '^(TestMediaForPlaybackResponseExposesSTRMTargetContainerWithoutPath|TestProxyCloudResolvedLinkForwardsRangeAndReturnsPartialResponse)$' -count=1`：通过。
- `npm run build --prefix web`：通过（TypeScript 检查和 Vite 生产构建完成；构建输出有仓库原有的大 chunk 提示）。
- `git diff --check`：通过。

## 代码无法改变的基础设施因素

- 家庭宽带上行带宽和高峰时段的运营商拥塞。
- frp 海外中转机的入口/出口带宽、CPU/连接数限制，以及 NAS 到中转机、客户端到中转机之间的网络时延、丢包和抖动。
- 外网客户端的下行带宽、无线网络质量，以及客户端设备自身的解码能力。
- 云盘/CDN 源站限速、跨境路由及源站 Range 支持情况。

这些因素决定了字节能以多快速度到达用户；本次代码修复减少了 `.strm` 本地 MP4 被误转码的 CPU 和首帧成本，并确保本地 seek 使用 Range，但不能提高链路可用带宽或修复丢包。
