package mdoc

import "testing"

func TestScriptComposition(t *testing.T) {
	for _, expr := range []string{
		`x^2`,
		`E=mc^2`,
		`x_i^2`,
		`x^2 + y`,
		`\frac{a}{b}^2`,
	} {
		if bmp := mathBitmap(expr, 18, false, false); bmp == nil {
			t.Fatalf("render %q: nothing drawn", expr)
		}
	}
}

func TestCasesFirstCell(t *testing.T) {
	expr := `\begin{cases} x^2 & x>0 \\ -x & x\le 0 \end{cases}`
	if bmp := mathBitmap(expr, 18, false, false); bmp == nil {
		t.Fatal("cases bitmap nil")
	}
}
