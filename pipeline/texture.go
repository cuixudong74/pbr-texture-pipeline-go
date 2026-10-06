// Package pipeline implements a PBR texture derivation and channel-packing
// pipeline: normal map generation from height/displacement sources, channel
// packing of data maps, color-space handling, input validation, content-keyed
// caching, and bounded-concurrency execution with cancellation.
package pipeline

import (
	"crypto/sha256"
	"encoding/binary"
	"math"
)

// ColorSpace describes how pixel values are encoded.
type ColorSpace int

const (
	// Linear holds linear-light values (data maps, or already-decoded color).
	Linear ColorSpace = iota
	// SRGB holds sRGB-encoded color values.
	SRGB
)

// Orientation describes the vertical axis convention of a texture.
type Orientation int

const (
	// OrientationYDown means the first row is the top of the image (typical files).
	OrientationYDown Orientation = iota
	// OrientationYUp means the first row is the bottom (typical GL uploads).
	OrientationYUp
)

// Texture is an in-process image with float32 pixels in row-major order,
// channels interleaved. All processing happens in linear space.
type Texture struct {
	Width       int
	Height      int
	Channels    int
	Data        []float32
	ColorSpace  ColorSpace
	Orientation Orientation
}

// NewTexture allocates a zeroed texture.
func NewTexture(w, h, channels int, cs ColorSpace, o Orientation) *Texture {
	return &Texture{
		Width:       w,
		Height:      h,
		Channels:    channels,
		Data:        make([]float32, w*h*channels),
		ColorSpace:  cs,
		Orientation: o,
	}
}

// Index returns the offset of channel c at pixel (x, y).
func (t *Texture) Index(x, y, c int) int {
	return (y*t.Width+x)*t.Channels + c
}

// At returns the value of channel c at pixel (x, y).
func (t *Texture) At(x, y, c int) float32 {
	return t.Data[t.Index(x, y, c)]
}

// Set writes the value of channel c at pixel (x, y).
func (t *Texture) Set(x, y, c int, v float32) {
	t.Data[t.Index(x, y, c)] = v
}

// Clone returns a deep copy.
func (t *Texture) Clone() *Texture {
	d := make([]float32, len(t.Data))
	copy(d, t.Data)
	return &Texture{
		Width:       t.Width,
		Height:      t.Height,
		Channels:    t.Channels,
		Data:        d,
		ColorSpace:  t.ColorSpace,
		Orientation: t.Orientation,
	}
}

// ContentHash returns a stable hash of pixel data and metadata.
func (t *Texture) ContentHash() [32]byte {
	h := sha256.New()
	var hdr [8]byte
	for _, v := range []int{t.Width, t.Height, t.Channels, int(t.ColorSpace), int(t.Orientation)} {
		binary.LittleEndian.PutUint64(hdr[:], uint64(int64(v)))
		h.Write(hdr[:])
	}
	var buf [4]byte
	for _, f := range t.Data {
		binary.LittleEndian.PutUint32(buf[:], math.Float32bits(f))
		h.Write(buf[:])
	}
	var sum [32]byte
	copy(sum[:], h.Sum(nil))
	return sum
}
