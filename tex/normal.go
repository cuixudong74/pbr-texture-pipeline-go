package tex

import "math"

// GradientMethod selects how height gradients are computed.
type GradientMethod int

const (
	// GradientSobel uses a 3x3 Sobel operator (smoother, noise-tolerant).
	GradientSobel GradientMethod = iota
	// GradientCentral uses central differences (sharper).
	GradientCentral
)

// NormalConfig controls height/displacement -> normal conversion.
type NormalConfig struct {
	Method   GradientMethod
	Strength float32 // scales the gradient; 1 is a neutral default
	// YUp selects the green-channel convention of the OUTPUT normal map:
	// true = OpenGL (green points toward +v), false = DirectX (green toward -v).
	YUp bool
}

// HeightToNormal derives a tangent-space normal map (3 channels, Linear,
// values encoded as n*0.5+0.5) from a single-channel height/displacement
// source. Sampling wraps at edges so tiled output has no seams. Every
// encoded texel decodes to a unit-length normal.
func HeightToNormal(src *Image, cfg NormalConfig) (*Image, error) {
	if src == nil {
		return nil, newValidationError(ErrInvalidConfig, nil, "normal: nil source")
	}
	if src.Channels < 1 {
		return nil, newValidationError(ErrChannelMismatch, nil,
			"normal: source needs at least 1 channel, has %d", src.Channels)
	}
	if cfg.Strength < 0 {
		return nil, newValidationError(ErrInvalidConfig, nil,
			"normal: strength must be >= 0, got %v", cfg.Strength)
	}
	w, h := src.Width, src.Height
	out := NewImage(w, h, 3, Linear, Direction{NormalYUp: cfg.YUp, OriginTopLeft: src.Dir.OriginTopLeft})
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var gx, gy float32
			switch cfg.Method {
			case GradientCentral:
				gx = (src.AtWrapped(x+1, y, 0) - src.AtWrapped(x-1, y, 0)) / 2
				gy = (src.AtWrapped(x, y+1, 0) - src.AtWrapped(x, y-1, 0)) / 2
			default: // GradientSobel
				gx = (src.AtWrapped(x+1, y-1, 0) + 2*src.AtWrapped(x+1, y, 0) + src.AtWrapped(x+1, y+1, 0) -
					src.AtWrapped(x-1, y-1, 0) - 2*src.AtWrapped(x-1, y, 0) - src.AtWrapped(x-1, y+1, 0)) / 8
				gy = (src.AtWrapped(x-1, y+1, 0) + 2*src.AtWrapped(x, y+1, 0) + src.AtWrapped(x+1, y+1, 0) -
					src.AtWrapped(x-1, y-1, 0) - 2*src.AtWrapped(x, y-1, 0) - src.AtWrapped(x+1, y-1, 0)) / 8
			}
			// Image rows grow downward when OriginTopLeft, while +v grows
			// upward: convert the image-space y gradient to a v gradient.
			gv := gy
			if src.Dir.OriginTopLeft {
				gv = -gy
			}
			nx := -gx * cfg.Strength
			ny := -gv * cfg.Strength
			if !cfg.YUp {
				ny = -ny // DirectX green channel points toward -v
			}
			nz := float32(1)
			inv := 1 / float32(math.Sqrt(float64(nx*nx+ny*ny+nz*nz)))
			out.Set(x, y, 0, nx*inv*0.5+0.5)
			out.Set(x, y, 1, ny*inv*0.5+0.5)
			out.Set(x, y, 2, nz*inv*0.5+0.5)
		}
	}
	return out, nil
}

// DecodeNormal reads the encoded texel at (x, y) of a normal map produced
// by HeightToNormal and returns the unit-length normal it represents.
func DecodeNormal(img *Image, x, y int) (nx, ny, nz float32) {
	nx = img.At(x, y, 0)*2 - 1
	ny = img.At(x, y, 1)*2 - 1
	nz = img.At(x, y, 2)*2 - 1
	return
}

// Canonical returns a stable string encoding of the config, suitable for
// cache keys: any config change yields a different key.
func (c NormalConfig) Canonical() string {
	m := "sobel"
	if c.Method == GradientCentral {
		m = "central"
	}
	y := "yup"
	if !c.YUp {
		y = "ydown"
	}
	return "method=" + m + ";strength=" + ftoa(c.Strength) + ";green=" + y
}
