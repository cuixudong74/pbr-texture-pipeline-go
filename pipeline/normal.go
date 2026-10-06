package pipeline

import (
	"fmt"
	"math"
)

// GradientFilter selects how height gradients are computed.
type GradientFilter int

const (
	// CentralDifference uses a 2-tap central difference with wrap-around.
	CentralDifference GradientFilter = iota
	// Sobel uses a 3x3 Sobel operator with wrap-around.
	Sobel
)

// NormalConvention selects the tangent-space handedness of the output.
type NormalConvention int

const (
	// OpenGLNormals stores +Y up (green channel points up).
	OpenGLNormals NormalConvention = iota
	// DirectXNormals stores -Y up (green channel points down).
	DirectXNormals
)

// NormalConfig controls height-to-normal conversion.
type NormalConfig struct {
	Strength       float32
	Filter         GradientFilter
	Convention     NormalConvention
	SourceChannel  int
	WrapHorizontal bool
	WrapVertical   bool
}

// GenerateNormal derives a tangent-space normal map from a height or
// displacement texture. The output is a 3-channel linear texture of unit
// vectors encoded to [0,1]. Edges wrap (per config) so tiled output has no
// visible seams.
func GenerateNormal(src *Texture, cfg NormalConfig) (*Texture, error) {
	if src == nil {
		return nil, &ValidationError{Kind: ErrInvalidConfig, Message: "nil source texture"}
	}
	if cfg.SourceChannel < 0 || cfg.SourceChannel >= src.Channels {
		return nil, &ValidationError{
			Kind: ErrChannelInsufficient,
			Message: fmt.Sprintf("source channel %d requested but texture has %d channels",
				cfg.SourceChannel, src.Channels),
		}
	}
	lin := DecodeToLinear(src)
	w, h := src.Width, src.Height

	sample := func(x, y int) float32 {
		if cfg.WrapHorizontal {
			x = ((x % w) + w) % w
		} else if x < 0 {
			x = 0
		} else if x >= w {
			x = w - 1
		}
		if cfg.WrapVertical {
			y = ((y % h) + h) % h
		} else if y < 0 {
			y = 0
		} else if y >= h {
			y = h - 1
		}
		return lin.At(x, y, cfg.SourceChannel)
	}

	out := NewTexture(w, h, 3, Linear, src.Orientation)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var dx, dy float32
			switch cfg.Filter {
			case Sobel:
				dx = (sample(x+1, y-1) + 2*sample(x+1, y) + sample(x+1, y+1) -
					sample(x-1, y-1) - 2*sample(x-1, y) - sample(x-1, y+1)) / 8
				dy = (sample(x-1, y+1) + 2*sample(x, y+1) + sample(x+1, y+1) -
					sample(x-1, y-1) - 2*sample(x, y-1) - sample(x+1, y-1)) / 8
			default: // CentralDifference
				dx = (sample(x+1, y) - sample(x-1, y)) / 2
				dy = (sample(x, y+1) - sample(x, y-1)) / 2
			}
			nx := -dx * cfg.Strength
			// Image rows run downward. For OpenGL (green up), a height
			// rising toward the bottom of the image tilts the normal toward
			// the top, i.e. +Y; DirectX uses the opposite handedness.
			ny := dy * cfg.Strength
			if cfg.Convention == DirectXNormals {
				ny = -ny
			}
			nz := float32(1.0)
			inv := float32(1.0 / math.Sqrt(float64(nx*nx+ny*ny+nz*nz)))
			nx, ny, nz = nx*inv, ny*inv, nz*inv
			out.Set(x, y, 0, nx*0.5+0.5)
			out.Set(x, y, 1, ny*0.5+0.5)
			out.Set(x, y, 2, nz*0.5+0.5)
		}
	}
	return out, nil
}
