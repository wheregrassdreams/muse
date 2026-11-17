package context

import "testing"

func TestComposeUsesNearestScopeAndKeepsOuterFacts(t *testing.T) {
	mode := NewKey[string]("editor.mode")
	hasSelection := NewKey[bool]("editor.hasSelection")
	searchOpen := NewKey[bool]("search.open")

	workspace := With(Scope{}, mode, "workspace-default")
	workspace = With(workspace, searchOpen, false)
	pane := With(Scope{}, mode, "pane-mode")
	control := With(Scope{}, mode, "insert")
	control = With(control, hasSelection, true)

	snapshot := Compose(workspace, pane, control)
	if got, ok := mode.Read(snapshot); !ok || got != "insert" {
		t.Fatalf("nearest mode = %q, present=%v; want insert", got, ok)
	}
	if got, ok := searchOpen.Read(snapshot); !ok || got {
		t.Fatalf("workspace fact = %v, present=%v; want false", got, ok)
	}
	if got, ok := hasSelection.Read(snapshot); !ok || !got {
		t.Fatalf("focused control fact = %v, present=%v; want true", got, ok)
	}
}

func TestSnapshotDoesNotChangeWhenScopeIsExtended(t *testing.T) {
	selected := NewKey[bool]("explorer.hasSelection")
	base := With(Scope{}, selected, false)
	before := Compose(base, Scope{}, Scope{})
	afterScope := With(base, selected, true)
	after := Compose(afterScope, Scope{}, Scope{})

	if got, ok := selected.Read(before); !ok || got {
		t.Fatalf("earlier snapshot changed: value=%v present=%v", got, ok)
	}
	if got, ok := selected.Read(after); !ok || !got {
		t.Fatalf("extended scope value=%v present=%v; want true", got, ok)
	}
}

func TestTypedKeysWithSameNameHaveIndependentIdentity(t *testing.T) {
	boolean := NewKey[bool]("feature.state")
	text := NewKey[string]("feature.state")
	scope := With(Scope{}, boolean, true)
	scope = With(scope, text, "ready")
	snapshot := Compose(scope, Scope{}, Scope{})

	if got, ok := boolean.Read(snapshot); !ok || !got {
		t.Fatalf("boolean fact = %v, present=%v; want true", got, ok)
	}
	if got, ok := text.Read(snapshot); !ok || got != "ready" {
		t.Fatalf("string fact = %q, present=%v; want ready", got, ok)
	}
}

func TestRulesComposeOverSnapshot(t *testing.T) {
	mode := NewKey[string]("editor.mode")
	selected := NewKey[bool]("explorer.hasSelection")
	snapshot := Compose(
		With(Scope{}, mode, "normal"),
		With(Scope{}, selected, true),
		Scope{},
	)

	canEdit := All(Equals(mode, "normal"), Not(Equals(selected, false)))
	if !canEdit.Matches(snapshot) {
		t.Fatal("All/Equals/Not should match the snapshot")
	}
	if Any(Equals(mode, "insert"), Equals(selected, false)).Matches(snapshot) {
		t.Fatal("Any should fail when no child rule matches")
	}
	if !All().Matches(snapshot) {
		t.Fatal("empty All should match")
	}
	if Any().Matches(snapshot) {
		t.Fatal("empty Any should not match")
	}
}

func TestAllCopiesItsInputRules(t *testing.T) {
	mode := NewKey[string]("editor.mode")
	snapshot := Compose(With(Scope{}, mode, "normal"), Scope{}, Scope{})
	rules := []Rule{Equals(mode, "normal")}
	combined := All(rules...)
	rules[0] = Equals(mode, "insert")

	if !combined.Matches(snapshot) {
		t.Fatal("All retained the caller's mutable rule slice")
	}
}

func TestZeroKeyCannotReadFacts(t *testing.T) {
	var missing Key[bool]
	got, ok := missing.Read(Compose(Scope{}, Scope{}, Scope{}))
	if ok || got {
		t.Fatalf("zero key read = %v, present=%v; want absent", got, ok)
	}
}
