# 第三方播放器（Emby 兼容接口）远程播放卡顿排查与修复

范围：第三方播放器经公网域名（frp 海外中转回 NAS）走 Emby/Jellyfin 兼容接口播放的路径。
网页播放器路径见 PLAYBACK_REMOTE_FIX.md，本次不重复其结论。

过程说明：本次排查与大部分实现由 Codex（gpt-6-luna）完成；其在实现落盘後、运行测试前撞中转用量上限中断（中转返回 refresh_token_invalidated，短期不可用）。此后由审定方接手：全量审 diff、在 NAS 容器内完成构建与两包全量测试、修正一处与新隐私口径冲突的旧测试期望（见测试结果）、并撰写本报告。未使用任何其他模型。

## 根因清单

### R1. 本地 STRM 媒体在 Emby MediaSource 里没有可信的播放元数据

- media 行对 .strm 统一存 `Container="strm"`，扫描器明确跳过 .strm 探测（`internal/service/scanner.go` 的 `mediaExtensionSupportsProbe`）。生产库实测（2026-11-04 只读聚合）：1,202 条媒体中 400 条是 strm 行，其中 **0 条有 bit_rate、仅 84 条有 duration_sec**。
- `internal/service/emby_playback.go:mediaSource`（旧）只按行拼 MediaSource：RunTimeTicks/MediaStreams 来自空行、逐源单独加载 probe 文档；`baseMediaSource` 的 Size 对本地 strm 目标虽有 stat，但码率只能靠行字段（恒 0）。Emby 客户端靠 Container/Bitrate/RunTimeTicks 选播放方式、算缓冲与进度，字段全空/全错会引发试探性请求与错误决策。
- 修复：
  - `internal/service/emby_media_sources.go:15` `mediaSourcesForItem` 对 PlaybackInfo 路径批量加载全部版本兄弟的已存 probe 文档（`internal/service/media_probe_document_store.go:55` `loadMediaProbeDocuments`），替代逐源加载；列表嵌入路径（asEmbedded）不加载，保持轻量。
  - `internal/service/emby_playback.go:176` `baseMediaSource` 以「元数据行」为准填 Size/RunTimeTicks/MediaStreams：本地 strm 目标用 `os.Stat` 取真实文件大小（廉价）；自引用 strm 用被引用行的元数据（见 R5）。
  - 诚实留空：本地 strm 无已存 probe 数据时，不用 .strm 文本文件大小编造码率（`baseMediaSource` 中对本地 strm 显式只用行存 BitRate，通常为 0），也不编造轨道——`mediaSource` 在本地 strm 且无已存轨道元数据时把 MediaStreams 置空，`internal/service/emby_media_sources.go:470` `mediaStreams` 对全空的 strm 行直接返回空。热路径上没有新增同步 ffprobe。

### R2. 客户端 DeviceProfile 已声明可直放时，PlaybackInfo 仍发转码邀约

- 转码器默认开启时，MediaSource 无条件带 `SupportsTranscoding` 与 TranscodingUrl；容器又长期显示为 "strm"（见 R1），不少客户端据此选 HLS 转码。NAS 为 2 核低功耗 CPU，4K 实时转码根本跟不上，表现为持续卡顿——这是与网页端同源的「strm 被判不支持」问题在协商层的残留。
- 修复：`internal/handler/emby_playback.go:415` `embyPreferDirectPlaybackForProfile`——PlaybackInfo 构建完成后，对每个 `SupportsDirectPlay=true` 且与请求 DeviceProfile 匹配的源（DirectPlayProfiles 的容器/视频/音频编码匹配 + Container/Codec Profiles 的 required 条件满足 + 不超过 MaxStaticBitrate），置 `SupportsTranscoding=false` 并删除 TranscodingUrl。判定是 fail-closed 的：容器/编码未知（空或 "unknown"）绝不匹配，条件值未知绝不算满足，宁可保留原转码邀约。

### R3. Emby 视频流路由每个请求做一整套 Item 构建（热路径重活）

- `internal/handler/emby_playback.go` 的 `embyVideoStreamHandler`（旧）先调 `svc.Emby.Item()`（条目详情+人物/ artwork +MediaSources+外挂字幕发现，只为做存在性/权限判断），再 `ResolveMediaSourceID`（再查行+再建一遍 MediaSources），GET 时再 `RememberMediaSourceSelection`（再查行+再建一遍 MediaSources），最后 `Stream.ServeFileForUser` 又按 ID 重查媒体行。每次起播/重连/seek 重连都要付这全套。
- 修复：新增 `internal/service/emby_media_source_preference.go:19` `ResolvePlaybackMedia`：一次 FindByID+可见性校验+一次版本兄弟查询+可选的显式选源校验与偏好写入，返回实体行；handler 改走昨晚网页端同款的 `ServeMediaForUser`（复用已加载行，不再重查）。偏好语义与旧路径等价（无 MediaSourceId 时回主行，旧 `ResolveMediaSourceID` 同样如此；偏好只在 PlaybackInfo 排序时生效，本次未动）。
- 附带同类修复：`internal/service/emby_items_detail.go` Item() 预先算一次版本兄弟并写入 context 缓存供自身 MediaSources 复用；`attachExternalSubtitleStreamsToItem` 改用兄弟映射表，消除逐源 FindByID 的 N+1。

### R4. MediaSource/Item 的 Path 泄漏本地文件路径，且私有地址会被 302 发给远程客户端

- `embyMediaSourcePath`（旧）对本地媒体直接返回解析后的本地目标绝对路径（如 NAS 上的 /vol1/.../*.mkv），Item 详情的 `Path` 更是原始 `m.Path`。把 Path 当本地文件/SMB 路径用的客户端在远程会先卡死超时；同时这也是路径泄漏（与网页端修复时「非管理员隐藏目标路径」的口径不一致，Emby 侧此前对所有用户都漏）。
- 另有旧逻辑 `embyAbsolutePlaybackURL`：对 Yamby/SenPlayer/Lenna 客户端把相对 URL 按**请求 Host**  absolutize——请求若经局域网地址进来，DirectStreamUrl/Path 就被写成局域网绝对地址发给客户端。
- 还有流层：HTTP 型 strm 目标若是私有主机地址，会走到 302 分支把局域网地址放进 Location 返回远程客户端。
- 修复：
  - `internal/service/emby_playback.go:233` `embyMediaSourcePath` 对非云盘媒体统一返回虚拟 Emby 路由 `/Videos/{id}/stream.{container}`；Item 的 Container 改走 `embyMediaContainer`、Path 改走同一 helper。
  - PlaybackInfo 的令牌附加不再 absolutize：删除 `embyAbsolutePlaybackURL`/`embyClientNeedsAbsolutePlaybackURL`，URL 一律保持同源相对（客户端按其登录用的公网域名解析）。
  - `internal/service/stream_file.go:73`：非本地型的 HTTP strm 目标若主机为私有地址（`internal/service/stream_redirect.go:136` `isPrivatePlaybackHost`：私有/环回 IP、无点号单标签主机名、.local/.lan/.internal/.home 后缀），直接返回明确的 `ErrCloudPlaybackUnavailable`（handler 映射为 502+原因），不再发局域网 302。云盘 CDN 的公开 302 不受影响。

### R5. MSG 自生成的 STRM（指向自身 /api/stream）在 Emby 播放时多绕一圈公网

- 由 MSG 生成的 .strm 内容是带域名的绝对 `/api/stream/{id}` URL（`internal/service/strm_url.go` `strmPlaybackURL`，生成时域名可能是公网域名，也可能是局域网地址）。第三方客户端播这类条目时，要先请求包装行、再被引向目标行的绝对 URL 重新经 frp 进来——多一整圈公网往返+双倍请求处理；且元数据描述的是包装行（恒 strm）而非内容本身。
- 修复：
  - `internal/service/stream_redirect.go:112` `internalSTRMStreamMediaID` 识别自引用：路径须为 `/api/stream/` 或 `/emby/api/stream/` 前缀，绝对 URL 的主机须匹配配置的公网 BaseURL、当前请求主机、或私有局域网主机。
  - `internal/service/emby_media_source_preference.go:71` `ResolveInternalSTRMTargetMedia` 在视频流路由内服务端跟链解析（上限 8 跳、环检、逐跳按当前用户可见性校验），直接出目标行字节，客户端只发一次请求。
  - PlaybackInfo/Item 侧由 `internal/service/emby_playback.go:280` `embySTRMMetadataMedia` 借被引用行的元数据填 Container/Size/Bitrate/RunTimeTicks/MediaStreams（仅元数据，不涉字节与路径）。

### 未发现/未改

- Sessions/Playing、Progress 等上报路径沿用既有的异步请求日志队列（见 PLAYBACK_REMOTE_FIX.md §2），本次审计未发现同步 ffprobe 或同步写库的新证据，未改动。
- Emby 视频流的 Range 行为与网页端共用 `http.ServeContent`，本次新增路由级测试锁定 206（见下）。

## 修改文件

- internal/handler/emby_playback.go：PlaybackInfo 注入请求 context、直放协商（R2)、相对 URL（R4)、视频流路由改走 ResolvePlaybackMedia+自引用解析+ServeMediaForUser（R3/R5)。
- internal/service/emby_playback.go:mediaSource/baseMediaSource/embyMediaSourcePath/embySTRMMetadataMedia（R1/R4/R5)。
- internal/service/emby_media_sources.go：probe 文档批量加载、字幕 N+1、空 strm 轨道守卫、兄弟 context 缓存（R1/R3)。
- internal/service/emby_media_source_preference.go:ResolvePlaybackMedia/ResolveInternalSTRMTargetMedia（R3/R5)。
- internal/service/stream_redirect.go：请求 context 通道、自引用识别、私有主机判定（R4/R5)。
- internal/service/stream_file.go：私有主机 HTTP 目标拒绝再重定向（R4)。
- internal/service/emby_items_detail.go:Item 的 Container/Path 安全化与兄弟预计算（R3/R4)。
- internal/service/subtitle.go：抽出 discoverMedia 供按实体行发现字幕（R3 配套）。
- 测试：internal/handler/emby_playback_info_routes_test.go、internal/handler/emby_playback_routes_test.go、internal/service/emby_playback_test.go、internal/service/media_bitrate_test.go、internal/service/stream_local_strm_test.go。

## 测试结果（NAS golang:1.25-alpine 容器实跑）

- `go build ./...`：通过。
- `go test ./internal/handler/`：全包通过（含定向 Emby/Playback 子集先行通过）。
- `go test ./internal/service/`：全包通过（342s)。
- 新增锁定测试：
  - `TestEmbyPlaybackInfoForLocalSTRMUsesTargetContainerAndSafeURLs`：本地 STRM→MP4 的 PlaybackInfo 得到 Container=mp4、Size=目标文件大小、无 probe 时 Bitrate/RunTimeTicks 保持 0、MediaStreams 不编造；DeviceProfile 声明 mp4 时 SupportsDirectPlay/DirectStream=true、SupportsTranscoding=false 且无 TranscodingUrl;Path/DirectStreamUrl 为同源相对路由；响应全文不含临时目录、局域网 IP、.strm 文件名与目标路径。
  - `TestEmbyVideoStreamRouteServesLocalSTRMRange`：经 Emby 视频路由对本地 STRM 目标发 Range 请求得 206 与正确 Content-Range。
- 旧测试期望修正（审定方接手时处理）:`TestEmbyPlaybackInfoForLocalISOSTRM` 原断言 MediaSource `Path` 等于本地 ISO 绝对路径——那正是 R4 要消除的泄漏，期望已改为虚拟路由 `/Videos/local-iso/stream.iso`（与 Codex 已更新的 `TestEmbyMediaSourcePathFallsBackToCloudDisplayPath` 本地用例口径一致）；`TestBaseMediaSourceDerivesAverageBitRate` 随 baseMediaSource 签名更新。

## 代码无法改变的基础设施因素（用库内数据说话）

- 生产库实测：4K 条目 410 个，已探测平均码率 8.0 Mbps、p90 16.8 Mbps、平均体积 3.62 GB;1080p 355 个，平均 4.5 Mbps、p90 8.1 Mbps。这些 WEB-DL 编码的瞬时峰值通常是均值的 2–3 倍，即 4K 直放需要链路**持续** hold 住约 20–30 Mbps 才能不缓冲。
- 链路是「家庭上行 → frp 海外中转（单机带宽/延迟）→ 客户端下行」三段串联，任一段低于片源峰值码率就必然卡顿，这不是代码能修的。本次修复消除的是代码层的**额外乘数**：不必要的 NAS 转码（R2)、自引用多绕一圈公网（R5)、发给客户端的局域网地址（R4)、每个流请求的重复构建（R3)。
- NAS 为 2 核低功耗平台（内存 3.7 GB)，4K 实时转码不具备可行性；本次明确没有做「加转码能力」，而是让能直放的片源真正走直放。若用户要播的 4K 片源峰值长期超过链路能力，剩余选项只有换低码率版本或提升链路（上行/中转带宽），不在代码范围内。

## 部署前风险与行为变更

1. **Path 语义变更**：MediaSource.Path / Item.Path 从本地文件路径变为虚拟流路由。按 Emby 协议 Path 本为信息字段、播放走 DirectStreamUrl；但个别把 Path 当本地路径用的客户端行为会变（变为走标准流 URL，预期是改善）。
2. **URL 不再 absolutize**:Yamby/SenPlayer/Lenna 以前会拿到按请求 Host 拼的绝对 URL，现在与其他客户端一致拿相对 URL。新 PlaybackInfo 测试以 Yamby UA + 局域网 Host 请求验证了相对化与无泄漏；上线后建议先用主力客户端实测一集。
3. **网页端边缘行为变更**：局域网地址的自引用 strm 经网页 `/api/stream` 播放时，旧行为是 302 到局域网 URL（远程必超时、局域网可用），新行为是明确报 502 不可用；Emby 路由已在服务端解析这类引用，网页端未做跟链解析。公网域名的自引用在网页端仍是旧 302 行为，未动。
4. **直放协商是新逻辑**（约 180 行 DeviceProfile 求值）：已按 fail-closed 设计且有测试锁定，但各客户端上报的 DeviceProfile 差异大；上线后若某客户端出现「以前能播现在播不了」，优先怀疑其 profile 过度声明（声称支持实际不支持的编码）——此时它会拿到直放而非转码。
5. **自引用识别的宽窄**：私有主机上的 `/api/stream/` 形状 URL 一律按自引用处理；若库里有故意指向另一台局域网服务同形路径的 strm，会按本机媒体 ID 解析，ID 不存在或不可见时安全回退（视频路由返回 404、元数据借用不发生）。
6. `embySTRMMetadataMedia` 对自引用源在 PlaybackInfo/Item 构建时多一次按 ID 查行，相对于被消除的重复构建可忽略。
