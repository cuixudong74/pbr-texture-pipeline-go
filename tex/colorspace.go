package tex

import (
	"math"
)

// SRGBToLinear decodes a single sRGB-encoded value to linear (IEC 61966-2-1).
func SRGBToLinear(v float32) float32 {
	if v <= 0.04045 {
		return v / 12.92
	}
	return float32(math.Pow((float64(v)+0.055)/1.055, 2.4))
}

// LinearToSRGB encodes a single linear value to sRGB.
func LinearToSRGB(v float32) float32 {
	if v <= 0.0031308 {
		return v * 12.92
	}
	return float32(1.055*math.Pow(float64(v), 1.0/2.4) - 0.055)
}

// ConvertSpace returns img converted to the target color space.
//
// isColor tells whether img is a color map: only color maps ever get a
// gamma transform. Data maps (normal/height/mask/ORM...) must be authored
// and kept Linear; passing isColor=false for them is a no-op on values so
// data is never accidentally gamma-warped. Converting to the space the
// image is already in is also a no-op on values.
func ConvertSpace(img *Image, target ColorSpace, isColor bool) (*Image, error) {
	if img == nil {
		return nil, newValidationError(ErrInvalidConfig, nil, "colorspace: nil image")
	}
	out := img.Clone()
	out.Space = target
	if !isColor || img.Space == target {
		return out, nil
	}
	var f func(float32) float32
	switch {
	case img.Space == SRGB && target == Linear:
		f = SRGBToLinear
	case img.Space == Linear && target == SRGB:
		f = LinearToSRGB
	default:
		return nil, newValidationError(ErrInvalidConfig, nil,
			"colorspace: unsupported conversion %s -> %s", img.Space, target)
	}
	for k, v := range out.Data {
		out.Data[k] = f(v)
	}
	return out, nil
}
