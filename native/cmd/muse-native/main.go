// Command muse-native opens Muse's fully native window: MyGo draws the icon
// rail, the context panel and the Markdown editor with no webview.
//
//	go run ./cmd/muse-native
package main

import "muse/app"

func main() {
	app.Run()
}
