import {
  ConfigInput,
  ConfigSelect,
  ConfigTextarea,
} from './NotifyChannelConfigControls'

type ProviderFieldsProps = {
  config: Record<string, string>
  updateConfig: (key: string, value: string) => void
}

const METHOD_OPTIONS = [
  { value: 'POST', label: 'POST' },
  { value: 'GET', label: 'GET' },
]

const TLS_OPTIONS = [
  { value: 'true', label: '启用' },
  { value: 'false', label: '关闭' },
]

export function TelegramFields({ config, updateConfig }: ProviderFieldsProps) {
  const control = { config, updateConfig }

  return (
    <>
      <ConfigInput
        {...control}
        required
        label="Bot Token"
        name="bot_token"
        placeholder="123456:ABC-DEF…"
      />
      <ConfigInput
        {...control}
        required
        label="管理员 Telegram ID"
        name="admin_user_ids"
        placeholder="多个用逗号分隔，如 123456789,987654321"
      />
      <ConfigInput
        {...control}
        label="绑定群组 ID"
        name="group_chat_id"
        placeholder="选填，如 -1001234567890；填写后群组成员可唤醒/绑定"
      />
      <ConfigInput
        {...control}
        label="绑定频道 ID"
        name="channel_chat_id"
        placeholder="选填，如 -1009876543210；填写后频道成员可唤醒/绑定"
      />
      <ConfigInput
        {...control}
        label="Telegram API 地址 (可选)"
        name="api_base_url"
        placeholder="默认 https://api.telegram.org；反代可填 https://tg.example.com"
      />
      <ConfigInput
        {...control}
        label="Telegram 代理 (可选)"
        name="proxy_url"
        placeholder="如 http://172.17.0.1:7890 或 socks5://172.17.0.1:1080"
      />
      <div className="rounded-2xl border border-primary-400/15 bg-primary-400/5 px-4 py-3 text-xs leading-6 text-ink-50">
        群组 ID、频道 ID 均为选填，可填一个、两个都填，也可以不填。管理功能始终仅管理员 Telegram ID 或已绑定的本地管理员可用；普通用户需要在已配置的群组或频道中，才能使用 <code>/start 用户名 密码</code> 绑定账号、切换隐藏成人媒体库和目录。不配置群组/频道时，普通用户不会被放行。若测试通知超时，可填写反代 API 地址或代理地址。
      </div>
    </>
  )
}

export function WechatFields({ config, updateConfig }: ProviderFieldsProps) {
  const control = { config, updateConfig }

  return (
    <ConfigInput
      {...control}
      required
      label="SendKey"
      name="sendkey"
      placeholder="SCT…"
    />
  )
}

export function BarkFields({ config, updateConfig }: ProviderFieldsProps) {
  const control = { config, updateConfig }

  return (
    <>
      <ConfigInput
        {...control}
        required
        label="设备 Key"
        name="device_key"
      />
      <ConfigInput
        {...control}
        label="服务器地址 (可选)"
        name="server"
        placeholder="https://api.day.app"
      />
    </>
  )
}

export function WebhookFields({ config, updateConfig }: ProviderFieldsProps) {
  const control = { config, updateConfig }

  return (
    <>
      <ConfigInput
        {...control}
        required
        label="URL"
        name="url"
        placeholder="https://example.com/notify"
      />
      <ConfigSelect
        {...control}
        label="Method"
        name="method"
        options={METHOD_OPTIONS}
        defaultValue="POST"
      />
      <ConfigTextarea
        {...control}
        label="Headers (JSON)"
        name="headers"
        rows={2}
        placeholder='{"Content-Type":"application/json"}'
      />
      <ConfigTextarea
        {...control}
        label="Body 模板 (支持 {{title}} {{message}})"
        name="body_template"
        rows={3}
        placeholder='{"title":"{{title}}","message":"{{message}}"}'
      />
    </>
  )
}

export function EmailFields({ config, updateConfig }: ProviderFieldsProps) {
  const control = { config, updateConfig }

  return (
    <>
      <ConfigInput
        {...control}
        required
        label="SMTP 地址"
        name="smtp_host"
        placeholder="smtp.gmail.com"
      />
      <div className="grid grid-cols-2 gap-3">
        <ConfigInput
          {...control}
          required
          label="SMTP 端口"
          name="smtp_port"
          placeholder="465"
        />
        <ConfigSelect
          {...control}
          label="TLS"
          name="tls"
          options={TLS_OPTIONS}
          defaultValue="true"
        />
      </div>
      <div className="grid grid-cols-2 gap-3">
        <ConfigInput
          {...control}
          required
          label="用户名"
          name="username"
        />
        <ConfigInput
          {...control}
          required
          label="密码"
          name="password"
          type="password"
        />
      </div>
      <ConfigInput
        {...control}
        required
        label="发件人"
        name="from"
        placeholder="noreply@example.com"
      />
      <ConfigInput
        {...control}
        required
        label="收件人 (多个用逗号分隔)"
        name="to"
        placeholder="user@example.com"
      />
    </>
  )
}
