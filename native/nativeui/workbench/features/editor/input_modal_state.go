package editor

// modalState retains host-only state for table transactions and prompt focus.
type modalState struct {
	registerTable      *tableRegister
	blockInsert        *tableBlockInsert
	searchInputFocused bool
}
