package layout

import "testing"

func TestVirtualBandUsesHalfViewportForOrdinaryMovement(t *testing.T) {
	band := VirtualBand(100, 80, 140, true)
	if band.Top != 60 || band.Bottom != 220 {
		t.Fatalf("VirtualBand() = %+v, want top=60 bottom=220", band)
	}
}

func TestVirtualBandUsesOneEighthViewportForLargeScrollJump(t *testing.T) {
	band := VirtualBand(100, 80, 140.01, true)
	if band.Top != 90 || band.Bottom != 190 {
		t.Fatalf("VirtualBand() = %+v, want top=90 bottom=190", band)
	}
}

func TestVirtualBandWithoutPreviousPositionUsesHalfViewport(t *testing.T) {
	band := VirtualBand(12, 40, -1000, false)
	if band.Top != -8 || band.Bottom != 72 {
		t.Fatalf("VirtualBand() = %+v, want top=-8 bottom=72", band)
	}
}

func TestVirtualBandLeavesNegativeBoundsUnclipped(t *testing.T) {
	band := VirtualBand(10, 80, 1000, true)
	if band.Top != 0 || band.Bottom != 100 {
		t.Fatalf("VirtualBand() = %+v, want top=0 bottom=100", band)
	}
	band = VirtualBand(0, 80, -1000, true)
	if band.Top != -10 || band.Bottom != 90 {
		t.Fatalf("jump band = %+v, want top=-10 bottom=90", band)
	}
}

func TestVirtualBandUsesStrictJumpThreshold(t *testing.T) {
	band := VirtualBand(100, 80, 140, true)
	if band.Top != 60 {
		t.Fatalf("threshold-equal scroll jump used reduced overscan: %+v", band)
	}
}
