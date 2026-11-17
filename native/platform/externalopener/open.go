package externalopener

import (
	"os/exec"
	"runtime"
)

// Open opens a URL or file with the operating system's default application.
func Open(href string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", href)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", href)
	default:
		cmd = exec.Command("xdg-open", href)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
