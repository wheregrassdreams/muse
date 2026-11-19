package media

import (
	"github.com/egoist/mygo/ui"
	texmath "muse/markdown/math"
	"muse/nativeui/components/mathvisual"
)

type MathVisual = mathvisual.Visual

func DrawMathRaster(p *ui.Painter, dst ui.Rect, raster *texmath.Raster, dark bool, font string) {
	mathvisual.ForRaster(raster, dark, font).Draw(p, dst)
}

func MathVisualFor(raster *texmath.Raster, dark bool, font string) *MathVisual {
	return mathvisual.ForRaster(raster, dark, font)
}
