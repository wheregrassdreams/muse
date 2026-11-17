package vim

import (
	"muse/editor/blocks/paragraph"
	"muse/editor/input"
)

// OperationForCommand translates keymap-owned prefix commands into editor
// operations. Commands that require source geometry remain host-resolved.
func OperationForCommand(command Command) (input.Operation, bool) {
	switch command.Kind {
	case CommandScrollLine:
		placement := input.CaretLineCenter
		switch command.Placement {
		case ScrollTop:
			placement = input.CaretLineTop
		case ScrollBottom:
			placement = input.CaretLineBottom
		}
		return input.Operation{Kind: input.OperationScrollCaretLine, Placement: placement}, true
	case CommandFold:
		fold := paragraph.FoldToggle
		switch command.Fold {
		case FoldClose:
			fold = paragraph.FoldClose
		case FoldCloseRecursive:
			fold = paragraph.FoldCloseRecursive
		case FoldOpen:
			fold = paragraph.FoldOpen
		case FoldOpenRecursive:
			fold = paragraph.FoldOpenRecursive
		case FoldToggleRecursive:
			fold = paragraph.FoldToggleRecursive
		}
		return input.Operation{Kind: input.OperationFoldAtCaret, Fold: fold}, true
	case CommandFoldAll:
		fold := paragraph.FoldCloseAll
		if command.Fold == FoldOpenAll {
			fold = paragraph.FoldOpenAll
		}
		return input.Operation{Kind: input.OperationFoldAll, Fold: fold}, true
	case CommandReveal:
		return input.Operation{Kind: input.OperationRevealCaret}, true
	default:
		return input.Operation{}, false
	}
}

// OperationForIntent translates context-free Vim intents into editor
// operations. Motion, selection, register, and insert effects that need source
// geometry or Editor-private interaction state are left for the feature host.
func OperationForIntent(intent Intent) (input.Operation, bool) {
	switch intent.Kind {
	case IntentFindCharacter:
		if intent.Character == "" {
			return input.Operation{Kind: input.OperationBeginCharacterFind, Direction: intent.Direction, Till: intent.Till, Count: intent.Count}, true
		}
		return input.Operation{Kind: input.OperationFindCharacter, Character: intent.Character, Direction: intent.Direction, Till: intent.Till, Count: intent.Count}, true
	case IntentBeginReplace:
		return input.Operation{Kind: input.OperationBeginReplace, Count: intent.Count}, true
	case IntentReplaceCharacter:
		return input.Operation{Kind: input.OperationReplaceCharacter, Count: intent.Count, Character: intent.Character}, true
	case IntentRepeatCharacterFind:
		return input.Operation{Kind: input.OperationRepeatCharacterFind, Reverse: intent.Reverse, Count: intent.Count}, true
	case IntentBeginSearch:
		return input.Operation{Kind: input.OperationBeginSearch, Direction: intent.Direction}, true
	case IntentRepeatSearch:
		return input.Operation{Kind: input.OperationRepeatSearch, Reverse: intent.Reverse, Count: intent.Count}, true
	case IntentPutRegister:
		return input.Operation{Kind: input.OperationPutRegister, Count: intent.Count, Before: intent.Before}, true
	default:
		return input.Operation{}, false
	}
}
