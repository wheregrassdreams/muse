// Package nativeui is Muse's fully native shell: MyGo draws the window, the
// icon rail, the context panel and the Markdown editor, with no webview and
// no frontend build.
//
// The layout is a 56px icon rail, one shared context panel (files / outline
// / search / AI) and the editor card, which shows the document as Typora
// does: one surface the user writes on, rendered as it reads.
package nativeui

func (a *App) memoryDiagnosticsSnapshot() diagnostics.Snapshot {
	if a == nil {
		return diagnostics.Snapshot{}
	}
	a.mainMu.Lock()
	var editor diagnostics.EditorSnapshot
	if note := a.noteEditor(); note != nil {
		state := note.MemorySnapshot()
		editor = diagnostics.EditorSnapshot{
			SourceBytes: state.SourceBytes, Leaves: state.Leaves, VirtualLayout: state.VirtualLayout,
			ImageCandidateLeaves: state.ImageCandidateLeaves, ImageAssetViews: state.ImageAssetViews,
			ImageAssets: state.ImageAssets, LoadedImageAssets: state.LoadedImageAssets,
			ImagePixelBytes: state.ImagePixelBytes,
		}
	}
	vaultReady := a.vault != nil && a.vaultSnapshot.Ready
	a.mainMu.Unlock()
	pending, active := markdownui.ImageDecodeQueueStats()
	cacheEntries, cacheBytes := markdownui.ImageAssetCacheStats()
	return diagnostics.Snapshot{
		VaultReady: vaultReady, Editor: editor,
		ImageCacheEntries: cacheEntries, ImageCacheEstimatedBytes: cacheBytes,
		ImageDecodePending: pending, ImageDecodeActive: active,
	}
}
