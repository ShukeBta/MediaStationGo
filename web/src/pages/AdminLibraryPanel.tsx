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
        roots={createForm.roots}
        onNameChange={createForm.setName}
        onTypeChange={createForm.setType}
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
        onScanLibrary={libraryActions.scanLibrary}
        onRemoveLibrary={libraryActions.removeLibrary}
      />
    </div>
  )
}
