import { useEffect, useState } from 'react'
import { ChevronLeft, ChevronRight } from 'lucide-react'

type PaginationProps = {
  page: number
  totalPages: number
  onPageChange: (page: number) => void
  className?: string
}

export function Pagination({ page, totalPages, onPageChange, className = '' }: PaginationProps) {
  const [draft, setDraft] = useState(String(page))
  const boundedTotal = Math.max(1, totalPages)

  useEffect(() => setDraft(String(page)), [page])

  const commit = () => {
    const parsed = Number.parseInt(draft, 10)
    const next = Number.isFinite(parsed) ? Math.min(boundedTotal, Math.max(1, parsed)) : page
    setDraft(String(next))
    if (next !== page) onPageChange(next)
  }

  return (
    <nav className={`flex min-w-0 flex-wrap items-center justify-center gap-2 ${className}`} aria-label="分页">
      <button
        type="button"
        className="cinema-raised-control group inline-flex h-11 shrink-0 items-center justify-center gap-1.5 rounded-xl border border-[var(--app-border)] bg-[var(--app-panel)] px-3 text-[var(--app-subtle)] transition-colors hover:border-[var(--app-accent)] hover:bg-[var(--app-hover)] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--app-accent)] disabled:pointer-events-none disabled:opacity-30 sm:px-4"
        disabled={page <= 1}
        onClick={() => onPageChange(Math.max(1, page - 1))}
        aria-label="上一页"
        title="上一页"
      >
        <ChevronLeft size={16} />
        <span className="hidden text-xs font-medium min-[400px]:inline">上一页</span>
      </button>
      <div className="flex h-11 min-w-0 items-center gap-2 whitespace-nowrap rounded-xl border border-[var(--app-border)] bg-[var(--app-panel-soft)] px-2.5 text-xs text-[var(--app-muted)]">
        <input
          type="number"
          min={1}
          max={boundedTotal}
          inputMode="numeric"
          className="h-8 w-11 rounded-lg border border-transparent bg-transparent px-1 text-center text-base font-semibold tabular-nums text-[var(--app-text)] outline-none transition-colors hover:bg-[var(--app-hover)] focus:border-[var(--app-accent)] focus:bg-[var(--app-panel)] sm:text-sm [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none [appearance:textfield]"
          style={{ width: `${Math.max(3, Math.min(9, String(boundedTotal).length + 1))}ch` }}
          value={draft}
          onChange={(event) => setDraft(event.target.value)}
          onBlur={commit}
          onKeyDown={(event) => {
            if (event.key === 'Enter') event.currentTarget.blur()
          }}
          aria-label="跳转页码"
        />
        <span className="select-none text-[var(--app-muted)]" aria-hidden="true">/</span>
        <span className="pr-1 tabular-nums" aria-label={`共 ${boundedTotal} 页`}>{boundedTotal}</span>
      </div>
      <button
        type="button"
        className="cinema-raised-control group inline-flex h-11 shrink-0 items-center justify-center gap-1.5 rounded-xl border border-[var(--app-border)] bg-[var(--app-panel)] px-3 text-[var(--app-subtle)] transition-colors hover:border-[var(--app-accent)] hover:bg-[var(--app-hover)] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--app-accent)] disabled:pointer-events-none disabled:opacity-30 sm:px-4"
        disabled={page >= boundedTotal}
        onClick={() => onPageChange(Math.min(boundedTotal, page + 1))}
        aria-label="下一页"
        title="下一页"
      >
        <span className="hidden text-xs font-medium min-[400px]:inline">下一页</span>
        <ChevronRight size={16} />
      </button>
    </nav>
  )
}
