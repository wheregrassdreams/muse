package math

import editormath "github.com/wheregrassdreams/muse-editor/markdown/math"

// TeXToUnicode returns the backend-neutral readable fallback for common TeX.
func TeXToUnicode(expr string) string { return editormath.TeXToUnicode(expr) }
