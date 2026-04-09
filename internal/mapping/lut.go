package mapping

// CompiledTransform is a pre-computed 256-entry lookup table for fast value
// transformation on the hot path. Instead of computing log/exp/scurve per
// event, the transform is evaluated once at profile load time.
type CompiledTransform struct {
	LUT [256]float64
}

// CompileTransform pre-computes a ValueTransform into a 256-entry LUT.
// Returns nil if vt is nil.
func CompileTransform(vt *ValueTransform) *CompiledTransform {
	if vt == nil {
		return nil
	}
	ct := &CompiledTransform{}
	for i := range ct.LUT {
		// Map 0..255 to the full input range, then transform.
		normalized := float64(i) / 255.0
		inMin, inMax := vt.InputRange[0], vt.InputRange[1]
		raw := inMin + normalized*(inMax-inMin)
		ct.LUT[i] = vt.Transform(raw)
	}
	return ct
}

// Lookup performs a fast O(1) value transform using the pre-computed LUT.
// The raw value is normalized to the input range and quantized to 0-255.
func (ct *CompiledTransform) Lookup(raw float64, vt *ValueTransform) float64 {
	inMin, inMax := vt.InputRange[0], vt.InputRange[1]
	if inMax == inMin {
		return ct.LUT[0]
	}
	normalized := (raw - inMin) / (inMax - inMin)
	if normalized <= 0 {
		return ct.LUT[0]
	}
	if normalized >= 1 {
		return ct.LUT[255]
	}
	idx := int(normalized * 255)
	return ct.LUT[idx]
}
