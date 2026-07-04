import { Loader2, RotateCcw, Wrench } from 'lucide-react'

import type { STRMOutputPreset } from '../api/strm'
import { currentOrigin } from './strmPageModel'
import { StrmRefreshStatus } from './StrmGenerateSectionParts'
import { StrmOutputDirPicker } from './StrmOutputDirPicker'
import type { useStrmRepairForm } from './useStrmRepairForm'

type StrmRepairSectionProps = ReturnType<typeof useStrmRepairForm> & {
  outputPresets: STRMOutputPreset[]
}

export function StrmRepairSection({
  baseURL,
  outputDir,
  outputPresets,
  repairing,
  refreshLibrary,
  result,
  runningMode,
  scrapeAfter,
  onPreview,
  onRepair,
  setBaseURL,
  setOutputDir,
  setRefreshLibrary,
  setScrapeAfter,
}: StrmRepairSectionProps) {
  return (
    <section className="glass-panel space-y-4">
      <div>
        <h2 className="font-display text-lg font-semibold text-ink-600">STRM 修复</h2>
        <p className="text-sm text-ink-50">批量修复已存在 .strm 文件中的本服务播放地址。</p>
      </div>
      <form onSubmit={onRepair} className="grid gap-3 md:grid-cols-4">
        <StrmOutputDirPicker
          className="md:col-span-2"
          required
          presets={outputPresets}
          placeholder="STRM 输出目录"
          value={outputDir}
          onChange={setOutputDir}
        />
        <input
          className="input-base"
          placeholder="http://NAS-IP:18080 或 https://media.example.com"
          value={baseURL}
          onChange={(e) => setBaseURL(e.target.value)}
        />
        <button
          type="button"
          className="rounded-2xl border border-primary-400/40 px-3 py-2 text-sm text-brand-500 transition hover:bg-primary-400/10"
          onClick={() => setBaseURL(currentOrigin())}
        >
          使用当前访问地址
        </button>
        <label className="flex min-h-10 items-center gap-2 rounded-2xl border border-gray-200 bg-white/70 px-3 py-2 text-sm text-ink-50 md:col-span-4">
          <input type="checkbox" checked={refreshLibrary} onChange={(e) => setRefreshLibrary(e.target.checked)} />
          修复后刷新媒体库
        </label>
        <label className={`flex min-h-10 items-center gap-2 rounded-2xl border border-gray-200 bg-white/70 px-3 py-2 text-sm text-ink-50 md:col-span-4 ${!refreshLibrary ? 'cursor-not-allowed opacity-50' : ''}`}>
          <input
            type="checkbox"
            checked={refreshLibrary && scrapeAfter}
            disabled={!refreshLibrary}
            onChange={(e) => setScrapeAfter(e.target.checked)}
          />
          刷新后自动刮削
        </label>
        <button
          type="button"
          disabled={repairing || !outputDir.trim()}
          className="inline-flex min-h-10 items-center justify-center gap-2 rounded-2xl border border-primary-400/40 px-3 py-2 text-sm font-medium text-brand-500 transition hover:bg-primary-400/10 disabled:cursor-not-allowed disabled:opacity-50 md:col-span-2"
          onClick={onPreview}
        >
          {runningMode === 'preview' ? <Loader2 size={16} className="animate-spin" /> : <RotateCcw size={16} />}
          {runningMode === 'preview' ? '预检中...' : '预检修复'}
        </button>
        <button type="submit" disabled={repairing || !outputDir.trim()} className="neon-button md:col-span-2">
          {runningMode === 'repair' ? <Loader2 size={16} className="animate-spin" /> : <Wrench size={16} />}
          {runningMode === 'repair' ? '修复中...' : '修复 STRM'}
        </button>
      </form>
      {result && (
        <div className="rounded-2xl border border-gray-200 bg-gray-50 p-4 text-sm text-ink-50">
          <div className="font-semibold text-ink-600">输出目录：{result.output_dir}</div>
          <div className="mt-1">
            {result.previewed ? `预检 ${result.previewed} · ` : ''}
            修复 {result.repaired} · 跳过 {result.skipped}
          </div>
          <StrmRefreshStatus refresh={result.refresh} />
          {result.errors && result.errors.length > 0 && (
            <div className="mt-2 text-red-500">
              失败 {result.errors.length} 条：{result.errors.slice(0, 3).join('；')}
            </div>
          )}
        </div>
      )}
    </section>
  )
}
