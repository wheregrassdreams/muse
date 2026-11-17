// Command muse-native opens Muse's fully native window: MyGo draws the icon
// rail, the context panel and the Markdown editor with no webview.
//
//	go run ./cmd/muse-native
package main

import (
	"log"
	"os"

	"github.com/egoist/mygo"

	"muse/app"
	"muse/nativeui"
	"muse/platform/process/vaultsearchindexer"
)

func main() {
	runtime, err := app.NewRuntime(mygo.Shell.TrashItem)
	if err != nil {
		log.Fatal(err)
	}
	defer runtime.Close()
	if len(os.Args) == 3 && os.Args[1] == vaultsearchindexer.WorkerArgument {
		if err := runtime.RebuildVaultSearchIndex(os.Args[2]); err != nil {
			runtime.Close()
			log.Fatal(err)
		}
		return
	}
	nativeui.Run(nativeui.Dependencies{
		Configuration:             runtime.Configuration,
		AssistantSettingsLocation: runtime.AssistantSettingsLocation,
		AssistantProvider:         runtime.AssistantProvider,
		AssistantStream:           runtime.AssistantStream,
		OpenExternal:              runtime.OpenExternal,
		Workspace:                 runtime.Workspace,
		WorkspaceEvents:           runtime.WorkspaceEvents,
		Documents:                 runtime.Documents,
		DocumentExportWriter:      runtime.DocumentExportWriter,
		DocumentMetadata:          runtime.DocumentMetadata,
		Recovery:                  runtime.Recovery,
		History:                   runtime.History,
		DocumentEvents:            runtime.DocumentEvents,
		VaultSearchReconciler:     runtime.VaultSearchReconciler,
		NewAgentWorkspace:         runtime.NewAgentWorkspace,
		NewVaultIndex:             runtime.NewVaultIndex,
		NewVaultCatalog:           runtime.NewVaultCatalog,
		FileOperations:            runtime.FileOperations,
		RecentItems:               runtime.RecentItems,
		NavIcons:                  runtime.NavIcons,
		ImageCacheStore:           runtime.ImageCacheStore,
		ImageLocalFiles:           runtime.ImageLocalFiles,
		TemporaryImages:           runtime.TemporaryImages,
		NewWatcher:                runtime.NewWatcher,
	})
}
