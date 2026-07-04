import { Loader2, Play, RefreshCw, Save } from 'lucide-react'

import type { RecognitionWordsConfig, RecognitionWordsTestResult } from '../api/recognitionWords'

export function RecognitionWordsHeader({
  config,
  onEnabledChange,
}: {
  config: RecognitionWordsConfig
  onEnabledChange: (enabled: boolean) => void
}) {
  return (
    <div className="flex flex-wrap items-center justify-between gap-3">
      <div>
        <div className="text-sm font-semibold text-ink-600">自定义识别词</div>
        <div className="text-xs text-sand-500">
          当前已加载 {config.rule_count} 条规则{config.synced_at ? ` · 上次同步 ${new Date(config.synced_at).toLocaleString()}` : ''}
        </div>
      </div>
      <label className="flex cursor-pointer items-center gap-2 text-sm text-ink-100">
        <input
          type="checkbox"
          className="h-4 w-4 accent-primary-400"
          checked={config.enabled}
          onChange={(event) => onEnabledChange(event.target.checked)}
        />
        启用
      </label>
    </div>
  )
}

export function RecognitionWordsEditors({
  config,
  onLocalTextChange,
  onSharedURLsChange,
}: {
  config: RecognitionWordsConfig
  onLocalTextChange: (value: string) => void
  onSharedURLsChange: (value: string) => void
}) {
  return (
    <div className="grid gap-4 lg:grid-cols-2">
      <label className="space-y-2">
        <div className="text-sm font-medium text-ink-100">共享识别词源</div>
        <textarea
          rows={7}
          className="input-base font-mono text-xs"
          value={config.shared_urls.join('\n')}
          onChange={(event) => onSharedURLsChange(event.target.value)}
        />
      </label>
      <label className="space-y-2">
        <div className="text-sm font-medium text-ink-100">本地识别词</div>
        <textarea
          rows={7}
          className="input-base font-mono text-xs"
          placeholder={'屏蔽词\n错误标题 => 正确标题\nS01E <> . >> EP-10'}
          value={config.local_text}
          onChange={(event) => onLocalTextChange(event.target.value)}
        />
      </label>
    </div>
  )
}

export function RecognitionWordsTester({
  input,
  result,
  testing,
  onInputChange,
  onTest,
}: {
  input: string
  result: RecognitionWordsTestResult | null
  testing: boolean
  onInputChange: (value: string) => void
  onTest: () => void
}) {
  return (
    <div className="rounded-lg border border-white/10 bg-black/10 p-3">
      <div className="grid gap-3 md:grid-cols-[1fr_auto]">
        <input
          className="input-base"
          placeholder="输入文件名或种子标题测试识别效果"
          value={input}
          onChange={(event) => onInputChange(event.target.value)}
        />
        <button type="button" className="neon-button" onClick={onTest} disabled={testing || !input.trim()}>
          {testing ? <Loader2 size={16} className="animate-spin" /> : <Play size={16} />}
          测试
        </button>
      </div>
      {result && <RecognitionWordsTestResultGrid result={result} />}
    </div>
  )
}

function RecognitionWordsTestResultGrid({ result }: { result: RecognitionWordsTestResult }) {
  return (
    <div className="mt-3 grid gap-2 text-xs text-ink-100 md:grid-cols-3">
      <RecognitionWordsResultCard label="识别后" value={result.output || '-'} mono />
      <RecognitionWordsResultCard label="标题" value={result.title || '-'} />
      <RecognitionWordsResultCard label="年份" value={result.year || '-'} />
    </div>
  )
}

function RecognitionWordsResultCard({
  label,
  mono,
  value,
}: {
  label: string
  mono?: boolean
  value: string | number
}) {
  return (
    <div className="rounded-md bg-white/5 p-2">
      <div className="text-sand-500">{label}</div>
      <div className={mono ? 'break-all font-mono' : ''}>{value}</div>
    </div>
  )
}

export function RecognitionWordsActions({
  saving,
  syncing,
  onSave,
  onSync,
}: {
  saving: boolean
  syncing: boolean
  onSave: () => void
  onSync: () => void
}) {
  return (
    <div className="flex flex-wrap justify-end gap-2">
      <button type="button" className="btn-secondary" onClick={onSync} disabled={syncing}>
        {syncing ? <Loader2 size={16} className="animate-spin" /> : <RefreshCw size={16} />}
        同步共享词
      </button>
      <button type="button" className="neon-button" onClick={onSave} disabled={saving}>
        {saving ? <Loader2 size={16} className="animate-spin" /> : <Save size={16} />}
        保存
      </button>
    </div>
  )
}
