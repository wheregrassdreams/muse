package markdown

import texmath "muse/markdown/math"

// TeXToUnicode returns a readable Unicode fallback for common TeX constructs.
func TeXToUnicode(expr string) string { return texmath.TeXToUnicode(expr) }
