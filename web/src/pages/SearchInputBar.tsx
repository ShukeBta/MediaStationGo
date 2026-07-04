import type { ChangeEvent, FormEvent } from 'react'

type SearchInputBarProps = {
  aiOn: boolean
  query: string
  onQueryChange: (query: string) => void
  onAISubmit: (event: FormEvent) => void
}

export function SearchInputBar({ aiOn, query, onQueryChange, onAISubmit }: SearchInputBarProps) {
  if (aiOn) {
    return (
      <form onSubmit={onAISubmit} className="flex flex-wrap gap-2">
        <input
          autoFocus
          className="input-base"
          placeholder='例如:"2010 年后的科幻电影" / "最近的动漫"'
          value={query}
          onChange={(e: ChangeEvent<HTMLInputElement>) => onQueryChange(e.target.value)}
        />
        <button type="submit" className="neon-button">
          搜索
        </button>
      </form>
    )
  }

  return (
    <input
      autoFocus
      className="input-base"
      placeholder="按标题搜索…"
      value={query}
      onChange={(e: ChangeEvent<HTMLInputElement>) => onQueryChange(e.target.value)}
    />
  )
}
