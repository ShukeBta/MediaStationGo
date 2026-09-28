import type { Media } from '../types'

export interface CategoryFacet {
  name: string
  count: number
}

const categoryOrder = [
  '华语电影',
  '日韩电影',
  '欧美电影',
  '动画电影',
  '纪录片',
  '演唱会',
  '国产剧',
  '日韩剧',
  '欧美剧',
  '综艺',
  '儿童',
  '国漫',
  '日番',
  '韩漫',
  '美漫',
  '其他',
  '成人',
]

const categoryRank = new Map(categoryOrder.map((name, index) => [name, index]))

export function buildCategoryFacets(items: Media[]): CategoryFacet[] {
  const categories = new Map<string, CategoryFacet>()
  for (const media of items) {
    const name = media.auto_category?.trim()
    if (!name) continue
    const key = name.toLocaleLowerCase()
    const current = categories.get(key)
    if (current) current.count += 1
    else categories.set(key, { name, count: 1 })
  }
  return sortCategoryFacets(Array.from(categories.values()))
}

export function sortCategoryFacets(categories: CategoryFacet[]): CategoryFacet[] {
  const collator = new Intl.Collator('zh-CN', { numeric: true, sensitivity: 'base' })
  return [...categories].sort((left, right) => {
    const leftRank = categoryRank.get(left.name) ?? categoryOrder.length
    const rightRank = categoryRank.get(right.name) ?? categoryOrder.length
    return leftRank - rightRank || collator.compare(left.name, right.name)
  })
}

export function mediaHasCategory(media: Media, category: string): boolean {
  const expected = category.trim()
  return !expected || media.auto_category?.trim().toLocaleLowerCase() === expected.toLocaleLowerCase()
}
