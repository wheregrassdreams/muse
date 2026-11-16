package nativeui

import (
	"log"
	"os/exec"
	"runtime"
)

// openExternal opens a URL or a file in the app the system has for it.
func openExternal(href string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", href)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", href)
	default:
		cmd = exec.Command("xdg-open", href)
	}
	if err := cmd.Run(); err != nil {
		log.Printf("open %s: %v", href, err)
	}
}
