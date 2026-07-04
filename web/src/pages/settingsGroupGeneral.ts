import type { SettingGroup } from './settingsGroupTypes'

export const generalSettingsGroup: SettingGroup = {
  key: 'general',
  label: '常规',
  description: '语言 / 转码引擎参数（API 密钥请在管理后台 → 外部API 配置）',
  items: [
    {
      key: 'tmdb.language',
      label: 'TMDb 元数据语言',
      type: 'select',
      options: [
        { value: 'zh-CN', label: '简体中文' },
        { value: 'zh-TW', label: '繁体中文' },
        { value: 'en-US', label: 'English' },
        { value: 'ja-JP', label: '日本語' },
      ],
    },
    {
      key: 'app.server_url',
      label: '公开访问域名 / STRM 域名',
      type: 'text',
      hint: '例如 http://NAS-IP:18080 或 https://media.example.com。填写后网盘媒体扫描会自动生成完整 STRM/302 播放入口；不填则使用同源相对路径。',
      placeholder: 'http://192.168.1.125:18080',
    },
    {
      key: 'playback.direct_only',
      label: '客户端直连解码（释放宿主机资源）',
      type: 'toggle',
      hint: '默认关闭。开启后宿主机不再进行任何 FFmpeg 转码，所有播放交给第三方客户端（Infuse / VLC / Emby 客户端等）或浏览器本地解码直连（direct play / 302 直链），大幅降低宿主机 CPU 占用。若客户端不支持源编码可能无法播放。',
      defaultValue: 'false',
    },
    {
      key: 'transcode.enabled',
      label: '启用转码',
      type: 'toggle',
      hint: '关闭后所有视频直连播放（「客户端直连解码」开启时本项自动失效）',
      defaultValue: 'true',
    },
    {
      key: 'transcode.hw_accel',
      label: '硬件编码器',
      type: 'select',
      hint: '只有开启下方「启用硬件加速」后才会使用；未开启时强制软件转码',
      defaultValue: 'none',
      options: [
        { value: 'none', label: '软件转码' },
        { value: 'nvenc', label: 'NVIDIA NVENC' },
        { value: 'qsv', label: 'Intel QSV' },
        { value: 'vaapi', label: 'VAAPI (Linux)' },
      ],
    },
    {
      key: 'transcode.hw_enabled',
      label: '启用硬件加速',
      type: 'toggle',
      hint: '关闭时即使选择了 NVENC/QSV/VAAPI，也不会调用硬件编码参数',
      defaultValue: 'false',
    },
    {
      key: 'transcode.max_jobs',
      label: '最大并发转码任务',
      type: 'number',
      hint: 'NAS 建议 1',
      defaultValue: '1',
    },
    {
      key: 'transcode.realtime',
      label: '按播放速度转码',
      type: 'toggle',
      hint: '开启后 ffmpeg 不会抢跑压完整片，可显著降低 CPU 峰值',
      defaultValue: 'true',
    },
    {
      key: 'transcode.threads',
      label: '软件转码线程数',
      type: 'number',
      hint: 'NAS 建议 1-2；仅软件转码生效',
      defaultValue: '2',
    },
    {
      key: 'transcode.idle_timeout_seconds',
      label: '转码空闲停止秒数',
      type: 'number',
      hint: '播放器关闭或停止请求分片后自动结束 ffmpeg',
      defaultValue: '120',
    },
    {
      key: 'ffmpeg.path',
      label: 'FFmpeg 路径',
      type: 'text',
      placeholder: 'ffmpeg',
    },
    {
      key: 'ffprobe.path',
      label: 'FFprobe 路径',
      type: 'text',
      placeholder: 'ffprobe',
    },
    {
      key: 'ffprobe.max_concurrent',
      label: 'FFprobe 最大并发',
      type: 'number',
      hint: 'NAS 建议 1；用于扫描、整理洗版和手动探测，避免同时启动多个 ffprobe 进程',
      defaultValue: '1',
    },
  ],
}

export const licenseSettingsGroup: SettingGroup = {
  key: 'license',
  label: '授权服务',
  description: '连接私有 MediaStationGo 授权服务；开源版默认最多 20 个用户，激活后按授权策略提升额度。',
  items: [
    {
      key: 'license.server_url',
      label: 'License Server 地址',
      type: 'text',
      placeholder: 'https://mgosever.3jzs.com',
    },
    {
      key: 'license.public_key',
      label: 'Ed25519 验签公钥',
      type: 'text',
      hint: '优先使用。客户端只保存公钥，无法伪造授权服务响应；自建 MgoSever 时填写私钥对应的公钥。',
      placeholder: 'MCowBQYDK2VwAyEA...',
    },
    {
      key: 'license.hmac_secret',
      label: 'HMAC 签名密钥（旧版兼容）',
      type: 'text',
      hint: '仅兼容旧版授权服务。新版本应使用 Ed25519 公钥，避免把共享密钥编译进客户端。',
    },
  ],
}
