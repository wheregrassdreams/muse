package editor

import tablemodel "github.com/wheregrassdreams/muse-editor/markdown/table"

// modalState retains host-only state for table transactions and prompt focus.
type modalState struct {
	registerTable      *tablemodel.Register
	registerBlock      []string
	blockInsert        *tableBlockInsert
	searchInputFocused bool
}
