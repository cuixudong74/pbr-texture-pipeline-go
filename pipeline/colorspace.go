package pipeline

import "math"

// DecodeToLinear returns a copy of t converted to linear space. Textures
// already in Linear space (all data maps) are returned with values unchanged;
// only textures declared SRGB are gamma-decoded.
func DecodeToLinear(t *Texture) *Texture {
	out := t.Clone()
	out.ColorSpace = Linear
	if t.ColorSpace != SRGB {
		return out
	}
	// Alpha (4th channel) is never gamma-transformed.
	for i := 0; i < t.Width*t.Height; i++ {
		base := i * t.Channels
		n := t.Channels
		if n == 4 {
			n = 3
		}
		for c := 0; c < n; c++ {
			out.Data[base+c] = srgbDecode(t.Data[base+c])
		}
	}
	return out
}

// EncodeFromLinear returns a copy of a linear texture encoded into the target
// color space. Linear targets (data maps) pass through untouched, so data
// maps are never accidentally gamma-transformed.
func EncodeFromLinear(t *Texture, target ColorSpace) *Texture {
	out := t.Clone()
	out.ColorSpace = target
	if target != SRGB {
		return out
	}
	for i := 0; i < t.Width*t.Height; i++ {
		base := i * t.Channels
		n := t.Channels
		if n == 4 {
			n = 3
		}
		for c := 0; c < n; c++ {
			out.Data[base+c] = srgbEncode(t.Data[base+c])
		}
	}
	return out
}

// srgbDecode converts one sRGB-encoded channel to linear (IEC 61966-2-1).
func srgbDecode(v float32) float32 {
	if v <= 0.04045 {
		return v / 12.92
	}
	return float32(math.Pow((float64(v)+0.055)/1.055, 2.4))
}

// srgbEncode converts one linear channel to sRGB encoding.
func srgbEncode(v float32) float32 {
	if v <= 0.0031308 {
		return v * 12.92
	}
	return float32(1.055*math.Pow(float64(v), 1.0/2.4) - 0.055)
}
