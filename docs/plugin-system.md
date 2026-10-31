# 插件管理

插件管理用于启停、配置和手动运行随 MediaStationGo 编译发布的 Go 扩展。只有当前有效的管理员账号可以访问；当前不提供插件市场、文件上传安装、动态加载或定时运行。

启用状态和配置保存到数据库设置中，重启后保留。最近一次成功结果、最近一次错误和运行状态只存在当前服务进程内存中，重启后清空；页面上的“运行记录”不是历史记录列表。

## 快速使用

1. 使用管理员账号登录，进入“管理后台”，在“统一管理入口”中打开“插件管理”。
2. 找到内置的“媒体库概览”，点击“启用插件”。新插件默认停用。
3. 按需要勾选“包含已停用的媒体库”，点击“保存配置”。有未保存修改时不能运行。
4. 点击“立即运行”，查看媒体库数量、媒体文件数量、完成时间和耗时。
5. 需要重新读取服务器状态时点击“刷新”。请求失败后可点击“重新加载”。

启用不会自动执行插件。运行过程中不能再次运行，也不能更新该插件的配置或启停状态。运行失败时，最近一次成功结果仍会保留；可刷新查看服务器记录的错误状态，再重试。

## 媒体库概览统计什么

内置插件 ID 是 `library-summary`，版本为 `1.0.0`，声明的能力是 `library:read`。它只查询数据库，不修改媒体，也不触发扫描。

| 配置 | 类型 | 默认值 | 作用 |
| --- | --- | --- | --- |
| `include_disabled` | boolean | `false` | 为 `true` 时纳入已停用媒体库及其媒体记录 |

默认统计已启用且未删除的媒体库，以及这些库下未删除的媒体记录。已删除媒体库、已删除媒体记录，以及没有匹配媒体库的媒体记录始终不计入。媒体文件数量是数据库 `Media` 记录数量，不是实时扫描磁盘所得的数量，也不是按作品去重后的数量。

## 调用管理 API

所有路径以 `/api/admin` 为前缀，使用应用的 JWT 身份认证。服务端每次检查账号有效性和当前管理员角色；旧令牌中的管理员身份不能绕过角色降级。

| 方法与路径 | 请求体 | 成功响应 |
| --- | --- | --- |
| `GET /api/admin/plugins` | 无 | `200`，`{"items": [...]}`，按插件 ID 排序 |
| `PATCH /api/admin/plugins/:id` | `enabled`、`config` 至少提供一个非空有效值 | `200`，更新后的 `PluginInfo` |
| `POST /api/admin/plugins/:id/run` | 无 | `200`，本次 `PluginRunResult`；请求同步等待运行结束 |

例如，启用示例插件并保存完整配置，PATCH 请求体为：

```json
{
  "enabled": true,
  "config": { "include_disabled": false }
}
```

`enabled: false` 是有效更新。`config` 是完整替换，不是按字段合并；必须包含 manifest 声明的所有字段，只能使用布尔值，不接受未知字段。PATCH 请求限制为 16 KiB，拒绝未知顶层字段、无效 JSON、多段 JSON，以及没有可更新内容的请求。

`PluginInfo` 包含以下字段：

| 字段 | 类型 | 含义 |
| --- | --- | --- |
| `id`, `name`, `description`, `version`, `author` | string | 插件标识和说明 |
| `capabilities` | string[] | 插件声明的能力标签 |
| `config_fields` | object[] | 每项包含 `key`、`label`、`description`、`type`、`default`；目前 `type` 只能为 `boolean`，`default` 为布尔值 |
| `enabled` | boolean | 持久化的启用状态 |
| `config` | object | 当前配置 |
| `status` | string | `disabled`、`ready`、`running` 或 `error` |
| `last_run` | object 或 null | 最近一次成功结果，初始为 null |
| `last_error` | string | 最近一次运行失败的通用提示，无错误时为空字符串 |

`PluginRunResult` 包含 `summary`（说明文本）、`metrics`（`label` 与整数 `value` 数组）、`completed_at`（UTC 完成时间）及 `duration_ms`（毫秒耗时）。成功运行清除 `last_error`；停用时状态显示为 `disabled`，不会删除已有结果。

### 错误处理

插件处理器的错误响应为 `{"error": "说明"}`；身份中间件使用 `{"code": 数字, "message": "说明"}`。

| HTTP 状态 | 条件 | 处理建议 |
| --- | --- | --- |
| `400` | 配置格式、字段、类型或请求体无效 | 按 manifest 提交完整配置 |
| `401` | 未登录、令牌无效或账号不存在 | 重新登录 |
| `403` | 非管理员、账号停用或过期 | 使用有效管理员账号 |
| `404` | 插件 ID 不存在 | 重新读取插件列表 |
| `409` | 插件停用，或同一插件正在运行 | 先启用，或等待当前运行结束 |
| `500` | 设置读取/保存失败、插件内部错误或 panic | 查看服务日志后重试 |
| `503` | 插件服务不可用，或运行被取消 | 确认服务状态后重试 |
| `504` | 插件运行 context 超时 | 查看插件执行逻辑和依赖响应时间 |

身份错误码包括 `40101`（身份无效）、`40301`（需要管理员）、`40302`（账号停用）和 `40303`（账号过期）。身份查询遇到暂时的数据库锁时也可返回 HTTP `503`、错误码 `50301` 及 `Retry-After: 1`。请求断开或服务关闭会取消运行 context；连接已断开时客户端不一定能收到错误响应。

## 开发新的 Go 插件

接口定义见 [plugin_types.go](../internal/service/plugin_types.go)，示例见 [plugin_library_summary.go](../internal/service/plugin_library_summary.go)。在 `internal/service` 包实现：

```go
type Plugin interface {
    Manifest() PluginManifest
    ValidateConfig(map[string]any) error
    Run(context.Context, map[string]any) (PluginRunResult, error)
}
```

`Manifest` 需要提供非空名称与版本；ID 必须匹配 `^[a-z][a-z0-9-]{0,63}$` 且不能重复。配置键必须非空且不能重复，目前只接受 `boolean` 字段。注册时会调用 `ValidateConfig` 验证默认配置，保存和加载配置时也会验证。

完成实现后，在 [plugins.go](../internal/service/plugins.go) 的 `NewPluginService` 中、`return s` 前调用 `s.Register` 注册实例。现有示例的注册方式如下，可作为依赖注入和错误处理的参考：

```go
if err := s.Register(&librarySummaryPlugin{db: repos.DB}); err != nil {
    panic(err)
}
```

注册只允许在构造服务、开始处理请求之前执行。添加新实现及注册代码后，需要重新构建并部署应用。能力标签用于描述扩展行为，当前不是权限沙箱；插件与应用在同一进程运行。

### 执行和存储约束

- 管理器复制并验证配置后传入 `Run`。启用状态与配置以 `plugins.<id>` 为设置键存储，值是包含 `enabled` 和 `config` 的 JSON。通用设置写入接口拒绝写入该命名空间，应使用插件 PATCH 接口。
- 同一进程内，同一插件只能有一个运行实例。运行时更新返回 `ErrPluginBusy`；此锁和运行记录不跨服务实例共享。
- 每次执行获得 10 秒超时的 context，并继承请求取消与服务关闭取消。插件必须主动遵守 context，例如数据库查询使用 `WithContext(ctx)`，外部请求传入 ctx，循环检查 `ctx.Err()`。
- 超时是协作式取消，不会强制终止 Go 函数。不遵守 context 的插件可能超过 10 秒才返回，期间仍处于运行状态。不要启动脱离本次运行管理的后台任务。
- 管理器恢复同一执行调用内的 panic，将失败保存为通用提示；详细错误交给服务日志。它不会保存完整运行历史。

## 验证修改

在仓库根目录运行插件服务与接口测试：

```powershell
go test ./internal/service -run TestPlugin -count=1
go test ./internal/handler -run 'TestAdminPlugin|TestAdminSettingsCannotWritePluginState' -count=1
```

检查前端类型、构建和 lint：

```powershell
npm --prefix web run build
npm --prefix web run lint
```

另需使用管理员与普通账号手动验证导航和权限，并检查启停、保存后运行、失败后刷新及窄屏布局。以上是复现检查命令，不代表当前环境已执行通过。
