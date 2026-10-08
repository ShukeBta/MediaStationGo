export interface LoginShowcaseItem {
  title: string
  overview: string
  artwork_url: string
  year: number
}

export function uniqueLoginShowcaseItems(items: LoginShowcaseItem[]): LoginShowcaseItem[] {
  const seen = new Set<string>()
  return items.filter((item) => {
    if (!item.title?.trim() || !item.artwork_url?.trim()) return false
    const key = `${item.title.trim().toLocaleLowerCase()}:${item.year || 0}`
    if (seen.has(key)) return false
    seen.add(key)
    return true
  })
}

export function loginShowcasePosters(items: LoginShowcaseItem[], index: number): LoginShowcaseItem[] {
  const count = Math.min(items.length, 7)
  return Array.from({ length: count }, (_, offset) => items[(index + offset - Math.floor(count / 2) + items.length) % items.length])
}
