import type { Media } from '../types'

export interface ActorFacet {
  name: string
  count: number
}

export function librarySupportsActorFilter(libraryType?: string): boolean {
  return libraryType === 'adult'
}

const nonActorFacetKeys = new Set([
  'actor',
  'actors',
  'actress',
  'actresses',
  'performer',
  'performers',
  '演员',
  '演員',
  '女优',
  '女優',
  '男优',
  '男優',
  '男性演员',
  '男性演員',
  '有码',
  '有碼',
  '无码',
  '無碼',
  '有码女优',
  '有碼女優',
  '无码女优',
  '無碼女優',
  '有码无码',
  '有碼無碼',
  'censored',
  'uncensored',
  'western',
  '欧美',
  '歐美',
])

export function buildActorFacets(items: Media[]): ActorFacet[] {
  const actors = new Map<string, ActorFacet>()
  for (const media of items) {
    for (const name of parseActorCSV(media.actors)) {
      if (!isActorFacetName(name)) continue
      const key = name.toLocaleLowerCase()
      const current = actors.get(key)
      if (current) {
        current.count += 1
      } else {
        actors.set(key, { name, count: 1 })
      }
    }
  }
  const collator = new Intl.Collator('zh-CN', { numeric: true, sensitivity: 'base' })
  return Array.from(actors.values()).sort((left, right) => collator.compare(left.name, right.name))
}

export function isActorFacetName(value: string): boolean {
  const key = value.trim().toLocaleLowerCase().replace(/[\s·・/_-]+/g, '')
  return Boolean(key) && !nonActorFacetKeys.has(key)
}

export function mediaHasActor(media: Media, actor: string): boolean {
  const expected = actor.trim().toLocaleLowerCase()
  if (!expected) return true
  return parseActorCSV(media.actors).some((name) => name.toLocaleLowerCase() === expected)
}

function parseActorCSV(value?: string): string[] {
  if (!value) return []
  const seen = new Set<string>()
  const out: string[] = []
  for (const part of value.split(',')) {
    const name = part.trim()
    const key = name.toLocaleLowerCase()
    if (!name || seen.has(key)) continue
    seen.add(key)
    out.push(name)
  }
  return out
}
