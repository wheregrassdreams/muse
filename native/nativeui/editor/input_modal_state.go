package editor

import tablemodel "muse/editor/blocks/table"

// modalState retains host-only state for table transactions and prompt focus.
type modalState struct {
	registerTable      *tablemodel.Register
	registerBlock      []string
	blockInsert        *tableBlockInsert
	searchInputFocused bool
}
