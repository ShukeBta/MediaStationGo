import { AdminLibraryCreateForm } from './AdminLibraryPanelSections'
import { AdminLibraryTable } from './AdminLibraryTable'
import { useAdminLibraryPanel } from './useAdminLibraryPanel'

export function AdminLibraryPanel() {
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
