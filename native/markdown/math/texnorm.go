package math

import editormath "github.com/wheregrassdreams/muse-editor/markdown/math"

// NormalizeTeX applies the backend-neutral TeX normalizations used by the
// formula renderer.
func NormalizeTeX(expr string) string { return editormath.NormalizeTeX(expr) }
