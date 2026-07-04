import type { SettingGroup } from './settingsGroupTypes'

export const cloudUploadSettingsGroup: SettingGroup = {
  key: 'cloud-upload',
  label: '网盘转存',
  description: '把本地媒体复制上传到外部存储。推荐：将 115、123、阿里等网盘挂载到 OpenList、CloudDrive2 或 Alist 后，使用桥接目标转存。',
  items: [
    {
      key: 'cloud.auto_sync_enabled',
      label: '夜间自动同步网盘媒体库',
      type: 'toggle',
      hint: '默认关闭。开启后仅在每天 23:00-05:00 按检查间隔触发；每次完整扫描所有启用网盘库一次后自动停止。手动扫描仍可随时执行。',
      defaultValue: 'false',
    },
    {
      key: 'cloud.sync_interval_seconds',
      label: '夜间窗口检查间隔秒数',
      type: 'number',
      hint: '最小 300 秒，建议 1800 秒；同一个夜间窗口成功同步后不会重复全量扫，避免大型网盘反复递归。',
      defaultValue: '1800',
    },
    {
      key: 'cloud.boot_scan_enabled',
      label: '启动后立即扫描网盘',
      type: 'toggle',
      hint: '默认关闭。仅排障或小型网盘建议开启；大型库请使用手动扫描或夜间自动同步。',
      defaultValue: 'false',
    },
    {
      key: 'cloud.upload_auto_enabled',
      label: '启用自动转存',
      type: 'toggle',
      hint: '开启后后台会按间隔扫描本地源目录，把视频、NFO、海报、字幕转存到目标存储；还需要在外部存储页开启该目标的“允许转存写入”。',
      defaultValue: 'false',
    },
    {
      key: 'cloud.upload_provider',
      label: '转存目标',
      type: 'select',
      defaultValue: 'alist',
      options: [
        { value: 'openlist', label: 'OpenList（推荐，可桥接 115/123/阿里等）' },
        { value: 'clouddrive2', label: 'CloudDrive2（推荐，可桥接 115/123/阿里等）' },
        { value: 'alist', label: 'Alist（可桥接多网盘）' },
        { value: 'webdav', label: 'WebDAV' },
        { value: 'cloud115', label: '115 原生（待接分片上传）' },
      ],
    },
    {
      key: 'cloud.upload_source_dir',
      label: '本地源目录',
      type: 'text',
      placeholder: '/media/电影 或 F:\\media\\Movies',
    },
    {
      key: 'cloud.upload_dest_path',
      label: '网盘目标目录',
      type: 'text',
      defaultValue: '/MediaStationGo',
      placeholder: '/MediaStationGo',
    },
    {
      key: 'cloud.upload_recursive',
      label: '递归扫描源目录',
      type: 'toggle',
      defaultValue: 'true',
    },
    {
      key: 'cloud.upload_sidecars',
      label: '同步 NFO / 海报 / 字幕',
      type: 'toggle',
      defaultValue: 'true',
    },
    {
      key: 'cloud.upload_overwrite',
      label: '覆盖远端同名文件',
      type: 'toggle',
      defaultValue: 'false',
    },
    {
      key: 'cloud.upload_transfer_mode',
      label: '自动转存方式',
      type: 'select',
      defaultValue: 'copy',
      hint: '复制会保留本地源文件；移动只在上传成功后删除本地文件。',
      options: [
        { value: 'copy', label: '复制' },
        { value: 'move', label: '移动' },
      ],
    },
    {
      key: 'cloud.upload_interval_seconds',
      label: '自动转存间隔秒数',
      type: 'number',
      hint: '最小 300 秒，建议 3600 秒或更高，避免频繁读盘和触发网盘风控。',
      defaultValue: '3600',
    },
  ],
}
