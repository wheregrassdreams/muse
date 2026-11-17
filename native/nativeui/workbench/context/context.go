// Package context provides typed, immutable UI facts and pure enablement rules.
// It is intentionally independent of Go's context.Context, the UI toolkit, and
// feature implementations.
package context

import "strings"

type keyIdentity struct{ marker byte }

// Key identifies one typed UI fact. Keys should be declared by the feature
// that owns the fact and reused wherever that fact is produced or evaluated.
type Key[T comparable] struct {
	identity *keyIdentity
	name     string
}

// NewKey creates a typed fact key. Names are for diagnostics; identity belongs
// to the returned key value and does not use a global registry.
func NewKey[T comparable](name string) Key[T] {
	name = strings.TrimSpace(name)
	if name == "" {
		panic("workbench/context: key name must not be empty")
	}
	return Key[T]{identity: &keyIdentity{marker: 1}, name: name}
}

// Name returns the descriptive name supplied when the key was created.
func (k Key[T]) Name() string { return k.name }

// Read retrieves this key's value from a resolved snapshot.
func (k Key[T]) Read(snapshot Snapshot) (T, bool) {
	var zero T
	if k.identity == nil {
		return zero, false
	}
	value, ok := snapshot.facts[k.identity]
	if !ok {
		return zero, false
	}
	result, ok := value.(T)
	if !ok {
		return zero, false
	}
	return result, true
}

// Scope is an immutable set of facts at one UI scope. With returns a new
// scope, so reusing an earlier scope cannot mutate snapshots already composed
// from it.
type Scope struct {
	facts map[*keyIdentity]any
}

// With returns a copy of scope with key set to value. Values must be
// comparable so keys remain typed and directly readable; callers should use
// immutable value data because this package does not deep-copy referenced
// objects stored inside comparable structs or pointers.
func With[T comparable](scope Scope, key Key[T], value T) Scope {
	if key.identity == nil {
		panic("workbench/context: cannot set a zero-value key")
	}
	facts := make(map[*keyIdentity]any, len(scope.facts)+1)
	for existing, existingValue := range scope.facts {
		facts[existing] = existingValue
	}
	facts[key.identity] = value
	return Scope{facts: facts}
}

// Snapshot is an immutable, resolved view of UI facts.
type Snapshot struct {
	facts map[*keyIdentity]any
}

// Compose overlays the focused-control facts over pane facts and pane facts
// over workspace facts. The nearest scope wins when the same key is present
// at more than one level.
func Compose(workspace, pane, focusedControl Scope) Snapshot {
	facts := make(map[*keyIdentity]any, len(workspace.facts)+len(pane.facts)+len(focusedControl.facts))
	for key, value := range workspace.facts {
		facts[key] = value
	}
	for key, value := range pane.facts {
		facts[key] = value
	}
	for key, value := range focusedControl.facts {
		facts[key] = value
	}
	return Snapshot{facts: facts}
}

// Rule is a pure predicate over a resolved UI context. Its evaluator is
// private, so callers compose the supported rule constructors instead of
// injecting callbacks with side effects.
type Rule struct {
	evaluate func(Snapshot) bool
}

// Matches reports whether the rule holds for snapshot. The zero Rule is an
// unconditional match, useful when a command has no additional condition.
func (r Rule) Matches(snapshot Snapshot) bool {
	if r.evaluate == nil {
		return true
	}
	return r.evaluate(snapshot)
}

// Equals matches when key is present and has the expected value.
func Equals[T comparable](key Key[T], expected T) Rule {
	return Rule{evaluate: func(snapshot Snapshot) bool {
		value, ok := key.Read(snapshot)
		return ok && value == expected
	}}
}

// Not negates rule.
func Not(rule Rule) Rule {
	return Rule{evaluate: func(snapshot Snapshot) bool { return !rule.Matches(snapshot) }}
}

// All matches when every rule matches. An empty All matches unconditionally.
func All(rules ...Rule) Rule {
	owned := append([]Rule(nil), rules...)
	return Rule{evaluate: func(snapshot Snapshot) bool {
		for _, rule := range owned {
			if !rule.Matches(snapshot) {
				return false
			}
		}
		return true
	}}
}

// Any matches when at least one rule matches. An empty Any never matches.
func Any(rules ...Rule) Rule {
	owned := append([]Rule(nil), rules...)
	return Rule{evaluate: func(snapshot Snapshot) bool {
		for _, rule := range owned {
			if rule.Matches(snapshot) {
				return true
			}
		}
		return false
	}}
}
