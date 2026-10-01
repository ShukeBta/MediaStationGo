import { useEffect, useState } from 'react'
import { resourceImportsAPI } from '../api/resourceImports'

export function useResourceImportCapability() {
  const [state, setState] = useState<'loading' | 'enabled' | 'disabled' | 'error'>('loading')
  const [attempt, setAttempt] = useState(0)
  useEffect(() => {
    const controller = new AbortController()
    setState('loading')
    resourceImportsAPI.capabilities(controller.signal)
      .then(({ enabled }) => { if (!controller.signal.aborted) setState(enabled ? 'enabled' : 'disabled') })
      .catch(() => { if (!controller.signal.aborted) setState('error') })
    return () => controller.abort()
  }, [attempt])
  return { state, retry: () => setAttempt((value) => value + 1) }
}
