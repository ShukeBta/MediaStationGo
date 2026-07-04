import type { FormEvent } from 'react'
import { useEffect, useState } from 'react'
import toast from 'react-hot-toast'

import { strmAPI } from '../api/strm'
import type { Library } from '../types'
import { apiErrorMessage, isHTTPURL } from './strmPageUtils'

export function useStrmImportForm(libraries: Library[]) {
  const [libraryID, setLibraryID] = useState('')
  const [title, setTitle] = useState('')
  const [url, setURL] = useState('')
  const [importing, setImporting] = useState(false)

  useEffect(() => {
    if (!libraryID && libraries[0]) setLibraryID(libraries[0].id)
  }, [libraries, libraryID])

  const onImport = async (event: FormEvent) => {
    event.preventDefault()
    const trimmedTitle = title.trim()
    const trimmedURL = url.trim()
    if (!libraryID || !trimmedTitle || !trimmedURL) return
    if (!isHTTPURL(trimmedURL)) {
      toast.error('URL 必须以 http:// 或 https:// 开头')
      return
    }

    setImporting(true)
    try {
      await strmAPI.importURL(libraryID, trimmedTitle, trimmedURL)
      toast.success(`已导入「${trimmedTitle}」`)
      setTitle('')
      setURL('')
    } catch (err: unknown) {
      toast.error(apiErrorMessage(err, '导入失败'))
    } finally {
      setImporting(false)
    }
  }

  return {
    importing,
    libraryID,
    onImport,
    setLibraryID,
    setTitle,
    setURL,
    title,
    url,
  }
}
