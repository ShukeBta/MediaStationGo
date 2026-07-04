import { ImageOff, ImagePlus } from 'lucide-react'

type EpisodeArtworkToggleProps = {
  checked: boolean
  onChange: (checked: boolean) => void
  title?: string
  className?: string
}

export function EpisodeArtworkToggle({
  checked,
  onChange,
  title = '关闭后仍会写入每集文字元数据，只跳过每集图片',
  className = '',
}: EpisodeArtworkToggleProps) {
  const Icon = checked ? ImagePlus : ImageOff
  const controlStyle = checked
    ? { backgroundColor: 'var(--app-brand-soft)', borderColor: 'var(--app-brand-border)', color: 'var(--app-brand-text)' }
    : undefined
  const emphasisStyle = checked
    ? { backgroundColor: 'var(--app-brand-emphasis)', color: 'var(--app-brand-text)' }
    : undefined

  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label="获取每集图片"
      title={title}
      onClick={() => onChange(!checked)}
      data-state={checked ? 'on' : 'off'}
      style={controlStyle}
      className={[
        'episode-artwork-toggle group inline-flex h-11 shrink-0 items-center gap-2 rounded-xl border px-3 text-sm font-semibold shadow-sm transition-all duration-200',
        checked ? 'episode-artwork-toggle--on' : '',
        'focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-500',
        'hover:-translate-y-0.5 hover:shadow-md active:translate-y-0 active:scale-[0.98]',
        className,
      ].join(' ')}
    >
      <span
        className="episode-artwork-toggle__icon grid h-7 w-7 place-items-center rounded-lg transition-colors"
        style={emphasisStyle}
      >
        <Icon size={16} />
      </span>
      <span className="whitespace-nowrap">每集图片</span>
      <span
        className="episode-artwork-toggle__state rounded-full px-2 py-0.5 text-[11px] font-bold"
        style={emphasisStyle}
      >
        {checked ? '开启' : '关闭'}
      </span>
    </button>
  )
}
