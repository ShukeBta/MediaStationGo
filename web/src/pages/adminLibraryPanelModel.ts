import type { Library, LibraryRoot } from '../types'
import type { LibraryRootInput } from '../api/library'

export type RootDraft = LibraryRootInput

export const emptyRootDraft = (): RootDraft => ({ name: '', path: '', enabled: true })

export const rootDraftKey = (libraryID: string, rootID: string) => `${libraryID}:${rootID}`

export function displayLibraryRootPath(path: string) {
  if (!path.toLowerCase().startsWith('cloud://')) return path
  return decodePercentEscapes(path)
}

export function displayLibraryRootName(name: string | undefined, path: string) {
  const value = name?.trim() || '默认路径'
  if (!path.toLowerCase().startsWith('cloud://') && !/%[0-9a-f]{2}/i.test(value)) return value
  return decodePercentEscapes(value)
}

function decodePercentEscapes(value: string) {
  return value.replace(/%[0-9a-f]{2}/gi, (token) => {
    try {
      return decodeURIComponent(token)
    } catch {
      return token
    }
  })
}

export function createRootPayload(roots: RootDraft[]) {
  return roots
    .map((root, index) => ({
      ...root,
      path: root.path.trim(),
      name: root.name?.trim(),
      sort_order: index,
    }))
    .filter((root) => root.path)
}

export function fallbackLibraryRoot(library: Library): LibraryRoot {
  return {
    id: '',
    library_id: library.id,
    path: library.path,
    enabled: library.enabled,
    name: '',
    sort_order: 0,
    created_at: library.created_at,
    updated_at: library.updated_at,
  }
}

export function apiErrorMessage(err: unknown, fallback: string) {
  return (err as { response?: { data?: { error?: string } } })?.response?.data?.error ?? fallback
}
