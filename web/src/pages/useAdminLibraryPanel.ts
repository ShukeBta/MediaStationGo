import { FormEvent, useEffect, useState } from 'react'
import toast from 'react-hot-toast'

import { libraryAPI } from '../api/library'
import type { Library, LibraryRoot } from '../types'
import { confirmAction } from '../components/confirmAction'
import { apiErrorMessage, createRootPayload, displayLibraryRootName, displayLibraryRootPath, emptyRootDraft, rootDraftKey, type RootDraft } from './adminLibraryPanelModel'

export function useAdminLibraryPanel() {
  const { libs, refresh } = useAdminLibraryList()
  const createForm = useCreateLibraryForm(refresh)
  const editableRoots = useEditableRootDrafts()
  const rootActions = useEditableLibraryRootActions(refresh, editableRoots)
  const libraryActions = useLibraryActions(refresh)

  return { libs, createForm, editableRoots, rootActions, libraryActions }
}

function useAdminLibraryList() {
  const [libs, setLibs] = useState<Library[]>([])
  const refresh = () => libraryAPI.list({ includeHidden: true }).then(setLibs)

  useEffect(() => {
    refresh().catch(() => undefined)
  }, [])

  return { libs, refresh }
}

function useCreateLibraryForm(refresh: () => Promise<void>) {
  const [name, setName] = useState('')
  const [roots, setRoots] = useState<RootDraft[]>([emptyRootDraft()])
  const [type, setType] = useState('movie')

  const handleCreate = async (e: FormEvent) => {
    e.preventDefault()
    try {
      const payload = createRootPayload(roots)
      if (payload.length === 0) {
        toast.error('请至少填写一个路径')
        return
      }
      await libraryAPI.createWithRoots(name, type, payload)
      toast.success('媒体库已保存')
      setName('')
      setRoots([emptyRootDraft()])
      await refresh()
    } catch (err: unknown) {
      toast.error(apiErrorMessage(err, '创建失败'))
    }
  }

  const updateRoot = (index: number, patch: Partial<RootDraft>) => {
    setRoots((prev) => prev.map((root, i) => (i === index ? { ...root, ...patch } : root)))
  }

  return {
    name,
    type,
    roots,
    setName,
    setType,
    updateRoot,
    addRoot: () => setRoots((prev) => [...prev, emptyRootDraft()]),
    removeRoot: (index: number) => setRoots((prev) => (prev.length <= 1 ? prev : prev.filter((_, i) => i !== index))),
    handleCreate,
  }
}

function useEditableRootDrafts() {
  const [rootDrafts, setRootDrafts] = useState<Record<string, RootDraft>>({})

  const editableRootDraft = (libraryID: string, root: LibraryRoot): RootDraft => {
    const key = rootDraftKey(libraryID, root.id)
    return rootDrafts[key] ?? {
      name: displayLibraryRootName(root.name, root.path),
      path: displayLibraryRootPath(root.path),
      enabled: root.enabled,
      sort_order: root.sort_order,
    }
  }

  const setEditableRootDraft = (libraryID: string, root: LibraryRoot, patch: Partial<RootDraft>) => {
    const key = rootDraftKey(libraryID, root.id)
    setRootDrafts((prev) => ({ ...prev, [key]: { ...editableRootDraft(libraryID, root), ...patch } }))
  }

  const clearEditableRootDraft = (libraryID: string, rootID: string) => {
    setRootDrafts((prev) => {
      const next = { ...prev }
      delete next[rootDraftKey(libraryID, rootID)]
      return next
    })
  }

  return { editableRootDraft, setEditableRootDraft, clearEditableRootDraft }
}

type EditableRootDrafts = ReturnType<typeof useEditableRootDrafts>

function useEditableLibraryRootActions(refresh: () => Promise<void>, drafts: EditableRootDrafts) {
  const saveLibraryRoot = async (libraryID: string, root: LibraryRoot) => {
    const draft = drafts.editableRootDraft(libraryID, root)
    if (!draft.path?.trim()) {
      toast.error('请填写路径')
      return
    }
    await libraryAPI.updateRoot(libraryID, root.id, {
      name: draft.name?.trim(),
      path: draft.path.trim(),
      enabled: draft.enabled,
      sort_order: draft.sort_order,
    })
    drafts.clearEditableRootDraft(libraryID, root.id)
    toast.success('路径已保存')
    await refresh()
  }

  const scanLibraryRoot = async (libraryID: string, root: LibraryRoot) => {
    if (!root.id) return
    await libraryAPI.scanRoot(libraryID, root.id)
    toast.success('路径扫描已加入后台任务')
  }

  const toggleLibraryRoot = async (libraryID: string, root: LibraryRoot) => {
    const enabled = !drafts.editableRootDraft(libraryID, root).enabled
    drafts.setEditableRootDraft(libraryID, root, { enabled })
    await libraryAPI.updateRoot(libraryID, root.id, { enabled })
    await refresh()
  }

  const removeLibraryRoot = async (library: Library, root: LibraryRoot) => {
    if (!(await confirmAction({ title: '删除媒体库路径', message: `确定删除「${displayLibraryRootPath(root.path)}」?`, confirmText: '删除' }))) return
    await libraryAPI.removeRoot(library.id, root.id)
    toast.success('路径已删除')
    await refresh()
  }

  return { saveLibraryRoot, scanLibraryRoot, toggleLibraryRoot, removeLibraryRoot }
}

function useLibraryActions(refresh: () => Promise<void>) {
  const scanLibrary = async (library: Library) => {
    const result = await libraryAPI.scan(library.id)
    if (result.queued) toast.success('云盘扫描已加入后台队列，会自动入库')
    else toast.success(`扫描完成，新增 ${result.added}，更新 ${result.updated ?? 0}`)
  }

  const removeLibrary = async (library: Library) => {
    if (!(await confirmAction({ title: '删除媒体库', message: `确定删除「${library.name}」?`, confirmText: '删除' }))) return
    await libraryAPI.remove(library.id)
    toast.success('已删除')
    await refresh()
  }

  return { scanLibrary, removeLibrary }
}
