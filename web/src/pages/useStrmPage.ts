import { useEffect, useState } from 'react'

import { libraryAPI } from '../api/library'
import { strmAPI, type STRMOutputPreset } from '../api/strm'
import type { Library } from '../types'
import { useStrmAttachForm } from './useStrmAttachForm'
import { useStrmGenerateForm } from './useStrmGenerateForm'
import { useStrmImportForm } from './useStrmImportForm'
import { useStrmRepairForm } from './useStrmRepairForm'
import { useStrmTreeGenerateForm } from './useStrmTreeGenerateForm'

export function useStrmPage() {
  const [libraries, setLibraries] = useState<Library[]>([])
  const [outputPresets, setOutputPresets] = useState<STRMOutputPreset[]>([])
  const generate = useStrmGenerateForm(libraries)
  const treeGenerate = useStrmTreeGenerateForm()
  const repair = useStrmRepairForm()
  const importForm = useStrmImportForm(libraries)
  const attach = useStrmAttachForm()

  useEffect(() => {
    libraryAPI.list().then(setLibraries).catch(() => undefined)
    strmAPI.outputPresets().then(setOutputPresets).catch(() => undefined)
  }, [])

  return {
    attach,
    generate,
    importForm,
    libraries,
    outputPresets,
    repair,
    treeGenerate,
  }
}
