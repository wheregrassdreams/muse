package quickopen

import (
	"github.com/egoist/mygo/ui"
	componenticons "muse/nativeui/ui/icons"
	workspaceui "muse/nativeui/workspace"
)

// NativePorts are the workbench capabilities needed to present Quick Open.
// Vault candidate construction, shared icon choice and overlay placement are
// adapted here so the composition root only routes a selected path.
type NativePorts struct {
	Vault   VaultQuickOpen
	Windows WindowHost
	OnOpen  func(path string)
	OnClose func()
}

func NewNativeHost(ports NativePorts) *Controller {
	return NewController(ControllerOptions{
		Candidates: NewVaultCandidates(ports.Vault),
		FileIcon:   componenticons.File(),
		IconTint:   func(theme *ui.Theme) ui.Color { return theme.TextMuted },
		Windows:    ports.Windows,
		Placement:  workspaceui.Placement{Host: "main", Layer: workspaceui.ModalOverlay},
		OnOpen:     ports.OnOpen,
		OnClose:    ports.OnClose,
	})
}
