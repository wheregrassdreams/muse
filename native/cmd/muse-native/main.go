// Command muse-native opens Muse's fully native window: MyGo draws the icon
// rail, the context panel and the Markdown editor with no webview.
//
//	go run ./cmd/muse-native
package main

import (
	"log"

	"github.com/egoist/mygo"

	"muse/app"
	"muse/nativeui"
)

func main() {
	runtime, err := app.NewRuntime(mygo.Shell.TrashItem)
	if err != nil {
		log.Fatal(err)
	}
	defer runtime.Close()
	nativeui.Run(nativeui.Dependencies{
		Workspace:         runtime.Workspace,
		WorkspaceEvents:   runtime.WorkspaceEvents,
		Documents:         runtime.Documents,
		Recovery:          runtime.Recovery,
		History:           runtime.History,
		DocumentEvents:    runtime.DocumentEvents,
		VaultSearch:       runtime.VaultSearch,
		NewVaultIndex:     runtime.NewVaultIndex,
		NewVaultCatalog:   runtime.NewVaultCatalog,
		FileOperations:    runtime.FileOperations,
		RecentItems:       runtime.RecentItems,
		NavIcons:          runtime.NavIcons,
		ImageCacheStore:   runtime.ImageCacheStore,
		NewWatcherBackend: runtime.NewWatcherBackend,
	})
}
