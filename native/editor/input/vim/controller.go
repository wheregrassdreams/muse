// Package vim owns keymap state and command translation, independent of the
// editor view and any UI toolkit.
package vim

import (
	"strconv"
	"strings"
	"time"

	"muse/editor/input"
)

type Mode string

const (
	ModeNormal      Mode = "Normal"
	ModeInsert      Mode = "Insert"
	ModeVisual      Mode = "Visual"
	ModeSelect      Mode = "Select"
	ModeBlockVisual Mode = "BlockVisual"
)

type RegisterKind uint8

const (
	RegisterNone RegisterKind = iota
	RegisterText
	RegisterBlock
	RegisterTable
)

// ViewState is an immutable snapshot for labels, prompts, and input routing.
type ViewState struct {
	Keymap               string
	Mode                 Mode
	Pending              string
	Count                int
	OperatorCount        int
	Linewise             bool
	Searching            bool
	SearchDirection      int
	SearchQuery          string
	SearchStartCaret     int
	SearchStartAnchor    int
	SearchStartInclusive bool
	SearchStartLinewise  bool
	LastSearch           string
	LastSearchDirection  int
	AwaitingCharacter    int
	AwaitingTill         bool
	AwaitingReplacement  bool
	CharacterCount       int
	PrefixStartedAt      time.Time
	Register             string
	RegisterValid        bool
	RegisterLine         bool
	RegisterBlock        []string
	RegisterKind         RegisterKind
	VimTextVisualActive  bool
	FindCharacter        string
	FindDirection        int
	FindTill             bool
	FindValid            bool
}

func (s ViewState) Clone() ViewState {
	s.RegisterBlock = append([]string(nil), s.RegisterBlock...)
	return s
}

// Controller owns mutable Vim/Helix state. State can be observed only through
// detached ViewState values and changed through typed methods.
type Controller struct{ state ViewState }

// CommandKind identifies a keymap-level operation that the editor host can
// execute without knowing how Vim's prefix grammar was entered.
type CommandKind uint8

const (
	CommandNone CommandKind = iota
	CommandCountPrefix
	CommandPendingPrefix
	CommandFirstLine
	CommandVertical
	CommandScrollLine
	CommandFold
	CommandFoldAll
	CommandReveal
)

// Command is an immutable, toolkit-neutral result of interpreting a Vim
// prefix sequence. Direction is -1 or +1; Count defaults to one.
type Command struct {
	Kind      CommandKind
	Direction int
	Count     int
	Placement uint8
	Fold      uint8
}

// IntentKind is the result of parsing one printable keymap event. It names
// editor behavior without carrying UI events or editor-owned state.
type IntentKind uint8

const (
	IntentConsumed IntentKind = iota + 1
	IntentPrefix
	IntentCommand
	IntentSearchText
	IntentMotion
	IntentOperator
	IntentEnterInsert
	IntentEnterVisual
	IntentEnterLineVisual
	IntentFindCharacter
	IntentRepeatCharacterFind
	IntentBeginReplace
	IntentReplaceCharacter
	IntentBeginSearch
	IntentRepeatSearch
	IntentPutRegister
	IntentUndo
	IntentDeleteCharacter
	IntentDeleteLineRemainder
	IntentOpenLine
	IntentBlockInsert
)

// MotionKind is a keymap-level navigation policy. Hosts resolve the motion
// against their document and structural layout.
type MotionKind uint8

const (
	MotionLeft MotionKind = iota + 1
	MotionRight
	MotionDown
	MotionUp
	MotionWordForward
	MotionWordBackward
	MotionWordEnd
	MotionLineStart
	MotionFirstNonBlank
	MotionLineEnd
	MotionDocumentEnd
)

// Intent is the complete typed interpretation of a printable key event.
// Count has already combined a motion count with a pending operator count.
type Intent struct {
	Kind      IntentKind
	Motion    MotionKind
	Count     int
	Operator  string
	Direction int
	Till      bool
	Reverse   bool
	Before    bool
	Linewise  bool
	BigWord   bool
	Character string
	InsertAt  string
	Command   Command
}

// Host is the narrow effect boundary for keymap commands. Implementations
// translate these values into editor operations without exposing editor
// state to the Vim controller.
type Host interface {
	DispatchVimCommand(Command)
}

const (
	ScrollTop uint8 = iota + 1
	ScrollCenter
	ScrollBottom
)

const (
	FoldClose uint8 = iota + 1
	FoldCloseRecursive
	FoldOpen
	FoldOpenRecursive
	FoldToggle
	FoldToggleRecursive
	FoldCloseAll
	FoldOpenAll
)

func New() *Controller {
	return &Controller{state: ViewState{Keymap: "default", Mode: ModeNormal}}
}

func (c *Controller) View() ViewState {
	if c == nil {
		return ViewState{Mode: ModeNormal}
	}
	return c.state.Clone()
}

func (c *Controller) SetKeymap(keymap string) {
	if c == nil {
		return
	}
	if keymap != "vim" && keymap != "helix" {
		keymap = "default"
	}
	c.state = ViewState{Keymap: keymap, Mode: ModeNormal}
}

func (c *Controller) Keymap() string {
	if c == nil || c.state.Keymap == "" {
		return "default"
	}
	return c.state.Keymap
}

func (c *Controller) Mode() Mode {
	if c == nil || c.state.Mode == "" {
		return ModeNormal
	}
	return c.state.Mode
}

func (c *Controller) SetTextVisualActive(active bool) {
	if c != nil {
		c.state.VimTextVisualActive = active
	}
}

func (c *Controller) SetMode(mode Mode) {
	if c != nil {
		c.state.Mode = mode
		if mode != ModeVisual {
			c.state.VimTextVisualActive = false
		}
	}
}

func (c *Controller) ModeLabel() string {
	if c == nil || c.Keymap() == "default" {
		return ""
	}
	s := c.state
	label := c.Keymap() + " " + string(c.Mode())
	if s.OperatorCount > 0 {
		label += " " + strconv.Itoa(s.OperatorCount)
	}
	label += s.Pending
	if s.Count > 0 {
		label += strconv.Itoa(s.Count)
	}
	return label
}

func (c *Controller) AcceptsText() bool {
	return c == nil || c.Keymap() == "default" || c.Mode() == ModeInsert
}

func (c *Controller) Pending() string {
	if c == nil {
		return ""
	}
	return c.state.Pending
}

func (c *Controller) SetPending(prefix string, started time.Time) {
	if c != nil {
		c.state.Pending = prefix
		c.state.PrefixStartedAt = started
	}
}

// Sequence returns the current multi-key sequence as a detached value.
func (c *Controller) Sequence() input.Sequence {
	if c == nil {
		return input.Sequence{}
	}
	s := c.state
	return input.Sequence{Count: s.Count, OperatorCount: s.OperatorCount, Pending: s.Pending, Linewise: s.Linewise}
}

// FeedSequenceText interprets count digits and Vim's g/z prefix commands.
// It returns handled=true when the text belongs to the prefix grammar. The
// host executes the returned typed command; editor state and UI effects stay
// outside this package.
func (c *Controller) FeedSequenceText(text string, now time.Time) (command Command, handled bool) {
	if c == nil || len([]rune(text)) != 1 {
		return Command{}, false
	}
	r := []rune(text)[0]
	if c.FeedCountDigit(r) {
		return Command{Kind: CommandCountPrefix}, true
	}
	if c.state.Pending == "g" {
		c.ClearPending()
		count := max(1, c.state.Count)
		c.state.Count = 0
		switch text {
		case "g":
			return Command{Kind: CommandFirstLine, Count: count}, true
		case "j":
			return Command{Kind: CommandVertical, Direction: 1, Count: count}, true
		case "k":
			return Command{Kind: CommandVertical, Direction: -1, Count: count}, true
		default:
			return Command{}, true
		}
	}
	if c.state.Pending == "z" {
		if now.Sub(c.state.PrefixStartedAt) > 900*time.Millisecond {
			c.ClearPending()
			return Command{}, false
		}
		c.ClearPending()
		count := max(1, c.state.Count)
		c.state.Count = 0
		switch text {
		case "t":
			return Command{Kind: CommandScrollLine, Placement: ScrollTop, Count: count}, true
		case "z":
			return Command{Kind: CommandScrollLine, Placement: ScrollCenter, Count: count}, true
		case "b":
			return Command{Kind: CommandScrollLine, Placement: ScrollBottom, Count: count}, true
		case "c":
			return Command{Kind: CommandFold, Fold: FoldClose}, true
		case "C":
			return Command{Kind: CommandFold, Fold: FoldCloseRecursive}, true
		case "o":
			return Command{Kind: CommandFold, Fold: FoldOpen}, true
		case "O":
			return Command{Kind: CommandFold, Fold: FoldOpenRecursive}, true
		case "a":
			return Command{Kind: CommandFold, Fold: FoldToggle}, true
		case "A":
			return Command{Kind: CommandFold, Fold: FoldToggleRecursive}, true
		case "v":
			return Command{Kind: CommandReveal}, true
		case "M":
			return Command{Kind: CommandFoldAll, Fold: FoldCloseAll}, true
		case "R":
			return Command{Kind: CommandFoldAll, Fold: FoldOpenAll}, true
		default:
			return Command{}, true
		}
	}
	if text == "g" || text == "z" && c.Keymap() == "vim" {
		c.SetPending(text, now)
		return Command{Kind: CommandPendingPrefix}, true
	}
	return Command{}, false
}

// HandleText advances the sequence state and dispatches any completed typed
// command. Count and pending-prefix events are consumed without host effects.
func (c *Controller) HandleText(text string, now time.Time, host Host) (CommandKind, bool) {
	command, handled := c.FeedSequenceText(text, now)
	if !handled {
		return CommandNone, false
	}
	if command.Kind != CommandNone && command.Kind != CommandCountPrefix && command.Kind != CommandPendingPrefix && host != nil {
		host.DispatchVimCommand(command)
	}
	return command.Kind, true
}

// InterpretText owns printable-key grammar for Vim and Helix. It only emits
// typed intents; document, selection, and UI effects remain host-owned.
func (c *Controller) InterpretText(text string, now time.Time) (Intent, bool) {
	if c == nil || c.Keymap() == "default" {
		return Intent{}, false
	}
	if c.state.Searching {
		if text == "" {
			return Intent{Kind: IntentConsumed}, true
		}
		c.state.SearchQuery += text
		return Intent{Kind: IntentSearchText, Character: text}, true
	}
	if c.AcceptsText() {
		return Intent{}, false
	}
	if c.state.AwaitingReplacement {
		count := max(1, c.state.CharacterCount)
		c.ClearAwaitingReplacement()
		return Intent{Kind: IntentReplaceCharacter, Count: count, Character: text}, true
	}
	if c.state.AwaitingCharacter != 0 {
		intent := Intent{Kind: IntentFindCharacter, Count: max(1, c.state.CharacterCount), Character: text,
			Direction: c.state.AwaitingCharacter, Till: c.state.AwaitingTill}
		c.ClearAwaitingCharacter()
		return intent, true
	}
	if len([]rune(text)) != 1 {
		return Intent{Kind: IntentConsumed}, true
	}
	command, handled := c.FeedSequenceText(text, now)
	if handled {
		if command.Kind == CommandCountPrefix || command.Kind == CommandPendingPrefix {
			return Intent{Kind: IntentPrefix}, true
		}
		return Intent{Kind: IntentCommand, Command: command}, true
	}

	count, operatorCount := c.ConsumeCounts()
	combinedCount := count * operatorCount
	if combinedCount < 1 {
		combinedCount = 1
	}
	if c.state.Pending != "" && text == c.state.Pending {
		operator := c.state.Pending
		c.ClearPending()
		c.state.OperatorCount = 0
		c.state.Linewise = true
		return Intent{Kind: IntentOperator, Operator: operator, Count: combinedCount, Linewise: true}, true
	}
	if motion, ok := parseMotion(text); ok {
		if c.state.Pending != "" {
			operator := c.state.Pending
			c.ClearPending()
			c.state.OperatorCount = 0
			return Intent{Kind: IntentOperator, Operator: operator, Count: combinedCount,
				Motion: motion, Linewise: c.state.Linewise, BigWord: text == "W" || text == "B" || text == "E"}, true
		}
		return Intent{Kind: IntentMotion, Motion: motion, Count: count,
			BigWord: text == "W" || text == "B" || text == "E"}, true
	}

	vimOnly := c.Keymap() == "vim"
	switch text {
	case "f", "F", "t", "T":
		if !vimOnly {
			return Intent{Kind: IntentConsumed}, true
		}
		direction, till := 1, false
		if text == "F" {
			direction = -1
		}
		if text == "t" {
			till = true
		}
		if text == "T" {
			direction, till = -1, true
		}
		c.SetAwaitingCharacter(direction, till, combinedCount)
		return Intent{Kind: IntentFindCharacter, Count: combinedCount, Direction: direction, Till: till}, true
	case ";", ",":
		if vimOnly {
			return Intent{Kind: IntentRepeatCharacterFind, Count: count, Reverse: text == ","}, true
		}
	case "r":
		if vimOnly {
			c.SetAwaitingReplacement(combinedCount)
			return Intent{Kind: IntentBeginReplace, Count: combinedCount}, true
		}
	case "/", "?":
		if vimOnly {
			direction := 1
			if text == "?" {
				direction = -1
			}
			return Intent{Kind: IntentBeginSearch, Direction: direction}, true
		}
	case "n", "N":
		if vimOnly {
			return Intent{Kind: IntentRepeatSearch, Count: count, Reverse: text == "N"}, true
		}
	case "i", "a":
		place := text
		return Intent{Kind: IntentEnterInsert, InsertAt: place}, true
	case "I", "A":
		if c.state.Mode == ModeBlockVisual {
			return Intent{Kind: IntentBlockInsert, InsertAt: text}, true
		}
		return Intent{Kind: IntentEnterInsert, InsertAt: text}, true
	case "o", "O":
		return Intent{Kind: IntentOpenLine, Before: text == "O"}, true
	case "v":
		c.state.VimTextVisualActive = vimOnly
		c.state.Mode = ModeVisual
		return Intent{Kind: IntentEnterVisual}, true
	case "V":
		c.state.VimTextVisualActive = false
		c.state.Mode = ModeVisual
		return Intent{Kind: IntentEnterLineVisual, Count: count}, true
	case "d", "c", "y":
		if c.state.Mode == ModeVisual || c.state.Mode == ModeSelect || c.state.Mode == ModeBlockVisual {
			return Intent{Kind: IntentOperator, Operator: text, Count: count}, true
		}
		c.BeginOperator(text, count)
		return Intent{Kind: IntentConsumed}, true
	case "p", "P":
		return Intent{Kind: IntentPutRegister, Count: count, Before: text == "P"}, true
	case "u":
		return Intent{Kind: IntentUndo}, true
	case "x", "X":
		return Intent{Kind: IntentDeleteCharacter, Count: count, Before: text == "X"}, true
	case "D", "C":
		return Intent{Kind: IntentDeleteLineRemainder, Operator: strings.ToLower(text)}, true
	}
	return Intent{Kind: IntentConsumed}, true
}

func parseMotion(text string) (MotionKind, bool) {
	switch text {
	case "h":
		return MotionLeft, true
	case "l":
		return MotionRight, true
	case "j":
		return MotionDown, true
	case "k":
		return MotionUp, true
	case "w", "W":
		return MotionWordForward, true
	case "b", "B":
		return MotionWordBackward, true
	case "e", "E":
		return MotionWordEnd, true
	case "0":
		return MotionLineStart, true
	case "^":
		return MotionFirstNonBlank, true
	case "$":
		return MotionLineEnd, true
	case "G":
		return MotionDocumentEnd, true
	default:
		return 0, false
	}
}

func (c *Controller) ClearPending() {
	if c != nil {
		c.state.Pending = ""
		c.state.PrefixStartedAt = time.Time{}
	}
}

func (c *Controller) FeedCountDigit(digit rune) bool {
	if c == nil || digit < '0' || digit > '9' || digit == '0' && c.state.Count == 0 {
		return false
	}
	c.state.Count = min(9999, c.state.Count*10+int(digit-'0'))
	return true
}

func (c *Controller) ConsumeCounts() (motion, operator int) {
	if c == nil {
		return 1, 1
	}
	motion, operator = max(1, c.state.Count), max(1, c.state.OperatorCount)
	c.state.Count = 0
	return motion, operator
}

func (c *Controller) BeginOperator(operator string, count int) {
	if c == nil {
		return
	}
	c.state.Pending = operator
	c.state.OperatorCount = max(1, count)
}

func (c *Controller) SetOperatorCount(count int) {
	if c != nil {
		c.state.OperatorCount = count
	}
}

func (c *Controller) SetCount(count int) {
	if c != nil {
		c.state.Count = max(0, count)
	}
}

func (c *Controller) Linewise() bool { return c != nil && c.state.Linewise }

func (c *Controller) SetLinewise(linewise bool) {
	if c != nil {
		c.state.Linewise = linewise
	}
}

func (c *Controller) ResetSequence() {
	if c == nil {
		return
	}
	c.state.Count = 0
	c.state.OperatorCount = 0
	c.state.Pending = ""
	c.state.Linewise = false
	c.state.PrefixStartedAt = time.Time{}
}

func (c *Controller) SetSearching(direction int, query string, caret, anchor int, inclusive, linewise bool) {
	if c == nil {
		return
	}
	c.state.Searching = true
	c.state.SearchDirection = direction
	c.state.SearchQuery = query
	c.state.SearchStartCaret = caret
	c.state.SearchStartAnchor = anchor
	c.state.SearchStartInclusive = inclusive
	c.state.SearchStartLinewise = linewise
	c.state.Mode = ModeNormal
}

func (c *Controller) UpdateSearchQuery(query string) {
	if c != nil {
		c.state.SearchQuery = query
	}
}

func (c *Controller) EndSearch() {
	if c != nil {
		c.state.Searching = false
		c.state.SearchQuery = ""
		c.state.Mode = ModeNormal
	}
}

func (c *Controller) SetLastSearch(query string, direction int) {
	if c != nil {
		c.state.LastSearch, c.state.LastSearchDirection = query, direction
	}
}

func (c *Controller) SetAwaitingCharacter(direction int, till bool, count int) {
	if c != nil {
		c.state.AwaitingCharacter = direction
		c.state.AwaitingTill = till
		c.state.CharacterCount = max(1, count)
	}
}

func (c *Controller) ClearAwaitingCharacter() {
	if c != nil {
		c.state.AwaitingCharacter = 0
		c.state.AwaitingTill = false
		c.state.CharacterCount = 0
	}
}

func (c *Controller) SetLastFind(character string, direction int, till bool) {
	if c == nil {
		return
	}
	c.state.FindCharacter, c.state.FindDirection, c.state.FindTill, c.state.FindValid = character, direction, till, character != ""
}

func (c *Controller) ClearLastFind() {
	if c == nil {
		return
	}
	c.state.FindCharacter, c.state.FindDirection, c.state.FindTill, c.state.FindValid = "", 0, false, false
}

func (c *Controller) SetAwaitingReplacement(count int) {
	if c != nil {
		c.state.AwaitingReplacement = true
		c.state.CharacterCount = max(1, count)
	}
}

func (c *Controller) ClearAwaitingReplacement() {
	if c != nil {
		c.state.AwaitingReplacement = false
		c.state.CharacterCount = 0
	}
}

func (c *Controller) SetTextRegister(text string, linewise bool) {
	if c == nil {
		return
	}
	c.state.Register = text
	c.state.RegisterValid = true
	c.state.RegisterLine = linewise
	c.state.RegisterBlock = nil
	c.state.RegisterKind = RegisterText
}

func (c *Controller) SetBlockRegister(rows []string) {
	if c == nil {
		return
	}
	c.state.RegisterBlock = append(c.state.RegisterBlock[:0], rows...)
	c.state.Register = ""
	c.state.RegisterValid = false
	c.state.RegisterLine = false
	c.state.RegisterKind = RegisterBlock
}

func (c *Controller) SetTableRegister() {
	if c == nil {
		return
	}
	c.state.Register = ""
	c.state.RegisterValid = false
	c.state.RegisterLine = false
	c.state.RegisterBlock = nil
	c.state.RegisterKind = RegisterTable
}

func (c *Controller) ClearRegister() {
	if c == nil {
		return
	}
	c.state.Register = ""
	c.state.RegisterValid = false
	c.state.RegisterLine = false
	c.state.RegisterBlock = nil
	c.state.RegisterKind = RegisterNone
}
