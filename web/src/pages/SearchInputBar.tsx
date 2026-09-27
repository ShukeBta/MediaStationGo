import type { ChangeEvent, FormEvent } from 'react'
import { X } from 'lucide-react'

type SearchInputBarProps = {
  aiOn: boolean
  query: string
  onQueryChange: (query: string) => void
  onClear: () => void
  onAISubmit: (event: FormEvent) => void
}

export function SearchInputBar({ aiOn, query, onQueryChange, onClear, onAISubmit }: SearchInputBarProps) {
  if (aiOn) {
    return (
      <form onSubmit={onAISubmit} className="flex flex-wrap gap-2">
        <SearchTextField
          placeholder='例如:"2010 年后的科幻电影" / "最近的动漫"'
          query={query}
          onQueryChange={onQueryChange}
          onClear={onClear}
        />
        <button type="submit" className="neon-button">
          搜索
        </button>
      </form>
    )
  }

  return (
    <SearchTextField
      placeholder="搜索片名、原名、演员或类型…"
      query={query}
      onQueryChange={onQueryChange}
      onClear={onClear}
    />
  )
}

function SearchTextField({
  placeholder,
  query,
  onQueryChange,
  onClear,
}: {
  placeholder: string
  query: string
  onQueryChange: (query: string) => void
  onClear: () => void
}) {
  return (
    <div className="relative min-w-0 flex-1">
      <input
        autoFocus
        className="input-base w-full pr-11"
        placeholder={placeholder}
        value={query}
        onChange={(event: ChangeEvent<HTMLInputElement>) => onQueryChange(event.target.value)}
      />
      {query.length > 0 && (
        <button
          type="button"
          onMouseDown={(event) => event.preventDefault()}
          onClick={onClear}
          title="清空搜索"
          aria-label="清空搜索"
          className="absolute right-3 top-1/2 flex h-7 w-7 -translate-y-1/2 items-center justify-center rounded-full text-[var(--app-muted)] transition-colors hover:bg-[var(--app-hover)] hover:text-[var(--app-text)]"
        >
          <X size={15} />
        </button>
      )}
    </div>
  )
}
