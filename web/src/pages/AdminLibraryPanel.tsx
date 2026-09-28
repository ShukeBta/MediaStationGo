import { useState } from 'react'
import { EmbyLibraryDisplayDialog } from './EmbyLibraryDisplayDialog'
import { AdminLibraryCreateForm } from './AdminLibraryPanelSections'
import { AdminLibraryTable } from './AdminLibraryTable'
import { useAdminLibraryPanel } from './useAdminLibraryPanel'

export function AdminLibraryPanel() {
  const [displayOpen, setDisplayOpen] = useState(false)
  const { libs, createForm, editableRoots, rootActions, libraryActions } = useAdminLibraryPanel()

  return (
    <div className="space-y-6">
      <AdminLibraryCreateForm
        name={createForm.name}
        type={createForm.type}
        titleMode={createForm.titleMode}
        coverURL={createForm.coverURL}
        roots={createForm.roots}
        onNameChange={createForm.setName}
        onTypeChange={createForm.setType}
        onTitleModeChange={createForm.setTitleMode}
        onCoverURLChange={createForm.setCoverURL}
        onRootChange={createForm.updateRoot}
        onAddRoot={createForm.addRoot}
        onRemoveRoot={createForm.removeRoot}
        onSubmit={createForm.handleCreate}
      />
      <div className="flex justify-end"><button type="button" className="btn-outline" onClick={() => setDisplayOpen(true)}>Emby 媒体库展示</button></div>
      {displayOpen && <EmbyLibraryDisplayDialog onClose={() => setDisplayOpen(false)} />}
      <AdminLibraryTable
        libs={libs}
        editableRootDraft={editableRoots.editableRootDraft}
        onEditableRootChange={editableRoots.setEditableRootDraft}
        onSaveRoot={rootActions.saveLibraryRoot}
        onScanRoot={rootActions.scanLibraryRoot}
        onToggleRoot={rootActions.toggleLibraryRoot}
        onRemoveRoot={rootActions.removeLibraryRoot}
        onToggleLibrary={libraryActions.toggleLibrary}
        onScanLibrary={libraryActions.scanLibrary}
        onTitleModeChange={libraryActions.updateLibraryTitleMode}
        onGenerateArtworkChange={libraryActions.updateLibraryGenerateArtwork}
        onRunGeneratedArtwork={libraryActions.runGeneratedArtwork}
        onCancelGeneratedArtwork={libraryActions.cancelGeneratedArtwork}
        onRemoveLibrary={libraryActions.removeLibrary}
        onAddLibraryRoot={libraryActions.addLibraryRoot}
        onEditLibraryCover={libraryActions.editLibraryCover}
      />
    </div>
  )
}
