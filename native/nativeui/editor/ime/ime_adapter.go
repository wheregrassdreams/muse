package ime

import "github.com/egoist/mygo/ui"

// Client is the MyGo-facing text-input boundary. The editor host supplies only
// the text-session and geometry capabilities defined in client.go.
var _ ui.TextInputClient = (*Client)(nil)
