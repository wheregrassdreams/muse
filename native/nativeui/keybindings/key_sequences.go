package keybindings

import (
	"errors"
	"fmt"
	"sort"
	"time"

	commands "muse/nativeui/commands"
	"muse/nativeui/context"
)

// KeyStroke is a platform-neutral key identity and modifier bit set. Key is a
// stable logical name (for example "K", "Escape", or "ArrowLeft"); Mods is
// owned by the caller and intentionally does not import a UI toolkit.
type KeyStroke struct {
	Key  string
	Mods uint32
}

// KeyBinding maps a complete sequence to an action and its opaque arguments.
type KeyBinding struct {
	Sequence []KeyStroke
	ActionID commands.ActionID
	Values   map[string]any
	When     context.Rule
}

var (
	ErrEmptyKeySequence  = errors.New("key sequence must not be empty")
	ErrEmptyKey          = errors.New("key stroke key must not be empty")
	ErrEmptyActionID     = errors.New("key binding action ID must not be empty")
	ErrDuplicateSequence = errors.New("duplicate key sequence")
)

type keySequenceNode struct {
	children map[KeyStroke]*keySequenceNode
	binding  *KeyBinding
}

// KeySequenceMatcher tracks key sequence progress. If a complete binding is
// also a prefix of a longer binding, the longer binding takes priority and
// the shorter one is emitted only after timeout or cancellation.
type KeySequenceMatcher struct {
	root    *keySequenceNode
	node    *keySequenceNode
	clock   func() time.Time
	timeout time.Duration
	started time.Time
	pending *KeyBinding
}

// KeySequenceResult describes one input event. Pending means the caller
// should wait for more input or call Expire. Cancelled means pending progress
// was cleared (Escape cancels by default).
type KeySequenceResult struct {
	ActionID  commands.ActionID
	Values    map[string]any
	Handled   bool
	Pending   bool
	Cancelled bool
	// Reprocess is set when an expired prefix action is returned from Match.
	// After handling that action, the caller should pass the same stroke to
	// Match again so a timeout cannot swallow unrelated input.
	Reprocess bool
}

func NewKeySequenceMatcher(timeout time.Duration, now func() time.Time, bindings ...KeyBinding) (*KeySequenceMatcher, error) {
	if timeout <= 0 {
		return nil, fmt.Errorf("key sequence timeout must be positive")
	}
	if now == nil {
		now = time.Now
	}
	m := &KeySequenceMatcher{root: &keySequenceNode{children: make(map[KeyStroke]*keySequenceNode)}, timeout: timeout, clock: now}
	m.node = m.root
	for _, binding := range bindings {
		if err := m.Register(binding); err != nil {
			return nil, err
		}
	}
	return m, nil
}

func (m *KeySequenceMatcher) Register(binding KeyBinding) error {
	if len(binding.Sequence) == 0 {
		return ErrEmptyKeySequence
	}
	if binding.ActionID == "" {
		return ErrEmptyActionID
	}
	for _, stroke := range binding.Sequence {
		if stroke.Key == "" {
			return ErrEmptyKey
		}
	}
	if m.root == nil {
		m.root = &keySequenceNode{children: make(map[KeyStroke]*keySequenceNode)}
		m.node = m.root
	}
	node := m.root
	for _, stroke := range binding.Sequence {
		child := node.children[stroke]
		if child == nil {
			child = &keySequenceNode{children: make(map[KeyStroke]*keySequenceNode)}
			node.children[stroke] = child
		}
		node = child
	}
	if node.binding != nil {
		return ErrDuplicateSequence
	}
	copyBinding := binding
	copyBinding.Sequence = append([]KeyStroke(nil), binding.Sequence...)
	if binding.Values != nil {
		copyBinding.Values = make(map[string]any, len(binding.Values))
		for key, value := range binding.Values {
			copyBinding.Values[key] = value
		}
	}
	node.binding = &copyBinding
	return nil
}

// Match consumes one stroke. On a mismatch within a prefix, it clears the
// old prefix and retries the stroke from the root so unrelated input can
// still begin a valid sequence.
func (m *KeySequenceMatcher) Match(stroke KeyStroke) KeySequenceResult {
	return m.MatchWithContext(stroke, context.Snapshot{})
}

// MatchWithContext consumes one stroke using the supplied resolved UI facts.
// A binding participates only while its rule matches the current snapshot;
// this is checked at each stroke so context changes during a pending sequence
// take effect immediately.
func (m *KeySequenceMatcher) MatchWithContext(stroke KeyStroke, snapshot context.Snapshot) KeySequenceResult {
	if m == nil || m.root == nil {
		return KeySequenceResult{}
	}
	if stroke.Key == "Escape" {
		wasPending := m.node != m.root || m.pending != nil
		m.reset()
		return KeySequenceResult{Handled: wasPending, Cancelled: wasPending}
	}
	if m.node != m.root && !m.hasEligible(m.node, snapshot) {
		m.reset()
	}
	if m.pending != nil && !m.pending.When.Matches(snapshot) {
		m.pending = nil
	}
	if !m.started.IsZero() && m.clock().Sub(m.started) >= m.timeout {
		result := m.ExpireWithContext(snapshot)
		// Expiration executes a pending exact match. Preserve that result and
		// let this stroke begin a new sequence on the next call.
		if result.ActionID != "" {
			result.Reprocess = true
			return result
		}
	}
	if child := m.node.children[stroke]; m.hasEligible(child, snapshot) {
		return m.advance(child, snapshot)
	}
	if m.node != m.root {
		m.reset()
		if child := m.root.children[stroke]; m.hasEligible(child, snapshot) {
			return m.advance(child, snapshot)
		}
	}
	return KeySequenceResult{}
}

func (m *KeySequenceMatcher) advance(node *keySequenceNode, snapshot context.Snapshot) KeySequenceResult {
	m.node = node
	bindingEligible := node.binding != nil && node.binding.When.Matches(snapshot)
	childrenEligible := false
	for _, child := range node.children {
		if m.hasEligible(child, snapshot) {
			childrenEligible = true
			break
		}
	}
	if bindingEligible && !childrenEligible {
		return m.result(node.binding)
	}
	if m.started.IsZero() {
		m.started = m.clock()
	}
	if bindingEligible {
		m.pending = node.binding
	} else {
		m.pending = nil
	}
	return KeySequenceResult{Handled: true, Pending: true}
}

// Expire finishes a pending prefix once its timeout elapsed. A prefix that
// has no complete binding simply resets and yields no action.
func (m *KeySequenceMatcher) Expire() KeySequenceResult {
	return m.ExpireWithContext(context.Snapshot{})
}

// ExpireWithContext finishes a pending prefix once its timeout elapsed. The
// pending exact binding is checked against the snapshot at expiration time,
// rather than the potentially stale snapshot from when the prefix began.
func (m *KeySequenceMatcher) ExpireWithContext(snapshot context.Snapshot) KeySequenceResult {
	if m == nil || m.root == nil {
		return KeySequenceResult{}
	}
	if m.node == nil || m.node == m.root || m.started.IsZero() || m.clock().Sub(m.started) < m.timeout {
		if m.node != nil && m.node != m.root && !m.hasEligible(m.node, snapshot) {
			m.reset()
			return KeySequenceResult{}
		}
		pending := m.node != nil && m.node != m.root
		return KeySequenceResult{Handled: pending, Pending: pending}
	}
	binding := m.pending
	m.reset()
	if binding == nil || !binding.When.Matches(snapshot) {
		return KeySequenceResult{}
	}
	return m.result(binding)
}

func (m *KeySequenceMatcher) hasEligible(node *keySequenceNode, snapshot context.Snapshot) bool {
	if node == nil {
		return false
	}
	if node.binding != nil && node.binding.When.Matches(snapshot) {
		return true
	}
	for _, child := range node.children {
		if m.hasEligible(child, snapshot) {
			return true
		}
	}
	return false
}

func (m *KeySequenceMatcher) Cancel() {
	m.reset()
}

// Pending reports whether a multi-stroke sequence is in progress.
func (m *KeySequenceMatcher) Pending() bool {
	return m != nil && m.node != nil && m.root != nil && m.node != m.root
}

// Candidates returns the valid next strokes for the current prefix together
// with root-level starts. This lets a caller intercept possible sequence
// strokes without hardcoding key maps. Results are deduplicated and sorted by
// logical key name, then modifier bits, for stable presentation and routing.
func (m *KeySequenceMatcher) Candidates() []KeyStroke {
	if m == nil || m.root == nil {
		return nil
	}
	set := make(map[KeyStroke]struct{})
	addChildren := func(node *keySequenceNode) {
		if node == nil {
			return
		}
		for stroke := range node.children {
			set[stroke] = struct{}{}
		}
	}
	addChildren(m.root)
	if m.Pending() {
		addChildren(m.node)
	}
	out := make([]KeyStroke, 0, len(set))
	for stroke := range set {
		out = append(out, stroke)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Key != out[j].Key {
			return out[i].Key < out[j].Key
		}
		return out[i].Mods < out[j].Mods
	})
	return out
}

func (m *KeySequenceMatcher) reset() {
	m.node = m.root
	m.started = time.Time{}
	m.pending = nil
}

func (m *KeySequenceMatcher) result(binding *KeyBinding) KeySequenceResult {
	values := make(map[string]any, len(binding.Values))
	for key, value := range binding.Values {
		values[key] = value
	}
	if binding.Values == nil {
		values = nil
	}
	return KeySequenceResult{ActionID: binding.ActionID, Values: values, Handled: true}
}
