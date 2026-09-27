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
    <nav className={`flex items-center justify-center gap-2 ${className}`} aria-label="分页">
      <button
        type="button"
        className="btn-outline h-9 w-9 justify-center p-0"
        disabled={page <= 1}
        onClick={() => onPageChange(Math.max(1, page - 1))}
        aria-label="上一页"
        title="上一页"
      >
        <ChevronLeft size={16} />
      </button>
      <div className="flex h-9 items-center gap-1 whitespace-nowrap rounded-lg border border-gray-200 bg-white px-2 text-sm text-ink-50">
        <input
          type="number"
          min={1}
          max={boundedTotal}
          inputMode="numeric"
          className="h-7 w-12 rounded border border-gray-200 bg-gray-50 px-1 text-center text-base font-semibold tabular-nums text-ink-600 outline-none focus:border-brand-400 sm:text-sm"
          value={draft}
          onChange={(event) => setDraft(event.target.value)}
          onBlur={commit}
          onKeyDown={(event) => {
            if (event.key === 'Enter') event.currentTarget.blur()
          }}
          aria-label="跳转页码"
        />
        <span className="tabular-nums">/ {boundedTotal}</span>
      </div>
      <button
        type="button"
        className="btn-outline h-9 w-9 justify-center p-0"
        disabled={page >= boundedTotal}
        onClick={() => onPageChange(Math.min(boundedTotal, page + 1))}
        aria-label="下一页"
        title="下一页"
      >
        <ChevronRight size={16} />
      </button>
    </nav>
  )
}
