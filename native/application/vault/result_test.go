package vault

import "testing"

func TestResultTouchesPath(t *testing.T) {
	result := Result{Changed: []string{"/vault/notes/../notes/a.md", "/vault/b.md"}}
	if !result.TouchesPath("/vault/notes/a.md") {
		t.Fatal("TouchesPath() missed a normalized changed path")
	}
	if result.TouchesPath("/vault/c.md") {
		t.Fatal("TouchesPath() matched an untouched path")
	}
	if (Result{Changed: []string{"/vault/a.md"}}).TouchesPath("") {
		t.Fatal("TouchesPath() matched an empty path")
	}
}
