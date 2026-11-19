package quickopen

import (
	"github.com/egoist/mygo/ui"
	componenticons "muse/nativeui/workbench/framework/components/icons"
	hosting "muse/nativeui/workbench/framework/hosting"
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
		Placement:  hosting.Placement{Host: "main", Layer: hosting.ModalOverlay},
		OnOpen:     ports.OnOpen,
		OnClose:    ports.OnClose,
	})
}
