// Package tex implements a PBR texture derivation and channel-packing
// pipeline: height/displacement -> tangent-space normal maps, multi-source
// channel packing, color-space handling, validation, caching and
// concurrent incremental processing. Everything runs in-process.
package tex

import "fmt"

// ColorSpace describes how pixel values are encoded.
type ColorSpace int

const (
	// Linear holds scene-linear values (data maps, decoded colors).
	Linear ColorSpace = iota
	// SRGB holds sRGB-encoded color values.
	SRGB
)

func (c ColorSpace) String() string {
	switch c {
	case Linear:
		return "linear"
	case SRGB:
		return "srgb"
	}
	return "unknown"
}

// Direction captures orientation metadata of a texture.
type Direction struct {
	// NormalYUp: true = OpenGL convention (green up), false = DirectX (green down).
	NormalYUp bool
	// OriginTopLeft: true when row 0 is the top of the image.
	OriginTopLeft bool
}

// Image is an in-memory float32 texture, row-major, channel-interleaved.
type Image struct {
	Width    int
	Height   int
	Channels int
	Space    ColorSpace
	Dir      Direction
	Data     []float32
}

// NewImage allocates a zeroed image. It panics on invalid dimensions,
// which indicates a programming error, not bad user input.
func NewImage(w, h, ch int, space ColorSpace, dir Direction) *Image {
	if w <= 0 || h <= 0 || ch <= 0 {
		panic(fmt.Sprintf("tex.NewImage: invalid geometry %dx%dx%d", w, h, ch))
	}
	return &Image{
		Width:    w,
		Height:   h,
		Channels: ch,
		Space:    space,
		Dir:      dir,
		Data:     make([]float32, w*h*ch),
	}
}

// index returns the flat offset of (x, y, c). Out-of-range access panics.
func (i *Image) index(x, y, c int) int {
	return (y*i.Width+x)*i.Channels + c
}

// At returns the value of channel c at pixel (x, y).
func (i *Image) At(x, y, c int) float32 { return i.Data[i.index(x, y, c)] }

// Set writes the value of channel c at pixel (x, y).
func (i *Image) Set(x, y, c int, v float32) { i.Data[i.index(x, y, c)] = v }

// AtWrapped returns channel c at pixel (x, y) with wrap-around addressing,
// so edge sampling of tiled textures never sees a border.
func (i *Image) AtWrapped(x, y, c int) float32 {
	x = ((x % i.Width) + i.Width) % i.Width
	y = ((y % i.Height) + i.Height) % i.Height
	return i.Data[i.index(x, y, c)]
}

// Clone returns a deep copy.
func (i *Image) Clone() *Image {
	if i == nil {
		return nil
	}
	out := *i
	out.Data = make([]float32, len(i.Data))
	copy(out.Data, i.Data)
	return &out
}

// Equal reports exact equality of pixels and metadata.
func (i *Image) Equal(o *Image) bool {
	if i == nil || o == nil {
		return i == o
	}
	if i.Width != o.Width || i.Height != o.Height || i.Channels != o.Channels ||
		i.Space != o.Space || i.Dir != o.Dir || len(i.Data) != len(o.Data) {
		return false
	}
	for k := range i.Data {
		if i.Data[k] != o.Data[k] {
			return false
		}
	}
	return true
}
