package mdoc

import "testing"

func TestCasesAndAligned(t *testing.T) {
	for _, expr := range []string{
		`\begin{cases} x^2 & x>0 \\ -x & x\le 0 \end{cases}`,
		`\begin{aligned} a &= b + c \\ d &= e \end{aligned}`,
		`\begin{array}{cc} 1 & 2 \\ 3 & 4 \end{array}`,
	} {
		bmp := mathBitmap(expr, 18, false, true)
		if bmp == nil {
			t.Fatalf("bitmap for %q is nil", expr)
		}
		if _, h := bmp.Size(); h < 40 {
			t.Fatalf("%q is %d high: the two rows are not both drawn", expr, h)
		}
	}
}
