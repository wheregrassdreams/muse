package vim

import textvim "github.com/wheregrassdreams/muse-editor/text/vim"

// Sequence is the detached sequence-state view used by the UI compatibility
// layer. The authoritative Vim sequence state and parsing live in muse-editor.
type Sequence = textvim.SequenceState
