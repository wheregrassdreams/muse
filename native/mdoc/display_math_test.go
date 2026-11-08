package mdoc

import "testing"

// metrics returns a formula's width and height in pixels, displayed or not.
func metrics(expr string, display bool) (int, int) {
	bmp := mathBitmap(expr, 18, false, display)
	if bmp == nil {
		return 0, 0
	}
	return bmp.Size()
}

// TestADisplayedFormulaSetsTheLimitsOfABigOperator: in a line of text the
// limits of \int and \sum sit beside the sign; a displayed formula puts them
// above and below it, which is what makes it a formula on a line of its own.
func TestADisplayedFormulaSetsTheLimitsOfABigOperator(t *testing.T) {
	for _, expr := range []string{`\sum_{i=1}^{n} i`, `\int_0^1 x^2 \, dx`} {
		iw, ih := metrics(expr, false)
		dw, dh := metrics(expr, true)
		if iw == 0 || dw == 0 {
			t.Fatalf("%s drew nothing", expr)
		}
		if dh <= ih {
			t.Fatalf("%s: displayed height %d is not more than inline %d", expr, dh, ih)
		}
		if dw >= iw {
			t.Fatalf("%s: displayed width %d is not less than inline %d", expr, dw, iw)
		}
	}
}

// TestADisplayedFormulaSetsFractionsFullSize: \frac in a line of text is
// text-style, its numerator and denominator shrunk; on a line of its own it
// is display-style, at the size of the text around it.
func TestADisplayedFormulaSetsFractionsFullSize(t *testing.T) {
	_, ih := metrics(`\frac{1}{2}`, false)
	_, dh := metrics(`\frac{1}{2}`, true)
	if ih == 0 || dh == 0 {
		t.Fatal("the fraction drew nothing")
	}
	if dh <= ih {
		t.Fatalf("displayed fraction height %d is not more than inline %d", dh, ih)
	}
}

// TestTheSpaceBeforeAnOperatorSurvivesComposition: a formula with a script
// is composed in parts here, and the space before its operators has to be
// put back — it is TeX's, not the source's.
func TestTheSpaceBeforeAnOperatorSurvivesComposition(t *testing.T) {
	aw, _ := metrics(`a^2+b^2`, true)
	bw, _ := metrics(`a^2b^2`, true)
	if aw <= bw {
		t.Fatalf("a^2+b^2 is %d wide, a^2b^2 is %d: the operator took no space", aw, bw)
	}
}

// TestTheFormulaOfABlockIsDisplayed: the block path asks for display style,
// the inline path does not.
func TestTheFormulaOfABlockIsDisplayed(t *testing.T) {
	block := RenderMath(`\sum_{i=1}^{n} i`, 18, false)
	inlineBmp, _ := RenderMathBaseline(`\sum_{i=1}^{n} i`, 18, false)
	if block == nil || inlineBmp == nil {
		t.Fatal("a formula drew nothing")
	}
	bw, bh := block.Size()
	iw, ih := inlineBmp.Size()
	if bh <= ih || bw >= iw {
		t.Fatalf("block %dx%d, inline %dx%d: the block is not set in display style", bw, bh, iw, ih)
	}
}
