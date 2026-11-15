// Muse is a fully native desktop Markdown editor. MyGo draws the window,
// icon rail, context panel and editor; there is no WebView and no frontend
// build in this binary.
package main

import "muse/nativeui"

func main() {
	nativeui.Run()
}
