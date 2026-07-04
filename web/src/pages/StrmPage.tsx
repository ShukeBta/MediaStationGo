import { StrmAttachSection } from './StrmAttachSection'
import { StrmGenerateSection } from './StrmGenerateSection'
import { StrmImportSection } from './StrmImportSection'
import { StrmPageHeader } from './StrmPageHeader'
import { StrmRepairSection } from './StrmRepairSection'
import { StrmTreeGenerateSection } from './StrmTreeGenerateSection'
import { useStrmPage } from './useStrmPage'

// StrmPage exposes the URL-as-file admin tooling backed by the Go server:
//   - import a brand-new media row directly from a (library, title, url)
//     tuple — useful for streaming-only entries with no on-disk file.
//   - search existing media and attach / detach a STRM URL so the player
//     issues a 302 redirect to the remote source instead of opening a
//     local file.
export function StrmPage() {
  const strm = useStrmPage()

  return (
    <div className="space-y-6">
      <StrmPageHeader />

      <StrmGenerateSection
        libraries={strm.libraries}
        generateLibraryID={strm.generate.generateLibraryID}
        baseURL={strm.generate.baseURL}
        outputDir={strm.generate.outputDir}
        outputPresets={strm.outputPresets}
        cloudPlaybackMode={strm.generate.cloudPlaybackMode}
        strmPlaybackEnabled={strm.generate.strmPlaybackEnabled}
        redirectProxyEnabled={strm.generate.redirectProxyEnabled}
        autoGenerate={strm.generate.autoGenerate}
        savingSettings={strm.generate.savingSettings}
        overwrite={strm.generate.overwrite}
        includeLocal={strm.generate.includeLocal}
        preserveTree={strm.generate.preserveTree}
        refreshLibrary={strm.generate.refreshLibrary}
        scrapeAfter={strm.generate.scrapeAfter}
        generating={strm.generate.generating}
        generateResult={strm.generate.generateResult}
        playbackStatus={strm.generate.playbackStatus}
        onGenerate={strm.generate.onGenerate}
        saveSTRMSettings={strm.generate.saveSTRMSettings}
        setGenerateLibraryID={strm.generate.setGenerateLibraryID}
        setBaseURL={strm.generate.setBaseURL}
        setOutputDir={strm.generate.setOutputDir}
        setCloudPlaybackMode={strm.generate.setCloudPlaybackMode}
        setStrmPlaybackEnabled={strm.generate.setStrmPlaybackEnabled}
        setRedirectProxyEnabled={strm.generate.setRedirectProxyEnabled}
        setAutoGenerate={strm.generate.setAutoGenerate}
        setOverwrite={strm.generate.setOverwrite}
        setIncludeLocal={strm.generate.setIncludeLocal}
        setPreserveTree={strm.generate.setPreserveTree}
        setRefreshLibrary={strm.generate.setRefreshLibrary}
        setScrapeAfter={strm.generate.setScrapeAfter}
      />

      <StrmTreeGenerateSection {...strm.treeGenerate} outputPresets={strm.outputPresets} />

      <StrmRepairSection {...strm.repair} outputPresets={strm.outputPresets} />

      <StrmImportSection
        libraries={strm.libraries}
        libraryID={strm.importForm.libraryID}
        title={strm.importForm.title}
        url={strm.importForm.url}
        importing={strm.importForm.importing}
        onImport={strm.importForm.onImport}
        setLibraryID={strm.importForm.setLibraryID}
        setTitle={strm.importForm.setTitle}
        setURL={strm.importForm.setURL}
      />

      <StrmAttachSection
        query={strm.attach.query}
        searching={strm.attach.searching}
        results={strm.attach.results}
        drafts={strm.attach.drafts}
        doSearch={strm.attach.doSearch}
        setQuery={strm.attach.setQuery}
        setDrafts={strm.attach.setDrafts}
        onAttach={strm.attach.onAttach}
        onDetach={strm.attach.onDetach}
      />
    </div>
  )
}
