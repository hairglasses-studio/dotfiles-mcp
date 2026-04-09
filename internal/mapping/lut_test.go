package mapping

import (
	"math"
	"testing"
)

func TestCompiledTransformLinear(t *testing.T) {
	vt := &ValueTransform{
		InputRange:  [2]float64{0, 65535},
		OutputRange: [2]float64{0, 1},
		Curve:       CurveLinear,
	}
	ct := CompileTransform(vt)

	// Endpoints.
	if ct.LUT[0] != 0 {
		t.Errorf("LUT[0] = %f, want 0", ct.LUT[0])
	}
	if math.Abs(ct.LUT[255]-1.0) > 0.001 {
		t.Errorf("LUT[255] = %f, want ~1.0", ct.LUT[255])
	}

	// Midpoint.
	mid := ct.Lookup(32767, vt)
	if math.Abs(mid-0.5) > 0.01 {
		t.Errorf("Lookup(32767) = %f, want ~0.5", mid)
	}
}

func TestCompiledTransformNil(t *testing.T) {
	if CompileTransform(nil) != nil {
		t.Error("expected nil for nil ValueTransform")
	}
}

func TestCompiledTransformEdges(t *testing.T) {
	vt := &ValueTransform{
		InputRange:  [2]float64{0, 100},
		OutputRange: [2]float64{0, 10},
		Curve:       CurveLinear,
	}
	ct := CompileTransform(vt)

	// Below range clamps to LUT[0].
	if ct.Lookup(-50, vt) != ct.LUT[0] {
		t.Error("below-range should clamp to LUT[0]")
	}

	// Above range clamps to LUT[255].
	if ct.Lookup(200, vt) != ct.LUT[255] {
		t.Error("above-range should clamp to LUT[255]")
	}
}
