package mdedit

import "testing"

func TestSizedImageDimensionsUsesIntrinsicRatioAndHTMLOverrides(t *testing.T) {
	tests := []struct {
		name       string
		size       imgSize
		width      float32
		wantWidth  float32
		wantHeight float32
	}{
		{name: "width derives height", size: imgSize{w: 100}, width: 500, wantWidth: 100, wantHeight: 50},
		{name: "height derives width", size: imgSize{h: 80}, width: 500, wantWidth: 160, wantHeight: 80},
		{name: "both dimensions override ratio", size: imgSize{w: 100, h: 90}, width: 500, wantWidth: 100, wantHeight: 90},
		{name: "zoom applies after dimensions", size: imgSize{w: 100, zoom: 2}, width: 500, wantWidth: 200, wantHeight: 100},
		{name: "content width clamps both axes", size: imgSize{h: 80}, width: 120, wantWidth: 120, wantHeight: 60},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, h := sizedImageDimensions(640, 320, tt.size, tt.width)
			if w != tt.wantWidth || h != tt.wantHeight {
				t.Fatalf("sizedImageDimensions = (%v,%v), want (%v,%v)", w, h, tt.wantWidth, tt.wantHeight)
			}
		})
	}
}

func TestSizedImageDimensionsFallsBackWhenIntrinsicSizeIsUnknown(t *testing.T) {
	// imageObject supplies a paragraph-like fallback geometry if header
	// inspection cannot identify dimensions. A single HTML dimension uses that
	// safe fallback ratio rather than dividing by zero or producing NaN.
	w, h := sizedImageDimensions(400, 40, imgSize{w: 120}, 300)
	if w != 120 || h != 12 {
		t.Fatalf("unknown-ratio width override = (%v,%v), want (120,12)", w, h)
	}
	w, h = sizedImageDimensions(400, 40, imgSize{h: 80}, 300)
	if w != 300 || h != 30 {
		t.Fatalf("unknown-ratio height override = (%v,%v), want content-clamped (300,30)", w, h)
	}
	w, h = sizedImageDimensions(400, 40, imgSize{w: 120, h: 60}, 300)
	if w != 120 || h != 60 {
		t.Fatalf("unknown-ratio explicit dimensions = (%v,%v), want (120,60)", w, h)
	}
	w, h = sizedImageDimensions(400, 40, imgSize{zoom: 2}, 1000)
	if w != 800 || h != 80 {
		t.Fatalf("unknown-ratio zoom = (%v,%v), want (800,80)", w, h)
	}
}
