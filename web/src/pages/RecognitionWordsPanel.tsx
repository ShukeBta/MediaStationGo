import { Loader2 } from 'lucide-react'

import {
  RecognitionWordsActions,
  RecognitionWordsEditors,
  RecognitionWordsHeader,
  RecognitionWordsTester,
} from './RecognitionWordsPanelSections'
import { useRecognitionWordsPanel } from './useRecognitionWordsPanel'

export function RecognitionWordsPanel() {
  const state = useRecognitionWordsPanel()

  if (state.loading) {
    return (
      <div className="flex justify-center py-12 text-ink-50">
        <Loader2 className="animate-spin" />
      </div>
    )
  }

  return (
    <div className="glass-panel space-y-4">
      <RecognitionWordsHeader config={state.config} onEnabledChange={state.updateEnabled} />
      <RecognitionWordsEditors
        config={state.config}
        onLocalTextChange={state.updateLocalText}
        onSharedURLsChange={state.updateSharedURLs}
      />
      <RecognitionWordsTester
        input={state.testInput}
        result={state.testResult}
        testing={state.testing}
        onInputChange={state.setTestInput}
        onTest={state.test}
      />
      <RecognitionWordsActions
        saving={state.saving}
        syncing={state.syncing}
        onSave={state.save}
        onSync={state.sync}
      />
    </div>
  )
}
