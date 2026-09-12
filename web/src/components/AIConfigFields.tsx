import { AI_PROTOCOLS, readAIOptions } from './aiConfigModel'

export function AIConfigFields({ provider, extra, onExtraChange, baseURL, onBaseURLChange }: {
  provider: string
  extra: string
  onExtraChange: (value: string) => void
  baseURL: string
  onBaseURLChange: (value: string) => void
}) {
  const options = readAIOptions(extra, provider)
  const isOpenAI = options.protocol === 'openai' || options.protocol === 'responses'
  return (
    <div className="w-full grid gap-3 md:grid-cols-2">
      <label className="text-xs text-ink-50">
        AI 接口协议
        <select className="input-base mt-1 w-full" value={options.protocol} onChange={(event) => {
          const protocol = event.target.value
          const next: typeof options = { ...options, protocol }
          delete next.provider
          const family = (value: string) => value === 'responses' ? 'openai' : value
          if (family(protocol) !== family(options.protocol)) next.model = ''
          onExtraChange(JSON.stringify(next))
          const currentBase = baseURL.trim().replace(/\/+$/, '')
          if (!currentBase || AI_PROTOCOLS.some((entry) => entry.base === currentBase)) {
            onBaseURLChange(AI_PROTOCOLS.find((entry) => entry.value === protocol)?.base || '')
          }
        }}>
          {AI_PROTOCOLS.map((entry) => <option key={entry.value} value={entry.value}>{entry.label}</option>)}
        </select>
      </label>
      <label className="text-xs text-ink-50">
        模型 ID
        <input className="input-base mt-1 w-full" required={!isOpenAI} value={options.model}
          placeholder={isOpenAI ? '留空使用服务端模型，默认 gpt-4o-mini' : '填写服务商提供或本地已安装的模型 ID'}
          onChange={(event) => onExtraChange(JSON.stringify({ ...options, model: event.target.value }))} />
      </label>
      <p className="text-xs text-sand-500 md:col-span-2">
        Base URL 支持自定义代理地址。更换服务协议后需重新填写密钥；Ollama 本地服务可留空。
      </p>
    </div>
  )
}
